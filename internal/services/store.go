package services

import (
	"time"

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

	// Login state (per-account lockout, Q20)
	GetLoginState(username string) (db.LoginState, error)
	SetLoginState(state db.LoginState) error

	// TOTP 2FA (T1, v0.3.0)
	GetTOTP(username string) (db.TOTPState, error)
	SetTOTP(state db.TOTPState) error
	DeleteTOTP(username string) error

	// Audit-log retention + chain verification (S1+S2)
	PruneAuditLogOlderThan(cutoff time.Time) (int64, error)
	VerifyAuditChain() (int64, error)

	// Auto-backup snapshot (S12)
	SnapshotTo(path string) error

	// Server-side session store (S5)
	CreateSession(id, username, expiresAt string) error
	GetSession(id string) (db.Session, error)
	TouchSession(id string) error
	RevokeSession(id string) error
	RevokeAllForUser(username string) error
	PruneExpiredSessions() (int64, error)
}

// Compile-time assertion that *db.DB satisfies Store. If a method is added or
// removed from *db.DB and the interface drifts, this line breaks the build.
var _ Store = (*db.DB)(nil)
