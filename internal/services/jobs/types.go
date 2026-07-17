// Package jobs hosts the job-orchestration sub-service: type definitions,
// pure helpers, and (in a follow-up move) the long-running goroutines that
// drive refresh / scan / firmware-check / firmware-install.
//
// MOVED FROM internal/services/app_jobs.go — v0.3.0 services-layer split
// (M7, docs/plans/phase-4b-refactor-block.md Block 4b.1.4). This first move
// lifts the types + pure helpers verbatim; the *AppService method bodies
// follow in subsequent commits as the Deps interface stabilizes. Backward
// compatibility is preserved via type aliases in internal/services so
// internal/mcp and internal/api keep importing services.* unchanged.
package jobs

import (
	"encoding/json"
	"fmt"
	"time"

	"shellyadmin/internal/core/firmware"
	"shellyadmin/internal/models"
)

// Job-progress staleness thresholds: a "running" job whose UpdatedAt is
// older than this is considered crashed and gets marked "interrupted" on
// the next status read, so a hung worker can't block manual triggers
// forever. The two grace windows differ because refresh re-probes the
// entire fleet (slow) while scan's per-target tick is fast.
const (
	StaleScanGrace    = 15 * time.Second
	StaleRefreshGrace = 2 * time.Minute
)

// Firmware-install runtime knobs. Operators can override the timeout +
// poll-interval via AppSettings; concurrency is hard-coded because the
// upstream Shelly cloud rate-limits firmware fetches around 5 parallel
// installs.
const (
	DefaultFirmwareInstallTimeout      = 10 * time.Minute
	DefaultFirmwareInstallQuietPeriod  = 150 * time.Second
	DefaultFirmwareInstallPollInterval = 5 * time.Second
	FirmwareInstallConcurrency         = 5
)

// ScanStatus is the polling shape the SPA and the MCP scan_status tool see.
type ScanStatus struct {
	Running bool             `json:"running"`
	Found   int              `json:"found"`
	Total   int              `json:"total"`
	Done    int              `json:"done"`
	Pending []map[string]any `json:"pending"`
}

// FirmwareStatus is the polling shape for the periodic check job.
type FirmwareStatus struct {
	Running bool              `json:"running"`
	Done    int               `json:"done"`
	Total   int               `json:"total"`
	Results []firmware.Result `json:"results"`
}

// ScanJobPayload is what gets persisted into jobs.payload at spawn time.
type ScanJobPayload struct {
	ExistingMACs []string `json:"existing_macs"`
}

// ScanJobResult is the completion-time payload.
type ScanJobResult struct {
	Pending []models.Device `json:"pending"`
}

// FirmwareJobResult is the completion-time payload for the periodic check.
type FirmwareJobResult struct {
	Results []firmware.Result `json:"results"`
}

// FirmwareInstallResult is the per-device row tracked inside a firmware_install
// job. Status flows triggered → updating → current/error/unknown.
type FirmwareInstallResult struct {
	IP      string `json:"ip"`
	MAC     string `json:"mac"`
	Stage   string `json:"stage"`
	FromVer string `json:"from_ver"`
	ToVer   string `json:"to_ver"`
	Status  string `json:"status"`
	Detail  string `json:"detail"`
}

// FirmwareInstallJobPayload is what gets persisted into jobs.payload.
type FirmwareInstallJobPayload struct {
	MACs  []string `json:"macs"`
	Stage string   `json:"stage"`
}

// FirmwareInstallJobResult is the completion-time payload.
type FirmwareInstallJobResult struct {
	Results []FirmwareInstallResult `json:"results"`
}

// FirmwareInstallStatus is the polling shape for the install job.
type FirmwareInstallStatus struct {
	Running bool                    `json:"running"`
	Done    int                     `json:"done"`
	Total   int                     `json:"total"`
	Results []FirmwareInstallResult `json:"results"`
}

// RefreshProbeTimeout returns the per-device RPC budget the refresh job uses,
// normalized so a zero / malformed AppSettings still yields a sane timeout.
func RefreshProbeTimeout(settings models.AppSettings) time.Duration {
	settings.Normalize()
	return time.Duration(settings.RefreshTimeout * float64(time.Second))
}

// ScanJobStale reports whether a "running" scan job's UpdatedAt is older
// than the grace window, indicating a crashed worker.
func ScanJobStale(job models.Job, now time.Time) (bool, error) {
	updatedAt, err := time.Parse(time.RFC3339, job.UpdatedAt)
	if err != nil {
		return false, err
	}
	return now.Sub(updatedAt) > StaleScanGrace, nil
}

// RefreshJobStale reports the same for a refresh job (longer grace window
// because refresh re-probes every device in the fleet).
func RefreshJobStale(job models.Job, now time.Time) (bool, error) {
	updatedAt, err := time.Parse(time.RFC3339, job.UpdatedAt)
	if err != nil {
		return false, err
	}
	return now.Sub(updatedAt) > StaleRefreshGrace, nil
}

