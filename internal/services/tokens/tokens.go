// Package tokens implements Personal Access Tokens (T3 from the
// consolidated review, docs/plans/phase-4c-auth-strategics.md Block
// 4c.2). PATs are long-lived, revocable, scope-tagged bearer tokens
// for headless callers (Home Assistant, cron jobs, scripts) so /api/*
// mutations don't have to fake the cookie + CSRF dance.
//
// Token format on the wire: `pat_<id>_<random>` where <id> is 8 hex
// chars and <random> is 64 hex chars (32 bytes / 256 bits of CSPRNG
// output). Total 79 chars including prefix. The id is the row key for
// fast O(1) lookup; the random is verified against the stored hash.
//
// Hash choice: sha256 of the random component, NOT argon2id. The
// random has 256 bits of entropy — argon2id's slow-derivation
// property protects low-entropy secrets (passwords) against offline
// brute force, but offers no marginal value here. Trading 80ms per
// PAT-auth'd request for argon2id would be cargo cult security.
// Comparison runs through subtle.ConstantTimeCompare to defeat
// timing-side-channel oracles. Hash blob is versioned (`sha256:...`)
// so a future scheme bump can co-exist with already-issued tokens.
package tokens

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"shellyadmin/internal/db"
)

// Wire-format constants. The id length is short enough to comfortably
// fit in a URL or audit-log line, long enough to make accidental
// collisions vanishingly unlikely (with one operator and bounded PAT
// counts, 2^32 ids are far more than needed).
const (
	TokenPrefix = "pat_"
	IDHexLen    = 8  // 4 bytes → 8 hex chars
	IDBytes     = 4  // raw bytes before hex
	RandomBytes = 32 // 32 bytes → 64 hex chars, 256 bits
	RandomHex   = RandomBytes * 2

	// MaxNameLen caps the operator-supplied label. Keeps audit log lines
	// short + protects the Settings UI from absurdly long values without
	// having to escape them.
	MaxNameLen = 100

	// MaxExpiresInDays bounds the explicit expiry window. 5 years matches
	// the GitHub PAT ceiling; 0 means "no expiry" which is allowed but
	// surfaced as a warning in the SPA.
	MaxExpiresInDays = 365 * 5

	// HashTag is the leading marker on token_hash blobs so a future
	// hash-scheme migration (sha256 → blake3, etc.) can co-exist.
	HashTag = "sha256:"
)

// Scope identifiers. The catalog is intentionally narrow — adding a
// new scope is a deliberate operator-facing decision, not something
// that should happen ad hoc. `admin` is the wildcard.
const (
	ScopeAdmin         = "admin"
	ScopeDevicesRead   = "devices:read"
	ScopeDevicesWrite  = "devices:write"
	ScopeFirmwareRead  = "firmware:read"
	ScopeFirmwareWrite = "firmware:write"
	ScopeProvision     = "provision"
	ScopeSettingsRead  = "settings:read"
	ScopeSettingsWrite = "settings:write"
)

// AllScopes is the sorted list of every catalog scope. Used by the
// validation path + surfaced to the SPA so the create-token form can
// render the checkbox list without duplicating the catalog.
var AllScopes = []string{
	ScopeAdmin,
	ScopeDevicesRead,
	ScopeDevicesWrite,
	ScopeFirmwareRead,
	ScopeFirmwareWrite,
	ScopeProvision,
	ScopeSettingsRead,
	ScopeSettingsWrite,
}

// ErrInvalidToken is returned by Lookup when the bearer string is
// malformed, points at an unknown id, fails the hash compare, or hits
// a revoked / expired row. All four cases collapse to the same error
// so the response shape doesn't tell an attacker whether a token id
// "exists but is revoked" vs. "never existed".
var ErrInvalidToken = errors.New("tokens: invalid bearer token")

// ErrInvalidScope is returned by Create when the supplied scope list
// contains an unknown entry. The Settings UI should never produce
// this; the check is defense-in-depth.
var ErrInvalidScope = errors.New("tokens: invalid scope")

// ErrEmptyScopes is returned by Create when no scopes are supplied.
// A scope-less PAT would be useless (every protected route checks at
// least one scope) so we refuse to issue it.
var ErrEmptyScopes = errors.New("tokens: at least one scope required")

// ErrNotFound is returned by Revoke when the operator targets a row
// that doesn't exist or belongs to a different user.
var ErrNotFound = errors.New("tokens: not found")

// Store is the narrow persistence surface the service depends on.
// *db.DB satisfies it structurally — Service is constructed against
// the AppService-level Store so tests can substitute a fake without
// implementing the full services.Store interface.
type Store interface {
	CreatePAT(p db.PAT) error
	GetPAT(id string) (db.PAT, error)
	ListPATs(username string) ([]db.PAT, error)
	TouchPAT(id string) error
	RevokePAT(id string) error
}

// Service owns the PAT create/list/revoke/lookup surface.
type Service struct {
	store Store
	now   func() time.Time
}

// New constructs a Service backed by the given store.
func New(store Store) *Service {
	return &Service{store: store}
}

