package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type runningTask struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// Download workers share a persistent task store. Unknown or changed remote sources are never silently appended.
type Engine struct {
	store          *Store
	quark          *Quark
	mu             sync.Mutex
	running        map[string]*runningTask
	client         *http.Client
	booted         bool
	slots          chan struct{}
	shareSlot      chan struct{}
	bandwidthMu    sync.Mutex
	bandwidthNext  time.Time
	sourceOverride func(context.Context, *Task) (string, error) // test-only injection
}

func newEngine(s *Store, q *Quark) *Engine {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = 32
	transport.Proxy = nil // Never send signed URLs through an implicit environment proxy.
	transport.DialContext = dialPublicHTTPS
	transport.ResponseHeaderTimeout = 30 * time.Second
	limit := s.snapshot().Settings.Concurrent
	if limit < 1 {
		limit = 3
	}
	return &Engine{store: s, quark: q, running: map[string]*runningTask{}, slots: make(chan struct{}, limit), shareSlot: make(chan struct{}, 1), client: &http.Client{Transport: transport, Timeout: 0, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 8 {
			return errors.New("重定向过多")
		}
		if err := validatePublicURL(req.Context(), req.URL.String()); err != nil {
			return err
		}
		// Never forward Quark account cookies to a third-party CDN after a redirect.
		if !quarkOwnedHost(req.URL.Hostname()) {
			req.Header.Del("Cookie")
		}
		return nil
	}}}
}

// prevent direct requests to loopback, RFC1918, link-local, and metadata endpoints.
// Resolve and validate again *at dial time* to prevent DNS rebinding between URL
// validation and the actual socket connection. TLS continues to verify the hostname.
func dialPublicHTTPS(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if port != "443" {
		return nil, errors.New("下载器只允许连接 HTTPS 443 端口")
	}
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, errors.New("下载地址解析为空")
	}
	for _, ip := range ips {
		if ip.Is4In6() {
			ip = netip.AddrFrom4(ip.As4())
		}
		if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return nil, errors.New("拒绝连接非公网目标")
		}
	}
	return (&net.Dialer{Timeout: 20 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
}

func validatePublicURL(ctx context.Context, raw string) error {
	u, e := url.Parse(raw)
	if e != nil {
		return e
	}
	if u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
		return errors.New("下载链接必须是公开 HTTPS 地址")
	}
	if u.Port() != "" && u.Port() != "443" {
		return errors.New("不允许自定义下载端口")
	}
	ips, e := net.DefaultResolver.LookupNetIP(ctx, "ip", u.Hostname())
	if e != nil {
		return e
	}
	if len(ips) == 0 {
		return errors.New("未解析到公网 IP")
	}
	for _, ip := range ips {
		if ip.Is4In6() {
			ip = netip.AddrFrom4(ip.As4())
		}
		if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("拒绝访问非公网目标")
		}
	}
	return nil
}

