package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"shellyadmin/internal/models"
)

func TestRedactCredentialsDropsSecrets(t *testing.T) {
	in := []models.Credential{
		{
			Name:     "site-a",
			Username: "admin",
			Password: "TopSecret!",
			HA1:      "deadbeef",
			Tags:     []string{"prod"},
		},
		{
			Name:     "site-b",
			Username: "ops",
			Password: "AnotherSecret",
			HA1:      "cafebabe",
		},
	}
	out := redactCredentials(in)
	if len(out) != 2 {
		t.Fatalf("got %d credentials, want 2", len(out))
	}
	for _, c := range out {
		blob, err := json.Marshal(c)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		body := string(blob)
		for _, secret := range []string{"TopSecret!", "AnotherSecret", "deadbeef", "cafebabe", "password", "ha1"} {
			if strings.Contains(strings.ToLower(body), strings.ToLower(secret)) {
				t.Errorf("redacted output leaks %q: %s", secret, body)
			}
		}
		if c.Name == "" || c.Username == "" {
			t.Errorf("redacted credential dropped non-secret field: %+v", c)
		}
	}
	if got := out[0].Tags; len(got) != 1 || got[0] != "prod" {
		t.Errorf("tags not preserved: %v", got)
	}
}

func TestRedactCredentialsCopiesTagsSlice(t *testing.T) {
	in := []models.Credential{{Name: "a", Tags: []string{"one", "two"}}}
	out := redactCredentials(in)
	out[0].Tags[0] = "MUTATED"
	if in[0].Tags[0] != "one" {
		t.Errorf("redactCredentials did not isolate tag slice; underlying slice was mutated to %v", in[0].Tags)
	}
}
