package firmware

import (
	"context"
	"strings"
	"testing"
)

// ReadAutoUpdateOnClient must distinguish three states based on the
// Schedule.List response: "off" (no matching shelly_service Shelly.Update
// job, or the job is disabled), "stable", or "beta" (taken from the job's
// params.stage). The "off" path is the safe default when the device hasn't
// configured auto-update at all.
func TestReadAutoUpdateOnClient(t *testing.T) {
	tests := []struct {
		name string
		jobs []any
		want string
	}{
		{
			name: "no jobs at all → off",
			jobs: []any{},
			want: AutoUpdateOff,
		},
		{
			name: "matching job, stage=stable → stable",
			jobs: []any{map[string]any{
				"id":     1,
				"enable": true,
				"calls": []any{map[string]any{
					"method": "Shelly.Update",
					"origin": "shelly_service",
					"params": map[string]any{"stage": "stable"},
				}},
			}},
			want: AutoUpdateStable,
		},
		{
			name: "matching job, stage=beta → beta",
			jobs: []any{map[string]any{
				"id":     2,
				"enable": true,
				"calls": []any{map[string]any{
					"method": "Shelly.Update",
					"origin": "shelly_service",
					"params": map[string]any{"stage": "beta"},
				}},
			}},
			want: AutoUpdateBeta,
		},
		{
			name: "disabled job → off (the device skips disabled schedules)",
			jobs: []any{map[string]any{
				"id":     3,
				"enable": false,
				"calls": []any{map[string]any{
					"method": "Shelly.Update",
					"origin": "shelly_service",
					"params": map[string]any{"stage": "stable"},
				}},
			}},
			want: AutoUpdateOff,
		},
		{
			name: "user-created Shelly.Update job (different origin) → off (we must not clobber it)",
			jobs: []any{map[string]any{
				"id":     4,
				"enable": true,
				"calls": []any{map[string]any{
					"method": "Shelly.Update",
					"origin": "user_script_42",
					"params": map[string]any{"stage": "stable"},
				}},
			}},
			want: AutoUpdateOff,
		},
		{
			name: "method case-insensitive match (shelly.update vs Shelly.Update)",
			jobs: []any{map[string]any{
				"id":     5,
				"enable": true,
				"calls": []any{map[string]any{
					"method": "shelly.update",
					"origin": "shelly_service",
					"params": map[string]any{"stage": "stable"},
				}},
			}},
			want: AutoUpdateStable,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeShelly(t)
			f.setMethod("Schedule.List", func(map[string]any) (any, *fakeRPCError) {
				return map[string]any{"jobs": tt.jobs}, nil
			})
			got, err := ReadAutoUpdateOnClient(context.Background(), newClient(), f.host())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got = %q, want %q", got, tt.want)
			}
		})
	}
}

// SetAutoUpdateOnClient(off) must delete all matching shelly_service
// Shelly.Update jobs and create no new ones. User-created jobs (different
// origin) must be left alone.
func TestSetAutoUpdateOnClientOffDeletesOnlyShellyServiceJobs(t *testing.T) {
	f := newFakeShelly(t)
	f.setMethod("Schedule.List", func(map[string]any) (any, *fakeRPCError) {
		return map[string]any{"jobs": []any{
			map[string]any{
				"id":     1,
				"enable": true,
				"calls": []any{map[string]any{
					"method": "Shelly.Update",
					"origin": "shelly_service",
					"params": map[string]any{"stage": "stable"},
				}},
			},
			map[string]any{
				"id":     2,
				"enable": true,
				"calls": []any{map[string]any{
					"method": "Shelly.Update",
					"origin": "user_script",
					"params": map[string]any{"stage": "beta"},
				}},
			},
		}}, nil
	})
	deleted := []any{}
	f.setMethod("Schedule.Delete", func(params map[string]any) (any, *fakeRPCError) {
		deleted = append(deleted, params["id"])
		return map[string]any{}, nil
	})

	if err := SetAutoUpdateOnClient(context.Background(), newClient(), f.host(), AutoUpdateOff); err != nil {
		t.Fatalf("SetAutoUpdateOnClient(off): %v", err)
	}
	if len(deleted) != 1 {
		t.Fatalf("Schedule.Delete called %d times, want 1", len(deleted))
	}
	// JSON unmarshal turns numeric ids into float64.
	if got, want := deleted[0], float64(1); got != want {
		t.Errorf("deleted id = %v, want %v (must not delete user-script job id=2)", got, want)
	}
	// And no Schedule.Create was issued — the unregistered handler would record
	// the call in f.calls. Search for it.
	for _, c := range f.calls {
		if c.method == "Schedule.Create" {
			t.Errorf("Schedule.Create was issued for mode=off; want none")
		}
	}
}

