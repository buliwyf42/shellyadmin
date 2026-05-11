// Package settings owns the encrypted-at-rest MCP-token envelope handling
// + the validate-before-save pipeline + the "keep current token on
// round-trip" semantics that the API GET/SaveSettings handlers depend on.
//
// MOVED FROM internal/services/app.go — v0.3.0 services-layer split (M7,
// docs/plans/phase-4b-refactor-block.md Block 4b.1). AppService.GetSettings
// and AppService.SaveSettings remain as delegators so api/handler_settings.go
// and existing tests compile unchanged.
package settings

import (
	"fmt"

	"shellyadmin/internal/core/secretbox"
	"shellyadmin/internal/models"
	"shellyadmin/internal/services/validation"
)

// Store is the narrow persistence surface settings needs.
type Store interface {
	GetSettings() (models.AppSettings, error)
	SaveSettings(s models.AppSettings) error
}

// OnSavedFn is the post-persist callback the host wires in. AppService
// passes ReconcileMCPFromSettings here so a settings update with a new
// MCP token automatically rotates the live listener; tests pass nil.
type OnSavedFn func()

// Service hosts the encrypted Get + the validate-then-encrypt Save
// pipeline.
type Service struct {
	store   Store
	onSaved OnSavedFn
}

// New constructs a Service backed by the given store. onSaved may be nil.
func New(store Store, onSaved OnSavedFn) *Service {
	if onSaved == nil {
		onSaved = func() {}
	}
	return &Service{store: store, onSaved: onSaved}
}

// Get returns the decrypted settings — internal callers see the plaintext
// MCP token. The API GET handler is the boundary that re-redacts to
// validation.MCPTokenRedacted ("<set>") before returning to the SPA.
func (s *Service) Get() (models.AppSettings, error) {
	settings, err := s.store.GetSettings()
	if err != nil {
		return settings, err
	}
	if settings.MCPToken != "" && secretbox.IsBlob(settings.MCPToken) {
		plain, derr := secretbox.OpenString(settings.MCPToken)
		if derr != nil {
			return settings, fmt.Errorf("decrypt mcp token: %w", derr)
		}
		settings.MCPToken = plain
	}
	return settings, nil
}

// Save runs the placeholder-token round-trip resolution, validates the
// (post-resolution) row, re-encrypts the MCP token if one is present,
// persists, and finally fires the onSaved callback so the host can
// reconcile the live MCP listener.
func (s *Service) Save(settings models.AppSettings) error {
	// "<set>" is the placeholder GET returns when a token is configured —
	// when the SPA round-trips settings back unchanged we must NOT overwrite
	// the stored token with a literal "<set>". Resolve it back to whatever
	// is currently persisted.
	if settings.MCPToken == validation.MCPTokenRedacted {
		current, err := s.store.GetSettings()
		if err != nil {
			return fmt.Errorf("read existing settings: %w", err)
		}
		if current.MCPToken != "" && secretbox.IsBlob(current.MCPToken) {
			plain, derr := secretbox.OpenString(current.MCPToken)
			if derr != nil {
				return fmt.Errorf("decrypt existing mcp token: %w", derr)
			}
			settings.MCPToken = plain
		} else {
			settings.MCPToken = current.MCPToken
		}
	}
	if err := validation.Settings(settings); err != nil {
		return err
	}
	if settings.MCPToken != "" {
		sealed, err := secretbox.SealString(settings.MCPToken)
		if err != nil {
			return fmt.Errorf("encrypt mcp token: %w", err)
		}
		settings.MCPToken = sealed
	}
	if err := s.store.SaveSettings(settings); err != nil {
		return err
	}
	// Reconcile the live MCP listener to match the new settings. No-op
	// when env-locked or when SetMCPParams was never called (e.g. unit
	// tests that don't exercise MCP).
	s.onSaved()
	return nil
}
