package services

import (
	"context"
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"shellyadmin/internal/core/compliance"
	"shellyadmin/internal/core/firmware"
	"shellyadmin/internal/core/provisioner"
	"shellyadmin/internal/core/scanner"
	"shellyadmin/internal/core/setters"
	"shellyadmin/internal/db"
	"shellyadmin/internal/models"
)

const (
	MaxTemplateBytes = 64 * 1024
	MaxJSONBytes     = 256 * 1024
	maxProvisionIPs  = 256
	maxSubnets       = 64
	maxScanTargets   = 65534
)

type AppService struct {
	db   *db.DB
	logf func(level, msg string)
	dataDir string
}

type ScanStatus struct {
	Running bool             `json:"running"`
	Found   int              `json:"found"`
	Total   int              `json:"total"`
	Done    int              `json:"done"`
	Pending []map[string]any `json:"pending"`
}

type FirmwareStatus struct {
	Running bool              `json:"running"`
	Done    int               `json:"done"`
	Total   int               `json:"total"`
	Results []firmware.Result `json:"results"`
}

type ScanJobPayload struct {
	ExistingMACs []string `json:"existing_macs"`
}

type ScanJobResult struct {
	Pending []models.Device `json:"pending"`
}

type FirmwareJobResult struct {
	Results []firmware.Result `json:"results"`
}

func NewAppService(database *db.DB, dataDir string, logf func(level, msg string)) *AppService {
	return &AppService{db: database, dataDir: dataDir, logf: logf}
}

func (s *AppService) GetDevices() ([]models.Device, error) {
	devices, err := s.db.ListDevices()
	if err != nil {
		return nil, err
	}
	settings, err := s.db.GetSettings()
	if err != nil {
		return nil, err
	}
	for i := range devices {
		devices[i].Compliant, devices[i].ComplianceIssues = compliance.Evaluate(devices[i], settings.Compliance)
	}
	return devices, nil
}

func (s *AppService) RefreshDevices(ctx context.Context) ([]models.Device, error) {
	if latest, err := s.db.GetLatestJob("refresh"); err == nil && latest.Status == "running" {
		return nil, errors.New("refresh already running")
	}
	jobID, err := s.db.CreateJob("refresh", "auto", "{}", 0)
	if err != nil {
		return nil, err
	}
	done := make(chan error, 1)
	go s.runRefreshJob(ctx, jobID, done)
	if err := <-done; err != nil {
		return nil, err
	}
	return s.GetDevices()
}

func (s *AppService) runRefreshJob(ctx context.Context, jobID int64, done chan<- error) {
	devices, err := s.db.ListDevices()
	if err != nil {
		_ = s.db.CompleteJob(jobID, "failed", "", err.Error(), 0, 0)
		done <- err
		return
	}
	settings, err := s.db.GetSettings()
	if err != nil {
		_ = s.db.CompleteJob(jobID, "failed", "", err.Error(), 0, 0)
		done <- err
		return
	}
	settings.Normalize()
	timeout := time.Duration(settings.ScanTimeout * float64(time.Second))
	limit := boundedConcurrency(settings.ScanConcurrency)
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	var mu sync.Mutex
	refreshed := make([]models.Device, 0, len(devices))
	_ = s.db.UpdateJobProgress(jobID, 0, len(devices), "")
	for _, device := range devices {
		device := device
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()
			if found := scanner.ProbeDevice(ctx, device.IP, timeout, s.Log); found != nil {
				mu.Lock()
				refreshed = append(refreshed, *found)
				mu.Unlock()
			}
			job, err := s.db.GetJob(jobID)
			if err == nil {
				_ = s.db.UpdateJobProgress(jobID, job.Done+1, len(devices), job.Result)
			}
		}()
	}
	wg.Wait()
	if err := s.db.UpsertDevices(refreshed); err != nil {
		_ = s.db.CompleteJob(jobID, "failed", "", err.Error(), len(devices), len(devices))
		done <- err
		return
	}
	body, _ := json.Marshal(map[string]any{"refreshed": len(refreshed)})
	_ = s.db.CompleteJob(jobID, "completed", string(body), "", len(devices), len(devices))
	done <- nil
}

func (s *AppService) ForgetDevice(target string) error {
	return s.db.ForgetDevice(target)
}

