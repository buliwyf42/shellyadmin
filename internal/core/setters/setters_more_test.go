package setters

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"shellyadmin/internal/core/shellyclient"
)

// recordingServer is a richer test fake than newCaptureServer in
// setters_test.go: it records every request so multi-call tests can verify
// the full RPC sequence, and it lets each test register a per-method
// response (status code + JSON body, optionally an RPC-error envelope).
//
// Defaults: any unregistered method returns 200 with an empty result. The
// non-standard Shelly 404 (handled by IsMethodNotFound) is opt-in via
// setRPCError.
type recordingServer struct {
	t        *testing.T
	srv      *httptest.Server
	mu       sync.Mutex
	calls    []rpcCall
	handlers map[string]rpcHandler
}

type rpcCall struct {
	method string
	params map[string]any
}

type rpcHandler struct {
	httpStatus int
	result     any
	rpcCode    int
	rpcMessage string
}

func newRecordingServer(t *testing.T) *recordingServer {
	t.Helper()
	r := &recordingServer{t: t, handlers: map[string]rpcHandler{}}
	r.srv = httptest.NewServer(http.HandlerFunc(r.serve))
	t.Cleanup(r.srv.Close)
	return r
}

func (r *recordingServer) host() string { return strings.TrimPrefix(r.srv.URL, "http://") }

func (r *recordingServer) setMethod(method string, h rpcHandler) {
	if h.httpStatus == 0 {
		h.httpStatus = http.StatusOK
	}
	r.handlers[method] = h
}

func (r *recordingServer) setRPCError(method string, code int, message string) {
	r.setMethod(method, rpcHandler{rpcCode: code, rpcMessage: message})
}

func (r *recordingServer) serve(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost || req.URL.Path != "/rpc" {
		http.Error(w, "test fake only handles POST /rpc", http.StatusBadRequest)
		return
	}
	raw, err := io.ReadAll(req.Body)
	if err != nil {
		r.t.Fatalf("read body: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		r.t.Fatalf("unmarshal body: %v (raw=%s)", err, string(raw))
	}
	method, _ := body["method"].(string)
	params, _ := body["params"].(map[string]any)
	r.mu.Lock()
	r.calls = append(r.calls, rpcCall{method: method, params: params})
	r.mu.Unlock()

	h, ok := r.handlers[method]
	if !ok {
		// Default: 200 with empty result. Tests that need 404 / -32601 set
		// it explicitly via setRPCError.
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": body["id"], "result": map[string]any{}})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(h.httpStatus)
	if h.rpcMessage != "" {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":    body["id"],
			"error": map[string]any{"code": h.rpcCode, "message": h.rpcMessage},
		})
		return
	}
	result := h.result
	if result == nil {
		result = map[string]any{}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"id": body["id"], "result": result})
}

func newSetterFor(server *recordingServer) *Setter {
	return NewWithClient(shellyclient.New(shellyclient.Options{Timeout: 2 * time.Second}))
}

// SetLocation must wrap lat + lon under params.config.location. The double
// nesting (params.config.X) is the SetConfig convention; flubbing it would
// silently drop the operator's location into the wrong slot on the device.
func TestSetLocationSendsLatLonUnderConfigLocation(t *testing.T) {
	srv := newRecordingServer(t)
	ok := newSetterFor(srv).SetLocation(context.Background(), srv.host(), 52.5, 13.4)
	if !ok {
		t.Fatalf("SetLocation returned false")
	}
	if len(srv.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(srv.calls))
	}
	if srv.calls[0].method != "Sys.SetConfig" {
		t.Errorf("method = %q, want Sys.SetConfig", srv.calls[0].method)
	}
	config, _ := srv.calls[0].params["config"].(map[string]any)
	location, _ := config["location"].(map[string]any)
	if got := location["lat"]; got != 52.5 {
		t.Errorf("lat = %v, want 52.5", got)
	}
	if got := location["lon"]; got != 13.4 {
		t.Errorf("lon = %v, want 13.4", got)
	}
}

// SetSNTPServer nests the sntp.server field under Sys.SetConfig.config.sntp
// — different shape from SetLocation, regression-prone on a refactor.
func TestSetSNTPServerNestsUnderConfigSNTP(t *testing.T) {
	srv := newRecordingServer(t)
	ok := newSetterFor(srv).SetSNTPServer(context.Background(), srv.host(), "pool.ntp.org")
	if !ok {
		t.Fatalf("SetSNTPServer returned false")
	}
	if srv.calls[0].method != "Sys.SetConfig" {
		t.Errorf("method = %q, want Sys.SetConfig", srv.calls[0].method)
	}
	config, _ := srv.calls[0].params["config"].(map[string]any)
	sntp, _ := config["sntp"].(map[string]any)
	if got := sntp["server"]; got != "pool.ntp.org" {
		t.Errorf("sntp.server = %v, want pool.ntp.org", got)
	}
}

