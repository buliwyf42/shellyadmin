package sessions

import (
	"database/sql"
	"testing"
	"time"

	"shellyadmin/internal/db"
)

// fakeStore is the per-sub-package fake the M7 plan calls for: implements
// only the methods sessions.Store actually needs, not the full services.Store.
type fakeStore struct {
	rows     map[string]db.Session
	touchErr error
}

func newFakeStore() *fakeStore { return &fakeStore{rows: map[string]db.Session{}} }

func (f *fakeStore) CreateSession(id, username, expiresAt string) error {
	f.rows[id] = db.Session{ID: id, Username: username, ExpiresAt: expiresAt}
	return nil
}

func (f *fakeStore) GetSession(id string) (db.Session, error) {
	row, ok := f.rows[id]
	if !ok {
		return db.Session{}, sql.ErrNoRows
	}
	return row, nil
}

func (f *fakeStore) TouchSession(id string) error {
	if f.touchErr != nil {
		return f.touchErr
	}
	if _, ok := f.rows[id]; !ok {
		return sql.ErrNoRows
	}
	return nil
}

func (f *fakeStore) RevokeSession(id string) error {
	row, ok := f.rows[id]
	if !ok {
		return nil
	}
	row.RevokedAt = time.Now().UTC().Format(time.RFC3339)
	f.rows[id] = row
	return nil
}

func (f *fakeStore) RevokeAllForUser(username string) error {
	for id, row := range f.rows {
		if row.Username == username {
			row.RevokedAt = time.Now().UTC().Format(time.RFC3339)
			f.rows[id] = row
		}
	}
	return nil
}

func TestIssueAndValidate_HappyPath(t *testing.T) {
	store := newFakeStore()
	svc := New(store)

	expires, err := svc.Issue("sid-1", "alice")
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if expires == "" {
		t.Fatal("Issue() returned empty expires_at")
	}

	ok, err := svc.Validator().ValidateSession("sid-1")
	if err != nil {
		t.Fatalf("ValidateSession() error = %v", err)
	}
	if !ok {
		t.Fatal("ValidateSession() = false, want true for fresh session")
	}
}

func TestValidate_MissingRowIsQuietFalse(t *testing.T) {
	svc := New(newFakeStore())
	ok, err := svc.Validator().ValidateSession("not-there")
	if err != nil {
		t.Fatalf("ValidateSession() unexpected error = %v", err)
	}
	if ok {
		t.Fatal("ValidateSession() = true, want false for missing row")
	}
}

func TestValidate_RevokedIsFalse(t *testing.T) {
	store := newFakeStore()
	svc := New(store)
	if _, err := svc.Issue("sid-1", "alice"); err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if err := svc.Revoke("sid-1"); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	ok, err := svc.Validator().ValidateSession("sid-1")
	if err != nil {
		t.Fatalf("ValidateSession() error = %v", err)
	}
	if ok {
		t.Fatal("ValidateSession() = true, want false for revoked row")
	}
}

func TestValidate_ExpiredIsFalse(t *testing.T) {
	store := newFakeStore()
	// Hand-craft an expired row; Issue() always sets 7d in the future.
	store.rows["sid-old"] = db.Session{
		ID:        "sid-old",
		Username:  "alice",
		ExpiresAt: time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339),
	}
	svc := New(store)
	ok, err := svc.Validator().ValidateSession("sid-old")
	if err != nil {
		t.Fatalf("ValidateSession() error = %v", err)
	}
	if ok {
		t.Fatal("ValidateSession() = true, want false for expired row")
	}
}

func TestRevoke_EmptyIDIsNoop(t *testing.T) {
	svc := New(newFakeStore())
	if err := svc.Revoke(""); err != nil {
		t.Errorf("Revoke(\"\") error = %v, want nil", err)
	}
}

func TestRevokeForUser_AffectsAllSessions(t *testing.T) {
	store := newFakeStore()
	svc := New(store)
	_, _ = svc.Issue("sid-1", "alice")
	_, _ = svc.Issue("sid-2", "alice")
	_, _ = svc.Issue("sid-3", "bob")

	if err := svc.RevokeForUser("alice"); err != nil {
		t.Fatalf("RevokeForUser() error = %v", err)
	}

	for _, id := range []string{"sid-1", "sid-2"} {
		ok, _ := svc.Validator().ValidateSession(id)
		if ok {
			t.Errorf("ValidateSession(%q) = true after RevokeForUser(alice), want false", id)
		}
	}
	ok, _ := svc.Validator().ValidateSession("sid-3")
	if !ok {
		t.Error("ValidateSession(sid-3) = false, but bob was not revoked")
	}
}

func TestValidator_NilSafetyOnEmptyID(t *testing.T) {
	v := New(newFakeStore()).Validator()
	if ok, err := v.ValidateSession(""); ok || err != nil {
		t.Errorf("ValidateSession(\"\") = (%v, %v), want (false, nil)", ok, err)
	}
	if err := v.TouchSession(""); err != nil {
		t.Errorf("TouchSession(\"\") error = %v, want nil", err)
	}
}
