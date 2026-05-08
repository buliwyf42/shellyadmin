package firmware

import (
	"context"
	"strings"
	"testing"

	"shellyadmin/internal/core/shellyclient"
)

// ListSupportedMethodsOnClient returns the device's method list, sorted, and
// drops non-string entries. Sort matters: callers compare against a static
// catalog and a deterministic order makes diffs reviewable.
func TestListSupportedMethodsOnClientSortsAndFilters(t *testing.T) {
	f := newFakeShelly(t)
	f.setMethod("Shelly.ListMethods", func(map[string]any) (any, *fakeRPCError) {
		return map[string]any{
			"methods": []any{
				"Shelly.Update",
				"BLE.SetConfig",
				"Shelly.GetDeviceInfo",
				"", // empty string — must be dropped
				42, // non-string — must be dropped
				"Sys.SetConfig",
			},
		}, nil
	})

	got, err := ListSupportedMethodsOnClient(context.Background(), newClient(), f.host())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"BLE.SetConfig", "Shelly.GetDeviceInfo", "Shelly.Update", "Sys.SetConfig"}
	if len(got) != len(want) {
		t.Fatalf("len(got) = %d, want %d (got=%v)", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("got[%d] = %q, want %q", i, got[i], w)
		}
	}
}

// CLAUDE.md flags this as a known Shelly quirk: the device emits
// non-standard JSON-RPC error code 404 (not -32601) when a method is not
// available on a particular model. shellyclient.IsMethodNotFound must
// recognise both. This is the per-package contract: the firmware caller
// gets back an error and IsMethodNotFound(err) returns true in both cases.
func TestListSupportedMethodsOnClientErrorIsMethodNotFoundFor404And32601(t *testing.T) {
	tests := []struct {
		name string
		code int
	}{
		{"shelly non-standard 404", 404},
		{"json-rpc standard -32601", -32601},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeShelly(t)
			f.setMethod("Shelly.ListMethods", func(map[string]any) (any, *fakeRPCError) {
				return nil, &fakeRPCError{Code: tt.code, Message: "method not found"}
			})
			_, err := ListSupportedMethodsOnClient(context.Background(), newClient(), f.host())
			if err == nil {
				t.Fatalf("err = nil, want non-nil")
			}
			if !shellyclient.IsMethodNotFound(err) {
				t.Errorf("IsMethodNotFound(%v) = false, want true (code=%d)", err, tt.code)
			}
		})
	}
}

// ListSupportedMethods (the …WithOptions wrapper) must short-circuit
// gen<2 without ever reaching the network. The unique "gen1" message in
// the returned error is the cheapest signal that the gen check fired
// before the http stack got involved.
func TestListSupportedMethodsShortCircuitsGen1(t *testing.T) {
	_, err := ListSupportedMethods(context.Background(), "127.0.0.1:1", 1, Options{})
	if err == nil {
		t.Fatalf("err = nil, want gen1 error")
	}
	if !strings.Contains(err.Error(), "gen1") {
		t.Errorf("err = %v, want to mention gen1 (otherwise the http path was hit)", err)
	}
}