// SetCoverTilt clamps percent into [0, 100] before sending. Letting an
// out-of-range value through would either be rejected by the device or
// (worse on some firmware) wrap around — the operator-side clamp is the
// safer guarantee.
func TestSetCoverTiltClampsPercent(t *testing.T) {
	tests := []struct {
		name  string
		given int
		want  float64 // JSON unmarshals numbers to float64
	}{
		{"in-range stays", 42, 42},
		{"negative clamps to 0", -5, 0},
		{"over 100 clamps to 100", 250, 100},
		{"upper boundary inclusive", 100, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newRecordingServer(t)
			ok := newSetterFor(srv).SetCoverTilt(context.Background(), srv.host(), 0, tt.given)
			if !ok {
				t.Fatalf("SetCoverTilt returned false")
			}
			if srv.calls[0].method != "Cover.GoToTilt" {
				t.Errorf("method = %q, want Cover.GoToTilt", srv.calls[0].method)
			}
			if got := srv.calls[0].params["pos"]; got != tt.want {
				t.Errorf("pos = %v, want %v", got, tt.want)
			}
			if got := srv.calls[0].params["id"]; got != float64(0) {
				t.Errorf("id = %v, want 0", got)
			}
		})
	}
}

// The 404 method-not-found path is the contract the bulk-action UI
// depends on: setters must return false (rather than panic / surface a
// raw RPC error) when the device doesn't expose the method. Both the
// Shelly non-standard 404 and the standard JSON-RPC -32601 must
// short-circuit cleanly.
func TestSettersReturnFalseOnMethodNotFound(t *testing.T) {
	for _, code := range []int{404, -32601} {
		t.Run("code_"+itoa(code), func(t *testing.T) {
			srv := newRecordingServer(t)
			srv.setRPCError("BLE.SetConfig", code, "method not found")
			ok := newSetterFor(srv).SetBLEEnabled(context.Background(), srv.host(), true)
			if ok {
				t.Errorf("SetBLEEnabled returned true on code=%d, want false", code)
			}
		})
	}
}

// CoverOpen is representative of the (bool, string) returner family. The
// detail string ("cover N opening") is what the bulk-action UI surfaces
// in the row's Detail column — flubbing it produces a worse operator
// experience but no functional break.
func TestCoverOpenSuccessReportsInstanceID(t *testing.T) {
	srv := newRecordingServer(t)
	ok, detail := newSetterFor(srv).CoverOpen(context.Background(), srv.host(), 1)
	if !ok {
		t.Fatalf("CoverOpen returned ok=false (detail=%q)", detail)
	}
	if detail != "cover 1 opening" {
		t.Errorf("detail = %q, want cover 1 opening", detail)
	}
	if srv.calls[0].method != "Cover.Open" {
		t.Errorf("method = %q, want Cover.Open", srv.calls[0].method)
	}
	if got := srv.calls[0].params["id"]; got != float64(1) {
		t.Errorf("params.id = %v, want 1", got)
	}
}

// BLEPair has the most complex return shape (ok, supported, message) — the
// supported=false case is the one the per-device action layer relies on
// to render "not supported on this firmware" instead of a hard failure.
// Documented behaviour: 404/-32601 → supported=false; auth sentinels →
// supported=true (because the device DOES expose the method, we just can't
// reach it).
func TestBLEPair(t *testing.T) {
	t.Run("happy path → ok=true, supported=true", func(t *testing.T) {
		srv := newRecordingServer(t)
		ok, supported, msg := newSetterFor(srv).BLEPair(context.Background(), srv.host())
		if !ok || !supported {
			t.Errorf("ok=%v supported=%v, want both true (msg=%q)", ok, supported, msg)
		}
	})
	t.Run("404 → supported=false (older firmware)", func(t *testing.T) {
		srv := newRecordingServer(t)
		srv.setRPCError("BLE.Pair", 404, "Not Found")
		ok, supported, msg := newSetterFor(srv).BLEPair(context.Background(), srv.host())
		if ok || supported {
			t.Errorf("ok=%v supported=%v, want both false (msg=%q)", ok, supported, msg)
		}
		if !strings.Contains(msg, "not supported") {
			t.Errorf("msg = %q, want 'not supported' wording", msg)
		}
	})
	t.Run("401 with Digest → ok=false but supported=true", func(t *testing.T) {
		srv := newRecordingServer(t)
		srv.srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("WWW-Authenticate", `Digest realm="shelly", qop="auth", nonce="abc", algorithm=SHA-256`)
			w.WriteHeader(401)
		})
		ok, supported, msg := newSetterFor(srv).BLEPair(context.Background(), srv.host())
		if ok {
			t.Errorf("ok=true on 401, want false")
		}
		if !supported {
			t.Errorf("supported=false on 401; auth-required must keep supported=true (the method exists, we just can't reach it)")
		}
		if !strings.Contains(msg, "authentication required") {
			t.Errorf("msg = %q, want auth-required wording", msg)
		}
	})
}

// itoa avoids dragging strconv into a single-line use site.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
