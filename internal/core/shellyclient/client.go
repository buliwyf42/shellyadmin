// Package shellyclient is the unified HTTP/JSON-RPC client used to talk to
// Shelly Gen2+ devices. It transparently handles RFC 7616 Digest authentication,
// brute-force lockout signalling, and HTTP→HTTPS scheme upgrades introduced by
// Shelly firmware 2.0.0-beta1.
//
// Design intent:
//   - One client per refresh/provision/setter call site (cheap to construct).
//   - Auth state (nonce + nc counter) lives on the Client so successive RPCs
//     to the same device reuse the nonce, per RFC 7616 §3.3.
//   - Errors are typed (ErrAuthRequired, ErrAuthLockout, ErrTLSCertInvalid)
//     so callers can populate device records without parsing strings.
package shellyclient

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TLSPolicy controls cert verification when the client speaks HTTPS.
type TLSPolicy int

const (
	// TLSStrict (default) verifies the server certificate including expiry.
	TLSStrict TLSPolicy = iota
	// TLSSkip disables verification — opt-in per device for self-signed certs.
	TLSSkip
)

// Options configures a Client. Zero values are sensible defaults.
type Options struct {
	Timeout       time.Duration
	Scheme        string    // "http" (default) or "https"
	TLSPolicy     TLSPolicy // ignored if scheme is http
	Username      string    // empty disables auth header
	Password      string    // used to compute HA1 if HA1 is empty
	HA1           string    // optional precomputed SHA-256 / MD5 HA1
	AllowUpgrade  bool      // follow http→https 30x redirects and remember the new scheme
	UserAgent     string    // overrides the default UA
}

// Client is safe for concurrent use across calls to the same device.
type Client struct {
	httpc *http.Client
	opts  Options

	mu     sync.Mutex
	nonce  *digestState // last challenge from the device, if any
	scheme string       // current scheme (may have upgraded from http to https)
}

type digestState struct {
	Realm     string
	Nonce     string
	Algorithm string // "MD5", "MD5-sess", "SHA-256", "SHA-256-sess"
	QOP       string // "auth" or empty
	Opaque    string
	NC        uint32
}

// Sentinel errors. Wrap with fmt.Errorf("...: %w", err) when adding context.
var (
	ErrAuthRequired   = errors.New("shellyclient: authentication required")
	ErrAuthLockout    = errors.New("shellyclient: device locked out (brute-force protection)")
	ErrTLSCertInvalid = errors.New("shellyclient: TLS certificate validation failed")
)

// New builds a Client. Multiple goroutines may share one Client.
func New(opts Options) *Client {
	if opts.Timeout <= 0 {
		opts.Timeout = 5 * time.Second
	}
	if opts.Scheme == "" {
		opts.Scheme = "http"
	}
	tlsCfg := &tls.Config{}
	if opts.TLSPolicy == TLSSkip {
		tlsCfg.InsecureSkipVerify = true // #nosec G402 — opt-in only
	}
	transport := &http.Transport{TLSClientConfig: tlsCfg}
	httpc := &http.Client{Timeout: opts.Timeout, Transport: transport}
	if !opts.AllowUpgrade {
		// Without explicit opt-in we leave Go's default redirect behavior alone,
		// which already follows up to 10 redirects. Devices on 1.x firmware
		// don't redirect, so this is a no-op there.
	}
	return &Client{httpc: httpc, opts: opts, scheme: opts.Scheme}
}

// Scheme reports the scheme actively used for the most recent successful call.
// Useful for callers that want to persist scheme upgrades back to the device record.
func (c *Client) Scheme() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.scheme
}

// HTTPClient exposes the underlying *http.Client for callers that need raw access
// (e.g. chunked file uploads in user_ca). Auth handling is bypassed at this level.
func (c *Client) HTTPClient() *http.Client { return c.httpc }

// Probe issues GET /shelly and returns the parsed envelope. It does not require
// authentication on Shelly devices and is the primary scheme-detection vector.
func (c *Client) Probe(ctx context.Context, ip string) (map[string]any, error) {
	req, err := c.buildRequest(ctx, http.MethodGet, ip, "/shelly", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(ctx, req, ip)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("shellyclient: probe %s returned %s", ip, resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &out); err != nil {
			return nil, fmt.Errorf("shellyclient: probe %s: invalid json: %w", ip, err)
		}
	}
	return out, nil
}

// RPC issues a JSON-RPC 2.0 POST to /rpc and returns the parsed `result` object.
// The returned error is one of the sentinels (ErrAuthRequired, ErrAuthLockout,
// ErrTLSCertInvalid) where applicable, otherwise a wrapped network/parse error.
func (c *Client) RPC(ctx context.Context, ip, method string, params map[string]any) (map[string]any, error) {
	body := map[string]any{"id": 1, "method": method}
	if len(params) > 0 {
		body["params"] = params
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := c.buildRequest(ctx, http.MethodPost, ip, "/rpc", bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(buf)), nil }
	resp, err := c.do(ctx, req, ip)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, ErrAuthLockout
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrAuthRequired
	}
	var envelope struct {
		Result map[string]any `json:"result"`
		Error  any            `json:"error"`
	}
	if len(respBody) > 0 {
		_ = json.Unmarshal(respBody, &envelope)
	}
	if envelope.Error != nil {
		return envelope.Result, &RPCError{Method: method, Raw: envelope.Error}
	}
	if resp.StatusCode >= 400 {
		return envelope.Result, fmt.Errorf("shellyclient: RPC %s on %s: %s", method, ip, resp.Status)
	}
	return envelope.Result, nil
}