func (s *AppService) RefreshDevice(ctx context.Context, target string) ([]models.Device, error) {
	devices, err := s.db.ListDevices()
	if err != nil {
		return nil, err
	}

	var current *models.Device
	for i := range devices {
		if devices[i].MAC == target || devices[i].IP == target {
			current = &devices[i]
			break
		}
	}
	if current == nil {
		return nil, fmt.Errorf("device not found")
	}

	settings, err := s.db.GetSettings()
	if err != nil {
		return nil, err
	}
	timeout := time.Duration(settings.ScanTimeout * float64(time.Second))
	probed := scanner.ProbeDevice(ctx, current.IP, timeout, s.Log)
	if probed == nil {
		current.ConsecutiveMisses++
		if current.ConsecutiveMisses >= 2 {
			current.Online = false
		}
		if err := s.db.UpsertDevice(*current); err != nil {
			return nil, err
		}
		return s.GetDevices()
	}

	probed.DeviceNum = current.DeviceNum
	probed.FirstSeen = current.FirstSeen
	probed.ConsecutiveMisses = 0
	probed.Online = true
	if err := s.db.UpsertDevice(*probed); err != nil {
		return nil, err
	}
	return s.GetDevices()
}

func (s *AppService) BulkAction(ctx context.Context, action string, macs []string, value string, lat, lon float64, enabled *bool) ([]map[string]string, error) {
	if len(macs) == 0 {
		return nil, errors.New("at least one device is required")
	}
	devices, err := s.db.ListDevices()
	if err != nil {
		return nil, err
	}
	settings, err := s.db.GetSettings()
	if err != nil {
		return nil, err
	}
	timeout := time.Duration(settings.ScanTimeout * float64(time.Second))
	index := map[string]models.Device{}
	for _, device := range devices {
		index[device.MAC] = device
	}
	results := make([]map[string]string, 0, len(macs))
	for _, mac := range macs {
		device, ok := index[mac]
		if !ok {
			results = append(results, map[string]string{"mac": mac, "status": "missing"})
			continue
		}
		success := false
		switch action {
		case "set_location":
			success = setters.SetLocation(ctx, device.IP, lat, lon, device.Gen, timeout)
		case "set_timezone":
			success = setters.SetTimezone(ctx, device.IP, value, device.Gen, timeout)
		case "set_mqtt_server":
			success = setters.SetMQTTServer(ctx, device.IP, value, device.Gen, timeout)
		case "set_mqtt_enabled":
			if enabled != nil {
				success = setters.SetMQTTEnabled(ctx, device.IP, *enabled, device.Gen, timeout)
			}
		case "set_24h":
			success = setters.SetTimeFormat24h(ctx, device.IP, device.Gen, timeout)
		default:
			return nil, fmt.Errorf("unsupported action: %s", action)
		}
		status := "failed"
		if success {
			status = "ok"
		}
		results = append(results, map[string]string{"mac": device.MAC, "ip": device.IP, "status": status})
	}
	return results, nil
}

func (s *AppService) StartScan() error {
	settings, err := s.db.GetSettings()
	if err != nil {
		return err
	}
	if err := ValidateSettings(settings); err != nil {
		return err
	}
	if latest, err := s.db.GetLatestJob("scan"); err == nil && latest.Status == "running" {
		return errors.New("scan already running")
	}
	devices, err := s.db.ListDevices()
	if err != nil {
		return err
	}
	existingMACs := make([]string, 0, len(devices))
	total := 0
	for _, device := range devices {
		existingMACs = append(existingMACs, device.MAC)
	}
	for _, subnet := range settings.Subnets {
		ips, err := scanner.ExpandCIDR(subnet)
		if err != nil {
			return err
		}
		total += len(ips)
	}
	if total > maxScanTargets {
		return fmt.Errorf("scan target count %d exceeds limit %d", total, maxScanTargets)
	}
	payload, _ := json.Marshal(ScanJobPayload{ExistingMACs: existingMACs})
	jobID, err := s.db.CreateJob("scan", "auto", string(payload), total)
	if err != nil {
		return err
	}
	go s.runScanJob(jobID, settings)
	return nil
}

