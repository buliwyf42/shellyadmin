package services

import (
	"context"
	"testing"

	"shellyadmin/internal/models"
	"shellyadmin/internal/services/validation"
)

// TestStartScanSucceedsWithEncryptedMCPToken is the end-to-end regression guard
// for the v0.5.1 fix. StartScan reads the RAW DB settings row, whose MCPToken is
// secretbox ciphertext (sealed by SaveSettings). Before the fix, that ciphertext
// was run through the URL-safe-alphabet check and scans failed with
// "mcp token must match [A-Za-z0-9_-]{16,128}" whenever an MCP token was set.
func TestStartScanSucceedsWithEncryptedMCPToken(t *testing.T) {
	database, svc := testService(t)
	// Drains the background scan goroutine before testService's db.Close (LIFO).
	t.Cleanup(func() { svc.Stop(context.Background()) })

	s := models.DefaultSettings()
	s.Subnets = []string{"10.255.255.252/30"} // obscure RFC1918 /30; scanner requires private/link-local ranges
	s.ScanTimeout = 0.2
	s.MCPEnabled = true
	s.MCPToken = "regressiontoken12345" // >=16 URL-safe chars: passes save-time validation, then sealed
	if err := svc.SaveSettings(s); err != nil {
		t.Fatalf("SaveSettings() error = %v", err)
	}

	// The raw row now holds ciphertext, distinct from the plaintext token...
	raw, err := database.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings() error = %v", err)
	}
	if raw.MCPToken == "" || raw.MCPToken == s.MCPToken {
		t.Fatalf("raw MCPToken = %q, want sealed ciphertext distinct from plaintext", raw.MCPToken)
	}
	// ...and that ciphertext would fail the old full validator's alphabet check,
	// which is exactly the condition StartScan must no longer trip on.
	if validation.MCPTokenPattern.MatchString(raw.MCPToken) {
		t.Fatalf("sealed token %q matches URL-safe pattern; test no longer exercises the bug", raw.MCPToken)
	}

	if err := svc.StartScan(); err != nil {
		t.Fatalf("StartScan() with encrypted MCP token in DB row error = %v", err)
	}

	job, err := database.GetLatestJob("scan")
	if err != nil {
		t.Fatalf("GetLatestJob() error = %v", err)
	}
	if job.Type != "scan" {
		t.Fatalf("GetLatestJob().Type = %q, want %q", job.Type, "scan")
	}
	if job.Total < 1 {
		t.Fatalf("GetLatestJob().Total = %d, want >= 1", job.Total)
	}
}