// No absolute paths, parent references, device names or NUL bytes accepted from remote filenames.
func safeRelative(path string) (string, error) {
	if path == "" || strings.Contains(path, "\\") || strings.ContainsRune(path, 0) || strings.HasPrefix(path, "/") {
		return "", errors.New("不安全的文件路径")
	}
	for _, p := range strings.Split(path, "/") {
		if p == "" || p == "." || p == ".." || len(p) > 240 {
			return "", errors.New("非法文件名")
		}
	}
	return path, nil
}
func checkDirectory(root, relative string) (string, error) {
	rel, e := safeRelative(relative)
	if e != nil {
		return "", e
	}
	if !filepath.IsAbs(root) {
		return "", errors.New("下载目录必须是绝对路径")
	}
	root = filepath.Clean(root)
	path := root
	for _, seg := range strings.Split(rel, "/") {
		path = filepath.Join(path, seg)
		if inf, e := os.Lstat(path); e == nil {
			if inf.Mode()&os.ModeSymlink != 0 {
				return "", errors.New("目标目录包含符号链接")
			}
		} else if !os.IsNotExist(e) {
			return "", e
		}
	}
	return path, nil
}
func diskFree(path string) (uint64, error) {
	var stat syscall.Statfs_t
	if e := syscall.Statfs(path, &stat); e != nil {
		return 0, e
	}
	return stat.Bavail * uint64(stat.Bsize), nil
}
func (e *Engine) start() {
	e.mu.Lock()
	if e.booted {
		e.mu.Unlock()
		return
	}
	e.booted = true
	e.mu.Unlock()
	for _, t := range e.store.snapshot().Tasks {
		if t.Status != "COMPLETED" && t.Status != "PAUSED" && t.Status != "CANCELLED" {
			e.launch(t.ID)
		}
	}
}
func (e *Engine) launch(id string) {
	e.mu.Lock()
	if _, ok := e.running[id]; ok {
		e.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	control := &runningTask{cancel: cancel, done: make(chan struct{})}
	e.running[id] = control
	e.mu.Unlock()
	go func() {
		defer func() {
			e.mu.Lock()
			delete(e.running, id)
			e.mu.Unlock()
			close(control.done)
		}()
		select {
		case e.slots <- struct{}{}:
			defer func() { <-e.slots }()
		case <-ctx.Done():
			return
		}
		if ctx.Err() == nil {
			t := e.store.task(id)
			if t != nil && t.Kind == "share" {
				select {
				case e.shareSlot <- struct{}{}:
					defer func() { <-e.shareSlot }()
				case <-ctx.Done():
					return
				}
			}
			e.run(ctx, id)
		}
	}()
}
func (e *Engine) pause(id string) error {
	e.mu.Lock()
	control := e.running[id]
	if control != nil {
		control.cancel()
	}
	e.mu.Unlock()
	if control != nil {
		<-control.done
	}
	if t := e.store.task(id); t == nil {
		return os.ErrNotExist
	} else if t.Status == "COMPLETED" {
		return nil
	}
	return e.store.setTask(id, func(t *Task) { t.Status = "PAUSED"; t.Speed = 0 })
}
func (e *Engine) stop() {
	e.mu.Lock()
	var tasks []*runningTask
	for _, control := range e.running {
		control.cancel()
		tasks = append(tasks, control)
	}
	e.mu.Unlock()
	deadline := time.NewTimer(12 * time.Second)
	defer deadline.Stop()
	for _, t := range tasks {
		select {
		case <-t.done:
		case <-deadline.C:
			return
		}
	}
}
func (e *Engine) run(ctx context.Context, id string) {
	progressCtx, cancelProgress := context.WithCancel(ctx)
	defer cancelProgress()
	go e.speedLoop(progressCtx, id)
	t := e.store.task(id)
	if t == nil {
		return
	}
	if t.Status == "PAUSED" {
		return
	}
	fail := func(err error) {
		if errors.Is(err, context.Canceled) {
			return
		}
		_ = e.store.setTask(id, func(t *Task) { t.Status = "FAILED"; t.Message = err.Error(); t.Speed = 0; t.Attempts++ })
	}
	if e.store.snapshot().Settings.DownloadDir == "" && t.Destination == "" {
		fail(errors.New("请先设置并授权 NAS 下载目录"))
		return
	}
	_ = e.store.setTask(id, func(t *Task) { t.Status = "RESOLVING"; t.Message = "" })
	t = e.store.task(id)
	if t.Kind == "share" && t.SavedID == "" {
		fid, err := e.quark.saveShared(ctx, t)
		if err != nil {
			fail(err)
			return
		}
		if err = e.store.setTask(id, func(t *Task) { t.SavedID = fid }); err != nil {
			fail(err)
			return
		}
	}
	t = e.store.task(id)
	settings := e.store.snapshot().Settings
	maxTry := settings.Retries + 1
	if maxTry > 11 {
		maxTry = 11
	}
	for attempt := 0; attempt < maxTry; attempt++ {
		if ctx.Err() != nil {
			return
		}
		err := e.download(ctx, e.store.task(id))
		if err == nil {
			if ctx.Err() != nil {
				return
			}
			_ = e.store.setTask(id, func(x *Task) { x.Status = "COMPLETED"; x.Message = ""; x.Speed = 0; x.Downloaded = x.Size })
			if t.Kind == "share" && settings.Cleanup {
				if err := e.quark.cleanup(context.Background(), e.store.task(id)); err != nil {
					_ = e.store.setTask(id, func(x *Task) { x.Message = "下载成功；云端转存文件清理失败，请手动检查" })
				}
			}
			return
		}
		if errors.Is(err, context.Canceled) || ctx.Err() != nil {
			return
		}
		if attempt == maxTry-1 || !retryable(err) {
			fail(err)
			return
		}
		delay := time.Duration(1<<min(attempt, 5)) * time.Second
		_ = e.store.setTask(id, func(x *Task) {
			x.Status = "RETRY_WAIT"
			x.Message = fmt.Sprintf("%s，%s 后重试", err, delay)
			x.Attempts++
		})
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return
		}
	}

}
func (e *Engine) source(ctx context.Context, t *Task) (string, error) {
	if e.sourceOverride != nil {
		return e.sourceOverride(ctx, t)
	}
	id := t.RemoteID
	if t.SavedID != "" {
		id = t.SavedID
	}
	u, err := e.quark.downloadURL(ctx, id)
	if err != nil {
		return "", err
	}
	if err = validatePublicURL(ctx, u); err != nil {
		return "", err
	}
	return u, nil
}

