package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const driveBase = "https://drive-pc.quark.cn/1/clouddrive"
const webUA = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/124 Safari/537.36"

// Download-link issuance and CDN requests use the same desktop client profile.
// Listing and QR authentication retain their existing browser profile.
const downloadAPI = "https://drive.quark.cn/1/clouddrive/file/download"
const downloadUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) quark-cloud-drive/2.5.20 Chrome/100.0.4896.160 Electron/18.3.5.4-b478491100 Safari/537.36 Channel/pckk_other_ch"

type Quark struct {
	store    *Store
	client   *http.Client
	mu       sync.Mutex
	qr       map[string]*QRSession
	previews map[string]*SharePreview
}

func newQuark(s *Store) *Quark {
	return &Quark{store: s, client: &http.Client{Timeout: 35 * time.Second}, qr: map[string]*QRSession{}, previews: map[string]*SharePreview{}}
}
func (q *Quark) request(ctx context.Context, method, endpoint string, params url.Values, body any, login bool) (map[string]any, error) {
	return q.requestWithUserAgent(ctx, method, endpoint, params, body, login, webUA)
}

func (q *Quark) requestWithUserAgent(ctx context.Context, method, endpoint string, params url.Values, body any, login bool, userAgent string) (map[string]any, error) {
	if params == nil {
		params = url.Values{}
	}
	if strings.Contains(endpoint, "/1/clouddrive/") {
		params.Set("pr", "ucpro")
		params.Set("fr", "pc")
		if endpoint != downloadAPI {
			params.Set("uc_param_str", "")
			params.Set("__t", strconv.FormatInt(time.Now().UnixMilli(), 10))
		}
	}
	u, e := url.Parse(endpoint)
	if e != nil {
		return nil, e
	}
	u.RawQuery = params.Encode()
	var r io.Reader
	if body != nil {
		buf, e := json.Marshal(body)
		if e != nil {
			return nil, e
		}
		r = bytes.NewReader(buf)
	}
	req, e := http.NewRequestWithContext(ctx, method, u.String(), r)
	if e != nil {
		return nil, e
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Referer", "https://pan.quark.cn/")
	req.Header.Set("Origin", "https://pan.quark.cn")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if login {
		cookie, e := q.store.credential()
		if e != nil {
			return nil, e
		}
		if cookie == "" {
			return nil, errors.New("请先扫码登录夸克")
		}
		req.Header.Set("Cookie", cookie)
	}
	resp, e := q.client.Do(req)
	if e != nil {
		return nil, e
	}
	defer resp.Body.Close()
	if login && len(resp.Cookies()) > 0 {
		q.rotateCookie(resp.Cookies())
	}
	if resp.StatusCode >= 400 {
		return nil, quarkHTTPError(resp.StatusCode, resp.Body)
	}
	var v map[string]any
	if e = json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&v); e != nil {
		return nil, e
	}
	status, _ := v["status"].(float64)
	code, _ := v["code"].(float64)
	if status != 0 && status != 200 && status != 2000000 || code != 0 || (v["code"] != nil && str(v["code"]) != "0" && str(v["code"]) != "OK" && str(v["code"]) != "200") {
		msg, _ := v["message"].(string)
		return nil, fmt.Errorf("夸克接口错误 %v/%v: %s", status, code, msg)
	}
	return v, nil
}

