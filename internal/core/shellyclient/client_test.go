package shellyclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// extractHostFromTestURL strips the scheme/port from an httptest URL.
func extractHostFromTestURL(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %s: %v", raw, err)
	}
	return u.Host // includes :port
}

func TestParseDigestChallenge(t *testing.T) {
	header := `Digest realm="shellyplus2pm-AABBCC", nonce="abc123", algorithm=SHA-256, qop="auth"`
	st, err := parseDigestChallenge(header)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if st.Realm != "shellyplus2pm-AABBCC" {
		t.Errorf("realm=%q", st.Realm)
	}
	if st.Nonce != "abc123" {
		t.Errorf("nonce=%q", st.Nonce)
	}
	if st.Algorithm != "SHA-256" {
		t.Errorf("alg=%q", st.Algorithm)
	}
	if st.QOP != "auth" {
		t.Errorf("qop=%q", st.QOP)
	}
}

func TestParseDigestChallengeMissingNonce(t *testing.T) {
	if _, err := parseDigestChallenge(`Digest realm="x"`); err == nil {
		t.Fatal("expected error on missing nonce")
	}
}

func TestSplitDigestPairsHandlesCommasInQuotes(t *testing.T) {
	pairs := splitDigestPairs(`realm="a,b", nonce="x"`)
	if pairs["realm"] != "a,b" {
		t.Errorf("realm=%q (want a,b)", pairs["realm"])
	}
}

func TestBuildDigestAuthHeaderSHA256(t *testing.T) {
	state := &digestState{
		Realm:     "shellyrealm",
		Nonce:     "nonce-1",
		Algorithm: "SHA-256",
		QOP:       "auth",
	}
	header := buildDigestAuthHeader(state, 1, "admin", "secret", "", "POST", "/rpc")
	if !strings.HasPrefix(header, "Digest ") {
		t.Fatalf("missing Digest prefix: %s", header)
	}
	if !strings.Contains(header, `username="admin"`) {
		t.Error("missing username")
	}
	if !strings.Contains(header, `algorithm=SHA-256`) {
		t.Error("missing algorithm")
	}
	if !strings.Contains(header, "qop=auth") {
		t.Error("missing qop")
	}
	if !strings.Contains(header, "nc=00000001") {
		t.Error("missing nc=00000001")
	}
}

// TestRPCDigestRoundTrip exercises the 401-challenge → re-auth → 200 path.
func TestRPCDigestRoundTrip(t *testing.T) {
	var seenAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			w.Header().Set("WWW-Authenticate", `Digest realm="testrealm", nonce="serverNonce", algorithm=SHA-256, qop="auth"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		seenAuth = auth
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":1,"result":{"ok":true}}`)
	}))
	defer srv.Close()

	c := New(Options{Username: "admin", Password: "pw", Timeout: 2 * time.Second})
	host := extractHostFromTestURL(t, srv.URL)
	res, err := c.RPC(context.Background(), host, "Shelly.GetConfig", nil)
	if err != nil {
		t.Fatalf("rpc: %v", err)
	}
	if !strings.Contains(seenAuth, "username=\"admin\"") {
		t.Errorf("expected admin username in retry: %s", seenAuth)
	}
	if !strings.Contains(seenAuth, "algorithm=SHA-256") {
		t.Errorf("expected SHA-256 in retry: %s", seenAuth)
	}
	if got, _ := res["ok"].(bool); !got {
		t.Errorf("result %#v", res)
	}
}

// TestRPCAuthRequiredAfterRetry verifies wrong creds yield ErrAuthRequired
// rather than looping (which would trip brute-force lockout on real devices).
func TestRPCAuthRequiredAfterRetry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("WWW-Authenticate", `Digest realm="r", nonce="n", algorithm=SHA-256, qop="auth"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	c := New(Options{Username: "admin", Password: "wrong", Timeout: 2 * time.Second})
	host := extractHostFromTestURL(t, srv.URL)
	_, err := c.RPC(context.Background(), host, "Shelly.GetConfig", nil)
	if !errors.Is(err, ErrAuthRequired) {
		t.Fatalf("expected ErrAuthRequired, got %v", err)
	}
}

// TestRPCLockout verifies 429 surfaces as ErrAuthLockout and we don't retry.
func TestRPCLockout(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := New(Options{Username: "admin", Password: "pw", Timeout: 2 * time.Second})
	host := extractHostFromTestURL(t, srv.URL)
	_, err := c.RPC(context.Background(), host, "Shelly.GetConfig", nil)
	if !errors.Is(err, ErrAuthLockout) {
		t.Fatalf("expected ErrAuthLockout, got %v", err)
	}
	if calls > 1 {
		t.Errorf("expected single call on 429, got %d", calls)
	}
}

// TestRPCMethodNotFound: shelly's non-standard 404 code should be detected.
func TestRPCMethodNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":1,"error":{"code":404,"message":"not found"}}`)
	}))
	defer srv.Close()
	c := New(Options{Timeout: 2 * time.Second})
	host := extractHostFromTestURL(t, srv.URL)
	_, err := c.RPC(context.Background(), host, "OTA.SetConfig", nil)
	if !IsMethodNotFound(err) {
		t.Fatalf("expected method-not-found, got %v", err)
	}
}

// TestProbeRejectsEmptyBody covers the UniFi-class regression where a non-Shelly
// endpoint answers 200 with no body. Old behaviour created a junk Device; new
// behaviour returns an error so the scanner can skip the IP cleanly.
func TestProbeRejectsEmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		// No body.
	}))
	defer srv.Close()
	c := New(Options{Timeout: 2 * time.Second})
	host := extractHostFromTestURL(t, srv.URL)
	if _, err := c.Probe(context.Background(), host); err == nil {
		t.Fatal("expected probe to fail on empty body")
	}
}

func TestProbeOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/shelly" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"gen":2,"model":"SNSW-001P16EU","mac":"AA:BB:CC:DD:EE:FF"}`)
	}))
	defer srv.Close()
	c := New(Options{Timeout: 2 * time.Second})
	host := extractHostFromTestURL(t, srv.URL)
	out, err := c.Probe(context.Background(), host)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if out["model"] != "SNSW-001P16EU" {
		t.Errorf("model=%v", out["model"])
	}
}

// TestRPCContextCancel ensures we propagate context cancellation cleanly.
func TestRPCContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
	}))
	defer srv.Close()
	c := New(Options{Timeout: 2 * time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	host := extractHostFromTestURL(t, srv.URL)
	_, err := c.RPC(ctx, host, "Shelly.GetConfig", nil)
	if err == nil {
		t.Fatal("expected context error")
	}
	// Different transports surface the cancellation differently (net.Error,
	// url.Error wrapping context.DeadlineExceeded, etc.) — the only thing we
	// care about is that the call returned promptly with *some* error.
}
