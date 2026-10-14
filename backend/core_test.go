package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestParseShare(t *testing.T) {
	tests := []struct{ url, id, code string }{
		{"https://pan.quark.cn/s/abc123?pwd=12ab", "abc123", "12ab"},
		{"转发： https://drive.quark.cn/s/ABC_123?pwd=x1&from=chat", "ABC_123", "x1"},
	}
	for _, test := range tests {
		id, code, err := parseShare(test.url)
		if err != nil || id != test.id || code != test.code {
			t.Fatalf("got %q %q %v", id, code, err)
		}
	}
	if _, _, e := parseShare("https://evil.com/s/abc"); e == nil {
		t.Fatal("accepted untrusted share URL")
	}
}
func TestPaths(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"../bad", "a/../b", "/etc/passwd", "a\\b", "a//b", "a/./b"} {
		if _, e := checkDirectory(root, name); e == nil {
			t.Errorf("unsafe %q", name)
		}
	}
	if _, e := checkDirectory(root, "电影/测试.mkv"); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(root, filepath.Join(root, "symlink")); e != nil {
		t.Fatal(e)
	}
	if _, e := checkDirectory(root, "symlink/evil"); e == nil {
		t.Fatal("followed symlink")
	}
}
func TestCredentialStoreDurability(t *testing.T) {
	dir := t.TempDir()
	s, e := newStore(dir)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.setCredential("__puus=very_sensitive; __uid=1"); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(filepath.Join(dir, "state.json"))
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Contains(b, []byte("very_sensitive")) {
		t.Fatal("stored plaintext credential")
	}
	s2, e := newStore(dir)
	if e != nil {
		t.Fatal(e)
	}
	cookie, e := s2.credential()
	if e != nil || cookie != "__puus=very_sensitive; __uid=1" {
		t.Fatalf("credential round trip failed: %v", e)
	}
	if stat, _ := os.Stat(filepath.Join(dir, "state.json")); stat.Mode().Perm() != 0600 {
		t.Fatal("credential file is not private")
	}
}
func makeTestSource(t *testing.T, content []byte) (*httptest.Server, *bool) {
	t.Helper()
	corrupt := new(bool)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hdr := r.Header.Get("Range")
		if !strings.HasPrefix(hdr, "bytes=") {
			http.Error(w, "range required", 400)
			return
		}
		var a, b int64
		if _, e := fmt.Sscanf(hdr, "bytes=%d-%d", &a, &b); e != nil {
			http.Error(w, "bad range", 416)
			return
		}
		if a < 0 || b >= int64(len(content)) || a > b {
			w.WriteHeader(416)
			return
		}
		if *corrupt {
			w.Header().Set("Content-Range", "bytes 0-0/1")
		} else {
			w.Header().Set("Content-Range", "bytes "+strconv.FormatInt(a, 10)+"-"+strconv.FormatInt(b, 10)+"/"+strconv.Itoa(len(content)))
		}
		w.Header().Set("Content-Length", strconv.FormatInt(b-a+1, 10))
		w.WriteHeader(206)
		_, _ = w.Write(content[a : b+1])
	}))
	return server, corrupt
}
func TestRangeAndResume(t *testing.T) {
	content := bytes.Repeat([]byte("quarkdrive_test_"), 90000)
	server, corrupt := makeTestSource(t, content)
	defer server.Close()
	dir := t.TempDir()
	store, e := newStore(filepath.Join(dir, "data"))
	if e != nil {
		t.Fatal(e)
	}
	_ = store.mutate(func(p *Persistent) error {
		p.Settings.DownloadDir = dir
		p.Settings.ReserveGiB = 0
		p.Settings.ChunkMiB = 1
		p.Settings.Connections = 2
		return nil
	})
	task := &Task{ID: makeID(), Kind: "personal", RemoteID: "f1", Name: "hello.bin", Relative: "samples/hello.bin", Destination: dir, Size: int64(len(content)), Status: "QUEUED", Created: time.Now()}
	_ = store.mutate(func(p *Persistent) error { p.Tasks = append(p.Tasks, task); return nil })
	eng := newEngine(store, nil)
	eng.sourceOverride = func(context.Context, *Task) (string, error) { return server.URL, nil }
	eng.client = server.Client()
	eng.client.CheckRedirect = nil
	*corrupt = true
	err := eng.download(context.Background(), task)
	if err == nil {
		t.Fatal("mismatched Content-Range was accepted")
	}
	*corrupt = false
	// Retry resumes the original per-chunk metadata and completes without overwriting another file.
	if err = eng.download(context.Background(), store.task(task.ID)); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "samples", "hello.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Fatal("file content mismatch")
	}
	// Replaying an already published task must be idempotent after a crash.
	if e := eng.download(context.Background(), store.task(task.ID)); e != nil {
		t.Fatalf("replay should succeed without replacing destination: %v", e)
	}
	got, err = os.ReadFile(filepath.Join(dir, "samples", "hello.bin"))
	if err != nil || !bytes.Equal(got, content) {
		t.Fatal("replay corrupted destination")
	}
}
func TestResumeLostTemporaryFile(t *testing.T) {
	content := []byte("data")
	server, _ := makeTestSource(t, content)
	defer server.Close()
	dir := t.TempDir()
	s, _ := newStore(filepath.Join(dir, "data"))
	_ = s.mutate(func(p *Persistent) error { p.Settings.DownloadDir = dir; p.Settings.ReserveGiB = 0; return nil })
	task := &Task{ID: makeID(), Kind: "personal", Name: "x", RemoteID: "x", Relative: "x", Destination: dir, Size: 4, Status: "DOWNLOADING", Downloaded: 4, Chunks: []Chunk{{Start: 0, End: 3, Done: true}}}
	_ = s.mutate(func(p *Persistent) error { p.Tasks = append(p.Tasks, task); return nil })
	e := newEngine(s, nil)
	e.sourceOverride = func(context.Context, *Task) (string, error) { return server.URL, nil }
	e.client = server.Client()
	if err := e.download(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "x"))
	if err != nil || !bytes.Equal(b, content) {
		t.Fatalf("restart produced stale data: %q %v", b, err)
	}
}
func TestGatewayAuth(t *testing.T) {
	s := &Server{dev: false, root: "/app/quarkdrive"}
	r := httptest.NewRequest("GET", "/app/quarkdrive/api/v1/health", nil)
	w := httptest.NewRecorder()
	s.serve(w, r)
	if w.Code != 403 {
		t.Fatalf("expected 403 got %d", w.Code)
	}
}
func TestRangeRejects200(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200); io.WriteString(w, "abcd") }))
	defer server.Close()
	e := &Engine{client: server.Client()}
	resp, err := e.rangeRequest(context.Background(), server.URL, 0, 3, 4)
	if err == nil {
		resp.Body.Close()
		t.Fatal("accepted non-206 range response")
	}
}

