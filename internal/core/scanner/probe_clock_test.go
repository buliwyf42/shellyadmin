package scanner

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"shellyadmin/internal/core/clock"
	"shellyadmin/internal/core/shellyclient"
)

// ProbeDeviceOnClient must stamp LastSeen from the injected Clock — without
// this, the refresh path on a fleet of N devices produces N different
// timestamps within a single scan tick, making "what just happened" hard to
// correlate. With FakeClock the test can pin the exact RFC3339 string.
func TestProbeDeviceOnClientStampsLastSeenFromClock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/shelly" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"mac":   "AABBCCDDEEFF",
			"gen":   2,
			"model": "SNSW-001X16EU",
			"name":  "test-device",
			"app":   "PlugSG3",
		})
	}))
	defer srv.Close()

	fake := clock.NewFake(time.Date(2026, 5, 8, 14, 30, 0, 0, time.UTC))
	client := shellyclient.New(shellyclient.Options{Timeout: 2 * time.Second})
	ip := srv.Listener.Addr().String()

	dev := ProbeDeviceOnClient(context.Background(), client, ip, "", fake, nopLog)
	if dev == nil {
		t.Fatalf("dev = nil, want a populated Device")
	}
	if dev.LastSeen != "2026-05-08T14:30:00Z" {
		t.Errorf("LastSeen = %q, want 2026-05-08T14:30:00Z (FakeClock value)", dev.LastSeen)
	}
	if dev.MAC != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("MAC = %q, want AA:BB:CC:DD:EE:FF (normalised)", dev.MAC)
	}
}

// reportProbeFailure is the small switch that maps shellyclient sentinels
// to partial Device records. AuthLockedUntil specifically is the one
// time-stamped output (anchor + 60s, matching Shelly fw 2.0.0-beta1's
// brute-force window) — this test pins the clock-injection contract so a
// future refactor can't silently drop back to bare time.Now().
//
// We test the helper directly rather than driving via ProbeDeviceOnClient
// because shellyclient.Probe (GET /shelly) never emits ErrAuthLockout —
// that sentinel comes from the RPC path only. The helper is what binds
// any future caller (e.g. an RPC-triggered path that produces ErrAuthLockout)
// to the FakeClock value.
func TestReportProbeFailureLockoutUsesClock(t *testing.T) {
	fake := clock.NewFake(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	dev := reportProbeFailure("10.1.2.3", shellyclient.ErrAuthLockout, "AA:BB:CC:DD:EE:FF", fake)
	if dev == nil {
		t.Fatalf("dev = nil, want partial Device for refresh path with knownMAC")
	}
	if !dev.AuthRequired {
		t.Errorf("AuthRequired = false, want true (lockout implies auth-required for the UI badge)")
	}
	want := "2026-01-01T00:01:00Z" // anchor + 60s
	if dev.AuthLockedUntil != want {
		t.Errorf("AuthLockedUntil = %q, want %q (clock + 60s)", dev.AuthLockedUntil, want)
	}
}

// reportProbeFailure must return nil on the scan path (knownMAC = "") for
// every sentinel — surfacing a partial Device without a positive Shelly
// identifier was the v0.0.16 / v0.1.1 / v0.1.2 regression class (UniFi
// UDM, nginx Basic auth, etc. leaking into the device list).
func TestReportProbeFailureScanPathAlwaysReturnsNil(t *testing.T) {
	for _, sentinel := range []error{shellyclient.ErrAuthRequired, shellyclient.ErrAuthLockout, shellyclient.ErrTLSCertInvalid} {
		if dev := reportProbeFailure("10.1.2.3", sentinel, "" /* scan path */, clock.Real()); dev != nil {
			t.Errorf("sentinel %v: dev = %#v, want nil for scan path", sentinel, dev)
		}
	}
}

// And on the refresh path, an unrecognised error must also return nil — we
// only carry forward state for the three known sentinels. Anything else
// (network timeout, generic 5xx) is "online state unknown" and the UI must
// not invent an authenticated/locked badge from a generic failure.
func TestReportProbeFailureRefreshPathUnrecognisedErrorReturnsNil(t *testing.T) {
	if dev := reportProbeFailure("10.1.2.3", errFakeUnknown, "AA:BB:CC", clock.Real()); dev != nil {
		t.Errorf("dev = %#v, want nil for unrecognised error", dev)
	}
}

var errFakeUnknown = errors.New("scanner_test: a generic network error")

// On the SCAN path (knownMAC = ""), a probe failure must produce nil. This
// is the bug-fix from v0.0.16 / v0.1.1 that keeps non-Shelly LAN gear
// (UniFi UDM with self-signed HTTPS, nginx with HTTP Basic auth) out of
// the device list. Tested here against the OnClient seam to guarantee the
// regression can't reappear via the new entry point.
func TestProbeDeviceOnClientReturnsNilOnScanPathFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// 401 with a non-Digest challenge — looks like nginx Basic auth, the
		// classic non-Shelly endpoint that v0.1.2 tightened.
		w.Header().Set("WWW-Authenticate", `Basic realm="restricted"`)
		w.WriteHeader(401)
	}))
	defer srv.Close()

	client := shellyclient.New(shellyclient.Options{Timeout: 2 * time.Second})
	ip := srv.Listener.Addr().String()

	dev := ProbeDeviceOnClient(context.Background(), client, ip, "" /* scan path */, nil, nopLog)
	if dev != nil {
		t.Errorf("dev = %#v, want nil (scan path must not surface failed probes)", dev)
	}
}

