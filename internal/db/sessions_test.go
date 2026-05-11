package db

import (
	"database/sql"
	"errors"
	"testing"
	"time"
)

// TestSessionLifecycle covers the four S5 transitions: create → get
// (active) → revoke → get (still present but flagged). This is the
// happy-path contract that RequireAuth + Logout rely on.
func TestSessionLifecycle(t *testing.T) {
	db := openTestDB(t)
	id := "sess-abcdef"
	exp := time.Now().Add(7 * 24 * time.Hour).UTC().Format(time.RFC3339)
	if err := db.CreateSession(id, "admin", exp); err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	got, err := db.GetSession(id)
	if err != nil {
		t.Fatalf("GetSession error = %v", err)
	}
	if got.Username != "admin" || got.RevokedAt != "" {
		t.Fatalf("active session has wrong shape: %+v", got)
	}
	if err := db.RevokeSession(id); err != nil {
		t.Fatalf("RevokeSession error = %v", err)
	}
	got2, err := db.GetSession(id)
	if err != nil {
		t.Fatalf("GetSession after revoke error = %v", err)
	}
	if got2.RevokedAt == "" {
		t.Fatalf("revoked_at should be set after RevokeSession, got %+v", got2)
	}
}

// TestPruneExpiredSessions verifies the background sweeper removes
// only rows whose expires_at is in the past. An active session must
// survive even if it was created a long time ago.
func TestPruneExpiredSessions(t *testing.T) {
	db := openTestDB(t)
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	future := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	if err := db.CreateSession("expired-1", "admin", past); err != nil {
		t.Fatalf("CreateSession expired error = %v", err)
	}
	if err := db.CreateSession("active-1", "admin", future); err != nil {
		t.Fatalf("CreateSession active error = %v", err)
	}
	n, err := db.PruneExpiredSessions()
	if err != nil {
		t.Fatalf("PruneExpiredSessions error = %v", err)
	}
	if n != 1 {
		t.Fatalf("pruned count = %d, want 1", n)
	}
	// Active session must still be retrievable.
	if _, err := db.GetSession("active-1"); err != nil {
		t.Fatalf("active session missing after prune: %v", err)
	}
	// Expired session must now return ErrNoRows.
	if _, err := db.GetSession("expired-1"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expired session still present: err=%v", err)
	}
}

// TestRevokeAllForUser is the password-rotation path: all of a user's
// active sessions are flipped to revoked, but sessions for OTHER
// users remain untouched.
func TestRevokeAllForUser(t *testing.T) {
	db := openTestDB(t)
	exp := time.Now().Add(7 * 24 * time.Hour).UTC().Format(time.RFC3339)
	_ = db.CreateSession("a-1", "admin", exp)
	_ = db.CreateSession("a-2", "admin", exp)
	_ = db.CreateSession("b-1", "alice", exp)

	if err := db.RevokeAllForUser("admin"); err != nil {
		t.Fatalf("RevokeAllForUser error = %v", err)
	}
	for _, id := range []string{"a-1", "a-2"} {
		row, _ := db.GetSession(id)
		if row.RevokedAt == "" {
			t.Errorf("expected %q to be revoked", id)
		}
	}
	row, _ := db.GetSession("b-1")
	if row.RevokedAt != "" {
		t.Errorf("alice's session unexpectedly revoked: %+v", row)
	}
}