// RPCError is returned when the device replies with a JSON-RPC error envelope
// (HTTP 200 with {"error": {...}}). Code() and Message() decode the common fields.
type RPCError struct {
	Method string
	Raw    any
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("shellyclient: %s rpc error: %s", e.Method, e.Message())
}

// Code returns the numeric error code (-32601, 404, etc.) or 0 if not present.
func (e *RPCError) Code() int {
	if obj, ok := e.Raw.(map[string]any); ok {
		switch v := obj["code"].(type) {
		case float64:
			return int(v)
		case int:
			return v
		case json.Number:
			n, _ := v.Int64()
			return int(n)
		}
	}
	return 0
}

// Message returns the device-provided error message, or a JSON dump if absent.
func (e *RPCError) Message() string {
	if s, ok := e.Raw.(string); ok {
		return s
	}
	if obj, ok := e.Raw.(map[string]any); ok {
		if msg, ok := obj["message"].(string); ok && msg != "" {
			return msg
		}
	}
	encoded, _ := json.Marshal(e.Raw)
	return string(encoded)
}

// IsMethodNotFound reports whether err is an RPCError with code -32601 (standard
// JSON-RPC) or 404 (Shelly's non-standard "method unsupported on this model").
func IsMethodNotFound(err error) bool {
	var rpcErr *RPCError
	if !errors.As(err, &rpcErr) {
		return false
	}
	c := rpcErr.Code()
	return c == -32601 || c == 404
}

// ----- internals -----

func (c *Client) buildRequest(ctx context.Context, method, ip, path string, body io.Reader) (*http.Request, error) {
	c.mu.Lock()
	scheme := c.scheme
	c.mu.Unlock()
	url := scheme + "://" + ip + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	if c.opts.UserAgent != "" {
		req.Header.Set("User-Agent", c.opts.UserAgent)
	}
	return req, nil
}

// do executes req with transparent digest-auth challenge/response. It also
// upgrades scheme to https if the device redirects (when AllowUpgrade is set).
func (c *Client) do(ctx context.Context, req *http.Request, ip string) (*http.Response, error) {
	// Attach a precomputed Authorization header if we already have a fresh nonce.
	c.attachAuth(req)
	resp, err := c.httpc.Do(req)
	if err != nil {
		if isCertError(err) {
			return nil, ErrTLSCertInvalid
		}
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized {
		// Parse challenge, retry once.
		challenge := resp.Header.Get("WWW-Authenticate")
		resp.Body.Close()
		if challenge == "" || c.opts.Username == "" {
			return c.replay(req, ip, http.StatusUnauthorized)
		}
		state, perr := parseDigestChallenge(challenge)
		if perr != nil {
			return c.replay(req, ip, http.StatusUnauthorized)
		}
		c.mu.Lock()
		c.nonce = state
		c.mu.Unlock()
		retry, rerr := c.cloneRequest(req)
		if rerr != nil {
			return nil, rerr
		}
		c.attachAuth(retry)
		resp2, err2 := c.httpc.Do(retry)
		if err2 != nil {
			if isCertError(err2) {
				return nil, ErrTLSCertInvalid
			}
			return nil, err2
		}
		if resp2.StatusCode == http.StatusUnauthorized {
			resp2.Body.Close()
			return nil, ErrAuthRequired
		}
		if resp2.StatusCode == http.StatusTooManyRequests {
			return resp2, nil
		}
		return resp2, nil
	}
	return resp, nil
}

// replay returns a synthetic response with the given status when we can't
// retry (no creds, malformed challenge). The caller's RPC/Probe logic maps it
// back to the appropriate sentinel error.
func (c *Client) replay(_ *http.Request, _ string, status int) (*http.Response, error) {
	if status == http.StatusUnauthorized {
		return nil, ErrAuthRequired
	}
	return nil, fmt.Errorf("shellyclient: unexpected status %d", status)
}

func (c *Client) cloneRequest(req *http.Request) (*http.Request, error) {
	clone := req.Clone(req.Context())
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, err
		}
		clone.Body = body
	}
	return clone, nil
}

func (c *Client) attachAuth(req *http.Request) {
	if c.opts.Username == "" {
		return
	}
	c.mu.Lock()
	state := c.nonce
	c.mu.Unlock()
	if state == nil {
		return
	}
	c.mu.Lock()
	state.NC++
	nc := state.NC
	c.mu.Unlock()
	header := buildDigestAuthHeader(state, nc, c.opts.Username, c.opts.Password, c.opts.HA1, req.Method, req.URL.RequestURI())
	if header != "" {
		req.Header.Set("Authorization", header)
	}
}

