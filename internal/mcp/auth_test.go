package mcp

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuth(t *testing.T) {
	var (
		called   bool
		seenPath string
	)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		seenPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})
	handler := auth("expected-token", next)

	cases := []struct {
		name       string
		path       string
		header     string
		wantStatus int
		wantNext   bool
		wantPath   string // path next handler should observe; empty when wantNext == false
	}{
		// Header form
		{"header valid", "/", "Bearer expected-token", http.StatusOK, true, "/"},
		{"header missing", "/", "", http.StatusUnauthorized, false, ""},
		{"header wrong token", "/", "Bearer wrong-token", http.StatusUnauthorized, false, ""},
		{"header missing prefix", "/", "expected-token", http.StatusUnauthorized, false, ""},
		{"header basic auth", "/", "Basic dXNlcjpwYXNz", http.StatusUnauthorized, false, ""},

		// URL-path form
		{"path token alone", "/expected-token", "", http.StatusOK, true, "/"},
		{"path token trailing slash", "/expected-token/", "", http.StatusOK, true, "/"},
		{"path token with suffix", "/expected-token/sub/path", "", http.StatusOK, true, "/sub/path"},
		{"path wrong token", "/wrong-token", "", http.StatusUnauthorized, false, ""},
		{"path empty root", "/", "", http.StatusUnauthorized, false, ""},
		{"path partial prefix match", "/expected-tokenx", "", http.StatusUnauthorized, false, ""},

		// Header takes precedence and wins even when path is wrong.
		{"valid header beats wrong path", "/wrong-token", "Bearer expected-token", http.StatusOK, true, "/wrong-token"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called = false
			seenPath = ""
			req := httptest.NewRequest(http.MethodPost, tc.path, nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if called != tc.wantNext {
				t.Errorf("next called = %v, want %v", called, tc.wantNext)
			}
			if tc.wantNext && seenPath != tc.wantPath {
				t.Errorf("next saw path %q, want %q", seenPath, tc.wantPath)
			}
		})
	}
}

func TestSplitPathToken(t *testing.T) {
	cases := []struct {
		in        string
		wantFirst string
		wantRest  string
		wantOK    bool
	}{
		{"", "", "", false},
		{"/", "", "", false},
		{"abc", "", "", false}, // no leading slash
		{"/abc", "abc", "", true},
		{"/abc/", "abc", "/", true},
		{"/abc/def", "abc", "/def", true},
		{"/abc/def/ghi", "abc", "/def/ghi", true},
	}
	for _, tc := range cases {
		first, rest, ok := splitPathToken(tc.in)
		if first != tc.wantFirst || rest != tc.wantRest || ok != tc.wantOK {
			t.Errorf("splitPathToken(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tc.in, first, rest, ok, tc.wantFirst, tc.wantRest, tc.wantOK)
		}
	}
}

func TestSanitizeRequestID(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"abc-123_def", "abc-123_def"},
		{"   trim-me  ", "trim-me"},
		{"has space", ""},
		{"has/slash", ""},
		{"has;semi", ""},
	}
	for _, tc := range cases {
		if got := sanitizeRequestID(tc.in); got != tc.want {
			t.Errorf("sanitizeRequestID(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}

	long := make([]byte, 80)
	for i := range long {
		long[i] = 'a'
	}
	got := sanitizeRequestID(string(long))
	if len(got) != 64 {
		t.Errorf("sanitizeRequestID truncation = %d chars, want 64", len(got))
	}
}
