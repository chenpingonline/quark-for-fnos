package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

func TestQuarkHTTPErrorDiagnostics(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"numeric", `{"code":32001,"status":400,"message":"private response","token":"TOP_SECRET"}`, "code=32001，status=400"},
		{"string code", `{"code":"32001"}`, "code=32001"},
		{"html", `<html>TOP_SECRET</html>`, "未返回可识别的错误码"},
		{"untrusted code", `{"code":"TOP_SECRET","message":"https://example.com/?token=TOP_SECRET"}`, "未返回可识别的错误码"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := quarkHTTPError(400, strings.NewReader(tc.body))
			if !strings.Contains(err.Error(), "HTTP 400") || !strings.Contains(err.Error(), tc.want) {
				t.Fatal(err)
			}
			if strings.Contains(err.Error(), "TOP_SECRET") || strings.Contains(err.Error(), "private response") || strings.Contains(err.Error(), "https://") {
				t.Fatal("upstream private data leaked")
			}
		})
	}
}

type quarkErrorTransport func(*http.Request) (*http.Response, error)

func (f quarkErrorTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDownloadURLReportsAPIStage(t *testing.T) {
	store, err := newStore(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	if err = store.setCredential("test-fixture"); err != nil {
		t.Fatal(err)
	}
	q := newQuark(store)
	q.client.Transport = quarkErrorTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" || r.URL.Path != "/1/clouddrive/file/download" {
			t.Fatalf("unexpected download request: %s %s", r.Method, r.URL.Path)
		}
		return &http.Response{StatusCode: 400, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"code":32001,"status":400}`))}, nil
	})
	_, err = q.downloadURL(context.Background(), "fixture-file")
	if err == nil || !strings.Contains(err.Error(), "获取文件下载地址失败") || !strings.Contains(err.Error(), "code=32001") {
		t.Fatalf("missing download failure context: %v", err)
	}
}

func TestDesktopDownloadProfileIsConsistent(t *testing.T) {
	store, err := newStore(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	if err = store.setCredential("test-fixture"); err != nil {
		t.Fatal(err)
	}
	q := newQuark(store)
	const signedURL = "https://dl-pc-zb.drive.quark.cn/file?token=fixture"
	var issuedUA string
	q.client.Transport = quarkErrorTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "drive.quark.cn" || r.Method != "POST" || r.URL.Path != "/1/clouddrive/file/download" {
			t.Fatalf("wrong download endpoint: %s %s", r.Method, r.URL.Host+r.URL.Path)
		}
		query := r.URL.Query()
		if len(query) != 2 || query.Get("pr") != "ucpro" || query.Get("fr") != "pc" {
			t.Fatalf("unexpected desktop parameters: %v", query)
		}
		issuedUA = r.Header.Get("User-Agent")
		if !strings.Contains(issuedUA, "quark-cloud-drive/") {
			t.Fatal("download issuance lacks desktop identity")
		}
		if r.Header.Get("Cookie") != "test-fixture" || r.Header.Get("Referer") != "https://pan.quark.cn/" {
			t.Fatal("missing authenticated download headers")
		}
		var body struct {
			FIDs []string `json:"fids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.FIDs) != 1 || body.FIDs[0] != "fixture-video" {
			t.Fatalf("incorrect file download body: %v", body)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"status":200,"code":0,"data":[{"download_url":"` + signedURL + `"}]}`))}, nil
	})
	link, err := q.downloadURL(context.Background(), "fixture-video")
	if err != nil || link != signedURL {
		t.Fatalf("download link unavailable: %v", err)
	}
	engine := newEngine(store, q)
	for _, byteRange := range []string{"bytes=0-0", "bytes=0-8388607", ""} {
		request, err := engine.newDownloadRequest(context.Background(), link, byteRange)
		if err != nil {
			t.Fatal(err)
		}
		if request.Header.Get("User-Agent") != issuedUA || request.Header.Get("Cookie") != "test-fixture" || request.Header.Get("Range") != byteRange {
			t.Fatal("CDN request differs from link issuance identity or requested range")
		}
	}
}

func TestOrdinaryAPIKeepsBrowserProfile(t *testing.T) {
	q := newQuark(nil)
	q.client.Transport = quarkErrorTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "drive-pc.quark.cn" || r.Header.Get("User-Agent") != webUA || !r.URL.Query().Has("__t") {
			t.Fatal("ordinary API profile was changed")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"status":200,"code":0,"data":{}}`))}, nil
	})
	if _, err := q.request(context.Background(), "GET", driveBase+"/file/sort", nil, nil, false); err != nil {
		t.Fatal(err)
	}
}