var contentRange = regexp.MustCompile(`^bytes (\d+)-(\d+)/(\d+)$`)

// Quark API authentication and signed CDN URLs are separate. Only a domain
// controlled by quark.cn may receive the login Cookie; a signed URL on any
// other host receives no account credentials, even if it redirects elsewhere.
func quarkOwnedHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	return host == "quark.cn" || strings.HasSuffix(host, ".quark.cn")
}

func (e *Engine) newDownloadRequest(ctx context.Context, raw string, byteRange string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", webUA)
	req.Header.Set("Referer", "https://pan.quark.cn/")
	req.Header.Set("Accept-Encoding", "identity")
	if byteRange != "" {
		req.Header.Set("Range", byteRange)
	}
	if quarkOwnedHost(req.URL.Hostname()) && e.quark != nil && e.quark.store != nil {
		cookie, err := e.quark.store.credential()
		if err != nil {
			return nil, errors.New("读取本地夸克登录凭据失败")
		}
		if cookie != "" {
			req.Header.Set("Cookie", cookie)
		}
	}
	return req, nil
}

// net/http's url.Error may contain the entire signed URL (with temporary
// tokens). Keep operational errors useful without persisting credential URLs.
func safeDownloadNetworkError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.New("下载网络 timeout：请求超时")
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return errors.New("下载网络 timeout：连接超时")
	}
	lower := strings.ToLower(err.Error())
	switch {
	case strings.Contains(lower, "connection reset"):
		return errors.New("下载连接 connection reset，请重试")
	case strings.Contains(lower, "eof"):
		return errors.New("下载连接 unexpected eof，请重试")
	case strings.Contains(lower, "certificate") || strings.Contains(lower, "x509"):
		return errors.New("下载服务器 TLS 证书校验失败")
	case strings.Contains(lower, "no such host"):
		return errors.New("下载域名 DNS 解析失败")
	default:
		return errors.New("下载网络请求失败（详情不含临时直链，检查 NAS 网络及 DNS）")
	}
}

func downloadHTTPError(code int, phase string) error {
	switch code {
	case 401, 403:
		return fmt.Errorf("%s HTTP %d：下载地址被拒绝（可能已过期、缺少访问权限或账号权益受限）", phase, code)
	case 429:
		return fmt.Errorf("%s HTTP 429：夸克下载服务限流，请稍后重试", phase)
	default:
		return fmt.Errorf("%s HTTP %d", phase, code)
	}
}