// Refresh path (knownMAC populated) on auth-required must produce a partial
// Device with AuthRequired=true and the carried-forward MAC. AuthLockedUntil
// stays empty (it's only set on 429 lockout, not on routine auth-required).
func TestProbeDeviceOnClientRefreshPathAuthRequiredKeepsMAC(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("WWW-Authenticate", `Digest realm="shelly", qop="auth", nonce="abc", algorithm=SHA-256`)
		w.WriteHeader(401)
	}))
	defer srv.Close()

	client := shellyclient.New(shellyclient.Options{Timeout: 2 * time.Second})
	ip := srv.Listener.Addr().String()
	const mac = "AA:BB:CC:DD:EE:FF"

	dev := ProbeDeviceOnClient(context.Background(), client, ip, mac, clock.Real(), nopLog)
	if dev == nil {
		t.Fatalf("dev = nil, want partial Device")
	}
	if dev.MAC != mac {
		t.Errorf("MAC = %q, want %q (must carry knownMAC forward)", dev.MAC, mac)
	}
	if !dev.AuthRequired {
		t.Errorf("AuthRequired = false, want true")
	}
	if dev.AuthLockedUntil != "" {
		t.Errorf("AuthLockedUntil = %q, want empty (only set on 429 lockout)", dev.AuthLockedUntil)
	}
	if !strings.Contains(dev.AuthError, "authentication required") {
		t.Errorf("AuthError = %q, want authentication-required text", dev.AuthError)
	}
}

// Nil clock must fall back to clock.Real() rather than panicking. Production
// callers go through ProbeDeviceWithOptions which fills Clock; the OnClient
// seam is meant to be friendly to callers that haven't read the source.
func TestProbeDeviceOnClientNilClockDoesNotPanic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"mac": "AABBCCDDEEFF", "gen": 2})
	}))
	defer srv.Close()

	client := shellyclient.New(shellyclient.Options{Timeout: 2 * time.Second})
	ip := srv.Listener.Addr().String()

	dev := ProbeDeviceOnClient(context.Background(), client, ip, "", nil, nopLog)
	if dev == nil {
		t.Fatalf("dev = nil, expected populated Device")
	}
	if _, err := time.Parse(time.RFC3339, dev.LastSeen); err != nil {
		t.Errorf("LastSeen = %q, not a valid RFC3339 (clock.Real fallback should produce one)", dev.LastSeen)
	}
}
