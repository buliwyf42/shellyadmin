// Package backup handles operator-driven backup export + import for the
// SQLite-backed configuration (settings + templates + credential groups +
// per-device assignments). The bundle is JSON-only; binary blobs (device
// inventory) are deliberately excluded.
//
// MOVED FROM internal/services/app_backup.go — v0.3.0 services-layer split
// (M7, docs/plans/phase-4b-refactor-block.md Block 4b.1). AppService keeps
// delegators on ExportBackup / ImportBackup so the API handlers and tests
// don't need to change. Validation goes through internal/services/validation;
// the credential-group mirror goes through credentials.Service.SaveGroup so
// every Save runs the same admin-mirror logic the SPA depends on.
package backup

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"shellyadmin/internal/models"
	"shellyadmin/internal/services/validation"
)

// BackupExport is the wire shape of a saved bundle. Version is the only
// field handlers may bump; new fields should be optional+omitempty so an
// older importer continues to round-trip.
type BackupExport struct {
	Version                int                      `json:"version"`
	Settings               models.AppSettings       `json:"settings"`
	Templates              map[string]string        `json:"templates"`
	CredentialGroups       []models.CredentialGroup `json:"credential_groups,omitempty"`
	DeviceGroupAssignments map[string]string        `json:"device_group_assignments,omitempty"`
}

// ImportReport is the diff the API returns from ImportBackup. DryRun=true
// means apply=false in the request; nothing was persisted, only counted.
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

// Store is the narrow persistence surface backup needs. *db.DB satisfies
// it structurally.
type Store interface {
	GetSettings() (models.AppSettings, error)
	SaveSettings(s models.AppSettings) error

	ListTemplates() (map[string]string, error)
	ListTemplateNames() ([]string, error)
	SaveTemplate(name, content, credentialRef string) error

	ListCredentialGroups() ([]models.CredentialGroup, error)
	DeleteCredentialGroup(name string) error

	ListDeviceCredentialGroupAssignments() ([]models.DeviceCredentialGroupAssignment, error)
	ReplaceDeviceCredentialGroupAssignments(assignments map[string]string) error
}

// GroupSaver mirrors credentials.Service.SaveGroup. Backup uses this so the
// admin-mirror credential row is created the same way the SaveCredentialGroup
// API does (which is exactly what credentials.Service.SaveGroup encodes).
type GroupSaver interface {
	SaveGroup(g models.CredentialGroup) error
}

// Service hosts Export + Import.
type Service struct {
	store  Store
	groups GroupSaver
	log    func(level, msg string)
}

// New constructs a Service. log may be nil (a no-op default is used).
func New(store Store, groups GroupSaver, log func(level, msg string)) *Service {
	if log == nil {
		log = func(string, string) {}
	}
	return &Service{store: store, groups: groups, log: log}
}

// Export builds a bundle from the persisted settings + templates +
// credential groups + assignments. includeSecrets=false redacts password /
// secret / ha1 fields anywhere they appear inside template JSON.
func (s *Service) Export(includeSecrets bool) (BackupExport, error) {
	settings, err := s.store.GetSettings()
	if err != nil {
		return BackupExport{}, err
	}
	templates, err := s.store.ListTemplates()
	if err != nil {
		return BackupExport{}, err
	}
	groups, err := s.store.ListCredentialGroups()
	if err != nil {
		return BackupExport{}, err
	}
	assignmentsList, err := s.store.ListDeviceCredentialGroupAssignments()
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
	s.log("INFO", fmt.Sprintf("backup export requested include_secrets=%t templates=%d groups=%d assignments=%d", includeSecrets, len(out), len(groups), len(assignments)))
	return BackupExport{
		Version:                3,
		Settings:               settings,
		Templates:              out,
		CredentialGroups:       groups,
		DeviceGroupAssignments: assignments,
	}, nil
}

