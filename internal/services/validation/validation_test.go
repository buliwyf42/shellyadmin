package validation

import (
	"strings"
	"testing"

	"shellyadmin/internal/models"
)

func TestScanParamsReturnsTargetCount(t *testing.T) {
	base := models.DefaultSettings()
	base.Subnets = []string{"10.0.0.0/29"}

	n, err := ScanParams(base)
	if err != nil {
		t.Fatalf("ScanParams() error = %v", err)
	}
	if n < 1 {
		t.Fatalf("ScanParams() count = %d, want >= 1", n)
	}

	withMDNS := base
	withMDNS.EnableMDNS = true
	m, err := ScanParams(withMDNS)
	if err != nil {
		t.Fatalf("ScanParams(mdns) error = %v", err)
	}
	if m != n+1 {
		t.Fatalf("ScanParams(mdns) count = %d, want %d (subnet count + 1 for mDNS)", m, n+1)
	}
}

func TestScanParamsRejectsEmptyTargets(t *testing.T) {
	s := models.DefaultSettings()
	s.Subnets = nil
	s.EnableMDNS = false

	if _, err := ScanParams(s); err == nil {
		t.Fatal("ScanParams() error = nil, want no-scan-targets error")
	} else if !strings.Contains(err.Error(), "no scan targets configured") {
		t.Fatalf("ScanParams() error = %q, want substring %q", err.Error(), "no scan targets configured")
	}
}

// TestScanParamsIgnoresMCPToken is the regression guard for the v0.5.1 fix:
// the jobs layer validates a raw DB row whose MCPToken is secretbox ciphertext
// (which never matches MCPTokenPattern). ScanParams must not look at it, while
// the full Settings validator still rejects it — proving the split is
// intentional, not accidental.
func TestScanParamsIgnoresMCPToken(t *testing.T) {
	cipherish := strings.Repeat("A", 100) + "+/=" // non-URL-safe, mimics sealed ciphertext
	if MCPTokenPattern.MatchString(cipherish) {
		t.Fatalf("test token %q unexpectedly matches the URL-safe pattern", cipherish)
	}

	s := models.DefaultSettings()
	s.Subnets = []string{"10.0.0.0/30"}
	s.MCPEnabled = true
	s.MCPToken = cipherish

	if _, err := ScanParams(s); err != nil {
		t.Fatalf("ScanParams() with ciphertext MCP token error = %v, want nil (token must be ignored)", err)
	}

	if err := Settings(s); err == nil {
		t.Fatal("Settings() with ciphertext MCP token error = nil, want MCP token format error")
	}
}

func TestScanParamsRejectsOutOfRangeConcurrency(t *testing.T) {
	s := models.DefaultSettings()
	s.Subnets = []string{"10.0.0.0/30"}
	s.ScanConcurrency = 9999

	if _, err := ScanParams(s); err == nil {
		t.Fatal("ScanParams() error = nil, want concurrency-out-of-range error")
	}
}

func TestTemplateAcceptsKnownSections(t *testing.T) {
	tpl := map[string]interface{}{
		"sys":      map[string]interface{}{"device": map[string]interface{}{"name": "x"}},
		"mqtt":     map[string]interface{}{"enable": true},
		"ui":       map[string]interface{}{"idle_brightness": 30}, // catch-all-served
		"gen2_rpc": map[string]interface{}{"Sys.SetConfig": map[string]interface{}{}},
		"ota":      map[string]interface{}{"enabled": true}, // legacy, tolerated
	}
	if err := Template(tpl); err != nil {
		t.Fatalf("Template() error = %v, want nil", err)
	}
}

func TestTemplateAcceptsMixedCaseSection(t *testing.T) {
	// applySection lowercases before dispatch, so "SYS" is a valid key.
	if err := Template(map[string]interface{}{"SYS": map[string]interface{}{}}); err != nil {
		t.Fatalf("Template() error = %v, want nil", err)
	}
}

func TestTemplateRejectsUnknownSection(t *testing.T) {
	err := Template(map[string]interface{}{"syss": map[string]interface{}{}})
	if err == nil {
		t.Fatal("Template() error = nil, want unknown-section error")
	}
	if !strings.Contains(err.Error(), `"syss"`) || !strings.Contains(err.Error(), "gen2_rpc") {
		t.Fatalf("error should name the section and the gen2_rpc escape hatch: %v", err)
	}
}

func TestTemplateRejectsUntrimmedSection(t *testing.T) {
	// "sys " would dispatch a broken "Sys .SetConfig" at runtime — the
	// validator must mirror applySection (lowercase, no trim) and reject it.
	if err := Template(map[string]interface{}{"sys ": map[string]interface{}{}}); err == nil {
		t.Fatal("Template() error = nil, want unknown-section error for untrimmed key")
	}
}