// Keep structured numeric error identifiers, never the raw response: it may
// include account identifiers, credentials or signed URLs.
func quarkHTTPError(httpStatus int, body io.Reader) error {
	var response map[string]json.RawMessage
	decoder := json.NewDecoder(io.LimitReader(body, 64<<10))
	details := []string{}
	if decoder.Decode(&response) == nil {
		for _, key := range []string{"code", "status"} {
			var number json.Number
			if json.Unmarshal(response[key], &number) == nil {
				if value, err := number.Int64(); err == nil {
					details = append(details, fmt.Sprintf("%s=%d", key, value))
				}
			}
		}
	}
	if len(details) > 0 {
		return fmt.Errorf("夸克接口 HTTP %d（%s）", httpStatus, strings.Join(details, "，"))
	}
	return fmt.Errorf("夸克接口 HTTP %d（未返回可识别的错误码）", httpStatus)
}
func dataMap(m map[string]any) map[string]any {
	v, _ := m["data"].(map[string]any)
	if v == nil {
		return map[string]any{}
	}
	return v
}
func str(v any) string {
	if v == nil {
		return ""
	}
	switch a := v.(type) {
	case string:
		return a
	case float64:
		return strconv.FormatFloat(a, 'f', -1, 64)
	default:
		return fmt.Sprint(a)
	}
}
func num(v any) int64 { a, _ := v.(float64); return int64(a) }

// Quark currently exposes modified timestamps in milliseconds on file rows.
// Preserve an unknown timestamp as zero rather than presenting an invented date.
func quarkTimeMs(m map[string]any) int64 {
	stamp := num(m["updated_at"])
	if stamp == 0 {
		stamp = num(m["created_at"])
	}
	if stamp > 0 && stamp < 100000000000 {
		stamp *= 1000
	} // seconds -> ms
	return stamp
}
func parseFiles(d map[string]any) FilePage {
	var files []File
	arr, _ := d["list"].([]any)
	for _, x := range arr {
		m, ok := x.(map[string]any)
		if !ok {
			continue
		}
		files = append(files, File{ID: str(m["fid"]), Name: str(m["file_name"]), Folder: num(m["file_type"]) == 0, Size: num(m["size"]), ModifiedAt: quarkTimeMs(m), Parent: str(m["pdir_fid"]), Token: str(m["share_fid_token"])})
	}
	if files == nil {
		files = []File{}
	}
	return FilePage{Files: files, Total: int(num(d["total"]))}
}
func (q *Quark) account(ctx context.Context) (map[string]any, error) {
	m, e := q.request(ctx, "GET", "https://pan.quark.cn/account/info", nil, nil, true)
	if e != nil {
		return nil, e
	}
	d := dataMap(m)
	if str(d["nickname"]) == "" && str(d["uid"]) == "" {
		return nil, errors.New("夸克登录状态无效，需要重新扫码")
	}
	return map[string]any{"nickname": str(d["nickname"]), "uid": str(d["uid"]), "authenticated": true}, nil
}
func (q *Quark) loginStart(ctx context.Context) (*QRSession, error) {
	params := url.Values{"client_id": {"532"}, "v": {"1.2"}, "request_id": {makeID()}}
	m, e := q.request(ctx, "GET", "https://uop.quark.cn/cas/ajax/getTokenForQrcodeLogin", params, nil, false)
	if e != nil {
		return nil, e
	}
	members, _ := dataMap(m)["members"].(map[string]any)
	token := str(members["token"])
	if token == "" {
		return nil, errors.New("二维码 token 为空")
	}
	link := "https://su.quark.cn/4_eMHBJ?token=" + url.QueryEscape(token) + "&client_id=532&ssb=weblogin&uc_param_str=&uc_biz_str=" + url.QueryEscape("S:custom|OPT:SAREA@0|OPT:IMMERSIVE@1|OPT:BACK_BTN_STYLE@0")
	session := &QRSession{ID: makeID(), URL: link, Status: "WAITING", Expire: time.Now().Add(5 * time.Minute), token: token}
	q.mu.Lock()
	q.qr[session.ID] = session
	q.mu.Unlock()
	return session, nil
}
func (q *Quark) qrSnapshot(id string) *QRSession {
	q.mu.Lock()
	defer q.mu.Unlock()
	s := q.qr[id]
	if s == nil {
		return nil
	}
	copy := *s
	return &copy
}
func (q *Quark) qrUpdate(id, status string) *QRSession {
	q.mu.Lock()
	defer q.mu.Unlock()
	s := q.qr[id]
	if s == nil {
		return nil
	}
	if s.Status != "SUCCESS" {
		s.Status = status
		if status == "SUCCESS" {
			s.token = ""
		}
	}
	copy := *s
	return &copy
}
func (q *Quark) loginPoll(ctx context.Context, id string) (*QRSession, error) {
	s := q.qrSnapshot(id)
	if s == nil {
		return nil, errors.New("登录会话不存在")
	}
	if time.Now().After(s.Expire) {
		return q.qrUpdate(id, "EXPIRED"), nil
	}
	if s.Status == "SUCCESS" || s.Status == "EXPIRED" || s.Status == "CANCELLED" {
		return s, nil
	}
	params := url.Values{"client_id": {"532"}, "v": {"1.2"}, "token": {s.token}, "request_id": {makeID()}}
	m, e := q.request(ctx, "GET", "https://uop.quark.cn/cas/ajax/getServiceTicketByQrcodeToken", params, nil, false)
	if e != nil {
		if strings.Contains(e.Error(), "50004002") {
			return q.qrUpdate(id, "EXPIRED"), nil
		}
		if strings.Contains(e.Error(), "50004004") {
			return q.qrUpdate(id, "CANCELLED"), nil
		}
		return q.qrUpdate(id, "WAITING"), nil
	}
	members, _ := dataMap(m)["members"].(map[string]any)
	ticket := str(members["service_ticket"])
	if ticket == "" {
		return q.qrUpdate(id, "SCANNED"), nil
	}
	u := "https://pan.quark.cn/account/info?st=" + url.QueryEscape(ticket) + "&lw=scan"
	req, e := http.NewRequestWithContext(ctx, "GET", u, nil)
	if e != nil {
		return nil, e
	}
	req.Header.Set("User-Agent", webUA)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Timeout: 20 * time.Second, Jar: jar}
	resp, e := client.Do(req)
	if e != nil {
		return nil, e
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("确认登录 HTTP %d", resp.StatusCode)
	}
	var all []string
	seen := map[string]bool{}
	for _, host := range []string{"https://pan.quark.cn/", "https://drive-pc.quark.cn/", "https://uop.quark.cn/"} {
		endpoint, _ := url.Parse(host)
		for _, c := range jar.Cookies(endpoint) {
			if !seen[c.Name] {
				seen[c.Name] = true
				all = append(all, c.Name+"="+c.Value)
			}
		}
	}
	if len(all) == 0 {
		return nil, errors.New("登录接口没有返回有效 Cookie")
	}
	if e = q.store.setCredential(strings.Join(all, "; ")); e != nil {
		return nil, e
	}
	return q.qrUpdate(id, "SUCCESS"), nil
}

