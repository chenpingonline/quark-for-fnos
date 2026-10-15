package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Set by scripts/build.sh from the fnOS manifest.
var appVersion = "development"

type Server struct {
	store  *Store
	quark  *Quark
	engine *Engine
	web    string
	dev    bool
	root   string
}

func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func errorReply(w http.ResponseWriter, status int, err error) {
	reply(w, status, map[string]any{"error": err.Error()})
}
func decode(r *http.Request, d any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(d)
}
func (s *Server) admin(r *http.Request) bool {
	if s.dev {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Trim-Isadmin"), "true") && r.Header.Get("X-Trim-Userid") != ""
}

// allowWriteOrigin checks browser-controlled Fetch Metadata before checking
// Origin. fnOS forwards requests through a Unix socket and may rewrite Host;
// modern browsers still send Sec-Fetch-Site: same-origin for a fetch made by
// the application itself. Never turn this into an unconditional Origin bypass.
// Non-browser clients also need the gateway administrator identity and the
// application-only request header enforced separately by serve().
func allowWriteOrigin(r *http.Request, dev bool) bool {
	site := strings.TrimSpace(strings.ToLower(r.Header.Get("Sec-Fetch-Site")))
	if site != "" && site != "same-origin" {
		return false
	}
	raw := r.Header.Get("Origin")
	if raw == "" {
		return true
	}
	origin, err := url.Parse(raw)
	if err != nil || (origin.Scheme != "http" && origin.Scheme != "https") ||
		origin.Host == "" || origin.User != nil || origin.Path != "" ||
		origin.RawQuery != "" || origin.Fragment != "" {
		return false
	}
	if strings.EqualFold(origin.Host, r.Host) {
		return true
	}
	// The proxy-facing r.Host may be localhost or an internal gateway name.
	// Only a browser-asserted same-origin fetch may bridge that difference,
	// and only when served exclusively through the fnOS Unix-socket gateway.
	return !dev && site == "same-origin"
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	// Revalidate UI resources so upgrades cannot mix old HTML with new scripts.
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; script-src 'self'; style-src 'self'; connect-src 'self'")
	path := strings.TrimPrefix(r.URL.Path, s.root)
	if path == "" && s.root != "" {
		http.Redirect(w, r, s.root+"/", http.StatusTemporaryRedirect)
		return
	}
	if path == "" || path == "/" {
		http.ServeFile(w, r, filepath.Join(s.web, "index.html"))
		return
	}
	if path == "/assets/app.js" || path == "/assets/app.css" || path == "/assets/qr.js" || path == "/assets/theme.js" || path == "/assets/file-pages.js" {
		http.ServeFile(w, r, filepath.Join(s.web, strings.TrimPrefix(path, "/")))
		return
	}
	if !strings.HasPrefix(path, "/api/v1/") {
		http.NotFound(w, r)
		return
	}
	if !s.admin(r) {
		errorReply(w, 403, errors.New("仅允许飞牛管理员经统一网关使用"))
		return
	}
	if r.Method != "GET" && r.Method != "HEAD" {
		if !allowWriteOrigin(r, s.dev) {
			errorReply(w, 403, errors.New("请求来源不匹配：请从飞牛应用入口打开，勿跨域调用"))
			return
		}
		if r.Header.Get("X-QuarkDrive-Request") != "1" {
			errorReply(w, 403, errors.New("缺少应用请求标识"))
			return
		}
	}
	sub := strings.TrimPrefix(path, "/api/v1/")
	switch {
	case sub == "health" && r.Method == "GET":
		reply(w, 200, map[string]any{"ok": true, "version": appVersion, "mode": map[bool]string{true: "development", false: "fnos"}[s.dev]})
	case sub == "auth/account" && r.Method == "GET":
		v, e := s.quark.account(r.Context())
		if e != nil {
			errorReply(w, 401, e)
		} else {
			reply(w, 200, v)
		}
	case sub == "auth/account" && r.Method == "DELETE":
		if e := s.store.setCredential(""); e != nil {
			errorReply(w, 500, e)
		} else {
			reply(w, 200, map[string]bool{"ok": true})
		}
	case sub == "auth/qr-sessions" && r.Method == "POST":
		v, e := s.quark.loginStart(r.Context())
		if e != nil {
			errorReply(w, 502, e)
		} else {
			reply(w, 200, v)
		}
	case strings.HasPrefix(sub, "auth/qr-sessions/") && r.Method == "GET":
		v, e := s.quark.loginPoll(r.Context(), strings.TrimPrefix(sub, "auth/qr-sessions/"))
		if e != nil {
			errorReply(w, 502, e)
		} else {
			reply(w, 200, v)
		}
	case sub == "files" && r.Method == "GET":
		v, e := s.quark.list(r.Context(), r.URL.Query().Get("parent"), pageArg(r))
		if e != nil {
			errorReply(w, 502, e)
		} else {
			reply(w, 200, v)
		}
	case sub == "files/search" && r.Method == "GET":
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		if q == "" {
			errorReply(w, 400, errors.New("请输入搜索关键字"))
			return
		}
		v, e := s.quark.search(r.Context(), q, pageArg(r))
		if e != nil {
			errorReply(w, 502, e)
		} else {
			reply(w, 200, v)
		}
	case sub == "shares/previews" && r.Method == "POST":
		var x struct {
			URL  string `json:"url"`
			Code string `json:"code"`
		}
		if e := decode(r, &x); e != nil {
			errorReply(w, 400, e)
			return
		}
		pr, files, e := s.quark.sharePreview(r.Context(), x.URL, x.Code)
		if e != nil {
			errorReply(w, 502, e)
		} else {
			reply(w, 200, map[string]any{"preview": pr, "files": files})
		}
	case strings.HasPrefix(sub, "shares/previews/") && r.Method == "GET":
		parts := strings.Split(sub, "/")
		if len(parts) < 4 || parts[3] != "files" {
			http.NotFound(w, r)
			return
		}
		s.quark.mu.Lock()
		pr := s.quark.previews[parts[2]]
		s.quark.mu.Unlock()
		if pr == nil || time.Now().After(pr.Expire) {
			errorReply(w, 404, errors.New("分享预览已过期，请重新解析"))
			return
		}
		v, e := s.quark.shareList(r.Context(), pr, r.URL.Query().Get("parent"), pageArg(r))
		if e != nil {
			errorReply(w, 502, e)
		} else {
			reply(w, 200, v)
		}
	case sub == "settings" && r.Method == "GET":
		p := s.store.snapshot()
		reply(w, 200, p.Settings)
	case sub == "settings" && r.Method == "PUT":
		var x Settings
		if e := decode(r, &x); e != nil {
			errorReply(w, 400, e)
			return
		}
		if x.Connections < 1 || x.Connections > 16 || x.Concurrent < 1 || x.Concurrent > 10 || x.ChunkMiB < 1 || x.ChunkMiB > 64 || x.LimitMiB < 0 || x.ReserveGiB < 0 || x.Retries < 0 || x.Retries > 10 {
			errorReply(w, 400, errors.New("下载参数超过允许范围"))
			return
		}
		allowed := false
		for _, p := range authorizedLocations(s.dev) {
			if p == x.DownloadDir {
				allowed = true
				break
			}
		}
		if !allowed {
			errorReply(w, 403, errors.New("所选目录不在 fnOS 授权目录列表中"))
			return
		}

		if x.DownloadDir != "" {
			stat, e := os.Stat(x.DownloadDir)
			if e != nil || !stat.IsDir() {
				errorReply(w, 400, errors.New("目标目录不存在"))
				return
			}
		}
		if e := s.store.mutate(func(p *Persistent) error { p.Settings = x; return nil }); e != nil {
			errorReply(w, 500, e)
		} else {
			reply(w, 200, x)
		}
	case sub == "storage/locations" && r.Method == "GET":
		dirs := authorizedLocations(s.dev)
		arr := make([]map[string]string, 0, len(dirs))
		for i, path := range dirs {
			arr = append(arr, map[string]string{"id": strconv.Itoa(i), "name": filepath.Base(path), "path": path})
		}
		reply(w, 200, map[string]any{"locations": arr})
	case sub == "tasks" && r.Method == "GET":
		reply(w, 200, map[string]any{"tasks": publicTasks(s.store.snapshot().Tasks)})
	case sub == "tasks" && r.Method == "POST":
		s.createTasks(w, r)
	case strings.HasPrefix(sub, "tasks/"):
		s.taskAction(w, r, strings.Split(sub, "/"))
	default:
		http.NotFound(w, r)
	}
}
func pageArg(r *http.Request) int {
	p, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if p < 1 {
		p = 1
	}
	if p > 10000 {
		p = 10000
	}
	return p
}
func (s *Server) createTasks(w http.ResponseWriter, r *http.Request) {
	var x struct {
		SourceKind string `json:"source_kind"`
		PreviewID  string `json:"preview_id"`
		Parent     string `json:"parent"`
		Selected   []File `json:"selected"`
		Search     string `json:"search"`
		Subdir     string `json:"subdir"`
	}
	if err := decode(r, &x); err != nil {
		errorReply(w, 400, err)
		return
	}
	if x.SourceKind != "share" && x.SourceKind != "personal" {
		errorReply(w, 400, errors.New("未知文件来源"))
		return
	}
	if len(x.Selected) == 0 || len(x.Selected) > 200 {
		errorReply(w, 400, errors.New("请选择 1–200 个文件"))
		return
	}
	subdir := strings.Trim(x.Subdir, "/")
	if subdir != "" {
		if _, e := safeRelative(subdir); e != nil {
			errorReply(w, 400, e)
			return
		}
	}
	base := s.store.snapshot().Settings.DownloadDir
	if base == "" {
		errorReply(w, 400, errors.New("尚未授权下载目录"))
		return
	}
	var pr *SharePreview
	if x.SourceKind == "share" {
		s.quark.mu.Lock()
		pr = s.quark.previews[x.PreviewID]
		s.quark.mu.Unlock()
		if pr == nil || time.Now().After(pr.Expire) {
			errorReply(w, 400, errors.New("分享预览已过期"))
			return
		}
	}
	now := time.Now()
	var tasks []*Task
	var walk func(context.Context, File, string, string, int) error
	walk = func(ctx context.Context, f File, parent, relative string, depth int) error {
		if depth > 25 || len(tasks) > 2000 {
			return errors.New("目录嵌套或文件总量超过安全限制")
		}
		if _, err := safeRelative(f.Name); err != nil {
			return err
		}
		name := f.Name
		if relative != "" {
			name = relative + "/" + name
		}
		if _, err := checkDirectory(base, name); err != nil {
			return err
		}
		if f.Folder {
			for page := 1; page <= 100; page++ {
				var p FilePage
				var err error
				if pr == nil {
					p, err = s.quark.list(ctx, f.ID, page)
				} else {
					p, err = s.quark.shareList(ctx, pr, f.ID, page)
				}
				if err != nil {
					return err
				}
				for _, child := range p.Files {
					if err = walk(ctx, child, f.ID, name, depth+1); err != nil {
						return err
					}
				}
				if len(p.Files) < 100 {
					break
				}
			}
			return nil
		}
		full := name
		if subdir != "" {
			full = subdir + "/" + name
		}
		if _, err := checkDirectory(base, full); err != nil {
			return err
		}
		tasks = append(tasks, &Task{ID: makeID(), Kind: x.SourceKind, Name: f.Name, RemoteID: f.ID, ShareParent: parent, Relative: full, Destination: base, ShareFileToken: f.Token, Size: f.Size, Status: "QUEUED", Created: now, Updated: now})
		if pr != nil {
			tasks[len(tasks)-1].ShareID = pr.ShareID
			tasks[len(tasks)-1].ShareToken = pr.Token
		}
		return nil
	}
	for _, item := range x.Selected { // Revalidate every selected ID against a provider-owned page.
		var lookup File
		found := false
		for page := 1; page <= 100 && !found; page++ {
			var p FilePage
			var err error
			if pr != nil {
				p, err = s.quark.shareList(r.Context(), pr, x.Parent, page)
			} else if x.Search != "" {
				p, err = s.quark.search(r.Context(), x.Search, page)
			} else {
				p, err = s.quark.list(r.Context(), x.Parent, page)
			}
			if err != nil {
				errorReply(w, 502, err)
				return
			}
			for _, v := range p.Files {
				if v.ID == item.ID {
					lookup = v
					found = true
					break
				}
			}
			if len(p.Files) < 100 {
				break
			}
		}
		if !found {
			errorReply(w, 400, errors.New("所选文件已变化，请刷新目录"))
			return
		}
		if err := walk(r.Context(), lookup, lookup.Parent, "", 0); err != nil {
			errorReply(w, 400, err)
			return
		}
	}
	if len(tasks) == 0 {
		errorReply(w, 400, errors.New("未找到可下载文件"))
		return
	}
	if err := s.store.mutate(func(p *Persistent) error {
		occupied := map[string]bool{}
		for _, old := range p.Tasks {
			occupied[filepath.Join(old.Destination, old.Relative)] = true
		}
		for _, task := range tasks {
			original := task.Relative
			for i := 0; i < 10000; i++ {
				candidate := original
				if i > 0 {
					ext := filepath.Ext(original)
					candidate = strings.TrimSuffix(original, ext) + fmt.Sprintf(" (%d)", i) + ext
				}
				full, err := checkDirectory(task.Destination, candidate)
				if err != nil {
					return err
				}
				_, err = os.Lstat(full)
				if !occupied[full] && errors.Is(err, os.ErrNotExist) {
					task.Relative = candidate
					occupied[full] = true
					break
				}
				if err != nil && !errors.Is(err, os.ErrNotExist) {
					return err
				}
				if i == 9999 {
					return errors.New("重复文件名数量超过限制")
				}
			}
		}
		p.Tasks = append(p.Tasks, tasks...)
		return nil
	}); err != nil {
		errorReply(w, 500, err)
		return
	}
	for _, t := range tasks {
		s.engine.launch(t.ID)
	}
	reply(w, 201, map[string]any{"created": len(tasks), "ids": taskIDs(tasks)})
}
func taskIDs(ts []*Task) []string {
	var ids []string
	for _, t := range ts {
		ids = append(ids, t.ID)
	}
	return ids
}
func (s *Server) taskAction(w http.ResponseWriter, r *http.Request, parts []string) {
	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}
	id := parts[1]
	t := s.store.task(id)
	if t == nil {
		http.NotFound(w, r)
		return
	}
	if len(parts) == 2 && r.Method == "GET" {
		reply(w, 200, publicTasks([]*Task{t})[0])
		return
	}
	if len(parts) != 3 || r.Method != "POST" {
		http.NotFound(w, r)
		return
	}
	switch parts[2] {
	case "pause":
		if err := s.engine.pause(id); err != nil {
			errorReply(w, 500, err)
			return
		}
	case "resume", "retry":
		if t.Status == "COMPLETED" {
			errorReply(w, 400, errors.New("已完成任务无需恢复"))
			return
		}
		if err := s.store.setTask(id, func(x *Task) { x.Status = "QUEUED"; x.Message = "" }); err != nil {
			errorReply(w, 500, err)
			return
		}
		s.engine.launch(id)
	default:
		http.NotFound(w, r)
		return
	}
	reply(w, 200, map[string]bool{"ok": true})
}
func main() {
	data := os.Getenv("QUARKDRIVE_DATA_DIR")
	if data == "" {
		data = "./data"
	}
	web := os.Getenv("QUARKDRIVE_WEB_DIR")
	if web == "" {
		web = "../frontend"
	}
	store, err := newStore(data)
	if err != nil {
		log.Fatal(err)
	}
	root := "/app/quarkdrive"
	dev := os.Getenv("QUARKDRIVE_DEV") == "1"
	if dev {
		root = ""
	}
	shared := os.Getenv("QUARKDRIVE_DOWNLOAD_DIR")
	if shared != "" {
		// Initialize the default only once. Do not overwrite an authorized
		// directory selected by the user every time the service restarts.
		p := store.snapshot()
		allowed := false
		for _, loc := range authorizedLocations(dev) {
			if loc == p.Settings.DownloadDir {
				allowed = true
				break
			}
		}
		if !allowed {
			_ = store.mutate(func(p *Persistent) error { p.Settings.DownloadDir = shared; return nil })
		}
	} else if dev && store.snapshot().Settings.DownloadDir == "" {
		dir, _ := filepath.Abs("./downloads")
		_ = os.MkdirAll(dir, 0750)
		_ = store.mutate(func(p *Persistent) error { p.Settings.DownloadDir = dir; return nil })
	}
	q := newQuark(store)
	en := newEngine(store, q)
	s := &Server{store: store, quark: q, engine: en, web: web, dev: dev, root: root}
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.serve)
	var listener net.Listener
	if dev {
		listener, err = net.Listen("tcp", "127.0.0.1:18088")
	} else {
		sock := os.Getenv("QUARKDRIVE_SOCKET")
		if sock == "" {
			sock = "/tmp/quarkdrive.sock"
		}
		_ = os.Remove(sock)
		listener, err = net.Listen("unix", sock)
		if err == nil {
			_ = os.Chmod(sock, 0660)
		}
	}
	if err != nil {
		log.Fatal(err)
	}
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	en.start()
	go func() {
		log.Printf("QuarkDrive listening %s", listener.Addr())
		if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("server: %v", err)
		}
	}()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	en.stop()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	_ = listener.Close()
	fmt.Println("QuarkDrive stopped")
}

// Ensures production package does not depend on external network to start.
var _ io.Reader

func publicTasks(tasks []*Task) []*Task {
	result := make([]*Task, 0, len(tasks))
	for _, t := range tasks {
		cp := *t
		cp.ShareToken = ""
		cp.ShareFileToken = ""
		result = append(result, &cp)
	}
	return result
}

func authorizedLocations(dev bool) []string {
	var paths []string
	seen := map[string]bool{}
	for _, v := range []string{os.Getenv("QUARKDRIVE_DOWNLOAD_DIR"), os.Getenv("TRIM_DATA_SHARE_PATHS"), os.Getenv("TRIM_DATA_ACCESSIBLE_PATHS")} {
		for _, part := range strings.Split(v, ":") {
			if part == "" || !filepath.IsAbs(part) {
				continue
			}
			cleaned := filepath.Clean(part)
			if !seen[cleaned] {
				seen[cleaned] = true
				paths = append(paths, cleaned)
			}
		}
	}
	if dev && len(paths) == 0 {
		projectDownloads, _ := filepath.Abs("./downloads")
		paths = append(paths, projectDownloads)
	}
	return paths
}