func (s *AppService) runScanJob(jobID int64, settings models.AppSettings) {
	settings.Normalize()
	timeout := time.Duration(settings.ScanTimeout * float64(time.Second))
	results := scanner.ScanSubnets(context.Background(), settings.Subnets, boundedConcurrency(settings.ScanConcurrency), timeout, s.Log, func() {
		job, err := s.db.GetJob(jobID)
		if err == nil {
			_ = s.db.UpdateJobProgress(jobID, job.Done+1, job.Total, job.Result)
		}
	})
	body, _ := json.Marshal(ScanJobResult{Pending: results})
	job, err := s.db.GetJob(jobID)
	if err != nil {
		return
	}
	_ = s.db.CompleteJob(jobID, "completed", string(body), "", job.Total, job.Total)
}

func (s *AppService) ScanStatus() (ScanStatus, error) {
	job, err := s.db.GetLatestJob("scan")
	if err != nil {
		return ScanStatus{Pending: []map[string]any{}}, nil
	}
	payload, _ := ParseScanPayload(job.Payload)
	result, _ := ParseScanResult(job.Result)
	existing := map[string]bool{}
	for _, mac := range payload.ExistingMACs {
		existing[mac] = true
	}
	pending := make([]map[string]any, 0, len(result.Pending))
	for _, device := range result.Pending {
		body, _ := json.Marshal(device)
		var raw map[string]any
		_ = json.Unmarshal(body, &raw)
		raw["is_new"] = !existing[device.MAC]
		pending = append(pending, raw)
	}
	return ScanStatus{
		Running: job.Status == "running",
		Found:   len(result.Pending),
		Total:   job.Total,
		Done:    job.Done,
		Pending: pending,
	}, nil
}

func (s *AppService) ConfirmScan(macs []string) (int, error) {
	job, err := s.db.GetLatestJob("scan")
	if err != nil {
		return 0, errors.New("no scan job available")
	}
	result, _ := ParseScanResult(job.Result)
	selected := make([]models.Device, 0, len(result.Pending))
	remaining := make([]models.Device, 0, len(result.Pending))
	if len(macs) == 0 {
		selected = result.Pending
	} else {
		wanted := map[string]bool{}
		for _, mac := range macs {
			wanted[mac] = true
		}
		for _, device := range result.Pending {
			if wanted[device.MAC] {
				selected = append(selected, device)
			} else {
				remaining = append(remaining, device)
			}
		}
	}
	if err := s.db.UpsertDevices(selected); err != nil {
		return 0, err
	}
	if len(macs) == 0 {
		remaining = []models.Device{}
	}
	if body, err := json.Marshal(ScanJobResult{Pending: remaining}); err == nil {
		_ = s.db.UpdateJobProgress(job.ID, job.Done, job.Total, string(body))
	}
	return len(selected), nil
}

func (s *AppService) StartFirmwareCheck(stage string) (int, error) {
	if stage == "" {
		stage = "stable"
	}
	if latest, err := s.db.GetLatestJob("firmware"); err == nil && latest.Status == "running" {
		return latest.Total, errors.New("firmware check already running")
	}
	devices, err := s.db.ListDevices()
	if err != nil {
		return 0, err
	}
	payload, _ := json.Marshal(map[string]string{"stage": stage})
	jobID, err := s.db.CreateJob("firmware_check", "auto", string(payload), len(devices))
	if err != nil {
		return 0, err
	}
	go s.runFirmwareJob(jobID, devices, stage)
	return len(devices), nil
}

func (s *AppService) runFirmwareJob(jobID int64, devices []models.Device, stage string) {
	results := make([]firmware.Result, 0, len(devices))
	for _, device := range devices {
		result := firmware.CheckOne(context.Background(), device, stage, 5*time.Second)
		results = append(results, result)
		body, _ := json.Marshal(FirmwareJobResult{Results: results})
		_ = s.db.UpdateJobProgress(jobID, len(results), len(devices), string(body))
	}
	body, _ := json.Marshal(FirmwareJobResult{Results: results})
	_ = s.db.CompleteJob(jobID, "completed", string(body), "", len(results), len(devices))
}

func (s *AppService) FirmwareStatus() (FirmwareStatus, error) {
	job, err := s.db.GetLatestJob("firmware_check")
	if err != nil {
		return FirmwareStatus{Results: []firmware.Result{}}, nil
	}
	result, _ := ParseFirmwareResult(job.Result)
	return FirmwareStatus{
		Running: job.Status == "running",
		Done:    job.Done,
		Total:   job.Total,
		Results: result.Results,
	}, nil
}

