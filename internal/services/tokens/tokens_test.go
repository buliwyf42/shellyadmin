package tokens

import (
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"shellyadmin/internal/db"
)

// fakeStore is an in-memory Store keyed on id. Mirrors db.PAT's row
// shape so the orchestrator's verification + expiry logic round-trips
// without a real SQLite handle.
type fakeStore struct {
	rows map[string]db.PAT
}

func newFakeStore() *fakeStore { return &fakeStore{rows: map[string]db.PAT{}} }

func (f *fakeStore) CreatePAT(p db.PAT) error {
	if _, exists := f.rows[p.ID]; exists {
		return errors.New("duplicate id")
	}
	f.rows[p.ID] = p
	return nil
}

func (f *fakeStore) GetPAT(id string) (db.PAT, error) {
	row, ok := f.rows[id]
	if !ok {
		return db.PAT{}, sql.ErrNoRows
	}
	return row, nil
}

func (f *fakeStore) ListPATs(username string) ([]db.PAT, error) {
	var out []db.PAT
	for _, r := range f.rows {
		if r.Username == username {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeStore) TouchPAT(id string) error {
	row, ok := f.rows[id]
	if !ok {
		return sql.ErrNoRows
	}
	row.LastUsedAt = time.Now().UTC().Format(time.RFC3339)
	f.rows[id] = row
	return nil
}

func (f *fakeStore) RevokePAT(id string) error {
	row, ok := f.rows[id]
	if !ok {
		return sql.ErrNoRows
	}
	if row.RevokedAt == "" {
		row.RevokedAt = time.Now().UTC().Format(time.RFC3339)
	}
	f.rows[id] = row
	return nil
}

// TestCreateProducesUsableToken locks in the round-trip contract:
// the plaintext from Create must verify via Lookup, and Lookup must
// return the same row + scopes the caller asked for.
func TestCreateProducesUsableToken(t *testing.T) {
	svc := New(newFakeStore())

	res, err := svc.Create("admin", "ha-bridge", []string{ScopeDevicesRead}, 0)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !strings.HasPrefix(res.Token, TokenPrefix) {
		t.Errorf("Token missing prefix: %q", res.Token)
	}
	wantLen := len(TokenPrefix) + IDHexLen + 1 + RandomHex
	if len(res.Token) != wantLen {
		t.Errorf("Token length = %d, want %d (%q)", len(res.Token), wantLen, res.Token)
	}
	if res.ID == "" || len(res.ID) != IDHexLen {
		t.Errorf("ID len = %d, want %d", len(res.ID), IDHexLen)
	}

	row, scopes, err := svc.Lookup(res.Token)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if row.ID != res.ID {
		t.Errorf("Lookup returned wrong id: got %q, want %q", row.ID, res.ID)
	}
	if len(scopes) != 1 || scopes[0] != ScopeDevicesRead {
		t.Errorf("scopes = %v, want [%q]", scopes, ScopeDevicesRead)
	}
}

func TestCreateRejectsEmptyScopes(t *testing.T) {
	svc := New(newFakeStore())
	_, err := svc.Create("admin", "no-scopes", nil, 0)
	if !errors.Is(err, ErrEmptyScopes) {
		t.Errorf("Create with no scopes: got %v, want ErrEmptyScopes", err)
	}
}

func TestCreateRejectsUnknownScope(t *testing.T) {
	svc := New(newFakeStore())
	_, err := svc.Create("admin", "bad", []string{"devices:read", "made:up"}, 0)
	if !errors.Is(err, ErrInvalidScope) {
		t.Errorf("Create with bad scope: got %v, want ErrInvalidScope", err)
	}
}

func TestCreateRejectsBlankName(t *testing.T) {
	svc := New(newFakeStore())
	_, err := svc.Create("admin", "   ", []string{ScopeAdmin}, 0)
	if err == nil {
		t.Errorf("Create with blank name: got nil error, want failure")
	}
}

func TestCreateScopesDeduplicated(t *testing.T) {
	store := newFakeStore()
	svc := New(store)
	res, err := svc.Create("admin", "dup", []string{ScopeDevicesRead, ScopeDevicesRead, ScopeAdmin}, 0)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(res.Scopes) != 2 {
		t.Errorf("Scopes = %v, want 2 entries (deduped)", res.Scopes)
	}
}

func TestLookupRejectsRevoked(t *testing.T) {
	store := newFakeStore()
	svc := New(store)
	res, _ := svc.Create("admin", "x", []string{ScopeAdmin}, 0)
	if err := svc.Revoke("admin", res.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	_, _, err := svc.Lookup(res.Token)
	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("Lookup of revoked: got %v, want ErrInvalidToken", err)
	}
}

func TestLookupRejectsExpired(t *testing.T) {
	store := newFakeStore()
	svc := New(store)
	fixed := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return fixed })
	res, _ := svc.Create("admin", "x", []string{ScopeAdmin}, 1)

	// Advance the clock past expiry.
	svc.SetClock(func() time.Time { return fixed.AddDate(0, 0, 2) })
	_, _, err := svc.Lookup(res.Token)
	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("Lookup of expired: got %v, want ErrInvalidToken", err)
	}
}

