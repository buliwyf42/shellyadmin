package db

// Personal Access Token rows (T3). MOVED FROM db.go — db-layer split by
// domain (post-v0.5.2 review item 6); bodies unchanged.

import "time"

// PAT is the Personal Access Token row (T3 from the consolidated
// review). TokenHash holds the argon2id PHC of the bearer string —
// plaintext never persists. Scopes is a JSON-encoded array of scope
// strings; the empty slice means "no scopes" which is rejected at
// creation time.
type PAT struct {
	ID         string
	Username   string
	Name       string
	TokenHash  string
	Scopes     string // JSON array
	CreatedAt  string
	LastUsedAt string
	ExpiresAt  string
	RevokedAt  string
}

// CreatePAT inserts a fresh row. The caller is responsible for
// generating id (8 hex chars, unique within the table) and tokenHash
// (argon2id PHC of the random component); this method is a thin
// persistence layer with no crypto knowledge of its own.
func (db *DB) CreatePAT(p PAT) error {
	if p.CreatedAt == "" {
		p.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := db.sql.Exec(
		`INSERT INTO personal_access_tokens(id, username, name, token_hash, scopes, created_at, last_used_at, expires_at, revoked_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Username, p.Name, p.TokenHash, p.Scopes, p.CreatedAt, p.LastUsedAt, p.ExpiresAt, p.RevokedAt,
	)
	return err
}

// GetPAT returns the row for id. Returns sql.ErrNoRows when the token
// does not exist — middleware treats that as "invalid bearer token,
// 401". Does NOT touch last_used_at; call TouchPAT for that on the
// successful-auth path.
func (db *DB) GetPAT(id string) (PAT, error) {
	var p PAT
	err := db.sql.QueryRow(
		`SELECT id, username, name, token_hash, scopes, created_at, last_used_at, expires_at, revoked_at
		 FROM personal_access_tokens WHERE id = ?`,
		id,
	).Scan(&p.ID, &p.Username, &p.Name, &p.TokenHash, &p.Scopes, &p.CreatedAt, &p.LastUsedAt, &p.ExpiresAt, &p.RevokedAt)
	return p, err
}

// ListPATs returns every PAT owned by username, including revoked +
// expired rows so the Settings UI can render the historical list with
// status badges. Sorted by created_at descending so the most recent
// token is at the top.
func (db *DB) ListPATs(username string) ([]PAT, error) {
	rows, err := db.sql.Query(
		`SELECT id, username, name, token_hash, scopes, created_at, last_used_at, expires_at, revoked_at
		 FROM personal_access_tokens WHERE username = ?
		 ORDER BY created_at DESC`,
		username,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PAT
	for rows.Next() {
		var p PAT
		if err := rows.Scan(&p.ID, &p.Username, &p.Name, &p.TokenHash, &p.Scopes, &p.CreatedAt, &p.LastUsedAt, &p.ExpiresAt, &p.RevokedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// TouchPAT bumps last_used_at on the row. Called by the auth
// middleware on every successful PAT-authenticated request so the
// operator can spot a forgotten-but-active token in the Settings UI.
// Best-effort: errors are returned but the caller may swallow them on
// the hot path.
func (db *DB) TouchPAT(id string) error {
	_, err := db.sql.Exec(
		`UPDATE personal_access_tokens SET last_used_at = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339), id,
	)
	return err
}

// RevokePAT marks the row as revoked. Idempotent — repeated calls do
// not flip the timestamp. Subsequent middleware lookups see the
// non-empty revoked_at and refuse the bearer token.
func (db *DB) RevokePAT(id string) error {
	_, err := db.sql.Exec(
		`UPDATE personal_access_tokens SET revoked_at = ? WHERE id = ? AND revoked_at = ''`,
		time.Now().UTC().Format(time.RFC3339), id,
	)
	return err
}
