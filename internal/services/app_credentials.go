package services

// Delegators to internal/services/credentials. The credential CRUD surface
// moved to its own sub-package in v0.3.0 (M7,
// docs/plans/phase-4b-refactor-block.md Block 4b.1); these AppService
// methods preserve the public surface (api/handler_credentials.go,
// api/handler_credential_groups.go) so call sites compile unchanged.

import (
	"shellyadmin/internal/models"
)

// ListCredentials returns every credential row.
func (s *AppService) ListCredentials() ([]models.Credential, error) {
	return s.creds.List()
}

// SaveCredential validates the input and persists it.
func (s *AppService) SaveCredential(c models.Credential) error {
	return s.creds.Save(c)
}

// DeleteCredential refuses to remove a credential a template still references.
func (s *AppService) DeleteCredential(name string) error {
	return s.creds.Delete(name)
}

// ListCredentialGroups returns every credential-group row.
func (s *AppService) ListCredentialGroups() ([]models.CredentialGroup, error) {
	return s.creds.ListGroups()
}

// SaveCredentialGroup persists a credential group and mirrors it as a
// credential under the same name with username="admin".
func (s *AppService) SaveCredentialGroup(group models.CredentialGroup) error {
	return s.creds.SaveGroup(group)
}

// DeleteCredentialGroup removes both the credential mirror and the group row.
func (s *AppService) DeleteCredentialGroup(name string) error {
	return s.creds.DeleteGroup(name)
}

// ListCredentialGroupAssignments returns the flat {mac: groupName} mapping.
func (s *AppService) ListCredentialGroupAssignments() (map[string]string, error) {
	return s.creds.ListGroupAssignments()
}

// SaveCredentialGroupAssignments assigns groupName to every mac in the input.
func (s *AppService) SaveCredentialGroupAssignments(macs []string, groupName string) error {
	return s.creds.SaveGroupAssignments(macs, groupName)
}
