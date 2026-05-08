package firmware

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"shellyadmin/internal/core/clock"
	"shellyadmin/internal/models"
)

// CheckOneOnClient happy path: Shelly.GetDeviceInfo refreshes the running
// version (covering the out-of-band-upgrade case) and stamps batch + fw_id;
// Shelly.CheckForUpdate populates per-channel availability; the resulting
// status is "ok" and the StableUpdate / BetaUpdate flags only fire when the
// channel version differs from the running one.
func TestCheckOneOnClientReportsAvailableUpdates(t *testing.T) {
	f := newFakeShelly(t)
	f.setMethod("Shelly.GetDeviceInfo", func(map[string]any) (any, *fakeRPCError) {
		return map[string]any{
			"ver":   "1.4.4",
			"batch": "2430-Broadwell",
			"fw_id": "20260423-102547/1.4.4-g8c7700a",
		}, nil
	})
	f.setMethod("Shelly.CheckForUpdate", func(map[string]any) (any, *fakeRPCError) {
		return map[string]any{
			"stable": map[string]any{"version": "1.5.0"},
			"beta":   map[string]any{"version": "1.5.1-beta"},
		}, nil
	})

	dev := models.Device{IP: f.host(), MAC: "AA:BB:CC", Gen: 2, FW: "1.4.0"}
	res := CheckOneOnClient(context.Background(), newClient(), dev, clock.NewFake(time.Date(2026, 5, 8, 14, 0, 0, 0, time.UTC)))

	if res.Status != "ok" {
		t.Fatalf("Status = %q (note=%q), want ok", res.Status, res.Note)
	}
	if res.CurrentVer != "1.4.4" {
		t.Errorf("CurrentVer = %q, want 1.4.4 (GetDeviceInfo should override Device.FW)", res.CurrentVer)
	}
	if res.StableVer != "1.5.0" || !res.StableUpdate {
		t.Errorf("Stable: ver=%q update=%v, want 1.5.0 + true", res.StableVer, res.StableUpdate)
	}
	if res.BetaVer != "1.5.1-beta" || !res.BetaUpdate {
		t.Errorf("Beta: ver=%q update=%v, want 1.5.1-beta + true", res.BetaVer, res.BetaUpdate)
	}
	if res.Batch != "2430-Broadwell" {
		t.Errorf("Batch = %q, want 2430-Broadwell", res.Batch)
	}
	if res.FWID != "20260423-102547/1.4.4-g8c7700a" {
		t.Errorf("FWID = %q, want 20260423-...", res.FWID)
	}
	if res.CheckedAt != "2026-05-08T14:00:00Z" {
		t.Errorf("CheckedAt = %q, want 2026-05-08T14:00:00Z (FakeClock value)", res.CheckedAt)
	}
}

// When the channel version equals the running version the update flag must
// be false — a tautological-looking case but it's the operator's primary
// signal that "this device is up to date".
func TestCheckOneOnClientUpdateFalseWhenChannelMatchesRunning(t *testing.T) {
	f := newFakeShelly(t)
	f.setMethod("Shelly.GetDeviceInfo", func(map[string]any) (any, *fakeRPCError) {
		return map[string]any{"ver": "1.5.0"}, nil
	})
	f.setMethod("Shelly.CheckForUpdate", func(map[string]any) (any, *fakeRPCError) {
		return map[string]any{
			"stable": map[string]any{"version": "1.5.0"},
		}, nil
	})

	dev := models.Device{IP: f.host(), Gen: 2, FW: "1.5.0"}
	res := CheckOneOnClient(context.Background(), newClient(), dev, clock.Real())
	if res.StableUpdate {
		t.Errorf("StableUpdate=true when channel matches running")
	}
	if res.BetaUpdate {
		t.Errorf("BetaUpdate=true when no beta entry was returned")
	}
}

// CheckOneOnClient must keep the persisted Device.FW when the
// GetDeviceInfo probe fails — partial failure mode of the dual-probe
// design called out in firmware.go's doc comment.
func TestCheckOneOnClientKeepsDeviceFWWhenGetDeviceInfoFails(t *testing.T) {
	f := newFakeShelly(t)
	// Don't register Shelly.GetDeviceInfo → fake returns 404 RPC error.
	f.setMethod("Shelly.CheckForUpdate", func(map[string]any) (any, *fakeRPCError) {
		return map[string]any{
			"stable": map[string]any{"version": "1.5.0"},
		}, nil
	})

	dev := models.Device{IP: f.host(), Gen: 2, FW: "1.4.0"}
	res := CheckOneOnClient(context.Background(), newClient(), dev, clock.Real())
	if res.Status != "ok" {
		t.Fatalf("Status = %q, want ok (CheckForUpdate succeeded)", res.Status)
	}
	if res.CurrentVer != "1.4.0" {
		t.Errorf("CurrentVer = %q, want 1.4.0 (preserved from Device.FW)", res.CurrentVer)
	}
}

