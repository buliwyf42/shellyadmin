package firmware

import "testing"

// IsNewer decides whether a channel version is offered as an available update.
// Every case below is a real (version, channel) pair observed on the Ebelsbach
// fleet on 2026-07-17, during Shelly's phased 2.0.0 rollout — when most devices
// sat on 2.0.0-beta3 while their model's stable channel still served 1.7.x.
// The string-inequality check this replaced flagged all of those as "update
// available", and the resulting Shelly.Update calls were silently ignored by
// the devices.
func TestIsNewer(t *testing.T) {
	tests := []struct {
		name      string
		candidate string
		current   string
		want      bool
	}{
		// The regression: stable is BEHIND a beta-running device.
		{"stable 1.7.5 behind beta3", "1.7.5", "2.0.0-beta3", false},
		{"vendor-suffixed stable behind beta3", "1.7.99-powerstripg4prod1", "2.0.0-beta3", false},
		{"plug vendor stable behind beta3", "1.8.99-plugmg3prod0", "2.0.0-beta3", false},

		// The case that must keep working: prerelease < release.
		{"beta3 to stable 2.0.0 is an upgrade", "2.0.0", "2.0.0-beta3", true},
		{"beta1 to beta3 is an upgrade", "2.0.0-beta3", "2.0.0-beta1", true},

		// Ordinary forward motion.
		{"1.7.5 to 2.0.0", "2.0.0", "1.7.5", true},
		{"same version is not an update", "2.0.0", "2.0.0", false},
		{"beta offered to a device already on stable", "2.0.0-beta3", "2.0.0", false},

		// Unparseable input falls back to string inequality, so an odd vendor
		// string can never hide a real update.
		{"garbage differs from current", "weird-build", "2.0.0", true},
		{"garbage equal to current", "weird-build", "weird-build", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsNewer(tc.candidate, tc.current); got != tc.want {
				t.Errorf("IsNewer(%q, %q) = %v, want %v", tc.candidate, tc.current, got, tc.want)
			}
		})
	}
}