// SetAutoUpdateOnClient(stable) must create a Schedule.Create with the
// daily-midnight timespec, the shelly_service origin marker, and the
// stage=stable param. Format mirrors what the device's own UI writes.
func TestSetAutoUpdateOnClientStableCreatesScheduleWithExpectedShape(t *testing.T) {
	f := newFakeShelly(t)
	f.setMethod("Schedule.List", func(map[string]any) (any, *fakeRPCError) {
		return map[string]any{"jobs": []any{}}, nil
	})
	var createParams map[string]any
	f.setMethod("Schedule.Create", func(params map[string]any) (any, *fakeRPCError) {
		createParams = params
		return map[string]any{"id": 99}, nil
	})

	if err := SetAutoUpdateOnClient(context.Background(), newClient(), f.host(), AutoUpdateStable); err != nil {
		t.Fatalf("SetAutoUpdateOnClient(stable): %v", err)
	}
	if createParams == nil {
		t.Fatalf("Schedule.Create was not called")
	}
	if got := createParams["enable"]; got != true {
		t.Errorf("enable = %v, want true", got)
	}
	if got := createParams["timespec"]; got != "0 0 0 * * 0,1,2,3,4,5,6" {
		t.Errorf("timespec = %v, want daily-midnight cron", got)
	}
	calls, ok := createParams["calls"].([]any)
	if !ok || len(calls) != 1 {
		t.Fatalf("calls = %v, want exactly 1 entry", createParams["calls"])
	}
	call := calls[0].(map[string]any)
	if got := call["method"]; got != "Shelly.Update" {
		t.Errorf("call.method = %v, want Shelly.Update", got)
	}
	if got := call["origin"]; got != autoUpdateOrigin {
		t.Errorf("call.origin = %v, want %s", got, autoUpdateOrigin)
	}
	callParams, _ := call["params"].(map[string]any)
	if got := callParams["stage"]; got != "stable" {
		t.Errorf("call.params.stage = %v, want stable", got)
	}
}

// SetAutoUpdateOnClient must reject invalid modes before issuing any
// network calls — operators should get a clear error rather than seeing
// the device fail to apply a junk schedule.
func TestSetAutoUpdateOnClientRejectsInvalidMode(t *testing.T) {
	f := newFakeShelly(t)
	err := SetAutoUpdateOnClient(context.Background(), newClient(), f.host(), "weekly")
	if err == nil {
		t.Fatalf("err = nil, want invalid-mode error")
	}
	if !strings.Contains(err.Error(), "invalid auto-update mode") {
		t.Errorf("err = %v, want invalid-mode message", err)
	}
	if len(f.calls) != 0 {
		t.Errorf("issued %d RPC calls before validation, want 0", len(f.calls))
	}
}

// Empty mode is canonicalised to "off" — covers the case where the
// provisioner template omits the field, a common backwards-compat path.
func TestSetAutoUpdateOnClientEmptyModeCanonicalisesToOff(t *testing.T) {
	f := newFakeShelly(t)
	f.setMethod("Schedule.List", func(map[string]any) (any, *fakeRPCError) {
		return map[string]any{"jobs": []any{}}, nil
	})
	if err := SetAutoUpdateOnClient(context.Background(), newClient(), f.host(), ""); err != nil {
		t.Fatalf("SetAutoUpdateOnClient(\"\"): %v", err)
	}
	for _, c := range f.calls {
		if c.method == "Schedule.Create" {
			t.Errorf("Schedule.Create was issued for empty mode; want none (empty == off)")
		}
	}
}
