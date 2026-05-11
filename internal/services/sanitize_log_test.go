package services

import (
	"strings"
	"testing"
)

// TestSanitizeLogMessage covers the redaction patterns the function
// guarantees against. Locks in S21 from the consolidated review — the
// audit pipeline relies on this function to keep credential material out
// of the log even if a future change inadvertently formats a Device or
// Credential struct into a message string. Adding a new sensitive
// field name? Extend secretPattern AND extend this test.
func TestSanitizeLogMessage(t *testing.T) {
	cases := []struct {
		name       string
		input      string
		wantNoLeak []string // substrings that must NOT appear in the output
		wantHas    []string // substrings that MUST appear (proves redaction happened)
	}{
		{
			name:       "password=plain",
			input:      `login attempt for admin with password=hunter2 from 192.168.1.5`,
			wantNoLeak: []string{"hunter2"},
			wantHas:    []string{"password=[redacted]"},
		},
		{
			name:       "password quoted",
			input:      `body: {"password":"hunter2","username":"admin"}`,
			wantNoLeak: []string{"hunter2"},
			wantHas:    []string{"password=[redacted]"},
		},
		{
			name:       "secret= form",
			input:      `cookie secret=abc123def from env`,
			wantNoLeak: []string{"abc123def"},
			wantHas:    []string{"secret=[redacted]"},
		},
		{
			name:       "ha1 form",
			input:      `digest ha1=5f4dcc3b5aa765d61d8327deb882cf99 device=192.168.1.42`,
			wantNoLeak: []string{"5f4dcc3b5aa765d61d8327deb882cf99"},
			wantHas:    []string{"ha1=[redacted]"},
		},
		{
			name:       "pass: with colon",
			input:      `template render pass: SecretValue123 for device shelly`,
			wantNoLeak: []string{"SecretValue123"},
			wantHas:    []string{"pass=[redacted]"},
		},
		{
			name:       "case-insensitive Password",
			input:      `Password: HuNtEr2`,
			wantNoLeak: []string{"HuNtEr2"},
			wantHas:    []string{"=[redacted]"},
		},
		{
			name:       "multiple secrets in one line",
			input:      `password=foo&secret=bar&ha1=baz`,
			wantNoLeak: []string{"foo", "bar", "baz"},
			wantHas:    []string{"password=[redacted]", "secret=[redacted]", "ha1=[redacted]"},
		},
		{
			name:       "no secret untouched",
			input:      `firmware check completed for 42 devices`,
			wantNoLeak: nil,
			wantHas:    []string{"firmware check completed for 42 devices"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SanitizeLogMessage(tc.input)
			for _, leak := range tc.wantNoLeak {
				if strings.Contains(got, leak) {
					t.Errorf("output leaked %q: %s", leak, got)
				}
			}
			for _, want := range tc.wantHas {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q: %s", want, got)
				}
			}
		})
	}
}
