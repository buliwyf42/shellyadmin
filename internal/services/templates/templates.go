// Package templates owns operator-supplied provisioner-template storage.
// Templates are JSON bundles applied per-device by internal/core/provisioner;
// this package manages the persistent CRUD around them + size + format
// validation + the optional credential_ref link.
//
// MOVED FROM internal/services/app.go — v0.3.0 services-layer split (M7,
// docs/plans/phase-4b-refactor-block.md Block 4b.1). AppService keeps
// delegators on ListTemplates / GetTemplate / SaveTemplate / DeleteTemplate
// so api/handler_templates.go and tests compile unchanged.
package templates

import (
	"encoding/json"
	"fmt"
	"strings"

	"shellyadmin/internal/models"
	"shellyadmin/internal/services/validation"
)

// Record is the SPA-facing shape of a single template. AppService and
// internal/api/handler_templates.go reference the alias services.TemplateRecord;
// this is the canonical definition.
type Record struct {
	Name          string `json:"name"`
	Content       string `json:"content"`
	CredentialRef string `json:"credential_ref"`
}

// Store is the narrow persistence surface this sub-service needs.
type Store interface {
	ListTemplateNames() ([]string, error)
	GetTemplate(name string) (string, string, error)
	SaveTemplate(name, content, credentialRef string) error
	DeleteTemplate(name string) error
	GetCredential(name string) (models.Credential, error)
}

// Service hosts the CRUD.
type Service struct {
	store Store
}

// New constructs a Service backed by the given store.
func New(store Store) *Service { return &Service{store: store} }

// List returns the names of every persisted template.
func (s *Service) List() ([]string, error) {
	return s.store.ListTemplateNames()
}

// Get returns one template by name, plus its credential_ref link.
func (s *Service) Get(name string) (Record, error) {
	content, credentialRef, err := s.store.GetTemplate(name)
	if err != nil {
		return Record{}, err
	}
	return Record{
		Name:          name,
		Content:       content,
		CredentialRef: credentialRef,
	}, nil
}

// Save validates size + JSON shape + credential_ref existence, then
// persists. The size cap (validation.MaxTemplateBytes) bounds memory
// pressure; the JSON parse + Template validator catches typos before
// they reach the provisioner.
func (s *Service) Save(name, content, credentialRef string) error {
	if len(content) > validation.MaxTemplateBytes {
		return fmt.Errorf("template exceeds %d bytes", validation.MaxTemplateBytes)
	}
	var body map[string]interface{}
	if err := json.Unmarshal([]byte(content), &body); err != nil {
		return err
	}
	if err := validation.Template(body); err != nil {
		return err
	}
	credentialRef = strings.TrimSpace(credentialRef)
	if credentialRef != "" {
		if _, err := s.store.GetCredential(credentialRef); err != nil {
			return fmt.Errorf("credential_ref %q not found", credentialRef)
		}
	}
	return s.store.SaveTemplate(name, content, credentialRef)
}

// Delete removes a template by name.
func (s *Service) Delete(name string) error {
	return s.store.DeleteTemplate(name)
}
