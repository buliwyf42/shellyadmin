package services

// Delegators to internal/services/backup. The export/import surface moved
// to its own sub-package in v0.3.0 (M7,
// docs/plans/phase-4b-refactor-block.md Block 4b.1); these AppService
// methods preserve the public surface (api/handler_backup.go,
// services/provision_backup_test.go) so call sites compile unchanged.

import (
	"shellyadmin/internal/services/backup"
)

// Type aliases re-export the backup payload + report shapes so existing
// API handlers and tests reference services.BackupExport unchanged.
type (
	BackupExport = backup.BackupExport
	ImportReport = backup.ImportReport
)

// ExportBackup delegates to internal/services/backup.Service.Export.
func (s *AppService) ExportBackup(includeSecrets bool) (BackupExport, error) {
	return s.backup.Export(includeSecrets)
}

// ImportBackup delegates to internal/services/backup.Service.Import.
func (s *AppService) ImportBackup(data BackupExport, apply bool) (ImportReport, error) {
	return s.backup.Import(data, apply)
}
