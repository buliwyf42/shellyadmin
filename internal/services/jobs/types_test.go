package jobs

import (
	"encoding/json"
	"testing"
	"time"

	"shellyadmin/internal/models"
)

func TestRefreshProbeTimeoutAppliesNormalize(t *testing.T) {
	// AppSettings.Normalize clamps RefreshTimeout to [0.2, 30]; passing 0
	// must therefore yield the post-normalize default (5s) rather than 0.
	got := RefreshProbeTimeout(models.AppSettings{RefreshTimeout: 0})
	if got <= 0 {
		t.Errorf("RefreshProbeTimeout(0) = %v, want post-normalize default", got)
	}
}

func TestScanJobStale(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	old := now.Add(-StaleScanGrace - time.Second).Format(time.RFC3339)
	fresh := now.Add(-StaleScanGrace + time.Second).Format(time.RFC3339)

	stale, err := ScanJobStale(models.Job{UpdatedAt: old}, now)
	if err != nil {
		t.Fatalf("ScanJobStale(old) error = %v", err)
	}
	if !stale {
		t.Error("ScanJobStale(old) = false, want true")
	}

	stale, err = ScanJobStale(models.Job{UpdatedAt: fresh}, now)
	if err != nil {
		t.Fatalf("ScanJobStale(fresh) error = %v", err)
	}
	if stale {
		t.Error("ScanJobStale(fresh) = true, want false")
	}
}

func TestRefreshJobStaleUsesLongerGrace(t *testing.T) {
	// 30s-old refresh is still fresh (grace is 2 min); 30s-old scan would
	// be stale. This is the whole reason the two helpers exist separately.
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	thirty := now.Add(-30 * time.Second).Format(time.RFC3339)
	stale, err := RefreshJobStale(models.Job{UpdatedAt: thirty}, now)
	if err != nil {
		t.Fatalf("RefreshJobStale error = %v", err)
	}
	if stale {
		t.Error("RefreshJobStale(30s) = true, want false (refresh grace is 2 min)")
	}
}

func TestFirmwareInstallTimeoutFromSettings(t *testing.T) {
	tests := []struct {
		name    string
		seconds float64
		want    time.Duration
	}{
		{"default when zero", 0, DefaultFirmwareInstallTimeout},
		{"default when negative", -10, DefaultFirmwareInstallTimeout},
		{"honours configured value", 600, 10 * time.Minute},
		{"sub-minute is allowed", 30, 30 * time.Second},
		{"fractional seconds OK", 0.5, 500 * time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FirmwareInstallTimeoutFromSettings(models.AppSettings{FirmwareInstallTimeout: tt.seconds})
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFirmwareInstallPollIntervalFromSettings(t *testing.T) {
	tests := []struct {
		name    string
		seconds float64
		want    time.Duration
	}{
		{"default when zero", 0, DefaultFirmwareInstallPollInterval},
		{"default when negative", -2, DefaultFirmwareInstallPollInterval},
		{"honours configured value", 10, 10 * time.Second},
		{"clamps sub-second up to 1 s", 0.25, 1 * time.Second},
		{"clamps above 60 s down to 60", 120, 60 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FirmwareInstallPollIntervalFromSettings(models.AppSettings{FirmwareInstallPollInterval: tt.seconds})
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFirmwareSchedulerDecision(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("disabled returns zero anchor and no emit", func(t *testing.T) {
		next, emit := FirmwareSchedulerDecision(now, 0, now.Add(time.Hour))
		if emit || !next.IsZero() {
			t.Errorf("emit=%v next=%v, want emit=false next=zero", emit, next)
		}
	})

	t.Run("first non-zero interval anchors without emitting", func(t *testing.T) {
		next, emit := FirmwareSchedulerDecision(now, 3600, time.Time{})
		if emit {
			t.Error("emit = true on initial anchor, want false")
		}
		if want := now.Add(time.Hour); !next.Equal(want) {
			t.Errorf("next = %v, want %v", next, want)
		}
	})

	t.Run("at-or-past deadline emits and re-anchors", func(t *testing.T) {
		anchor := now.Add(-time.Second)
		next, emit := FirmwareSchedulerDecision(now, 3600, anchor)
		if !emit {
			t.Error("emit = false past deadline, want true")
		}
		if want := now.Add(time.Hour); !next.Equal(want) {
			t.Errorf("next = %v, want %v (re-anchored from now, not anchor)", next, want)
		}
	})
}

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
			if got := FormatTimeout(tt.d); got != tt.want {
				t.Errorf("FormatTimeout(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

func TestTargetVersion(t *testing.T) {
	d := models.Device{FWAvailableStable: "1.0.0", FWAvailableBeta: "1.1.0-beta"}
	if got := TargetVersion(d, "stable"); got != "1.0.0" {
		t.Errorf("stable = %q, want 1.0.0", got)
	}
	if got := TargetVersion(d, "beta"); got != "1.1.0-beta" {
		t.Errorf("beta = %q, want 1.1.0-beta", got)
	}
	if got := TargetVersion(d, ""); got != "1.0.0" {
		t.Errorf("empty stage = %q, want stable fallback 1.0.0", got)
	}
}

func TestIsInstallTerminal(t *testing.T) {
	terminal := []string{"current", "error", "unknown", "skipped"}
	for _, s := range terminal {
		if !IsInstallTerminal(s) {
			t.Errorf("IsInstallTerminal(%q) = false, want true", s)
		}
	}
	nonTerminal := []string{"triggered", "updating", "queued", ""}
	for _, s := range nonTerminal {
		if IsInstallTerminal(s) {
			t.Errorf("IsInstallTerminal(%q) = true, want false", s)
		}
	}
}

func TestParseScanPayloadEmptyIsZero(t *testing.T) {
	got, err := ParseScanPayload("")
	if err != nil {
		t.Fatalf("ParseScanPayload(\"\") error = %v", err)
	}
	if len(got.ExistingMACs) != 0 {
		t.Errorf("ExistingMACs = %v, want empty", got.ExistingMACs)
	}
}

func TestParseScanPayloadRoundTrip(t *testing.T) {
	in := ScanJobPayload{ExistingMACs: []string{"AA:BB", "CC:DD"}}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got, err := ParseScanPayload(string(raw))
	if err != nil {
		t.Fatalf("ParseScanPayload error = %v", err)
	}
	if len(got.ExistingMACs) != 2 || got.ExistingMACs[0] != "AA:BB" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}

func TestParseScanResultEmptyHasNonNilPending(t *testing.T) {
	got, err := ParseScanResult("")
	if err != nil {
		t.Fatalf("ParseScanResult(\"\") error = %v", err)
	}
	if got.Pending == nil {
		t.Error("Pending = nil, want empty-but-non-nil slice (caller may JSON-marshal)")
	}
}

func TestParseFirmwareResultEmptyHasNonNilResults(t *testing.T) {
	got, err := ParseFirmwareResult("")
	if err != nil {
		t.Fatalf("ParseFirmwareResult(\"\") error = %v", err)
	}
	if got.Results == nil {
		t.Error("Results = nil, want empty-but-non-nil slice")
	}
}
