package services

import (
	"testing"
	"time"

	"shellyadmin/internal/models"
)

// firmwareInstallTimeoutFromSettings is the bridge between
// AppSettings.FirmwareInstallTimeout (operator-facing seconds, with a sane
// default) and the time.Duration the install_job uses. The behaviour matters
// because misreading 0/negative as "disable timeout" would let the install
// job hang forever waiting on a stuck device.
func TestFirmwareInstallTimeoutFromSettings(t *testing.T) {
	tests := []struct {
		name     string
		seconds  float64
		expected time.Duration
	}{
		{"default when zero", 0, defaultFirmwareInstallTimeout},
		{"default when negative", -10, defaultFirmwareInstallTimeout},
		{"honours configured value", 600, 10 * time.Minute},
		{"sub-minute is allowed", 30, 30 * time.Second},
		{"fractional seconds OK", 0.5, 500 * time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := firmwareInstallTimeoutFromSettings(models.AppSettings{FirmwareInstallTimeout: tt.seconds})
			if got != tt.expected {
				t.Errorf("firmwareInstallTimeoutFromSettings(%v) = %v, want %v", tt.seconds, got, tt.expected)
			}
		})
	}
}

// firmwareSchedulerDecision is the per-tick logic of the firmware-check
// scheduler. The tests below cover the four reachable branches and verify
// the nextRun anchor stays sensible across a realistic sequence of calls
// (anchor → wait → fire → re-anchor).
func TestFirmwareSchedulerDecision(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	day := 24 * time.Hour

	t.Run("disabled (interval=0) returns zero anchor and no emit", func(t *testing.T) {
		next, emit := firmwareSchedulerDecision(now, 0, now.Add(day))
		if emit {
			t.Errorf("emit = true, want false when interval=0")
		}
		if !next.IsZero() {
			t.Errorf("next = %v, want zero when interval=0", next)
		}
	})

	t.Run("first non-zero interval anchors but does not emit", func(t *testing.T) {
		next, emit := firmwareSchedulerDecision(now, 3600, time.Time{})
		if emit {
			t.Errorf("emit = true, want false on initial anchor")
		}
		if want := now.Add(time.Hour); !next.Equal(want) {
			t.Errorf("next = %v, want %v", next, want)
		}
	})

	t.Run("before deadline neither emits nor re-anchors", func(t *testing.T) {
		anchor := now.Add(time.Hour)
		next, emit := firmwareSchedulerDecision(now, 3600, anchor)
		if emit {
			t.Errorf("emit = true, want false before deadline")
		}
		if !next.Equal(anchor) {
			t.Errorf("next = %v, want anchor %v", next, anchor)
		}
	})

	t.Run("at-or-past deadline emits and re-anchors", func(t *testing.T) {
		anchor := now.Add(-time.Second) // overdue by 1 s
		next, emit := firmwareSchedulerDecision(now, 3600, anchor)
		if !emit {
			t.Errorf("emit = false, want true past deadline")
		}
		if want := now.Add(time.Hour); !next.Equal(want) {
			t.Errorf("next = %v, want %v (re-anchored from now+interval, not anchor+interval)", next, want)
		}
	})

	t.Run("re-enabling after disable re-anchors fresh, no immediate emit", func(t *testing.T) {
		// Simulate operator disabling (anchor cleared) then re-enabling.
		next, emit := firmwareSchedulerDecision(now, 3600, time.Time{})
		if emit {
			t.Errorf("emit = true, want false after re-enable")
		}
		if want := now.Add(time.Hour); !next.Equal(want) {
			t.Errorf("next = %v, want %v", next, want)
		}
	})
}

// TestFormatTimeout pins the rendering used in install-job per-device
// detail lines so a future timeout-formatter refactor can't silently drift.
func TestFormatTimeout(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{5 * time.Minute, "5 min"},
		{1 * time.Minute, "1 min"},
		{90 * time.Second, "1.5 min"},
		{45 * time.Second, "45 sec"},
		{0, "0 sec"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := formatTimeout(tt.d); got != tt.want {
				t.Errorf("formatTimeout(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}
