package audit

import (
	"testing"
)

func TestShouldForwardRespectsLevelFloor(t *testing.T) {
	cases := []struct {
		level, minLevel string
		want            bool
	}{
		// Empty minLevel defaults to INFO+.
		{"DEBUG", "", false},
		{"INFO", "", true},
		{"WARN", "", true},
		{"ERROR", "", true},
		// Explicit DEBUG floor lets everything through.
		{"DEBUG", "DEBUG", true},
		{"INFO", "DEBUG", true},
		// Explicit WARN floor drops INFO + DEBUG.
		{"DEBUG", "WARN", false},
		{"INFO", "WARN", false},
		{"WARN", "WARN", true},
		{"ERROR", "WARN", true},
		// Unknown floor defaults to INFO.
		{"INFO", "gibberish", true},
		{"DEBUG", "gibberish", false},
		// Case-insensitive on both axes.
		{"warn", "info", true},
	}
	for _, tc := range cases {
		got := ShouldForward(tc.level, tc.minLevel)
		if got != tc.want {
			t.Errorf("ShouldForward(%q,%q) = %v, want %v", tc.level, tc.minLevel, got, tc.want)
		}
	}
}

func TestValidateWebhookURL(t *testing.T) {
	cases := []struct {
		url     string
		wantErr bool
	}{
		{"", false}, // empty = disabled, valid
		{"https://hooks.example.com/audit", false},
		{"http://10.0.0.5:8080/sink", false},
		// Rejected:
		{"ftp://example.com/", true},
		{"file:///tmp/x", true},
		{"://no-scheme", true},
		{"/relative/path", true},
		{"https://", true}, // no host
	}
	for _, tc := range cases {
		err := ValidateWebhookURL(tc.url)
		gotErr := err != nil
		if gotErr != tc.wantErr {
			t.Errorf("ValidateWebhookURL(%q) error = %v, wantErr = %v", tc.url, err, tc.wantErr)
		}
	}
}
