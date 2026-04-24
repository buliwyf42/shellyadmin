package util

import "strings"

// FirstNonEmpty returns the first value whose trimmed form is non-empty.
// The original (untrimmed) value is returned.
func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
