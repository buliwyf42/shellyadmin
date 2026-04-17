package services

import (
	"context"
	"strings"
	"testing"
	"time"

	"shellyadmin/internal/models"
)

// --- Provision ---

func TestProvisionRejectsEmptyIPs(t *testing.T) {
	_, service := testService(t)
	_, err := service.Provision(context.Background(), nil, map[string]interface{}{}, "")
	if err == nil || !strings.Contains(err.Error(), "ips required") {
		t.Fatalf("expected 'ips required' error, got %v", err)
	}
}

func TestProvisionRejectsTooManyIPs(t *testing.T) {
	_, service := testService(t)
	ips := make([]string, maxProvisionIPs+1)
	for i := range ips {
		ips[i] = "192.168.1.1"
	}
	_, err := service.Provision(context.Background(), ips, map[string]interface{}{}, "")
	if err == nil || !strings.Contains(err.Error(), "too many devices") {
		t.Fatalf("expected 'too many devices' error, got %v", err)
	}
}

func TestProvisionRejectsInvalidIP(t *testing.T) {
	_, service := testService(t)
	_, err := service.Provision(context.Background(), []string{"not-an-ip"}, map[string]interface{}{}, "")
	if err == nil || !strings.Contains(err.Error(), "invalid ip") {
		t.Fatalf("expected 'invalid ip' error, got %v", err)
	}
}

func TestProvisionRejectsNonLocalIP(t *testing.T) {
	_, service := testService(t)
	_, err := service.Provision(context.Background(), []string{"8.8.8.8"}, map[string]interface{}{}, "")
	if err == nil || !strings.Contains(err.Error(), "not in an allowed local range") {
		t.Fatalf("expected non-local range error, got %v", err)
	}
}

func TestProvisionRejectsUnknownCredentialRef(t *testing.T) {
	_, service := testService(t)
	_, err := service.Provision(context.Background(), []string{"192.168.1.10"}, map[string]interface{}{}, "ghost-cred")
	if err == nil || !strings.Contains(err.Error(), "credential_ref") {
		t.Fatalf("expected 'credential_ref' error, got %v", err)
	}
}

// TestProvisionSkipsAuthRequiredWithoutCredential exercises the happy pre-flight
// path without any network calls: a device in the DB has auth_required=true and
// no credential_ref is supplied, so Provision returns a precheck-skipped result
// and never reaches provisioner.ProvisionDevice.
func TestProvisionSkipsAuthRequiredWithoutCredential(t *testing.T) {
	database, service := testService(t)

	if err := database.UpsertDevice(models.Device{
		MAC:          "AA:BB:CC:DD:EE:FF",
		IP:           "192.168.1.50",
		Gen:          2,
		AuthRequired: true,
	}); err != nil {
		t.Fatalf("UpsertDevice() error = %v", err)
	}

	results, err := service.Provision(context.Background(), []string{"192.168.1.50"}, map[string]interface{}{}, "")
	if err != nil {
		t.Fatalf("Provision() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results len = %d, want 1", len(results))
	}
	resultList, ok := results[0]["results"].([]map[string]any)
	if !ok {
		t.Fatalf("results[0].results is not []map[string]any: %#v", results[0]["results"])
	}
	if len(resultList) != 1 || resultList[0]["status"] != "skipped" {
		t.Fatalf("expected single skipped result, got %#v", resultList)
	}
	if detail, _ := resultList[0]["detail"].(string); !strings.Contains(detail, "credential_ref is missing") {
		t.Fatalf("expected 'credential_ref is missing' detail, got %q", detail)
	}
}

// --- ImportBackup ---

func TestImportBackupRejectsMissingVersion(t *testing.T) {
	_, service := testService(t)
	_, err := service.ImportBackup(BackupExport{}, false)
	if err == nil || !strings.Contains(err.Error(), "missing version") {
		t.Fatalf("expected missing-version error, got %v", err)
	}
}

func TestImportBackupRejectsInvalidTemplate(t *testing.T) {
	_, service := testService(t)
	backup := BackupExport{
		Version:   2,
		Settings:  models.AppSettings{Subnets: []string{"192.168.1.0/30"}},
		Templates: map[string]string{"bad": "{this is not json"},
	}
	_, err := service.ImportBackup(backup, false)
	if err == nil || !strings.Contains(err.Error(), "invalid json") {
		t.Fatalf("expected 'invalid json' error, got %v", err)
	}
}

func TestImportBackupRejectsDuplicateGroup(t *testing.T) {
	_, service := testService(t)
	backup := BackupExport{
		Version:  2,
		Settings: models.AppSettings{Subnets: []string{"192.168.1.0/30"}},
		CredentialGroups: []models.CredentialGroup{
			{Name: "dupe", Password: "p1"},
			{Name: "dupe", Password: "p2"},
		},
	}
	_, err := service.ImportBackup(backup, false)
	if err == nil || !strings.Contains(err.Error(), "duplicate group") {
		t.Fatalf("expected 'duplicate group' error, got %v", err)
	}
}

