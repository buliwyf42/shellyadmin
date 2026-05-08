package firmware

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"shellyadmin/internal/core/shellyclient"
)

// rpcCall captures one POST /rpc invocation from the device under test
// (which in the test world is one of these helpers driving a httptest server).
type rpcCall struct {
	method string
	params map[string]any
	body   map[string]any
}

// fakeShelly is a single fixture covering the surfaces firmware/* exercises.
// Tests register handlers per-method; an unregistered method returns the
// non-standard Shelly 404 RPC error so IsMethodNotFound paths can be tested
// realistically. Calls are recorded in order so a test can assert "the right
// RPCs were issued, in the right shape".
type fakeShelly struct {
	t        *testing.T
	srv      *httptest.Server
	handlers map[string]func(params map[string]any) (any, *fakeRPCError)
	calls    []rpcCall
}

type fakeRPCError struct {
	Code    int
	Message string
}

// newFakeShelly returns a httptest server and host:port string suitable for
// passing as `ip` into the firmware OnClient seams. The caller registers
// per-method handlers via setMethod before issuing the call.
func newFakeShelly(t *testing.T) *fakeShelly {
	t.Helper()
	f := &fakeShelly{t: t, handlers: map[string]func(map[string]any) (any, *fakeRPCError){}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeShelly) host() string {
	return strings.TrimPrefix(f.srv.URL, "http://")
}

func (f *fakeShelly) setMethod(method string, handler func(params map[string]any) (any, *fakeRPCError)) {
	f.handlers[method] = handler
}

func (f *fakeShelly) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/rpc" {
		http.Error(w, "test fake only handles POST /rpc", http.StatusBadRequest)
		return
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		f.t.Fatalf("read body: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		f.t.Fatalf("unmarshal body: %v (raw=%s)", err, string(raw))
	}
	method, _ := body["method"].(string)
	params, _ := body["params"].(map[string]any)
	f.calls = append(f.calls, rpcCall{method: method, params: params, body: body})

	handler, ok := f.handlers[method]
	if !ok {
		// Shelly's non-standard 404 — IsMethodNotFound() handles both
		// 404 and -32601, so this is the more realistic default.
		writeRPCEnvelope(w, body["id"], nil, &fakeRPCError{Code: 404, Message: "Not Found"})
		return
	}
	result, rpcErr := handler(params)
	writeRPCEnvelope(w, body["id"], result, rpcErr)
}

func writeRPCEnvelope(w http.ResponseWriter, id any, result any, rpcErr *fakeRPCError) {
	envelope := map[string]any{"id": id, "src": "test"}
	if rpcErr != nil {
		envelope["error"] = map[string]any{"code": rpcErr.Code, "message": rpcErr.Message}
	} else {
		envelope["result"] = result
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(envelope)
}

// newClient builds a shellyclient.Client suitable for talking to the fake
// httptest server (5s timeout — generous for CI; tests complete in ms).
func newClient() *shellyclient.Client {
	return shellyclient.New(shellyclient.Options{Timeout: 5 * time.Second})
}
