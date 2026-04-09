package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"os"
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
	db      *db.DB
	logf    func(level, msg string)
	dataDir string

	mu              sync.Mutex
	activeProvision map[string]bool
	activeFirmware  map[string]bool
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

type BackupExport struct {
	Version                int                      `json:"version"`
	Settings               models.AppSettings       `json:"settings"`
	Templates              map[string]string        `json:"templates"`
	CredentialGroups       []models.CredentialGroup `json:"credential_groups,omitempty"`
	DeviceGroupAssignments map[string]string        `json:"device_group_assignments,omitempty"`
}

type TemplateRecord struct {
	Name          string `json:"name"`
	Content       string `json:"content"`
	CredentialRef string `json:"credential_ref"`
}

type ImportReport struct {
	DryRun            bool     `json:"dry_run"`
	SettingsWillApply bool     `json:"settings_will_apply"`
	TemplatesCreate   []string `json:"templates_create"`
	TemplatesUpdate   []string `json:"templates_update"`
	GroupsCreate      []string `json:"groups_create"`
	GroupsUpdate      []string `json:"groups_update"`
	GroupsDelete      []string `json:"groups_delete"`
	AssignmentsCreate int      `json:"assignments_create"`
	AssignmentsUpdate int      `json:"assignments_update"`
	AssignmentsDelete int      `json:"assignments_delete"`
}