// FirmwareInstallTimeoutFromSettings returns the per-device install cap
// (10 min default if unset).
func FirmwareInstallTimeoutFromSettings(s models.AppSettings) time.Duration {
	if s.FirmwareInstallTimeout > 0 {
		return time.Duration(s.FirmwareInstallTimeout * float64(time.Second))
	}
	return DefaultFirmwareInstallTimeout
}

// FirmwareInstallQuietPeriodFromSettings returns how long installOne keeps its
// hands off a device after triggering the update (150s default if unset). Zero
// is NOT an opt-out: it means "unset" and takes the default, matching
// models.AppSettings.Normalize — a row written before the field existed carries
// 0 and must not silently disable the wait. See the field comment on
// models.AppSettings.FirmwareInstallQuietPeriod for the rationale.
func FirmwareInstallQuietPeriodFromSettings(s models.AppSettings) time.Duration {
	if s.FirmwareInstallQuietPeriod > 0 {
		return time.Duration(s.FirmwareInstallQuietPeriod * float64(time.Second))
	}
	return DefaultFirmwareInstallQuietPeriod
}

// FirmwareInstallPollIntervalFromSettings mirrors the timeout helper above.
// Bounds [1, 60] match models.AppSettings.Normalize so a freshly-loaded
// AppSettings round-trips identically; this clamp is defensive against
// settings rows that pre-date the field (where it lands as 0).
func FirmwareInstallPollIntervalFromSettings(s models.AppSettings) time.Duration {
	v := s.FirmwareInstallPollInterval
	if v <= 0 {
		return DefaultFirmwareInstallPollInterval
	}
	if v < 1 {
		v = 1
	} else if v > 60 {
		v = 60
	}
	return time.Duration(v * float64(time.Second))
}

// FirmwareSchedulerDecision is the per-tick logic of runFirmwareCheckScheduler.
// `now` is wall-clock time; `intervalSec` is the configured cadence (0 means
// disabled); `nextRun` is the previously-scheduled fire time (zero value =
// "anchor on first non-zero interval seen"). Returns the new nextRun anchor
// and whether the caller should fire StartFirmwareCheck right now.
func FirmwareSchedulerDecision(now time.Time, intervalSec int, nextRun time.Time) (time.Time, bool) {
	if intervalSec <= 0 {
		return time.Time{}, false
	}
	if nextRun.IsZero() {
		return now.Add(time.Duration(intervalSec) * time.Second), false
	}
	if now.Before(nextRun) {
		return nextRun, false
	}
	return now.Add(time.Duration(intervalSec) * time.Second), true
}

// FormatTimeout renders an install timeout as a short human phrase
// ("5 min", "90 sec") for the install_job's per-device detail line.
func FormatTimeout(d time.Duration) string {
	if d >= time.Minute && d%time.Minute == 0 {
		return fmt.Sprintf("%d min", int(d/time.Minute))
	}
	if d >= time.Minute {
		return fmt.Sprintf("%.1f min", d.Minutes())
	}
	return fmt.Sprintf("%d sec", int(d.Seconds()))
}

// TargetVersion returns the firmware version the device is expected to
// reach for the given stage ("stable"|"beta"), or "" when no candidate is
// known.
func TargetVersion(d models.Device, stage string) string {
	if stage == "beta" {
		return d.FWAvailableBeta
	}
	return d.FWAvailableStable
}

// IsInstallTerminal reports whether the per-device install status is in a
// final state (no further polling needed).
func IsInstallTerminal(status string) bool {
	switch status {
	case "current", "error", "unknown", "skipped":
		return true
	}
	return false
}

// ParseScanPayload deserializes the persisted scan-job payload.
func ParseScanPayload(raw string) (ScanJobPayload, error) {
	if raw == "" {
		return ScanJobPayload{}, nil
	}
	var payload ScanJobPayload
	err := json.Unmarshal([]byte(raw), &payload)
	return payload, err
}

// ParseScanResult deserializes the persisted scan-completion payload,
// defaulting to an empty (non-nil) Pending slice when raw=="".
func ParseScanResult(raw string) (ScanJobResult, error) {
	if raw == "" {
		return ScanJobResult{Pending: []models.Device{}}, nil
	}
	var result ScanJobResult
	err := json.Unmarshal([]byte(raw), &result)
	return result, err
}

// ParseFirmwareResult deserializes the persisted firmware-check payload,
// defaulting to an empty (non-nil) Results slice when raw=="".
func ParseFirmwareResult(raw string) (FirmwareJobResult, error) {
	if raw == "" {
		return FirmwareJobResult{Results: []firmware.Result{}}, nil
	}
	var result FirmwareJobResult
	err := json.Unmarshal([]byte(raw), &result)
	return result, err
}
