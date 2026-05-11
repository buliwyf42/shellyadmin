package util

import (
	"os"
	"strings"
)

// DecodeSecretValue reads a secret from the environment, supporting the
// docker-style "_FILE" indirection: if ENVKEY_FILE points at a readable
// file, its trimmed contents are returned; otherwise the trimmed ENVKEY
// value is returned. Empty when neither is set.
//
// MOVED FROM internal/services/app.go — v0.3.0 services-layer split (M7).
// Lives in util/ because the only caller is cmd/shellyctl/main.go's boot
// path; keeping it in services means main.go imports services just to
// reach this helper.
func DecodeSecretValue(envKey string) string {
	if value := os.Getenv(envKey + "_FILE"); value != "" {
		body, err := os.ReadFile(value)
		if err == nil {
			return strings.TrimSpace(string(body))
		}
	}
	return strings.TrimSpace(os.Getenv(envKey))
}