func (e *Engine) rangeRequest(ctx context.Context, u string, start, end, total int64) (*http.Response, error) {
	req, err := e.newDownloadRequest(ctx, u, fmt.Sprintf("bytes=%d-%d", start, end))
	if err != nil {
		return nil, err
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, safeDownloadNetworkError(err)
	}
	if resp.StatusCode != http.StatusPartialContent {
		resp.Body.Close()
		return nil, downloadHTTPError(resp.StatusCode, "分片下载")
	}
	parts := contentRange.FindStringSubmatch(resp.Header.Get("Content-Range"))
	if len(parts) != 4 || parts[1] != strconv.FormatInt(start, 10) || parts[2] != strconv.FormatInt(end, 10) || parts[3] != strconv.FormatInt(total, 10) {
		resp.Body.Close()
		return nil, errors.New("Content-Range 与请求不一致")
	}
	return resp, nil
}
func (e *Engine) download(ctx context.Context, t *Task) error {
	settings := e.store.snapshot().Settings
	if settings.Connections < 1 {
		settings.Connections = 1
	}
	if settings.Connections > 16 {
		settings.Connections = 16
	}
	if settings.ChunkMiB < 1 {
		settings.ChunkMiB = 8
	}
	root := t.Destination
	if root == "" {
		root = settings.DownloadDir
	}
	target, err := checkDirectory(root, t.Relative)
	if err != nil {
		return err
	}
	// A crash between atomic publication and writing COMPLETED must not turn a
	// successfully delivered file into a false error on restart.
	if _, statErr := os.Lstat(target); statErr == nil && t.SHA256 != "" {
		actual, hashErr := sha256File(target)
		if hashErr != nil {
			return hashErr
		}
		if actual == t.SHA256 {
			return nil
		}
		return errors.New("目标文件存在，但与任务 SHA256 不匹配，拒绝覆盖")
	}
	dir := filepath.Dir(target)
	if err = os.MkdirAll(dir, 0750); err != nil {
		return err
	}
	if _, err = checkDirectory(root, t.Relative); err != nil {
		return err
	}
	stat, _ := os.Stat(dir)
	if stat == nil || !stat.IsDir() {
		return errors.New("目录不可用")
	}
	free, err := diskFree(dir)
	if err != nil {
		return err
	}
	if t.Size > 0 && free < uint64(t.Size-t.Downloaded)+uint64(settings.ReserveGiB)*1024*1024*1024 {
		return fmt.Errorf("NAS 存储空间不足：可用 %.1f GiB，设置预留 %d GiB（可在应用设置调整）", float64(free)/(1024*1024*1024), settings.ReserveGiB)
	}
	url, err := e.source(ctx, t)
	if err != nil {
		return err
	}
	rangeOK, err := e.probe(ctx, url, t)
	if err != nil {
		return err
	}
	tempDir := filepath.Join(root, ".quarkdrive-part")
	if err = os.MkdirAll(tempDir, 0700); err != nil {
		return err
	}
	tmp := filepath.Join(tempDir, t.ID+".part")
	if _, statErr := os.Stat(tmp); os.IsNotExist(statErr) && t.Downloaded > 0 { // lost temp file => invalidate all chunk checkpoints
		_ = e.store.setTask(t.ID, func(x *Task) {
			x.Downloaded = 0
			for i := range x.Chunks {
				x.Chunks[i].Done = false
			}
		})
		t = e.store.task(t.ID)
	}
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if t.Size < 0 {
		return errors.New("文件大小不合法")
	}
	// A recorded DONE is never trusted after a restart without checking bytes.
	// Checkpoints from older versions without checksums are re-downloaded.
	if len(t.Chunks) > 0 {
		var doneBytes int64
		var changed bool
		for i := range t.Chunks {
			ch := &t.Chunks[i]
			if !ch.Done {
				continue
			}
			valid := false
			if ch.SHA256 != "" {
				digest, hashErr := sha256RangeFile(f, ch.Start, ch.End-ch.Start+1)
				valid = hashErr == nil && digest == ch.SHA256
			}
			if valid {
				doneBytes += ch.End - ch.Start + 1
			} else {
				ch.Done = false
				ch.SHA256 = ""
				changed = true
			}
		}
		if changed || doneBytes != t.Downloaded {
			if err := e.store.setTask(t.ID, func(x *Task) { x.Chunks = t.Chunks; x.Downloaded = doneBytes }); err != nil {
				return err
			}
			t = e.store.task(t.ID)
		}
	}
	if err = f.Truncate(t.Size); err != nil {
		return err
	}
	if len(t.Chunks) == 0 {
		size := int64(settings.ChunkMiB) * 1024 * 1024
		var parts []Chunk
		for at := int64(0); at < t.Size; at += size {
			end := at + size - 1
			if end >= t.Size {
				end = t.Size - 1
			}
			parts = append(parts, Chunk{Start: at, End: end})
		}
		if t.Size == 0 {
			parts = []Chunk{}
		}
		_ = e.store.setTask(t.ID, func(t *Task) { t.Chunks = parts })
	}
	t = e.store.task(t.ID)
	if t.Size == 0 {
		return e.publishVerified(f, t.ID, tmp, target)
	}
	if !rangeOK {
		// No resumable Range support: discard old checkpoints and download a full object from offset zero.
		_ = e.store.setTask(t.ID, func(x *Task) {
			x.Chunks = nil
			x.Downloaded = 0
			x.Message = "远端不支持 Range，正在单连接下载（中断后从头开始）"
			x.Status = "DOWNLOADING"
		})
		if err = e.downloadSequential(ctx, url, f, t.ID, t.Size, settings.LimitMiB); err != nil {
			return err
		}
		return e.publishVerified(f, t.ID, tmp, target)
	}

	_ = e.store.setTask(t.ID, func(t *Task) { t.Status = "DOWNLOADING" })
	// Bounded worker pool. Chunks are committed to the durable checkpoint only after file.Sync().
	workers := settings.Connections
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	var first error
	var failMu sync.Mutex
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	for idx, part := range t.Chunks {
		if part.Done {
			continue
		}
		select {
		case <-child.Done():
			break
		case sem <- struct{}{}:
		}
		if child.Err() != nil {
			break
		}
		wg.Add(1)
		go func(idx int, part Chunk) {
			defer wg.Done()
			defer func() { <-sem }()
			u := url
			var last error
			for try := 0; try < 3; try++ {
				if child.Err() != nil {
					return
				}
				resp, err := e.rangeRequest(child, u, part.Start, part.End, t.Size)
				if err == nil && t.ETag != "" && resp.Header.Get("ETag") != "" && t.ETag != resp.Header.Get("ETag") {
					resp.Body.Close()
					err = errors.New("远端文件 ETag 变化，拒绝续传")
				}
				if err == nil {
					var written int64
					buf := make([]byte, 128*1024)
					off := part.Start
					for {
						n, rerr := resp.Body.Read(buf)
						if n > 0 {
							if off+int64(n) > part.End+1 {
								err = errors.New("分片响应超出声明长度")
								break
							}
							if limitErr := e.throttle(child, int64(n), settings.LimitMiB); limitErr != nil {
								err = limitErr
								break
							}
							if _, werr := f.WriteAt(buf[:n], off); werr != nil {
								err = werr
								break
							}
							off += int64(n)
							written += int64(n)
						}
						if rerr == io.EOF {
							break
						}
						if rerr != nil {
							err = rerr
							break
						}
					}
					resp.Body.Close()
					if err == nil && written != part.End-part.Start+1 {
						err = errors.New("分片实际字节数不符")
					}
					if err == nil {
						if err = f.Sync(); err == nil {
							var digest string
							digest, err = sha256RangeFile(f, part.Start, written)
							if err == nil {
								err = e.store.setTask(t.ID, func(x *Task) {
									x.Chunks[idx].Done = true
									x.Chunks[idx].SHA256 = digest
									x.Downloaded += written
									x.Status = "DOWNLOADING"
								})
								if err == nil {
									return
								}
							}
						}
					}
				}
				last = err
				if try < 2 {
					select {
					case <-time.After(time.Duration(try+1) * time.Second):
					case <-child.Done():
						return
					}
					nu, nerr := e.source(child, t)
					if nerr == nil {
						u = nu
					}
				}
			}
			failMu.Lock()
			if first == nil {
				first = fmt.Errorf("分片 %d: %w", idx, last)
				cancel()
			}
			failMu.Unlock()
		}(idx, part)
	}
	wg.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if first != nil {
		return first
	}
	_ = e.store.setTask(t.ID, func(t *Task) { t.Status = "VERIFYING" })
	return e.publishVerified(f, t.ID, tmp, target)
}