func TestShareSecretsEncryptedAtRest(t *testing.T) {
	dir := t.TempDir()
	s, err := newStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	id := makeID()
	err = s.mutate(func(p *Persistent) error {
		p.Tasks = append(p.Tasks, &Task{ID: id, ShareToken: "sensitive_sharing_token", ShareFileToken: "another_secret", Status: "QUEUED"})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte("sensitive_sharing_token")) || bytes.Contains(b, []byte("another_secret")) {
		t.Fatal("share tokens persisted in plaintext")
	}
	restored, err := newStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	tk := restored.task(id)
	if tk.ShareToken != "sensitive_sharing_token" || tk.ShareFileToken != "another_secret" {
		t.Fatal("encrypted share token lost")
	}
}

func TestPauseReturnsWhenQueuedOrCanceled(t *testing.T) {
	dir := t.TempDir()
	s, err := newStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	id := makeID()
	if err = s.mutate(func(p *Persistent) error {
		p.Tasks = append(p.Tasks, &Task{ID: id, Kind: "personal", Status: "QUEUED"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	e := newEngine(s, nil)
	// Consume the only scheduling slot to simulate a queued task.
	e.slots = make(chan struct{}, 1)
	e.slots <- struct{}{}
	e.launch(id)
	done := make(chan error, 1)
	go func() { done <- e.pause(id) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("pause deadlocked on task done signal")
	}
	if got := s.task(id).Status; got != "PAUSED" {
		t.Fatalf("status %s", got)
	}
}

func TestFallbackSequentialWhenNoRange(t *testing.T) {
	body := bytes.Repeat([]byte("norange"), 10000)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		_, _ = w.Write(body)
	}))
	defer server.Close()
	dir := t.TempDir()
	s, err := newStore(filepath.Join(dir, "data"))
	if err != nil {
		t.Fatal(err)
	}
	_ = s.mutate(func(p *Persistent) error { p.Settings.DownloadDir = dir; p.Settings.ReserveGiB = 0; return nil })
	task := &Task{ID: makeID(), Kind: "personal", Relative: "norange.bin", Destination: dir, Size: int64(len(body))}
	_ = s.mutate(func(p *Persistent) error { p.Tasks = append(p.Tasks, task); return nil })
	e := newEngine(s, nil)
	e.client = server.Client()
	e.sourceOverride = func(context.Context, *Task) (string, error) { return server.URL, nil }
	if err = e.download(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "norange.bin"))
	if err != nil || !bytes.Equal(body, got) {
		t.Fatalf("sequential output incorrect: %v", err)
	}
}