func (s *AppService) FirmwareUpdate(ctx context.Context, macs []string, stage string) ([]firmware.UpdateResult, error) {
	if stage == "" {
		stage = "stable"
	}
	devices, err := s.db.ListDevices()
	if err != nil {
		return nil, err
	}
	index := map[string]models.Device{}
	for _, device := range devices {
		index[device.MAC] = device
	}
	results := make([]firmware.UpdateResult, 0, len(macs))
	for _, mac := range macs {
		if device, ok := index[mac]; ok {
			results = append(results, firmware.TriggerUpdate(ctx, device.IP, device.Gen, stage, 10*time.Second))
		}
	}
	return results, nil
}

func (s *AppService) RecoverInterruptedJobs() error {
	jobs, err := s.db.ListInterruptedRestartableJobs()
	if err != nil {
		return err
	}
	for _, job := range jobs {
		switch job.Type {
		case "scan":
			settings, err := s.db.GetSettings()
			if err != nil {
				continue
			}
			payload := job.Payload
			total := job.Total
			newJobID, err := s.db.CreateJob("scan", "auto", payload, total)
			if err != nil {
				continue
			}
			go s.runScanJob(newJobID, settings)
			s.Log("INFO", fmt.Sprintf("auto-restarted interrupted job scan:%d as job:%d", job.ID, newJobID))
		case "refresh":
			newJobID, err := s.db.CreateJob("refresh", "auto", "{}", 0)
			if err != nil {
				continue
			}
			go s.runRefreshJob(context.Background(), newJobID, make(chan error, 1))
			s.Log("INFO", fmt.Sprintf("auto-restarted interrupted job refresh:%d as job:%d", job.ID, newJobID))
		case "firmware_check":
			var stagePayload map[string]string
			_ = json.Unmarshal([]byte(job.Payload), &stagePayload)
			stage := stagePayload["stage"]
			devices, err := s.db.ListDevices()
			if err != nil {
				continue
			}
			newJobID, err := s.db.CreateJob("firmware_check", "auto", job.Payload, len(devices))
			if err != nil {
				continue
			}
			go s.runFirmwareJob(newJobID, devices, stage)
			s.Log("INFO", fmt.Sprintf("auto-restarted interrupted job firmware_check:%d as job:%d", job.ID, newJobID))
		}
	}
	return nil
}

func (s *AppService) Provision(ctx context.Context, ips []string, template map[string]interface{}) ([]map[string]any, error) {
	if len(ips) == 0 {
		return nil, errors.New("ips required")
	}
	if len(ips) > maxProvisionIPs {
		return nil, fmt.Errorf("too many devices requested")
	}
	if err := ValidateTemplate(template); err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(ips))
	for _, ip := range ips {
		info, results := provisioner.ProvisionDevice(ctx, ip, template, 10*time.Second)
		body, _ := json.Marshal(map[string]any{"info": info, "results": results})
		var raw map[string]any
		_ = json.Unmarshal(body, &raw)
		out = append(out, raw)
	}
	return out, nil
}

func (s *AppService) GetSettings() (models.AppSettings, error) {
	return s.db.GetSettings()
}

func (s *AppService) SaveSettings(settings models.AppSettings) error {
	if err := ValidateSettings(settings); err != nil {
		return err
	}
	return s.db.SaveSettings(settings)
}

func (s *AppService) ListTemplates() ([]string, error) {
	return s.db.ListTemplateNames()
}

func (s *AppService) GetTemplate(name string) (string, error) {
	return s.db.GetTemplate(name)
}

func (s *AppService) SaveTemplate(name, content string) error {
	if len(content) > MaxTemplateBytes {
		return fmt.Errorf("template exceeds %d bytes", MaxTemplateBytes)
	}
	var body map[string]interface{}
	if err := json.Unmarshal([]byte(content), &body); err != nil {
		return err
	}
	if err := ValidateTemplate(body); err != nil {
		return err
	}
	if HasRawAuthPass(body) {
		return errors.New("auth.pass must use ${ENV:VAR_NAME} instead of a raw password")
	}
	return s.db.SaveTemplate(name, content)
}

func (s *AppService) DeleteTemplate(name string) error {
	return s.db.DeleteTemplate(name)
}