// Import validates the bundle, then either reports the diff (apply=false)
// or applies it (apply=true). Settings always count as "will apply" because
// the import is meant to be a full restore.
func (s *Service) Import(data BackupExport, apply bool) (ImportReport, error) {
	if data.Version == 0 {
		return ImportReport{}, errors.New("backup payload missing version")
	}
	if err := validation.Settings(data.Settings); err != nil {
		return ImportReport{}, fmt.Errorf("invalid settings: %w", err)
	}

	existingNames, err := s.store.ListTemplateNames()
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
		if len(content) > validation.MaxTemplateBytes {
			return ImportReport{}, fmt.Errorf("template %q exceeds %d bytes", trimmed, validation.MaxTemplateBytes)
		}
		var body map[string]interface{}
		if err := json.Unmarshal([]byte(content), &body); err != nil {
			return ImportReport{}, fmt.Errorf("template %q is invalid json", trimmed)
		}
		if err := validation.Template(body); err != nil {
			return ImportReport{}, fmt.Errorf("template %q is invalid: %w", trimmed, err)
		}
		if existing[trimmed] {
			report.TemplatesUpdate = append(report.TemplatesUpdate, trimmed)
		} else {
			report.TemplatesCreate = append(report.TemplatesCreate, trimmed)
		}
	}

	existingGroupsList, err := s.store.ListCredentialGroups()
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
		password := strings.TrimSpace(group.Password)
		ha1 := strings.TrimSpace(group.HA1)
		if name == "" {
			return ImportReport{}, errors.New("group name cannot be empty")
		}
		if password == "" && ha1 == "" {
			return ImportReport{}, fmt.Errorf("group %q requires password or ha1", name)
		}
		if _, exists := incomingGroups[name]; exists {
			return ImportReport{}, fmt.Errorf("duplicate group %q in backup", name)
		}
		sanitized := models.CredentialGroup{
			Name:     name,
			Password: password,
			HA1:      ha1,
			Tags:     sanitizeTags(group.Tags),
		}
		incomingGroups[name] = sanitized
		if currentGroup, exists := existingGroups[name]; !exists {
			report.GroupsCreate = append(report.GroupsCreate, name)
		} else if currentGroup.Password != sanitized.Password || currentGroup.HA1 != sanitized.HA1 || strings.Join(currentGroup.Tags, "\x00") != strings.Join(sanitized.Tags, "\x00") {
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

	existingAssignmentsList, err := s.store.ListDeviceCredentialGroupAssignments()
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
		s.log("INFO", fmt.Sprintf("backup import dry-run requested templates_create=%d templates_update=%d groups_create=%d groups_update=%d groups_delete=%d assignments_create=%d assignments_update=%d assignments_delete=%d",
			len(report.TemplatesCreate), len(report.TemplatesUpdate), len(report.GroupsCreate), len(report.GroupsUpdate), len(report.GroupsDelete),
			report.AssignmentsCreate, report.AssignmentsUpdate, report.AssignmentsDelete))
		return report, nil
	}

	if err := s.store.SaveSettings(data.Settings); err != nil {
		return ImportReport{}, err
	}
	for name, content := range data.Templates {
		if err := s.store.SaveTemplate(strings.TrimSpace(name), content, ""); err != nil {
			return ImportReport{}, err
		}
	}
	if data.Version >= 2 {
		for _, group := range data.CredentialGroups {
			sanitized := incomingGroups[strings.TrimSpace(group.Name)]
			if err := s.groups.SaveGroup(sanitized); err != nil {
				return ImportReport{}, err
			}
		}
		for _, groupName := range report.GroupsDelete {
			if err := s.store.DeleteCredentialGroup(groupName); err != nil {
				return ImportReport{}, err
			}
		}
		if err := s.store.ReplaceDeviceCredentialGroupAssignments(incomingAssignments); err != nil {
			return ImportReport{}, err
		}
	}
	s.log("INFO", fmt.Sprintf("backup import applied templates_create=%d templates_update=%d groups_create=%d groups_update=%d groups_delete=%d assignments_create=%d assignments_update=%d assignments_delete=%d",
		len(report.TemplatesCreate), len(report.TemplatesUpdate), len(report.GroupsCreate), len(report.GroupsUpdate), len(report.GroupsDelete),
		report.AssignmentsCreate, report.AssignmentsUpdate, report.AssignmentsDelete))
	return report, nil
}

// --- Helpers ---

// sanitizeTags trims, deduplicates, and drops empty tag entries. Mirrored
// from internal/services/app.go (cycle prevents the import).
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