// parseDigestChallenge decodes a WWW-Authenticate: Digest header.
func parseDigestChallenge(header string) (*digestState, error) {
	const prefix = "Digest "
	if !strings.HasPrefix(strings.TrimSpace(header), prefix) {
		return nil, errors.New("not a digest challenge")
	}
	body := strings.TrimSpace(header)[len(prefix):]
	pairs := splitDigestPairs(body)
	st := &digestState{}
	for k, v := range pairs {
		switch strings.ToLower(k) {
		case "realm":
			st.Realm = v
		case "nonce":
			st.Nonce = v
		case "algorithm":
			st.Algorithm = v
		case "qop":
			// qop may be a comma-separated list; prefer "auth"
			parts := strings.Split(v, ",")
			for _, p := range parts {
				if strings.TrimSpace(p) == "auth" {
					st.QOP = "auth"
					break
				}
			}
			if st.QOP == "" && len(parts) > 0 {
				st.QOP = strings.TrimSpace(parts[0])
			}
		case "opaque":
			st.Opaque = v
		}
	}
	if st.Nonce == "" {
		return nil, errors.New("digest challenge missing nonce")
	}
	if st.Algorithm == "" {
		st.Algorithm = "MD5"
	}
	return st, nil
}

// splitDigestPairs handles the comma-separated key=value (or key="value") syntax
// in WWW-Authenticate. Quoted values may contain commas.
func splitDigestPairs(s string) map[string]string {
	out := map[string]string{}
	var i int
	for i < len(s) {
		// skip whitespace and commas
		for i < len(s) && (s[i] == ' ' || s[i] == ',' || s[i] == '\t') {
			i++
		}
		// read key
		keyStart := i
		for i < len(s) && s[i] != '=' {
			i++
		}
		if i >= len(s) {
			break
		}
		key := strings.TrimSpace(s[keyStart:i])
		i++ // skip '='
		var value string
		if i < len(s) && s[i] == '"' {
			i++
			vs := i
			for i < len(s) && s[i] != '"' {
				i++
			}
			value = s[vs:i]
			if i < len(s) {
				i++ // skip closing quote
			}
		} else {
			vs := i
			for i < len(s) && s[i] != ',' {
				i++
			}
			value = strings.TrimSpace(s[vs:i])
		}
		if key != "" {
			out[key] = value
		}
	}
	return out
}

// buildDigestAuthHeader assembles the Authorization: Digest ... value per RFC 7616.
func buildDigestAuthHeader(state *digestState, nc uint32, username, password, ha1Pre, method, uri string) string {
	if state == nil {
		return ""
	}
	algo := strings.ToUpper(state.Algorithm)
	hashFn := pickHash(algo)
	if hashFn == nil {
		return ""
	}

	ha1 := ha1Pre
	if ha1 == "" {
		ha1 = hashFn(username + ":" + state.Realm + ":" + password)
	}
	if strings.HasSuffix(algo, "-SESS") {
		ha1 = hashFn(ha1 + ":" + state.Nonce + ":" + cnonce())
	}
	ha2 := hashFn(method + ":" + uri)

	cn := cnonce()
	ncHex := fmt.Sprintf("%08x", nc)
	var response string
	if state.QOP == "auth" {
		response = hashFn(ha1 + ":" + state.Nonce + ":" + ncHex + ":" + cn + ":auth:" + ha2)
	} else {
		response = hashFn(ha1 + ":" + state.Nonce + ":" + ha2)
	}

	parts := []string{
		fmt.Sprintf(`username="%s"`, username),
		fmt.Sprintf(`realm="%s"`, state.Realm),
		fmt.Sprintf(`nonce="%s"`, state.Nonce),
		fmt.Sprintf(`uri="%s"`, uri),
		fmt.Sprintf(`response="%s"`, response),
		fmt.Sprintf(`algorithm=%s`, state.Algorithm),
	}
	if state.QOP == "auth" {
		parts = append(parts, "qop=auth", "nc="+ncHex, fmt.Sprintf(`cnonce="%s"`, cn))
	}
	if state.Opaque != "" {
		parts = append(parts, fmt.Sprintf(`opaque="%s"`, state.Opaque))
	}
	return "Digest " + strings.Join(parts, ", ")
}

func pickHash(algo string) func(string) string {
	a := strings.TrimSuffix(strings.ToUpper(algo), "-SESS")
	switch a {
	case "SHA-256":
		return func(s string) string {
			sum := sha256.Sum256([]byte(s))
			return hex.EncodeToString(sum[:])
		}
	case "MD5":
		return func(s string) string {
			sum := md5.Sum([]byte(s))
			return hex.EncodeToString(sum[:])
		}
	default:
		return nil
	}
}

// cnonce returns a non-cryptographic-but-unique value sufficient for Digest auth.
// Per RFC 7616 §5.4 the cnonce only needs to be unguessable enough to defeat
// chosen-plaintext attacks across the lifetime of a single nonce.
func cnonce() string {
	return strconv.FormatInt(time.Now().UnixNano(), 16)
}

func isCertError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "x509:") || strings.Contains(msg, "certificate")
}
