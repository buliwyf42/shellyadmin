package firmware

import "testing"

func TestIsFeatureFrozen(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  bool
	}{
		{"Plus 1 (this fleet)", "SNSW-001X16EU", true},
		{"Plus 2 PM V2 (this fleet)", "SNSW-102P16EU", true},
		{"Plus 2 PM base SKU", "SNSW-002P16EU", true},
		{"Plus H&T", "SNSN-0013A", true},
		{"BLU Gateway Gen2", "SNGW-BT01", true},
		{"Plus i4 DC variant deliberately omitted", "SNSN-0D24X", false},
		{"BLU Gateway Gen3 deliberately omitted", "S3GW-1DBT001", false},
		{"unknown SKU", "SNSW-999X99EU", false},
		{"empty model", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsFeatureFrozen(tc.model); got != tc.want {
				t.Errorf("IsFeatureFrozen(%q) = %v, want %v", tc.model, got, tc.want)
			}
		})
	}
}