// CheckOneOnClient surfaces a CheckForUpdate failure as Status="error" with
// a friendlyRPCError-formatted note. The persisted CurrentVer is preserved.
func TestCheckOneOnClientSurfacesCheckForUpdateError(t *testing.T) {
	f := newFakeShelly(t)
	f.setMethod("Shelly.GetDeviceInfo", func(map[string]any) (any, *fakeRPCError) {
		return map[string]any{"ver": "1.4.4"}, nil
	})
	// Shelly.CheckForUpdate → 404
	dev := models.Device{IP: f.host(), Gen: 2, FW: "1.4.0"}
	res := CheckOneOnClient(context.Background(), newClient(), dev, clock.Real())
	if res.Status != "error" {
		t.Fatalf("Status = %q, want error", res.Status)
	}
	if res.Note == "" {
		t.Errorf("Note is empty; expected friendlyRPCError text")
	}
}

// CheckOneWithOptions must short-circuit gen<2 without ever building a
// shellyclient. We can't easily detect "no client built" but we can detect
// "no RPC call issued" because there's no fake server attached.
func TestCheckOneWithOptionsShortCircuitsGen1(t *testing.T) {
	dev := models.Device{IP: "127.0.0.1:1", MAC: "AA:BB:CC", Gen: 1, FW: "1.0.0"}
	res := CheckOneWithOptions(context.Background(), dev, Options{Timeout: 100 * time.Millisecond})
	if res.Status != "na" {
		t.Errorf("Status = %q, want na", res.Status)
	}
	if !strings.Contains(res.Note, "gen1") {
		t.Errorf("Note = %q, want gen1 reference", res.Note)
	}
}

// TriggerUpdateOnClient sends Shelly.Update with the requested stage. The
// stage parameter is required by the device — flubbing it would silently
// install the wrong channel.
func TestTriggerUpdateOnClientSendsRequestedStage(t *testing.T) {
	f := newFakeShelly(t)
	f.setMethod("Shelly.Update", func(params map[string]any) (any, *fakeRPCError) {
		if got := params["stage"]; got != "beta" {
			t.Errorf("params.stage = %v, want beta", got)
		}
		return map[string]any{}, nil
	})

	res := TriggerUpdateOnClient(context.Background(), newClient(), f.host(), "beta")
	if res.Status != "triggered" {
		t.Fatalf("Status = %q (detail=%q), want triggered", res.Status, res.Detail)
	}
}

// TriggerUpdateOnClient maps shellyclient sentinels to friendly messages so
// the bulk-install UI can show "device locked" rather than a wrapped error.
// Replacing the fake-Shelly handler with a status-only one bypasses the
// JSON-RPC envelope layer entirely — that's exactly what shellyclient does
// when it sees 401 / 429 at the transport level. Note that 401 must carry
// a Digest challenge header to be recognised as a Shelly auth-required
// response; a bare 401 looks like a non-Shelly endpoint to shellyclient.
func TestTriggerUpdateOnClientHandlesAuthSentinels(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		wantDetail string
	}{
		{
			name: "401 with Digest challenge → authentication required",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("WWW-Authenticate", `Digest realm="shelly", qop="auth", nonce="abc", algorithm=SHA-256`)
				w.WriteHeader(401)
			},
			wantDetail: "authentication required",
		},
		{
			name: "429 → device locked",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(429)
			},
			wantDetail: "device locked (brute-force protection)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeShelly(t)
			f.srv.Config.Handler = tt.handler
			res := TriggerUpdateOnClient(context.Background(), newClient(), f.host(), "stable")
			if res.Status != "failed" {
				t.Errorf("Status = %q, want failed", res.Status)
			}
			if res.Detail != tt.wantDetail {
				t.Errorf("Detail = %q, want %q", res.Detail, tt.wantDetail)
			}
		})
	}
}

// GetDeviceFirmwareOnClient prefers the "ver" field when present, falls back
// to "fw" for older firmware, and returns "" + error when the RPC fails. The
// install_job's polling loop relies on the empty-vs-non-empty distinction.
func TestGetDeviceFirmwareOnClientPrefersVerFallsBackToFW(t *testing.T) {
	tests := []struct {
		name string
		body map[string]any
		want string
	}{
		{"ver wins over fw", map[string]any{"ver": "1.5.0", "fw": "ignored"}, "1.5.0"},
		{"fw fallback when ver empty", map[string]any{"ver": "", "fw": "1.4.0"}, "1.4.0"},
		{"fw fallback when ver absent", map[string]any{"fw": "1.4.0"}, "1.4.0"},
		{"empty when both absent", map[string]any{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeShelly(t)
			f.setMethod("Shelly.GetDeviceInfo", func(map[string]any) (any, *fakeRPCError) {
				return tt.body, nil
			})
			got, err := GetDeviceFirmwareOnClient(context.Background(), newClient(), f.host())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetDeviceFirmwareOnClientReturnsErrorOnRPCFailure(t *testing.T) {
	f := newFakeShelly(t)
	// No Shelly.GetDeviceInfo handler registered → fake returns 404 RPC error.
	got, err := GetDeviceFirmwareOnClient(context.Background(), newClient(), f.host())
	if err == nil {
		t.Fatalf("err = nil, want non-nil for unregistered method")
	}
	if got != "" {
		t.Errorf("got = %q, want empty string on error", got)
	}
}