func sha256RangeFile(f *os.File, start, length int64) (string, error) {
	if length < 0 {
		return "", errors.New("无效分片长度")
	}
	h := sha256.New()
	n, err := io.Copy(h, io.NewSectionReader(f, start, length))
	if err != nil {
		return "", err
	}
	if n != length {
		return "", io.ErrUnexpectedEOF
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func (e *Engine) publishVerified(f *os.File, id, tmp, target string) error {
	if err := f.Sync(); err != nil {
		return err
	}
	digest, err := sha256File(tmp)
	if err != nil {
		return err
	}
	if err = e.store.setTask(id, func(t *Task) { t.SHA256 = digest; t.Status = "VERIFYING" }); err != nil {
		return err
	}
	return finishFile(f, tmp, target)
}
func finishFile(f *os.File, tmp, target string) error {
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if _, err := os.Lstat(target); err == nil { // default safety: never overwrite users' existing files
		return errors.New("目标文件已存在，未覆盖；请更换目录或重命名")
	} else if !os.IsNotExist(err) {
		return err
	}
	// Hard-link creates the destination only if it does not exist. Unlike
	// Rename, it cannot replace a file written concurrently by another process.
	if err := os.Link(tmp, target); err != nil {
		return err
	}
	if err := os.Remove(tmp); err != nil {
		return err
	}
	dir, e := os.Open(filepath.Dir(target))
	if e == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}
func sha256File(path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	_, e = io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), e
}

// Simple global rate pacing. A single timeline is shared by all active tasks.
func (e *Engine) throttle(ctx context.Context, bytes int64, limitMiB int) error {
	if limitMiB <= 0 {
		return ctx.Err()
	}
	duration := time.Duration(float64(bytes) / float64(limitMiB*1024*1024) * float64(time.Second))
	e.bandwidthMu.Lock()
	now := time.Now()
	if e.bandwidthNext.Before(now) {
		e.bandwidthNext = now
	}
	ready := e.bandwidthNext
	e.bandwidthNext = e.bandwidthNext.Add(duration)
	e.bandwidthMu.Unlock()
	if wait := time.Until(ready); wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	return ctx.Err()
}

func retryable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, str := range []string{"timeout", "deadline", "connection reset", "broken pipe", "temporary", "eof", "http 429", "http 500", "http 502", "http 503", "http 504", "过期"} {
		if strings.Contains(msg, str) {
			return true
		}
	}
	return false
}

