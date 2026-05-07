package services

import (
	"shellyadmin/internal/db"
	"shellyadmin/internal/models"
)

// Store is the persistence surface AppService depends on. It enumerates every
// method AppService currently calls on *db.DB, which already satisfies this
// interface structurally — no adapter is needed at construction time.
//
// The interface exists so services-layer tests can substitute a fake without
// standing up a real SQLite database for every case. Handler-side interface
// extraction is out of scope for this file; see docs/roadmap.md.
type Store interface {
	// Lifecycle
	MarkRunningJobsInterrupted() error

	// Devices
	ListDevices() ([]models.Device, error)
	UpsertDevice(device models.Device) error
	UpsertDevices(scanned []models.Device) error
	ForgetDevice(target string) error

	// Settings
	GetSettings() (models.AppSettings, error)
	SaveSettings(settings models.AppSettings) error

	// Jobs
	CreateJob(jobType, restartPolicy, payload string, total int) (int64, error)
	UpdateJobProgress(id int64, done, total int, result string) error
	IncrementJobDone(id int64) error
	CompleteJob(id int64, status, result, errText string, done, total int) error
	InterruptJob(id int64, errText string) error
	GetLatestJob(jobType string) (models.Job, error)
	GetJob(id int64) (models.Job, error)
	ListInterruptedRestartableJobs() ([]models.Job, error)

	// Templates
	ListTemplateNames() ([]string, error)
	ListTemplates() (map[string]string, error)
	GetTemplate(name string) (string, string, error)
	SaveTemplate(name, content, credentialRef string) error
	DeleteTemplate(name string) error

	// Credentials
	ListCredentials() ([]models.Credential, error)
	GetCredential(name string) (models.Credential, error)
	SaveCredential(c models.Credential) error
	DeleteCredential(name string) error

	// Credential groups and device assignments
	ListCredentialGroups() ([]models.CredentialGroup, error)
	SaveCredentialGroup(group models.CredentialGroup) error
	DeleteCredentialGroup(name string) error
	ListDeviceCredentialGroupAssignments() ([]models.DeviceCredentialGroupAssignment, error)
	SaveDeviceCredentialGroupAssignments(macs []string, groupName string) error
	ReplaceDeviceCredentialGroupAssignments(assignments map[string]string) error

	// Logs
	GetLogs(level, search string) ([]db.LogEntry, error)
	GetLogsFiltered(level, search, risk string) ([]db.LogEntry, error)
	GetLogsForExport(level, search string, limit int) ([]db.LogEntry, error)
	GetLogsForExportFiltered(level, search, risk string, limit int) ([]db.LogEntry, error)
	ClearLogs() (int64, error)
}

// Compile-time assertion that *db.DB satisfies Store. If a method is added or
// removed from *db.DB and the interface drifts, this line breaks the build.
var _ Store = (*db.DB)(nil)