func TestLookupRejectsMalformedToken(t *testing.T) {
	svc := New(newFakeStore())
	cases := []string{
		"",
		"not-a-pat",
		"pat_",
		"pat_short_xxx",
		"pat_GGGGGGGG_" + strings.Repeat("a", RandomHex),                                     // bad hex in id
		TokenPrefix + strings.Repeat("a", IDHexLen) + "_" + strings.Repeat("z", RandomHex),   // bad hex in random
		TokenPrefix + strings.Repeat("a", IDHexLen) + "_" + strings.Repeat("a", RandomHex-1), // wrong length
	}
	for _, raw := range cases {
		_, _, err := svc.Lookup(raw)
		if !errors.Is(err, ErrInvalidToken) {
			t.Errorf("Lookup(%q): got %v, want ErrInvalidToken", raw, err)
		}
	}
}

func TestLookupRejectsWrongRandomSameID(t *testing.T) {
	store := newFakeStore()
	svc := New(store)
	res, _ := svc.Create("admin", "x", []string{ScopeAdmin}, 0)

	// Swap the random component for a different (still well-formed) one.
	body := strings.TrimPrefix(res.Token, TokenPrefix)
	sep := strings.IndexByte(body, '_')
	wrong := TokenPrefix + body[:sep] + "_" + strings.Repeat("0", RandomHex)
	_, _, err := svc.Lookup(wrong)
	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("Lookup with wrong random: got %v, want ErrInvalidToken", err)
	}
}

func TestListIncludesRevokedAndExpired(t *testing.T) {
	store := newFakeStore()
	svc := New(store)
	fixed := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return fixed })

	_, _ = svc.Create("admin", "active", []string{ScopeAdmin}, 30)
	revRes, _ := svc.Create("admin", "revoked", []string{ScopeAdmin}, 30)
	expRes, _ := svc.Create("admin", "expired", []string{ScopeAdmin}, 1)
	_ = svc.Revoke("admin", revRes.ID)

	svc.SetClock(func() time.Time { return fixed.AddDate(0, 0, 5) })
	list, err := svc.List("admin")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("List returned %d rows, want 3", len(list))
	}
	byName := map[string]ListedPAT{}
	for _, row := range list {
		byName[row.Name] = row
	}
	if !byName["revoked"].Revoked {
		t.Errorf("revoked row not flagged Revoked")
	}
	if !byName["expired"].Expired {
		t.Errorf("expired row not flagged Expired (now %v vs expires_at %v)", svc.clock(), byName["expired"].ExpiresAt)
	}
	if byName["active"].Revoked || byName["active"].Expired {
		t.Errorf("active row flagged Revoked/Expired: %+v", byName["active"])
	}
	// Ensure the lookup-side ID = expRes.ID (sanity)
	if _, ok := byName["expired"]; !ok {
		t.Errorf("expired row name missing: %v / id=%q", byName, expRes.ID)
	}
}

func TestRevokeOnlyByOwner(t *testing.T) {
	store := newFakeStore()
	svc := New(store)
	res, _ := svc.Create("admin", "x", []string{ScopeAdmin}, 0)
	err := svc.Revoke("other", res.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Revoke by non-owner: got %v, want ErrNotFound", err)
	}
}

func TestHasScopeAdminWildcard(t *testing.T) {
	if !HasScope([]string{ScopeAdmin}, ScopeDevicesWrite) {
		t.Errorf("admin scope failed to satisfy devices:write")
	}
	if HasScope([]string{ScopeDevicesRead}, ScopeDevicesWrite) {
		t.Errorf("devices:read incorrectly satisfied devices:write")
	}
	if !HasScope([]string{ScopeDevicesRead, ScopeFirmwareRead}, ScopeFirmwareRead) {
		t.Errorf("explicit scope match failed")
	}
}

func TestLookupTouchesLastUsedAt(t *testing.T) {
	store := newFakeStore()
	svc := New(store)
	res, _ := svc.Create("admin", "x", []string{ScopeAdmin}, 0)
	if store.rows[res.ID].LastUsedAt != "" {
		t.Errorf("LastUsedAt set before Lookup")
	}
	_, _, err := svc.Lookup(res.Token)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if store.rows[res.ID].LastUsedAt == "" {
		t.Errorf("LastUsedAt not bumped after Lookup")
	}
}