// A single-byte preflight establishes size and source identity before trusting saved checkpoints.
func (e *Engine) probe(ctx context.Context, raw string, t *Task) (bool, error) {
	if t.Size == 0 {
		return true, nil
	}
	req, err := e.newDownloadRequest(ctx, raw, "bytes=0-0")
	if err != nil {
		return false, err
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return false, safeDownloadNetworkError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		return false, nil
	}
	if resp.StatusCode != 206 {
		return false, downloadHTTPError(resp.StatusCode, "下载链接探测")
	}
	m := contentRange.FindStringSubmatch(resp.Header.Get("Content-Range"))
	if len(m) != 4 || m[1] != "0" || m[2] != "0" || m[3] != strconv.FormatInt(t.Size, 10) {
		return false, errors.New("下载源大小或 Range 元数据与网盘信息不一致")
	}
	etag, modified := resp.Header.Get("ETag"), resp.Header.Get("Last-Modified")
	if t.ETag != "" && etag != "" && t.ETag != etag {
		return false, errors.New("远端文件 ETag 变化，拒绝续传")
	}
	if t.LastModified != "" && modified != "" && t.LastModified != modified {
		return false, errors.New("远端文件最后修改时间已变化，拒绝续传")
	}
	if t.ETag == "" && etag != "" || t.LastModified == "" && modified != "" {
		if err := e.store.setTask(t.ID, func(x *Task) {
			if x.ETag == "" {
				x.ETag = etag
			}
			if x.LastModified == "" {
				x.LastModified = modified
			}
		}); err != nil {
			return false, err
		}
	}
	return true, nil
}

// When a server ignores Range, only a complete 200 response can be committed.
func (e *Engine) downloadSequential(ctx context.Context, u string, f *os.File, taskID string, total int64, limit int) error {
	req, err := e.newDownloadRequest(ctx, u, "")
	if err != nil {
		return err
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return safeDownloadNetworkError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return downloadHTTPError(resp.StatusCode, "顺序下载")
	}
	if resp.ContentLength >= 0 && resp.ContentLength != total {
		return errors.New("顺序下载文件大小与云端元数据不一致")
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	buf := make([]byte, 128*1024)
	n := int64(0)
	reported := int64(0)
	for {
		m, re := resp.Body.Read(buf)
		if m > 0 {
			if err = e.throttle(ctx, int64(m), limit); err != nil {
				return err
			}
			if _, err = f.Write(buf[:m]); err != nil {
				return err
			}
			n += int64(m)
			if n > total {
				return errors.New("下载内容超过云端声明大小")
			}
			if n-reported >= 1024*1024 {
				reported = n
				if err := e.store.setTask(taskID, func(t *Task) { t.Downloaded = n }); err != nil {
					return err
				}
			}
		}
		if re == io.EOF {
			break
		}
		if re != nil {
			return re
		}
	}
	if n != total {
		return errors.New("顺序下载未获得完整文件")
	}
	return f.Sync()
}

func (e *Engine) speedLoop(ctx context.Context, id string) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	last := int64(0)
	if t := e.store.task(id); t != nil {
		last = t.Downloaded
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			t := e.store.task(id)
			if t == nil {
				return
			}
			rate := t.Downloaded - last
			last = t.Downloaded
			if rate < 0 {
				rate = 0
			}
			if t.Status == "DOWNLOADING" {
				_ = e.store.setTask(id, func(x *Task) { x.Speed = rate })
			}
		}
	}
}
