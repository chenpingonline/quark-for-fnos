package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestDownloadRequestHeadersOnlyTrustQuarkDomains(t *testing.T) {
	dir := t.TempDir()
	s, err := newStore(filepath.Join(dir, "store"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.setCredential("__puus=secret; __uid=1"); err != nil {
		t.Fatal(err)
	}
	e := newEngine(s, newQuark(s))
	for _, tc := range []struct {
		url    string
		cookie bool
	}{
		{"https://dl-pc-zb.drive.quark.cn/a?token=signed", true},
		{"https://drive.quark.cn/a", true},
		{"https://fakequark.cn/a", false},
		{"https://quark.cn.evil.example/a", false},
		{"https://cdn.example.com/a?token=signed", false},
	} {
		req, err := e.newDownloadRequest(context.Background(), tc.url, "bytes=0-0")
		if err != nil {
			t.Fatal(err)
		}
		got := req.Header.Get("Cookie") != ""
		if got != tc.cookie {
			t.Errorf("%s should cookie=%v got=%v", tc.url, tc.cookie, got)
		}
		if req.Header.Get("Referer") != "https://pan.quark.cn/" || req.Header.Get("Range") != "bytes=0-0" {
			t.Fatalf("missing headers on %s", tc.url)
		}
	}
}
func TestRangeHTTP403DiagnosticsDoNotLeakSignedURL(t *testing.T) {
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Referer") != "https://pan.quark.cn/" {
			t.Errorf("missing Referer")
		}
		if r.Header.Get("Range") != "bytes=0-0" {
			t.Errorf("missing Range")
		}
		w.WriteHeader(403)
	}))
	defer s.Close()
	e := &Engine{client: s.Client()}
	_, err := e.rangeRequest(context.Background(), s.URL+"/a?access_token=DO_NOT_LOG", 0, 0, 1)
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("expected clear 403, got %v", err)
	}
	if strings.Contains(err.Error(), "DO_NOT_LOG") {
		t.Fatal("download credential exposed")
	}
}
func TestDownloadNetworkErrorsDoNotExposeSignedURL(t *testing.T) {
	err := safeDownloadNetworkError(&fakeSignedURLError{})
	if err == nil || strings.Contains(err.Error(), "TOP_SECRET") {
		t.Fatal("signed URL leaked")
	}
}

type fakeSignedURLError struct{}

func (e *fakeSignedURLError) Error() string {
	return "Get https://cdn.example.org/file?token=TOP_SECRET: refused"
}