// SetClock overrides the wall-clock source for tests. Production never
// calls this; expiry checks use time.Now() through s.clock().
func (s *Service) SetClock(fn func() time.Time) {
	s.now = fn
}

func (s *Service) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now().UTC()
}

// CreateResult is what Create returns. Token is shown to the operator
// exactly once — the SPA copies it to clipboard then drops the field
// from memory. ID + ExpiresAt are stored client-side for the list view.
type CreateResult struct {
	Token     string   `json:"token"`
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Scopes    []string `json:"scopes"`
	CreatedAt string   `json:"created_at"`
	ExpiresAt string   `json:"expires_at,omitempty"`
}

// ListedPAT is the metadata-only shape returned by List. Notably no
// token_hash — the secret never crosses the API boundary after
// Create. Revoked + Expired are derived booleans so the SPA renders
// a clean status badge without re-implementing the comparison.
type ListedPAT struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Scopes     []string `json:"scopes"`
	CreatedAt  string   `json:"created_at"`
	LastUsedAt string   `json:"last_used_at,omitempty"`
	ExpiresAt  string   `json:"expires_at,omitempty"`
	RevokedAt  string   `json:"revoked_at,omitempty"`
	Revoked    bool     `json:"revoked"`
	Expired    bool     `json:"expired"`
}

// Create mints a new PAT for username. Scopes must be non-empty and
// every entry must be in AllScopes. ExpiresInDays = 0 means no expiry
// (the row's expires_at stays empty). Returns the plaintext token
// (operator copies it once) + the persisted metadata.
func (s *Service) Create(username, name string, scopes []string, expiresInDays int) (CreateResult, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return CreateResult{}, errors.New("tokens: name is required")
	}
	if len(name) > MaxNameLen {
		return CreateResult{}, fmt.Errorf("tokens: name exceeds %d chars", MaxNameLen)
	}
	scopes = dedupeAndSort(scopes)
	if len(scopes) == 0 {
		return CreateResult{}, ErrEmptyScopes
	}
	for _, sc := range scopes {
		if !IsValidScope(sc) {
			return CreateResult{}, fmt.Errorf("%w: %q", ErrInvalidScope, sc)
		}
	}
	if expiresInDays < 0 {
		return CreateResult{}, errors.New("tokens: expires_in_days cannot be negative")
	}
	if expiresInDays > MaxExpiresInDays {
		return CreateResult{}, fmt.Errorf("tokens: expires_in_days exceeds max %d", MaxExpiresInDays)
	}

	idBytes := make([]byte, IDBytes)
	if _, err := rand.Read(idBytes); err != nil {
		return CreateResult{}, fmt.Errorf("tokens: read random id: %w", err)
	}
	id := hex.EncodeToString(idBytes)

	randomBytes := make([]byte, RandomBytes)
	if _, err := rand.Read(randomBytes); err != nil {
		return CreateResult{}, fmt.Errorf("tokens: read random: %w", err)
	}
	randomHex := hex.EncodeToString(randomBytes)
	token := TokenPrefix + id + "_" + randomHex

	scopesJSON, err := json.Marshal(scopes)
	if err != nil {
		return CreateResult{}, fmt.Errorf("tokens: marshal scopes: %w", err)
	}

	now := s.clock().UTC()
	expiresAt := ""
	if expiresInDays > 0 {
		expiresAt = now.AddDate(0, 0, expiresInDays).Format(time.RFC3339)
	}

	row := db.PAT{
		ID:        id,
		Username:  username,
		Name:      name,
		TokenHash: hashRandom(randomHex),
		Scopes:    string(scopesJSON),
		CreatedAt: now.Format(time.RFC3339),
		ExpiresAt: expiresAt,
	}
	if err := s.store.CreatePAT(row); err != nil {
		return CreateResult{}, fmt.Errorf("tokens: persist: %w", err)
	}
	return CreateResult{
		Token:     token,
		ID:        id,
		Name:      name,
		Scopes:    scopes,
		CreatedAt: row.CreatedAt,
		ExpiresAt: expiresAt,
	}, nil
}

// List returns every PAT owned by username, including revoked + expired
// rows. The Settings UI uses the Revoked / Expired booleans to render
// status badges.
func (s *Service) List(username string) ([]ListedPAT, error) {
	rows, err := s.store.ListPATs(username)
	if err != nil {
		return nil, err
	}
	out := make([]ListedPAT, 0, len(rows))
	for _, r := range rows {
		scopes, _ := decodeScopes(r.Scopes)
		expired := isExpired(r.ExpiresAt, s.clock())
		out = append(out, ListedPAT{
			ID:         r.ID,
			Name:       r.Name,
			Scopes:     scopes,
			CreatedAt:  r.CreatedAt,
			LastUsedAt: r.LastUsedAt,
			ExpiresAt:  r.ExpiresAt,
			RevokedAt:  r.RevokedAt,
			Revoked:    r.RevokedAt != "",
			Expired:    expired,
		})
	}
	return out, nil
}