func (s *AppService) GetLogs(level, search string) ([]db.LogEntry, error) {
	return s.db.GetLogs(level, search)
}

func (s *AppService) GetDebugLogs(search string, tail int) ([]string, error) {
	if tail <= 0 || tail > 500 {
		tail = 200
	}
	file, err := os.Open(filepath.Join(s.dataDir, "shellyctl.log"))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	lines, err := readTailLines(file, tail*4)
	if err != nil {
		return nil, err
	}
	filtered := make([]string, 0, tail)
	for _, line := range lines {
		if search == "" || strings.Contains(strings.ToLower(line), strings.ToLower(search)) {
			filtered = append(filtered, line)
		}
	}
	if len(filtered) > tail {
		filtered = filtered[len(filtered)-tail:]
	}
	return filtered, nil
}

func (s *AppService) Log(level, msg string) {
	s.logf(level, SanitizeLogMessage(msg))
}

func ValidateSettings(settings models.AppSettings) error {
	settings.Normalize()
	if len(settings.Subnets) > maxSubnets {
		return fmt.Errorf("too many subnets configured")
	}
	if settings.ScanConcurrency < 1 || settings.ScanConcurrency > 256 {
		return fmt.Errorf("scan concurrency must be between 1 and 256")
	}
	if settings.ScanTimeout < 0.2 || settings.ScanTimeout > 30 {
		return fmt.Errorf("scan timeout must be between 0.2 and 30 seconds")
	}
	total := 0
	for _, subnet := range settings.Subnets {
		ips, err := scanner.ExpandCIDR(subnet)
		if err != nil {
			return err
		}
		total += len(ips)
	}
	if total > maxScanTargets {
		return fmt.Errorf("scan target count %d exceeds limit %d", total, maxScanTargets)
	}
	return nil
}

func ValidateTemplate(template map[string]interface{}) error {
	body, err := json.Marshal(template)
	if err != nil {
		return err
	}
	if len(body) > MaxTemplateBytes {
		return fmt.Errorf("template exceeds %d bytes", MaxTemplateBytes)
	}
	return nil
}

func HasRawAuthPass(template map[string]interface{}) bool {
	authRaw, ok := template["auth"].(map[string]interface{})
	if !ok {
		return false
	}
	pass, ok := authRaw["pass"].(string)
	if !ok || pass == "" {
		return false
	}
	return !strings.HasPrefix(pass, "${ENV:")
}

var secretPattern = regexp.MustCompile(`(?i)(password|pass|secret|ha1)\s*[:=]\s*("[^"]*"|[^,\s]+)`)

func SanitizeLogMessage(msg string) string {
	return secretPattern.ReplaceAllString(msg, `$1=[redacted]`)
}

func ParseScanPayload(raw string) (ScanJobPayload, error) {
	if raw == "" {
		return ScanJobPayload{}, nil
	}
	var payload ScanJobPayload
	err := json.Unmarshal([]byte(raw), &payload)
	return payload, err
}

func ParseScanResult(raw string) (ScanJobResult, error) {
	if raw == "" {
		return ScanJobResult{Pending: []models.Device{}}, nil
	}
	var result ScanJobResult
	err := json.Unmarshal([]byte(raw), &result)
	return result, err
}

func ParseFirmwareResult(raw string) (FirmwareJobResult, error) {
	if raw == "" {
		return FirmwareJobResult{Results: []firmware.Result{}}, nil
	}
	var result FirmwareJobResult
	err := json.Unmarshal([]byte(raw), &result)
	return result, err
}

func BoundedConcurrency(value int) int {
	switch {
	case value <= 0:
		return 32
	case value > 128:
		return 128
	default:
		return value
	}
}

func boundedConcurrency(value int) int {
	return BoundedConcurrency(value)
}

func DecodeSecretValue(envKey string) string {
	if value := os.Getenv(envKey + "_FILE"); value != "" {
		body, err := os.ReadFile(value)
		if err == nil {
			return strings.TrimSpace(string(body))
		}
	}
	return strings.TrimSpace(os.Getenv(envKey))
}

func readTailLines(r io.Reader, limit int) ([]string, error) {
	scanner := bufio.NewScanner(r)
	lines := make([]string, 0, limit)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > limit {
			lines = lines[1:]
		}
	}
	return lines, scanner.Err()
}
