// Package credentials owns credential + credential-group + per-device-assignment
// CRUD: list, save, delete with the usual server-side validation (non-empty
// fields, secret hygiene, cross-references to templates).
//
// MOVED FROM internal/services/app_credentials.go — v0.3.0 services-layer
// split (M7, docs/plans/phase-4b-refactor-block.md Block 4b.1). AppService
// delegates ListCredentials / SaveCredential / DeleteCredential and the
// group + assignment surface to this package; existing API handlers
// (api/handler_credentials.go etc.) compile unchanged.
package credentials

import (
	"errors"
	"fmt"
	"strings"

	"shellyadmin/internal/models"
)

// Store is the narrow persistence surface needed by the credentials
// sub-service. *db.DB satisfies it structurally; tests can substitute a fake.
type Store interface {
	ListCredentials() ([]models.Credential, error)
	SaveCredential(c models.Credential) error
	DeleteCredential(name string) error

	ListCredentialGroups() ([]models.CredentialGroup, error)
	SaveCredentialGroup(group models.CredentialGroup) error
	DeleteCredentialGroup(name string) error

	ListDeviceCredentialGroupAssignments() ([]models.DeviceCredentialGroupAssignment, error)
	SaveDeviceCredentialGroupAssignments(macs []string, groupName string) error

	// Template reads are needed only by Delete: it refuses to remove a
	// credential a template still references.
	ListTemplateNames() ([]string, error)
	GetTemplate(name string) (string, string, error)
}

// Service owns credential CRUD. Construct via New and let AppService delegate.
type Service struct {
	store Store
}

// New constructs a Service backed by the given store.
func New(store Store) *Service { return &Service{store: store} }

// List returns every credential row.
func (s *Service) List() ([]models.Credential, error) {
	return s.store.ListCredentials()
}

// Save validates required fields (name, username, password-or-ha1) and
// persists the credential.
func (s *Service) Save(c models.Credential) error {
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
	return s.store.SaveCredential(c)
}

// Delete refuses to remove a credential that is still referenced by a
// template.
func (s *Service) Delete(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("credential name is required")
	}
	templates, err := s.store.ListTemplateNames()
	if err != nil {
		return err
	}
	for _, templateName := range templates {
		_, credentialRef, err := s.store.GetTemplate(templateName)
		if err != nil {
			continue
		}
		if credentialRef == name {
			return fmt.Errorf("credential is referenced by template %q", templateName)
		}
	}
	return s.store.DeleteCredential(name)
}

// ListGroups returns every credential-group row.
func (s *Service) ListGroups() ([]models.CredentialGroup, error) {
	return s.store.ListCredentialGroups()
}

// SaveGroup validates and persists a credential group, then mirrors it into
// the credentials table under the same name with username="admin" (the
// device-side default) so existing credential-resolution logic keeps working
// without a group-aware code path.
func (s *Service) SaveGroup(group models.CredentialGroup) error {
	group.Name = strings.TrimSpace(group.Name)
	if group.Name == "" {
		return errors.New("group name is required")
	}
	if strings.TrimSpace(group.Password) == "" && strings.TrimSpace(group.HA1) == "" {
		return errors.New("group requires password or ha1")
	}
	group.Tags = sanitizeTags(group.Tags)
	if err := s.store.SaveCredentialGroup(group); err != nil {
		return err
	}
	return s.store.SaveCredential(models.Credential{
		Name:     group.Name,
		Username: "admin",
		Password: group.Password,
		HA1:      group.HA1,
		Tags:     group.Tags,
	})
}

// DeleteGroup removes both the credential mirror and the group row. The
// credential mirror's template-reference check runs first (via Delete) so
// referenced groups are refused with the same error shape as referenced
// credentials.
func (s *Service) DeleteGroup(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("group name is required")
	}
	if err := s.Delete(name); err != nil {
		return err
	}
	return s.store.DeleteCredentialGroup(name)
}

// ListGroupAssignments returns every device→group mapping as a flat
// {mac: groupName} map for the SPA.
func (s *Service) ListGroupAssignments() (map[string]string, error) {
	assignments, err := s.store.ListDeviceCredentialGroupAssignments()
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, assignment := range assignments {
		out[assignment.MAC] = assignment.GroupName
	}
	return out, nil
}

// SaveGroupAssignments assigns groupName to every mac in the input, after
// deduplication + group existence check. groupName=="" clears the
// assignment for the given macs.
func (s *Service) SaveGroupAssignments(macs []string, groupName string) error {
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
		groups, err := s.store.ListCredentialGroups()
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
	return s.store.SaveDeviceCredentialGroupAssignments(cleaned, groupName)
}

// sanitizeTags trims, deduplicates, and drops empty tag entries. Mirrored
// from internal/services/app.go because the credentials sub-package cannot
// import services (cycle); the helper is small and pure.
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
