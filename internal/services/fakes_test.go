package services

import (
	"errors"
	"testing"

	"shellyadmin/internal/db"
	"shellyadmin/internal/models"
)

// fakeStore is a minimal in-memory Store for tests that do not need SQLite.
// Only the methods actually exercised by the caller are populated; the rest
// return zero values or errUnimplemented so unexpected calls surface loudly.
type fakeStore struct {
	devices  map[string]models.Device
	settings models.AppSettings
}

func newFakeStore() *fakeStore {
	return &fakeStore{devices: map[string]models.Device{}}
}

var errUnimplemented = errors.New("fakeStore: method not implemented for this test")

// --- Devices & settings (implemented) ---

func (f *fakeStore) ListDevices() ([]models.Device, error) {
	out := make([]models.Device, 0, len(f.devices))
	for _, d := range f.devices {
		out = append(out, d)
	}
	return out, nil
}

func (f *fakeStore) GetSettings() (models.AppSettings, error) { return f.settings, nil }

func (f *fakeStore) UpsertDevice(device models.Device) error {
	f.devices[device.MAC] = device
	return nil
}

// --- Everything else (unused by the demo test) ---

func (f *fakeStore) MarkRunningJobsInterrupted() error   { return errUnimplemented }
func (f *fakeStore) UpsertDevices([]models.Device) error { return errUnimplemented }
func (f *fakeStore) ForgetDevice(string) error           { return errUnimplemented }
func (f *fakeStore) SaveSettings(models.AppSettings) error {
	return errUnimplemented
}
func (f *fakeStore) CreateJob(string, string, string, int) (int64, error) {
	return 0, errUnimplemented
}
func (f *fakeStore) UpdateJobProgress(int64, int, int, string) error { return errUnimplemented }
func (f *fakeStore) IncrementJobDone(int64) error                    { return errUnimplemented }
func (f *fakeStore) CompleteJob(int64, string, string, string, int, int) error {
	return errUnimplemented
}
func (f *fakeStore) InterruptJob(int64, string) error { return errUnimplemented }
func (f *fakeStore) GetLatestJob(string) (models.Job, error) {
	return models.Job{}, errUnimplemented
}
func (f *fakeStore) GetJob(int64) (models.Job, error) {
	return models.Job{}, errUnimplemented
}
func (f *fakeStore) ListInterruptedRestartableJobs() ([]models.Job, error) {
	return nil, errUnimplemented
}
func (f *fakeStore) ListTemplateNames() ([]string, error) { return nil, errUnimplemented }
func (f *fakeStore) ListTemplates() (map[string]string, error) {
	return nil, errUnimplemented
}
func (f *fakeStore) GetTemplate(string) (string, string, error) { return "", "", errUnimplemented }
func (f *fakeStore) SaveTemplate(string, string, string) error  { return errUnimplemented }
func (f *fakeStore) DeleteTemplate(string) error                { return errUnimplemented }
func (f *fakeStore) ListCredentials() ([]models.Credential, error) {
	return nil, errUnimplemented
}
func (f *fakeStore) GetCredential(string) (models.Credential, error) {
	return models.Credential{}, errUnimplemented
}
func (f *fakeStore) SaveCredential(models.Credential) error { return errUnimplemented }
func (f *fakeStore) DeleteCredential(string) error          { return errUnimplemented }
func (f *fakeStore) ListCredentialGroups() ([]models.CredentialGroup, error) {
	return nil, errUnimplemented
}
func (f *fakeStore) SaveCredentialGroup(models.CredentialGroup) error { return errUnimplemented }
func (f *fakeStore) DeleteCredentialGroup(string) error               { return errUnimplemented }
func (f *fakeStore) ListDeviceCredentialGroupAssignments() ([]models.DeviceCredentialGroupAssignment, error) {
	return nil, errUnimplemented
}
func (f *fakeStore) SaveDeviceCredentialGroupAssignments([]string, string) error {
	return errUnimplemented
}
func (f *fakeStore) ReplaceDeviceCredentialGroupAssignments(map[string]string) error {
	return errUnimplemented
}
func (f *fakeStore) GetLogs(string, string) ([]db.LogEntry, error) {
	return nil, errUnimplemented
}
func (f *fakeStore) GetLogsForExport(string, string, int) ([]db.LogEntry, error) {
	return nil, errUnimplemented
}
func (f *fakeStore) ClearLogs() (int64, error) { return 0, errUnimplemented }

// Compile-time guarantee: fakeStore implements Store.
var _ Store = (*fakeStore)(nil)

// TestPreviewBulkAction_UsesFakeStore proves AppService can run against an
// in-memory Store without touching SQLite. Keeps parity with the integration
// test in device_surface_test.go but via the interface seam.
func TestPreviewBulkAction_UsesFakeStore(t *testing.T) {
	fake := newFakeStore()
	_ = fake.UpsertDevice(models.Device{MAC: "AA:BB:CC:DD:EE:01", IP: "192.168.1.10", Name: "online", Online: true, Gen: 2})
	_ = fake.UpsertDevice(models.Device{MAC: "AA:BB:CC:DD:EE:02", IP: "192.168.1.11", Name: "offline", Online: false, Gen: 2})

	service := NewAppService(fake, t.TempDir(), func(string, string) {})
	preview, err := service.PreviewBulkAction(BulkActionRequest{
		Action: "reboot",
		MACs:   []string{"AA:BB:CC:DD:EE:01", "AA:BB:CC:DD:EE:02"},
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("PreviewBulkAction() error = %v", err)
	}
	if len(preview.Targets) != 2 {
		t.Fatalf("preview.Targets = %d, want 2", len(preview.Targets))
	}
	if !preview.Targets[0].Eligible {
		t.Errorf("first target should be eligible")
	}
	if preview.Targets[1].Eligible {
		t.Errorf("offline target should not be eligible")
	}
}