func TestImportBackupDryRunDoesNotPersist(t *testing.T) {
	database, service := testService(t)
	backup := BackupExport{
		Version:  2,
		Settings: models.AppSettings{Subnets: []string{"192.168.1.0/30"}, ScanTimeout: 2, RefreshTimeout: 5, ScanConcurrency: 32},
		Templates: map[string]string{
			"factory-reset": `{"sys":{"device":{"name":"reset-me"}}}`,
		},
		CredentialGroups: []models.CredentialGroup{
			{Name: "site-a", Password: "secret"},
		},
	}
	report, err := service.ImportBackup(backup, false)
	if err != nil {
		t.Fatalf("ImportBackup(dry-run) error = %v", err)
	}
	if !report.DryRun {
		t.Fatalf("report.DryRun = false, want true")
	}
	if len(report.TemplatesCreate) != 1 || report.TemplatesCreate[0] != "factory-reset" {
		t.Fatalf("templates_create = %v, want [factory-reset]", report.TemplatesCreate)
	}
	if len(report.GroupsCreate) != 1 || report.GroupsCreate[0] != "site-a" {
		t.Fatalf("groups_create = %v, want [site-a]", report.GroupsCreate)
	}

	names, err := database.ListTemplateNames()
	if err != nil {
		t.Fatalf("ListTemplateNames() error = %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("dry-run persisted templates: %v", names)
	}
	groups, err := database.ListCredentialGroups()
	if err != nil {
		t.Fatalf("ListCredentialGroups() error = %v", err)
	}
	if len(groups) != 0 {
		t.Fatalf("dry-run persisted groups: %#v", groups)
	}
}

func TestImportBackupApplyWritesData(t *testing.T) {
	database, service := testService(t)
	backup := BackupExport{
		Version:  2,
		Settings: models.AppSettings{Subnets: []string{"192.168.1.0/30"}, ScanTimeout: 2, RefreshTimeout: 5, ScanConcurrency: 32},
		Templates: map[string]string{
			"mqtt-setup": `{"mqtt":{"enable":true}}`,
		},
		CredentialGroups: []models.CredentialGroup{
			{Name: "site-a", Password: "secret", Tags: []string{"demo"}},
		},
	}
	report, err := service.ImportBackup(backup, true)
	if err != nil {
		t.Fatalf("ImportBackup(apply) error = %v", err)
	}
	if report.DryRun {
		t.Fatalf("report.DryRun = true, want false")
	}
	names, err := database.ListTemplateNames()
	if err != nil {
		t.Fatalf("ListTemplateNames() error = %v", err)
	}
	if len(names) != 1 || names[0] != "mqtt-setup" {
		t.Fatalf("templates = %v, want [mqtt-setup]", names)
	}
	groups, err := database.ListCredentialGroups()
	if err != nil {
		t.Fatalf("ListCredentialGroups() error = %v", err)
	}
	if len(groups) != 1 || groups[0].Name != "site-a" {
		t.Fatalf("groups = %#v, want [site-a]", groups)
	}
}

// --- Graceful shutdown (item 5 coverage) ---

func TestAppServiceStopWithNoJobsReturnsPromptly(t *testing.T) {
	_, service := testService(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	start := time.Now()
	service.Stop(ctx)
	if elapsed := time.Since(start); elapsed > 1*time.Second {
		t.Fatalf("Stop() took %v, expected near-immediate return", elapsed)
	}
	if service.ctx.Err() == nil {
		t.Fatalf("service.ctx.Err() = nil, expected cancellation")
	}
}

func TestLinkedContextCancelsOnServiceStop(t *testing.T) {
	_, service := testService(t)
	parent := context.Background()
	derived, cancel := service.linkedContext(parent)
	defer cancel()

	if derived.Err() != nil {
		t.Fatalf("derived.Err() = %v before Stop, want nil", derived.Err())
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	service.Stop(shutdownCtx)

	select {
	case <-derived.Done():
	case <-time.After(1 * time.Second):
		t.Fatalf("derived context was not cancelled after service.Stop()")
	}
}

func TestLinkedContextCancelsOnParentCancel(t *testing.T) {
	_, service := testService(t)
	parent, parentCancel := context.WithCancel(context.Background())
	derived, cancel := service.linkedContext(parent)
	defer cancel()

	parentCancel()

	select {
	case <-derived.Done():
	case <-time.After(1 * time.Second):
		t.Fatalf("derived context was not cancelled when parent was cancelled")
	}
}