var shareURL = regexp.MustCompile(`(?i)https?://(?:pan|drive)\.quark\.cn/s/([A-Za-z0-9_-]+)(?:\?[^\s<>]*)?`)

func parseShare(raw string) (string, string, error) {
	match := shareURL.FindStringSubmatch(raw)
	if len(match) < 2 {
		return "", "", errors.New("只能解析 pan.quark.cn/s/... 或 drive.quark.cn/s/... 分享地址")
	}
	u, e := url.Parse(match[0])
	if e != nil {
		return "", "", e
	}
	return match[1], u.Query().Get("pwd"), nil
}
func (q *Quark) list(ctx context.Context, parent string, page int) (FilePage, error) {
	if parent == "" {
		parent = "0"
	}
	if page < 1 {
		page = 1
	}
	param := url.Values{"pdir_fid": {parent}, "_page": {strconv.Itoa(page)}, "_size": {"100"}, "_sort": {"file_type:asc,file_name:asc"}, "_fetch_total": {"1"}}
	m, e := q.request(ctx, "GET", driveBase+"/file/sort", param, nil, true)
	if e != nil {
		return FilePage{}, e
	}
	r := parseFiles(dataMap(m))
	r.Page = page
	return r, nil
}
func (q *Quark) search(ctx context.Context, k string, page int) (FilePage, error) {
	if page < 1 {
		page = 1
	}
	p := url.Values{"q": {k}, "_page": {strconv.Itoa(page)}, "_size": {"100"}, "_fetch_total": {"1"}, "_sort": {"file_name:asc"}, "_is_hl": {"1"}}
	m, e := q.request(ctx, "GET", driveBase+"/file/search", p, nil, true)
	if e != nil {
		return FilePage{}, e
	}
	r := parseFiles(dataMap(m))
	r.Page = page
	return r, nil
}
func (q *Quark) sharePreview(ctx context.Context, raw, pass string) (*SharePreview, FilePage, error) {
	id, pwd, e := parseShare(raw)
	if e != nil {
		return nil, FilePage{}, e
	}
	if pass == "" {
		pass = pwd
	}
	m, e := q.request(ctx, "POST", driveBase+"/share/sharepage/token", nil, map[string]any{"pwd_id": id, "passcode": pass}, true)
	if e != nil {
		return nil, FilePage{}, e
	}
	token := str(dataMap(m)["stoken"])
	if token == "" {
		return nil, FilePage{}, errors.New("分享口令无效或已过期")
	}
	pr := &SharePreview{ID: makeID(), ShareID: id, Token: token, Title: "分享文件", Expire: time.Now().Add(30 * time.Minute)}
	q.mu.Lock()
	q.previews[pr.ID] = pr
	q.mu.Unlock()
	files, e := q.shareList(ctx, pr, "0", 1)
	return pr, files, e
}
func (q *Quark) shareList(ctx context.Context, pr *SharePreview, parent string, page int) (FilePage, error) {
	if parent == "" {
		parent = "0"
	}
	params := url.Values{"pwd_id": {pr.ShareID}, "stoken": {pr.Token}, "pdir_fid": {parent}, "force": {"0"}, "_page": {strconv.Itoa(page)}, "_size": {"100"}}
	m, e := q.request(ctx, "GET", driveBase+"/share/sharepage/detail", params, nil, true)
	if e != nil {
		return FilePage{}, e
	}
	d := dataMap(m)
	if pr.Title == "分享文件" && str(d["title"]) != "" {
		pr.Title = str(d["title"])
	}
	p := parseFiles(d)
	p.Page = page
	return p, nil
}
func (q *Quark) downloadURL(ctx context.Context, fid string) (string, error) {
	m, e := q.requestWithUserAgent(ctx, "POST", downloadAPI, nil, map[string]any{"fids": []string{fid}}, true, downloadUA)
	if e != nil {
		return "", fmt.Errorf("获取文件下载地址失败：%w", e)
	}
	list, _ := m["data"].([]any)
	if len(list) == 0 {
		return "", errors.New("直链为空")
	}
	v, _ := list[0].(map[string]any)
	u := str(v["download_url"])
	if u == "" {
		return "", errors.New("接口未返回下载地址")
	}
	return u, nil
}
func (q *Quark) createFolder(ctx context.Context, name, parent string) (string, error) {
	m, e := q.request(ctx, "POST", driveBase+"/file", nil, map[string]any{"file_name": name, "pdir_fid": parent, "dir_init_lock": false, "dir_path": ""}, true)
	if e != nil {
		return "", e
	}
	id := str(dataMap(m)["fid"])
	if id == "" {
		return "", errors.New("创建临时文件夹未返回 fid")
	}
	return id, nil
}
func (q *Quark) ensureTemp(ctx context.Context) (string, error) {
	p := q.store.snapshot()
	if p.TempFolderID != "" {
		return p.TempFolderID, nil
	}
	for page := 1; page <= 20; page++ {
		lst, e := q.list(ctx, "0", page)
		if e != nil {
			return "", e
		}
		for _, f := range lst.Files {
			if f.Name == "QuarkDriveTemp" && f.Folder {
				return f.ID, q.store.mutate(func(p *Persistent) error { p.TempFolderID = f.ID; return nil })
			}
		}
		if len(lst.Files) < 100 {
			break
		}
	}
	id, e := q.createFolder(ctx, "QuarkDriveTemp", "0")
	if e != nil {
		return "", e
	}
	return id, q.store.mutate(func(p *Persistent) error { p.TempFolderID = id; return nil })
}
func (q *Quark) saveShared(ctx context.Context, t *Task) (string, error) {
	root, e := q.ensureTemp(ctx)
	if e != nil {
		return "", e
	}
	folder, e := q.createFolder(ctx, "task-"+t.ID[:12], root)
	if e != nil {
		return "", e
	}
	_ = q.store.setTask(t.ID, func(x *Task) { x.TempFolderID = folder })
	body := map[string]any{"fid_list": []string{t.RemoteID}, "fid_token_list": []string{t.ShareFileToken}, "pwd_id": t.ShareID, "stoken": t.ShareToken, "pdir_fid": t.ShareParent, "scene": "link", "to_pdir_fid": folder}
	m, e := q.request(ctx, "POST", driveBase+"/share/sharepage/save", nil, body, true)
	if e != nil {
		return "", e
	}
	taskID := str(dataMap(m)["task_id"])
	if taskID == "" {
		return "", errors.New("转存任务 ID 为空")
	}
	for i := 0; i < 30; i++ {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(2 * time.Second):
		}
		params := url.Values{"task_id": {taskID}, "retry_index": {"0"}}
		result, e := q.request(ctx, "GET", driveBase+"/task", params, nil, true)
		if e != nil {
			continue
		}
		d := dataMap(result)
		save, _ := d["save_as"].(map[string]any)
		for _, key := range []string{"save_as_top_fids", "save_as_select_top_fids"} {
			a, _ := save[key].([]any)
			if len(a) > 0 {
				fid := str(a[0])
				if fid != "" {
					return fid, nil
				}
			}
		}
	}
	return "", errors.New("分享转存超时；已保留远端临时文件夹，请检查网盘")
}
func (q *Quark) cleanup(ctx context.Context, t *Task) error {
	if t.SavedID == "" {
		return nil
	}
	_, e := q.request(ctx, "POST", driveBase+"/file/delete", nil, map[string]any{"action_type": 2, "filelist": []string{t.SavedID}, "exclude_fids": []string{}}, true)
	return e
}

