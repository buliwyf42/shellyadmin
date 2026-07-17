package models

import "testing"

// The install job spends FirmwareInstallQuietPeriod deliberately not looking at
// the device (polling an in-flight OTA starves it). That makes the two knobs
// interact: a timeout that does not clear the quiet period by a download's
// margin would expire mid-OTA and report a healthy update as "unknown".
// Normalize enforces the floor, which also repairs rows carrying the
// pre-v0.5.6 default of 300.
func TestNormalizeKeepsInstallTimeoutAboveQuietPeriod(t *testing.T) {
	tests := []struct {
		name        string
		timeout     float64
		quiet       float64
		wantTimeout float64
		wantQuiet   float64
	}{
		{"legacy 300s row is raised past the quiet period", 300, 150, 300, 150},
		{"timeout under the floor is raised", 120, 150, 300, 150},
		{"operator's generous timeout is left alone", 900, 150, 900, 150},
		{"zero timeout takes the default", 0, 150, 600, 150},
		{"quiet period opt-out still keeps a polling window", 100, 0, 150, 0},
		{"quiet period is clamped, timeout follows it up", 200, 9000, 750, 600},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := DefaultSettings()
			s.FirmwareInstallTimeout = tc.timeout
			s.FirmwareInstallQuietPeriod = tc.quiet
			s.Normalize()
			if s.FirmwareInstallQuietPeriod != tc.wantQuiet {
				t.Errorf("QuietPeriod = %v, want %v", s.FirmwareInstallQuietPeriod, tc.wantQuiet)
			}
			if s.FirmwareInstallTimeout != tc.wantTimeout {
				t.Errorf("Timeout = %v, want %v", s.FirmwareInstallTimeout, tc.wantTimeout)
			}
			if s.FirmwareInstallTimeout <= s.FirmwareInstallQuietPeriod {
				t.Errorf("timeout %v leaves no polling window after quiet period %v",
					s.FirmwareInstallTimeout, s.FirmwareInstallQuietPeriod)
			}
		})
	}
}
