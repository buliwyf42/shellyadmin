// Package validation hosts the pure normalize-then-validate pipeline for
// operator-supplied input (AppSettings, provisioner templates, audit webhook
// URLs). Pulling these out of services means sub-packages (backup, jobs)
// can call them without an import cycle.
//
// MOVED FROM internal/services/app.go (ValidateSettings, ValidateTemplate)
// + internal/services/audit_webhook.go (validateWebhookURL) — v0.3.0
// services-layer split (M7, docs/plans/phase-4b-refactor-block.md Block
// 4b.1). Backward-compat aliases in internal/services preserve the existing
// services.MCPTokenRedacted / services.MaxTemplateBytes / services.ValidateSettings
// surface so api/handler_* and existing tests compile unchanged.
package validation

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"

	"shellyadmin/internal/core/scanner"
	"shellyadmin/internal/models"
)

const (
	// MaxTemplateBytes is the per-template payload cap. Keeps the embedded
	// SQLite from getting flooded by a runaway JSON file.
	MaxTemplateBytes = 64 * 1024

	// MaxSubnets caps how many CIDRs a single AppSettings.Subnets list can
	// hold; a sanity check, not a hard architectural limit.
	MaxSubnets = 64

	// MaxScanTargets is the absolute ceiling on the total subnet+mDNS
	// target count a scan can configure. Larger fleets should split into
	// multiple scan windows rather than try one giant sweep.
	MaxScanTargets = 65534

	// MCPTokenRedacted is the placeholder API GET handlers substitute for
	// a non-empty MCP token before returning settings to the SPA, and the
	// value SaveSettings interprets as "keep the existing token". Any
	// other value (including the empty string) replaces the stored token.
	MCPTokenRedacted = "<set>"
)

// MCPTokenPattern restricts MCP tokens to the URL-safe alphabet so both
// transport forms (Authorization header + URL path prefix) keep working
// unconditionally. The path form interprets "/" as a separator, so a
// reserved-char token would break.
var MCPTokenPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,128}$`)

// Settings runs the canonical normalize-then-validate pipeline over an
// AppSettings row. SaveSettings + the backup-import path both gate on
// this so a bad row never lands in the persistent store.
func Settings(settings models.AppSettings) error {
	settings.Normalize()
	if len(settings.Subnets) > MaxSubnets {
		return fmt.Errorf("too many subnets configured")
	}
	if settings.ScanConcurrency < 1 || settings.ScanConcurrency > 256 {
		return fmt.Errorf("scan concurrency must be between 1 and 256")
	}
	if settings.ScanTimeout < 0.2 || settings.ScanTimeout > 30 {
		return fmt.Errorf("scan timeout must be between 0.2 and 30 seconds")
	}
	if settings.RefreshTimeout < 0.2 || settings.RefreshTimeout > 30 {
		return fmt.Errorf("refresh timeout must be between 0.2 and 30 seconds")
	}
	total := 0
	for _, subnet := range settings.Subnets {
		ips, err := scanner.ExpandCIDR(subnet)
		if err != nil {
			return err
		}
		total += len(ips)
	}
	if settings.EnableMDNS {
		total++
	}
	if total == 0 {
		return errors.New("no scan targets configured; add at least one subnet in Settings or enable mDNS discovery")
	}
	if total > MaxScanTargets {
		return fmt.Errorf("scan target count %d exceeds limit %d", total, MaxScanTargets)
	}
	if mode := settings.Compliance.WSTLSMode; mode != "" && mode != "no_validation" && mode != "default" && mode != "user" {
		return fmt.Errorf("websocket tls mode must be no_validation, default, or user")
	}
	if settings.Compliance.RPCUDPPort != nil && *settings.Compliance.RPCUDPPort < 0 {
		return fmt.Errorf("rpc udp port must be 0 or greater")
	}
	if lat := settings.Compliance.Lat; lat != nil && (*lat < -90 || *lat > 90) {
		return fmt.Errorf("latitude must be between -90 and 90")
	}
	if lon := settings.Compliance.Lon; lon != nil && (*lon < -180 || *lon > 180) {
		return fmt.Errorf("longitude must be between -180 and 180")
	}
	if settings.MCPEnabled && len(settings.MCPToken) < 16 {
		return fmt.Errorf("mcp token must be at least 16 characters when MCP is enabled")
	}
	// T11 — webhook URL must be a valid absolute http(s) URL with a host.
	// Empty disables the forwarder.
	if err := WebhookURL(settings.AuditWebhookURL); err != nil {
		return err
	}
	// MCP auth accepts the token either as Authorization: Bearer or as the
	// first URL path segment. The path-form interprets "/" as a segment
	// separator, so a token containing "/" (or other URL-reserved chars)
	// breaks the path auth. Restrict to URL-safe charset to keep both
	// transport forms working unconditionally.
	if settings.MCPToken != "" && settings.MCPToken != MCPTokenRedacted {
		if !MCPTokenPattern.MatchString(settings.MCPToken) {
			return fmt.Errorf("mcp token must match [A-Za-z0-9_-]{16,128} (URL-safe alphabet, 16-128 chars)")
		}
	}
	// Fail-fast on bad regex patterns in custom rules. Without this, a typo
	// in the UI would silently classify every device as "mismatch" because
	// the compile error is swallowed at evaluation time (compliance.go:checkOp).
	for i, rule := range settings.Compliance.CustomRules {
		if rule.Op != "regex" {
			continue
		}
		if _, err := regexp.Compile(rule.Value); err != nil {
			label := rule.Label
			if label == "" {
				label = fmt.Sprintf("#%d", i+1)
			}
			return fmt.Errorf("custom rule %q has invalid regex: %v", label, err)
		}
	}
	return nil
}

// Template caps the JSON-marshalled size of a provisioner template at
// MaxTemplateBytes. The backup-import path uses this on every template
// in the incoming bundle.
func Template(template map[string]interface{}) error {
	body, err := json.Marshal(template)
	if err != nil {
		return err
	}
	if len(body) > MaxTemplateBytes {
		return fmt.Errorf("template exceeds %d bytes", MaxTemplateBytes)
	}
	return nil
}

// WebhookURL accepts only absolute http(s) URLs with a host — rejects
// file:// + relative paths + missing scheme so a typo in the settings UI
// surfaces immediately rather than at first delivery.
func WebhookURL(raw string) error {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid webhook url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("webhook url must be http:// or https://")
	}
	if u.Host == "" {
		return fmt.Errorf("webhook url must include a host")
	}
	return nil
}