// Refresh only the authentication cookies rotated by Quark responses.
func (q *Quark) rotateCookie(fresh []*http.Cookie) {
	updates := map[string]string{}
	for _, c := range fresh {
		if c.Value == "" {
			continue
		}
		switch c.Name {
		case "__puus", "__pus", "__kps", "__uid", "__kp":
			updates[c.Name] = c.Value
		}
	}
	if len(updates) == 0 {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	existing, e := q.store.credential()
	if e != nil || existing == "" {
		return
	}
	ordered := []string{}
	cookieMap := map[string]string{}
	for _, v := range strings.Split(existing, ";") {
		parts := strings.SplitN(strings.TrimSpace(v), "=", 2)
		if len(parts) != 2 {
			continue
		}
		k := parts[0]
		if _, ok := cookieMap[k]; !ok {
			ordered = append(ordered, k)
		}
		cookieMap[k] = parts[1]
	}
	changed := false
	for k, v := range updates {
		if cookieMap[k] != v {
			cookieMap[k] = v
			changed = true
		}
		found := false
		for _, old := range ordered {
			if old == k {
				found = true
				break
			}
		}
		if !found {
			ordered = append(ordered, k)
		}
	}
	if !changed {
		return
	}
	var parts []string
	for _, k := range ordered {
		parts = append(parts, k+"="+cookieMap[k])
	}
	_ = q.store.setCredential(strings.Join(parts, "; "))
}