// Revoke marks the row revoked. The username argument scopes the
// revoke to tokens the caller owns — a stolen session cookie can't
// revoke a PAT belonging to a different operator (future-proofing for
// the multi-user model; in v0.3.0 there's only one operator anyway).
func (s *Service) Revoke(username, id string) error {
	row, err := s.store.GetPAT(id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if row.Username != username {
		return ErrNotFound
	}
	return s.store.RevokePAT(id)
}

// Lookup is the middleware-side path. Accepts the full bearer token
// (`pat_<id>_<random>`), verifies it, and returns the row + parsed
// scopes on success. All failure modes collapse to ErrInvalidToken so
// the response timing / shape doesn't differentiate "no such id" from
// "right id, wrong random" from "revoked" from "expired".
//
// On success, last_used_at is bumped (best-effort; a touch failure
// does NOT fail the auth check).
func (s *Service) Lookup(rawToken string) (db.PAT, []string, error) {
	id, random, ok := parseToken(rawToken)
	if !ok {
		return db.PAT{}, nil, ErrInvalidToken
	}
	row, err := s.store.GetPAT(id)
	if errors.Is(err, sql.ErrNoRows) {
		return db.PAT{}, nil, ErrInvalidToken
	}
	if err != nil {
		return db.PAT{}, nil, err
	}
	if !verifyRandom(random, row.TokenHash) {
		return db.PAT{}, nil, ErrInvalidToken
	}
	if row.RevokedAt != "" {
		return db.PAT{}, nil, ErrInvalidToken
	}
	if isExpired(row.ExpiresAt, s.clock()) {
		return db.PAT{}, nil, ErrInvalidToken
	}
	scopes, err := decodeScopes(row.Scopes)
	if err != nil {
		return db.PAT{}, nil, ErrInvalidToken
	}
	// Best-effort last-used bump. A failure here doesn't invalidate the
	// auth — the row is alive, the operator has the right secret, and
	// the next successful request will retry the bump anyway.
	_ = s.store.TouchPAT(id)
	return row, scopes, nil
}

// IsValidScope reports whether s is in the catalog. Exported so the
// handler validation can use it before reaching the service layer.
func IsValidScope(s string) bool {
	for _, sc := range AllScopes {
		if sc == s {
			return true
		}
	}
	return false
}

// HasScope reports whether `granted` (a PAT's scope list) satisfies
// `required` (the per-route scope demand). The `admin` scope is the
// wildcard — a PAT with `admin` passes every check.
func HasScope(granted []string, required string) bool {
	for _, sc := range granted {
		if sc == ScopeAdmin || sc == required {
			return true
		}
	}
	return false
}

// --- internals ---

// parseToken validates the wire-format shape and returns the id +
// random components on a clean parse. ok==false collapses every kind
// of malformed input to a single "no" so the caller can't tell which
// field failed.
func parseToken(raw string) (id, random string, ok bool) {
	if !strings.HasPrefix(raw, TokenPrefix) {
		return "", "", false
	}
	body := raw[len(TokenPrefix):]
	sep := strings.IndexByte(body, '_')
	if sep != IDHexLen {
		return "", "", false
	}
	id = body[:sep]
	random = body[sep+1:]
	if len(random) != RandomHex {
		return "", "", false
	}
	if !isLowerHex(id) || !isLowerHex(random) {
		return "", "", false
	}
	return id, random, true
}

func isLowerHex(s string) bool {
	for _, c := range s {
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') {
			continue
		}
		return false
	}
	return true
}

// hashRandom is the persistence side of the token_hash column. We
// sha256 the random component as-is (no salt — the random is already
// 256 bits of entropy, salting protects low-entropy inputs from
// rainbow tables and offers nothing here). The output is hex so the
// blob survives a SQLite TEXT column without base64 escaping.
func hashRandom(randomHex string) string {
	sum := sha256.Sum256([]byte(randomHex))
	return HashTag + hex.EncodeToString(sum[:])
}

// verifyRandom is the lookup-side counterpart. Constant-time compare
// defeats the textbook "fail on first differing byte" timing attack,
// even though the underlying 256-bit entropy already makes that
// attack infeasible in practice. Cost is negligible.
func verifyRandom(randomHex, stored string) bool {
	if !strings.HasPrefix(stored, HashTag) {
		return false
	}
	want, err := hex.DecodeString(stored[len(HashTag):])
	if err != nil {
		return false
	}
	got := sha256.Sum256([]byte(randomHex))
	return subtle.ConstantTimeCompare(got[:], want) == 1
}

func decodeScopes(s string) ([]string, error) {
	if s == "" {
		return nil, nil
	}
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, fmt.Errorf("tokens: decode scopes: %w", err)
	}
	return out, nil
}

func isExpired(expiresAt string, now time.Time) bool {
	if expiresAt == "" {
		return false
	}
	exp, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return false
	}
	return now.After(exp)
}

// dedupeAndSort normalises the scope list so two PATs with the same
// effective scope set serialise identically. Lower-cases entries on
// the way through; AllScopes is already lower-case.
func dedupeAndSort(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
