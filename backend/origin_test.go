package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAllowWriteOriginBehindGateway(t *testing.T) {
	tests := []struct {
		name, host, origin, site string
		dev, allowed             bool
	}{
		{"normal same origin", "nas.example:5666", "https://nas.example:5666", "same-origin", false, true},
		{"fnOS rewritten host", "unix-gateway", "https://nas.example:5666", "same-origin", false, true},
		{"fnOS proxy via domain", "127.0.0.1", "https://my-nas.example", "same-origin", false, true},
		{"cross site block", "unix-gateway", "https://evil.example", "cross-site", false, false},
		{"same site but cross origin block", "unix-gateway", "https://other.example", "same-site", false, false},
		{"unknown metadata conservative", "unix-gateway", "https://nas.example:5666", "", false, false},
		{"local dev same host", "localhost:18088", "http://localhost:18088", "same-origin", true, true},
		{"local dev mismatch blocked", "localhost:18088", "https://evil.example", "same-origin", true, false},
		{"malformed origin", "unix-gateway", "http://user@nas.example/path", "same-origin", false, false},
		{"no origin but cross site", "unix-gateway", "", "cross-site", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "http://"+tt.host+"/app/quarkdrive/api/v1/settings", strings.NewReader("{}"))
			if tt.origin != "" {
				r.Header.Set("Origin", tt.origin)
			}
			if tt.site != "" {
				r.Header.Set("Sec-Fetch-Site", tt.site)
			}
			if got := allowWriteOrigin(r, tt.dev); got != tt.allowed {
				t.Fatalf("allowed=%v want=%v", got, tt.allowed)
			}
		})
	}
}

func TestServeOriginAndAdminEnforcement(t *testing.T) {
	s := &Server{root: "/app/quarkdrive", dev: false}
	mk := func(origin, site, marker, admin string) int {
		r := httptest.NewRequest(http.MethodPut, "http://internal-gateway/app/quarkdrive/api/v1/settings", strings.NewReader("{}"))
		r.Header.Set("Origin", origin)
		r.Header.Set("Sec-Fetch-Site", site)
		r.Header.Set("X-QuarkDrive-Request", marker)
		r.Header.Set("X-Trim-Isadmin", admin)
		r.Header.Set("X-Trim-Userid", "1000")
		w := httptest.NewRecorder()
		s.serve(w, r)
		return w.Code
	}
	// Missing/bad JSON is rejected only after passing the CSRF gate.
	if got := mk("https://nas.example", "same-origin", "1", "true"); got != http.StatusBadRequest {
		t.Fatalf("legitimate proxy request returned %d, want 400 (settings validation)", got)
	}
	if got := mk("https://evil.example", "cross-site", "1", "true"); got != http.StatusForbidden {
		t.Fatalf("cross site %d", got)
	}
	if got := mk("https://nas.example", "same-origin", "", "true"); got != http.StatusForbidden {
		t.Fatalf("missing marker %d", got)
	}
	if got := mk("https://nas.example", "same-origin", "1", "false"); got != http.StatusForbidden {
		t.Fatalf("non-admin %d", got)
	}
}
