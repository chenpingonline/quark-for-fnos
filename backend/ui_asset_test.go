package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Regression: v0.1.2 included theme.js in the UI but forgot to serve it.
func TestThemeScriptIsServed(t *testing.T) {
	s := &Server{web: filepath.Join("..", "frontend"), root: "/app/quarkdrive"}
	req := httptest.NewRequest(http.MethodGet, "http://nas/app/quarkdrive/assets/theme.js", nil)
	rec := httptest.NewRecorder()
	s.serve(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "QuarkDriveTheme") {
		t.Fatalf("theme bridge unavailable: HTTP %d", rec.Code)
	}
}

// Every local script and stylesheet referenced by the shipped page must be
// reachable through both the standalone server and the fnOS gateway prefix.
func TestReferencedUIAssetsAreServed(t *testing.T) {
	web := filepath.Join("..", "frontend")
	html, err := os.ReadFile(filepath.Join(web, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	refs := regexp.MustCompile(`(?:src|href)="(\./assets/[^" ]+)"`).FindAllSubmatch(html, -1)
	if len(refs) == 0 {
		t.Fatal("no frontend assets found")
	}
	// Module dependencies are fetched separately by browsers and need routes too.
	bridge, err := os.ReadFile(filepath.Join(web, "assets", "fnos-theme.js"))
	if err != nil {
		t.Fatal(err)
	}
	imports := regexp.MustCompile(`from ['"](\.\./vendor/[^'"]+)['"]`).FindAllSubmatch(bridge, -1)
	if len(imports) == 0 {
		t.Fatal("official SDK module dependency missing")
	}
	for _, imported := range imports {
		relative := filepath.ToSlash(filepath.Clean(filepath.Join("assets", string(imported[1]))))
		refs = append(refs, [][]byte{nil, []byte("./" + relative)})
	}
	for _, badge := range []string{"vip", "svip", "zvip"} {
		refs = append(refs, [][]byte{nil, []byte("./assets/membership/" + badge + ".png")})
	}
	for _, root := range []string{"", "/app/quarkdrive"} {
		for _, ref := range refs {
			asset := strings.TrimPrefix(string(ref[1]), ".")
			t.Run(root+asset, func(t *testing.T) {
				s := &Server{web: web, root: root}
				rec := httptest.NewRecorder()
				s.serve(rec, httptest.NewRequest(http.MethodGet, "http://nas"+root+asset, nil))
				if rec.Header().Get("Cache-Control") != "no-cache" {
					t.Fatal("frontend assets must revalidate after application upgrades")
				}
				expected, err := os.ReadFile(filepath.Join(web, strings.TrimPrefix(asset, "/")))
				if err != nil {
					t.Fatal(err)
				}
				if rec.Code != http.StatusOK || rec.Body.String() != string(expected) {
					t.Fatalf("frontend resource unavailable: HTTP %d", rec.Code)
				}
				contentType := rec.Header().Get("Content-Type")
				if strings.HasSuffix(asset, ".js") && !strings.Contains(contentType, "javascript") {
					t.Fatalf("script has invalid content type %q", contentType)
				}
			})
		}
	}
}

func TestQuarkModifiedTimestamp(t *testing.T) {
	for _, tc := range []struct {
		source map[string]any
		want   int64
	}{
		{map[string]any{"updated_at": float64(1791961800000)}, 1791961800000},
		{map[string]any{"created_at": float64(1791961800)}, 1791961800000},
		{map[string]any{}, 0},
	} {
		if got := quarkTimeMs(tc.source); got != tc.want {
			t.Errorf("got %d, want %d", got, tc.want)
		}
	}
}
