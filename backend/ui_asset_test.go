package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
