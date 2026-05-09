// Package mcp implements a read-only Model Context Protocol server that
// exposes ShellyAdmin's services.AppService surface to LLM-driven agents.
// See docs/adr/0011-mcp-read-only-server.md for the design rationale.
package mcp

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// auth wraps next so only authenticated requests reach it. Two equivalent
// auth methods are accepted; pick whichever is more convenient for the
// client:
//
//   - Authorization: Bearer <token> header (spec-conformant; default)
//   - URL whose first path segment IS the token (e.g.
//     http://host:8101/<token>/...). The matched prefix is stripped
//     before reaching next, so the wrapped MCP handler sees the request
//     at the same path the header form would. Convenient for MCP
//     clients that don't make custom headers easy to configure.
//
// Both comparisons are constant-time to avoid timing leaks. An empty
// token is rejected at construction in [Build]; callers should never
// reach this middleware unless the operator opted in.
func auth(token string, next http.Handler) http.Handler {
	expectedHeader := []byte("Bearer " + token)
	expectedToken := []byte(token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Header form.
		if got := []byte(r.Header.Get("Authorization")); subtle.ConstantTimeCompare(got, expectedHeader) == 1 {
			next.ServeHTTP(w, r)
			return
		}
		// URL-path form: /<token>[/...].
		if first, rest, ok := splitPathToken(r.URL.Path); ok {
			if subtle.ConstantTimeCompare([]byte(first), expectedToken) == 1 {
				r2 := r.Clone(r.Context())
				if rest == "" {
					rest = "/"
				}
				r2.URL.Path = rest
				r2.URL.RawPath = ""
				next.ServeHTTP(w, r2)
				return
			}
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}

// splitPathToken extracts the first path segment from p. Returns the
// segment, the remainder (with leading slash, or "" if the segment
// was the whole path), and true when there was a non-empty segment.
//
//	"/"             → "", "", false
//	"/abc"          → "abc", "", true
//	"/abc/"         → "abc", "/", true
//	"/abc/d/e"      → "abc", "/d/e", true
//	""              → "", "", false
func splitPathToken(p string) (first, rest string, ok bool) {
	if len(p) < 2 || p[0] != '/' {
		return "", "", false
	}
	body := p[1:]
	if i := strings.IndexByte(body, '/'); i >= 0 {
		return body[:i], body[i:], true
	}
	return body, "", true
}