func NewAppService(database *db.DB, dataDir string, logf func(level, msg string)) *AppService {
	return &AppService{
		db:              database,
		dataDir:         dataDir,
		logf:            logf,
		activeProvision: map[string]bool{},
		activeFirmware:  map[string]bool{},
	}
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
	if limit > len(devices) {
		limit = len(devices)
	}
	if limit < 1 {
		limit = 1
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	refreshed := make([]models.Device, 0, len(devices))
	work := make(chan models.Device)
	_ = s.db.UpdateJobProgress(jobID, 0, len(devices), "")

	for i := 0; i < limit; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for device := range work {
				select {
				case <-ctx.Done():
					_ = s.db.IncrementJobDone(jobID)
					continue
				default:
				}
				if found := scanner.ProbeDevice(ctx, device.IP, timeout, s.Log); found != nil {
					mu.Lock()
					refreshed = append(refreshed, *found)
					mu.Unlock()
				}
				_ = s.db.IncrementJobDone(jobID)
			}
		}()
	}
	for _, device := range devices {
		work <- device
	}
	close(work)
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
		required, reason := checkAuthRequired(ctx, current.IP, timeout)
		if required {
			current.AuthRequired = true
			current.AuthError = reason
			current.Online = true
			current.ConsecutiveMisses = 0
		} else {
			current.ConsecutiveMisses++
			if current.ConsecutiveMisses >= 2 {
				current.Online = false
			}
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
	probed.AuthRequired = false
	probed.AuthError = ""
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
	ipToDevice := map[string]models.Device{}
	for _, device := range devices {
		ipToDevice[device.IP] = device
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
		_ = s.db.IncrementJobDone(jobID)
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
	if latest, err := s.db.GetLatestJob("firmware_check"); err == nil && latest.Status == "running" {
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
	requested := make([]string, 0, len(macs))
	for _, mac := range macs {
		if _, ok := index[mac]; ok {
			requested = append(requested, "mac:"+mac)
		}
	}
	allowed, skipped := s.reserveFirmwareTargets(requested)
	defer s.releaseFirmwareTargets(allowed)

	allowedSet := make(map[string]bool, len(allowed))
	for _, key := range allowed {
		allowedSet[key] = true
	}

	results := make([]firmware.UpdateResult, 0, len(macs)+len(skipped))
	for _, key := range skipped {
		mac := strings.TrimPrefix(key, "mac:")
		if device, ok := index[mac]; ok {
			results = append(results, firmware.UpdateResult{
				IP:     device.IP,
				Status: "skipped",
				Detail: "device busy with provisioning",
			})
		}
	}
	for _, mac := range macs {
		if device, ok := index[mac]; ok {
			if !allowedSet["mac:"+mac] {
				continue
			}
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

func (s *AppService) Provision(ctx context.Context, ips []string, template map[string]interface{}, credentialRef string) ([]map[string]any, error) {
	if len(ips) == 0 {
		return nil, errors.New("ips required")
	}
	if len(ips) > maxProvisionIPs {
		return nil, fmt.Errorf("too many devices requested")
	}
	if latest, err := s.db.GetLatestJob("scan"); err == nil && latest.Status == "running" {
		return nil, errors.New("provision blocked while scan is running")
	}
	for _, raw := range ips {
		addr, err := netip.ParseAddr(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("invalid ip: %q", raw)
		}
		if !isProvisionTargetAllowed(addr) {
			return nil, fmt.Errorf("provision target %q is not in an allowed local range", raw)
		}
	}
	if err := ValidateTemplate(template); err != nil {
		return nil, err
	}
	credentialRef = strings.TrimSpace(credentialRef)
	if credentialRef != "" {
		if _, err := s.db.GetCredential(credentialRef); err != nil {
			return nil, fmt.Errorf("credential_ref %q not found", credentialRef)
		}
	}

	devices, err := s.db.ListDevices()
	if err != nil {
		return nil, err
	}
	ipToDevice := map[string]models.Device{}
	ipToKey := map[string]string{}
	for _, device := range devices {
		ipToDevice[device.IP] = device
		key := "ip:" + device.IP
		if device.MAC != "" {
			key = "mac:" + device.MAC
		}
		ipToKey[device.IP] = key
	}
	genPolicy := deriveTemplateGenPolicy(template)
	requestedKeys := make([]string, 0, len(ips))
	keyToIP := map[string]string{}
	precheckSkipped := []map[string]any{}
	for _, ip := range ips {
		device, known := ipToDevice[ip]
		if known && device.AuthRequired && credentialRef == "" {
			precheckSkipped = append(precheckSkipped, map[string]any{
				"info": map[string]any{"ip": ip},
				"results": []map[string]any{
					{"section": "precheck", "status": "skipped", "detail": "auth required but credential_ref is missing"},
				},
			})
			continue
		}
		if known {
			if incompatible, detail := isGenIncompatible(device, genPolicy); incompatible {
				precheckSkipped = append(precheckSkipped, map[string]any{
					"info": map[string]any{"ip": ip},
					"results": []map[string]any{
						{"section": "precheck", "status": "skipped", "detail": detail},
					},
				})
				continue
			}
		}
		key := ipToKey[ip]
		if key == "" {
			key = "ip:" + ip
		}
		requestedKeys = append(requestedKeys, key)
		keyToIP[key] = ip
	}
	allowedKeys, skippedKeys := s.reserveProvisionTargets(requestedKeys)
	defer s.releaseProvisionTargets(allowedKeys)

	allowed := make([]string, 0, len(allowedKeys))
	for _, key := range allowedKeys {
		allowed = append(allowed, keyToIP[key])
	}

	out := make([]map[string]any, 0, len(ips))
	out = append(out, precheckSkipped...)
	for _, skipped := range skippedKeys {
		out = append(out, map[string]any{
			"info": map[string]any{
				"ip": keyToIP[skipped],
			},
			"results": []map[string]any{
				{"section": "precheck", "status": "skipped", "detail": "device busy with firmware update"},
			},
		})
	}
	for _, ip := range allowed {
		info, results := provisioner.ProvisionDevice(ctx, ip, template, 10*time.Second)
		authRequired := false
		authReason := ""
		for _, section := range results {
			if section.Status == "failed" && (strings.Contains(section.Detail, "401") || strings.Contains(section.Detail, "403")) {
				authRequired = true
				authReason = section.Detail
				break
			}
		}
		if authRequired {
			if device, ok := ipToDevice[ip]; ok {
				device.AuthRequired = true
				device.AuthError = authReason
				_ = s.db.UpsertDevice(device)
			}
		}
		body, _ := json.Marshal(map[string]any{"info": info, "results": results})
		var raw map[string]any
		_ = json.Unmarshal(body, &raw)
		out = append(out, raw)
	}
	return out, nil
}

func isProvisionTargetAllowed(addr netip.Addr) bool {
	// Block clearly unsafe destinations for server-side network calls.
	if addr.IsLoopback() || addr.IsMulticast() || addr.IsUnspecified() {
		return false
	}
	// Allow only local network targets (RFC1918/ULA and link-local).
	return addr.IsPrivate() || addr.IsLinkLocalUnicast()
}

func checkAuthRequired(ctx context.Context, ip string, timeout time.Duration) (bool, string) {
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+ip+"/shelly", nil)
	if err != nil {
		return false, ""
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, ""
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return true, resp.Status
	}
	return false, ""
}

type templateGenPolicy struct {
	hasGen1Only bool
	hasGen2Only bool
	hasDual     bool
}

func deriveTemplateGenPolicy(template map[string]interface{}) templateGenPolicy {
	policy := templateGenPolicy{}
	for section := range template {
		switch strings.ToLower(strings.TrimSpace(section)) {
		case "gen1_http":
			policy.hasGen1Only = true
		case "mqtt", "sys":
			policy.hasDual = true
		case "gen2_rpc", "ws", "ble", "matter", "cloud", "wifi", "kvs", "ota", "auth":
			policy.hasGen2Only = true
		default:
			// Unknown top-level sections map to <Section>.SetConfig and are gen2+ semantics.
			policy.hasGen2Only = true
		}
	}
	return policy
}

func isGenIncompatible(device models.Device, policy templateGenPolicy) (bool, string) {
	if device.Gen <= 1 {
		if policy.hasGen2Only && !policy.hasGen1Only && !policy.hasDual {
			return true, "template targets gen2+ sections while device is gen1"
		}
		return false, ""
	}
	if policy.hasGen1Only && !policy.hasGen2Only && !policy.hasDual {
		return true, "template targets gen1-only sections while device is gen2+"
	}
	return false, ""
}

func (s *AppService) reserveProvisionTargets(requested []string) (allowed []string, skipped []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, key := range requested {
		if key == "" {
			continue
		}
		if s.activeFirmware[key] {
			skipped = append(skipped, key)
			continue
		}
		s.activeProvision[key] = true
		allowed = append(allowed, key)
	}
	return allowed, skipped
}

func (s *AppService) releaseProvisionTargets(keys []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, key := range keys {
		delete(s.activeProvision, key)
	}
}

func (s *AppService) reserveFirmwareTargets(requested []string) (allowed []string, skipped []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, key := range requested {
		if key == "" {
			continue
		}
		if s.activeProvision[key] {
			skipped = append(skipped, key)
			continue
		}
		s.activeFirmware[key] = true
		allowed = append(allowed, key)
	}
	return allowed, skipped
}

func (s *AppService) releaseFirmwareTargets(keys []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, key := range keys {
		delete(s.activeFirmware, key)
	}
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

func (s *AppService) GetTemplate(name string) (TemplateRecord, error) {
	content, credentialRef, err := s.db.GetTemplate(name)
	if err != nil {
		return TemplateRecord{}, err
	}
	return TemplateRecord{
		Name:          name,
		Content:       content,
		CredentialRef: credentialRef,
	}, nil
}

func (s *AppService) SaveTemplate(name, content, credentialRef string) error {
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
	credentialRef = strings.TrimSpace(credentialRef)
	if credentialRef != "" {
		if _, err := s.db.GetCredential(credentialRef); err != nil {
			return fmt.Errorf("credential_ref %q not found", credentialRef)
		}
	}
	return s.db.SaveTemplate(name, content, credentialRef)
}

func (s *AppService) DeleteTemplate(name string) error {
	return s.db.DeleteTemplate(name)
}

func (s *AppService) ListCredentials() ([]models.Credential, error) {
	return s.db.ListCredentials()
}

func (s *AppService) SaveCredential(c models.Credential) error {
	c.Name = strings.TrimSpace(c.Name)
	c.Username = strings.TrimSpace(c.Username)
	if c.Name == "" {
		return errors.New("credential name is required")
	}
	if c.Username == "" {
		return errors.New("credential username is required")
	}
	if strings.TrimSpace(c.Password) == "" && strings.TrimSpace(c.HA1) == "" {
		return errors.New("credential requires password or ha1")
	}
	c.Tags = sanitizeTags(c.Tags)
	return s.db.SaveCredential(c)
}

func (s *AppService) DeleteCredential(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("credential name is required")
	}
	templates, err := s.db.ListTemplateNames()
	if err != nil {
		return err
	}
	for _, templateName := range templates {
		_, credentialRef, err := s.db.GetTemplate(templateName)
		if err != nil {
			continue
		}
		if credentialRef == name {
			return fmt.Errorf("credential is referenced by template %q", templateName)
		}
	}
	return s.db.DeleteCredential(name)
}

func (s *AppService) ListCredentialGroups() ([]models.CredentialGroup, error) {
	return s.db.ListCredentialGroups()
}

func (s *AppService) SaveCredentialGroup(group models.CredentialGroup) error {
	group.Name = strings.TrimSpace(group.Name)
	group.Username = strings.TrimSpace(group.Username)
	if group.Name == "" {
		return errors.New("group name is required")
	}
	if group.Username == "" {
		return errors.New("group username is required")
	}
	if strings.TrimSpace(group.Password) == "" && strings.TrimSpace(group.HA1) == "" {
		return errors.New("group requires password or ha1")
	}
	group.Tags = sanitizeTags(group.Tags)
	if err := s.db.SaveCredentialGroup(group); err != nil {
		return err
	}
	return s.db.SaveCredential(models.Credential{
		Name:     group.Name,
		Username: group.Username,
		Password: group.Password,
		HA1:      group.HA1,
		Tags:     group.Tags,
	})
}

func (s *AppService) DeleteCredentialGroup(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("group name is required")
	}
	if err := s.DeleteCredential(name); err != nil {
		return err
	}
	return s.db.DeleteCredentialGroup(name)
}

func (s *AppService) ListCredentialGroupAssignments() (map[string]string, error) {
	assignments, err := s.db.ListDeviceCredentialGroupAssignments()
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, assignment := range assignments {
		out[assignment.MAC] = assignment.GroupName
	}
	return out, nil
}

func (s *AppService) SaveCredentialGroupAssignments(macs []string, groupName string) error {
	groupName = strings.TrimSpace(groupName)
	cleaned := make([]string, 0, len(macs))
	seen := map[string]bool{}
	for _, mac := range macs {
		trimmed := strings.TrimSpace(mac)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		cleaned = append(cleaned, trimmed)
	}
	if len(cleaned) == 0 {
		return errors.New("macs required")
	}
	if groupName != "" {
		groups, err := s.db.ListCredentialGroups()
		if err != nil {
			return err
		}
		found := false
		for _, group := range groups {
			if group.Name == groupName {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("group %q not found", groupName)
		}
	}
	return s.db.SaveDeviceCredentialGroupAssignments(cleaned, groupName)
}

func (s *AppService) ExportBackup(includeSecrets bool) (BackupExport, error) {
	settings, err := s.db.GetSettings()
	if err != nil {
		return BackupExport{}, err
	}
	templates, err := s.db.ListTemplates()
	if err != nil {
		return BackupExport{}, err
	}
	groups, err := s.db.ListCredentialGroups()
	if err != nil {
		return BackupExport{}, err
	}
	assignmentsList, err := s.db.ListDeviceCredentialGroupAssignments()
	if err != nil {
		return BackupExport{}, err
	}
	assignments := map[string]string{}
	for _, assignment := range assignmentsList {
		assignments[assignment.MAC] = assignment.GroupName
	}
	out := map[string]string{}
	for name, content := range templates {
		if includeSecrets {
			out[name] = content
			continue
		}
		out[name] = redactTemplateSecrets(content)
	}
	s.Log("INFO", fmt.Sprintf("backup export requested include_secrets=%t templates=%d groups=%d assignments=%d", includeSecrets, len(out), len(groups), len(assignments)))
	return BackupExport{
		Version:                2,
		Settings:               settings,
		Templates:              out,
		CredentialGroups:       groups,
		DeviceGroupAssignments: assignments,
	}, nil
}

func (s *AppService) ImportBackup(data BackupExport, apply bool) (ImportReport, error) {
	if data.Version == 0 {
		return ImportReport{}, errors.New("backup payload missing version")
	}
	if err := ValidateSettings(data.Settings); err != nil {
		return ImportReport{}, fmt.Errorf("invalid settings: %w", err)
	}

	existingNames, err := s.db.ListTemplateNames()
	if err != nil {
		return ImportReport{}, err
	}
	existing := map[string]bool{}
	for _, name := range existingNames {
		existing[name] = true
	}

	report := ImportReport{
		DryRun:            !apply,
		SettingsWillApply: true,
		TemplatesCreate:   []string{},
		TemplatesUpdate:   []string{},
		GroupsCreate:      []string{},
		GroupsUpdate:      []string{},
		GroupsDelete:      []string{},
	}
	for name, content := range data.Templates {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			return ImportReport{}, errors.New("template name cannot be empty")
		}
		if len(content) > MaxTemplateBytes {
			return ImportReport{}, fmt.Errorf("template %q exceeds %d bytes", trimmed, MaxTemplateBytes)
		}
		var body map[string]interface{}
		if err := json.Unmarshal([]byte(content), &body); err != nil {
			return ImportReport{}, fmt.Errorf("template %q is invalid json", trimmed)
		}
		if err := ValidateTemplate(body); err != nil {
			return ImportReport{}, fmt.Errorf("template %q is invalid: %w", trimmed, err)
		}
		if existing[trimmed] {
			report.TemplatesUpdate = append(report.TemplatesUpdate, trimmed)
		} else {
			report.TemplatesCreate = append(report.TemplatesCreate, trimmed)
		}
	}

	existingGroupsList, err := s.db.ListCredentialGroups()
	if err != nil {
		return ImportReport{}, err
	}
	existingGroups := map[string]models.CredentialGroup{}
	for _, group := range existingGroupsList {
		existingGroups[group.Name] = group
	}
	incomingGroups := map[string]models.CredentialGroup{}
	for _, group := range data.CredentialGroups {
		name := strings.TrimSpace(group.Name)
		username := strings.TrimSpace(group.Username)
		password := strings.TrimSpace(group.Password)
		ha1 := strings.TrimSpace(group.HA1)
		if name == "" {
			return ImportReport{}, errors.New("group name cannot be empty")
		}
		if username == "" {
			return ImportReport{}, fmt.Errorf("group %q missing username", name)
		}
		if password == "" && ha1 == "" {
			return ImportReport{}, fmt.Errorf("group %q requires password or ha1", name)
		}
		if _, exists := incomingGroups[name]; exists {
			return ImportReport{}, fmt.Errorf("duplicate group %q in backup", name)
		}
		sanitized := models.CredentialGroup{
			Name:     name,
			Username: username,
			Password: password,
			HA1:      ha1,
			Tags:     sanitizeTags(group.Tags),
		}
		incomingGroups[name] = sanitized
		if currentGroup, exists := existingGroups[name]; !exists {
			report.GroupsCreate = append(report.GroupsCreate, name)
		} else if currentGroup.Username != sanitized.Username || currentGroup.Password != sanitized.Password || currentGroup.HA1 != sanitized.HA1 || strings.Join(currentGroup.Tags, "\x00") != strings.Join(sanitized.Tags, "\x00") {
			report.GroupsUpdate = append(report.GroupsUpdate, name)
		}
	}
	if data.Version >= 2 {
		for name := range existingGroups {
			if _, exists := incomingGroups[name]; !exists {
				report.GroupsDelete = append(report.GroupsDelete, name)
			}
		}
	}

	existingAssignmentsList, err := s.db.ListDeviceCredentialGroupAssignments()
	if err != nil {
		return ImportReport{}, err
	}
	existingAssignments := map[string]string{}
	for _, assignment := range existingAssignmentsList {
		existingAssignments[assignment.MAC] = assignment.GroupName
	}
	incomingAssignments := map[string]string{}
	if data.Version >= 2 {
		for mac, groupName := range data.DeviceGroupAssignments {
			trimmedMAC := strings.TrimSpace(mac)
			trimmedGroup := strings.TrimSpace(groupName)
			if trimmedMAC == "" || trimmedGroup == "" {
				continue
			}
			if _, exists := incomingGroups[trimmedGroup]; !exists {
				return ImportReport{}, fmt.Errorf("assignment for mac %q references unknown group %q", trimmedMAC, trimmedGroup)
			}
			incomingAssignments[trimmedMAC] = trimmedGroup
		}
	}
	for mac, newGroup := range incomingAssignments {
		if oldGroup, exists := existingAssignments[mac]; !exists {
			report.AssignmentsCreate++
		} else if oldGroup != newGroup {
			report.AssignmentsUpdate++
		}
	}
	if data.Version >= 2 {
		for mac := range existingAssignments {
			if _, exists := incomingAssignments[mac]; !exists {
				report.AssignmentsDelete++
			}
		}
	}

	if !apply {
		s.Log("INFO", fmt.Sprintf("backup import dry-run requested templates_create=%d templates_update=%d groups_create=%d groups_update=%d groups_delete=%d assignments_create=%d assignments_update=%d assignments_delete=%d",
			len(report.TemplatesCreate), len(report.TemplatesUpdate), len(report.GroupsCreate), len(report.GroupsUpdate), len(report.GroupsDelete),
			report.AssignmentsCreate, report.AssignmentsUpdate, report.AssignmentsDelete))
		return report, nil
	}

	if err := s.db.SaveSettings(data.Settings); err != nil {
		return ImportReport{}, err
	}
	for name, content := range data.Templates {
		if err := s.db.SaveTemplate(strings.TrimSpace(name), content, ""); err != nil {
			return ImportReport{}, err
		}
	}
	if data.Version >= 2 {
		for _, group := range data.CredentialGroups {
			sanitized := incomingGroups[strings.TrimSpace(group.Name)]
			if err := s.SaveCredentialGroup(sanitized); err != nil {
				return ImportReport{}, err
			}
		}
		for _, groupName := range report.GroupsDelete {
			if err := s.db.DeleteCredentialGroup(groupName); err != nil {
				return ImportReport{}, err
			}
		}
		if err := s.db.ReplaceDeviceCredentialGroupAssignments(incomingAssignments); err != nil {
			return ImportReport{}, err
		}
	}
	s.Log("INFO", fmt.Sprintf("backup import applied templates_create=%d templates_update=%d groups_create=%d groups_update=%d groups_delete=%d assignments_create=%d assignments_update=%d assignments_delete=%d",
		len(report.TemplatesCreate), len(report.TemplatesUpdate), len(report.GroupsCreate), len(report.GroupsUpdate), len(report.GroupsDelete),
		report.AssignmentsCreate, report.AssignmentsUpdate, report.AssignmentsDelete))
	return report, nil
}

func (s *AppService) GetLogs(level, search string) ([]db.LogEntry, error) {
	return s.db.GetLogs(level, search)
}

func (s *AppService) Log(level, msg string) {
	s.logf(level, SanitizeLogMessage(msg))
}

func redactTemplateSecrets(content string) string {
	var body map[string]any
	if err := json.Unmarshal([]byte(content), &body); err != nil {
		return content
	}
	redacted := redactSecretValue(body)
	encoded, err := json.MarshalIndent(redacted, "", "  ")
	if err != nil {
		return content
	}
	return string(encoded)
}

func redactSecretValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := map[string]any{}
		for key, child := range typed {
			lower := strings.ToLower(strings.TrimSpace(key))
			if looksSecretKey(lower) {
				out[key] = "[redacted]"
				continue
			}
			out[key] = redactSecretValue(child)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, child := range typed {
			out[i] = redactSecretValue(child)
		}
		return out
	default:
		return value
	}
}

func looksSecretKey(key string) bool {
	for _, token := range []string{"pass", "password", "secret", "ha1"} {
		if strings.Contains(key, token) {
			return true
		}
	}
	return false
}

func sanitizeTags(tags []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, tag := range tags {
		trimmed := strings.TrimSpace(tag)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		out = append(out, trimmed)
	}
	return out
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
