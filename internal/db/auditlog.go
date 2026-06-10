package db

// Audit-log persistence: hash-chained append-only writes (S2), filtered
// reads, retention pruning. MOVED FROM db.go — db-layer split by domain
// (post-v0.5.2 review item 6); bodies unchanged.

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"strings"
	"time"
)

type LogEntry struct {
	ID        int    `json:"id"`
	TS        string `json:"ts"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
	// RiskLevel is set on audit rows that record an action execution
	// (catalog risk: low/medium/high). Empty on every other audit row,
	// including HTTP request logs and job lifecycle events.
	RiskLevel string `json:"risk_level,omitempty"`
}

func (db *DB) AddLog(level, message string) error {
	return db.AddLogWithRequestID(level, message, "")
}

// AddLogWithRequestID persists an audit entry tagged with the originating
// HTTP request's correlation ID (empty for jobs triggered outside a request).
func (db *DB) AddLogWithRequestID(level, message, requestID string) error {
	return db.AddLogWithAttrs(level, message, requestID, "")
}

// AddLogWithAttrs is the full audit-write surface, accepting structured
// attributes the higher layers want preserved alongside the message body.
// `riskLevel` is empty for non-action rows; action-execution rows pass the
// catalog risk so a future compliance query can SELECT WHERE risk_level
// IN (...) without regex-parsing the message body.
//
// S2 — also writes a `prev_hash` chain link: SHA-256 hex of the previous
// row's serialised "ts|level|message|request_id|risk_level|prev_hash"
// form. Verifying the chain (services.VerifyAuditChain) walks rows in
// id order, recomputes the link, and reports any mismatch. A tamperer
// who deletes a row after-the-fact breaks the chain at the next link.
func (db *DB) AddLogWithAttrs(level, message, requestID, riskLevel string) error {
	tx, err := db.sql.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// Look up the most recent row's hash-chain anchor; empty when the
	// table is empty (chain bootstrap).
	var prevTS, prevLevel, prevMsg, prevReqID, prevRisk, prevHash string
	err = tx.QueryRow(
		`SELECT ts, level, message, request_id, risk_level, prev_hash
		 FROM audit_log ORDER BY id DESC LIMIT 1`,
	).Scan(&prevTS, &prevLevel, &prevMsg, &prevReqID, &prevRisk, &prevHash)
	chainAnchor := ""
	if err == nil {
		chainAnchor = chainLink(prevTS, prevLevel, prevMsg, prevReqID, prevRisk, prevHash)
	} else if err != sql.ErrNoRows {
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO audit_log(ts, level, message, request_id, risk_level, prev_hash) VALUES (?, ?, ?, ?, ?, ?)`,
		now(), level, message, requestID, riskLevel, chainAnchor,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// chainLink is the canonical serialisation of an audit row that feeds
// the SHA-256 chain hash. Pipe-separated fields; the field names are
// fixed by this definition — adding a column to audit_log without
// extending chainLink invalidates the chain.
func chainLink(ts, level, message, requestID, riskLevel, prevHash string) string {
	body := ts + "|" + level + "|" + message + "|" + requestID + "|" + riskLevel + "|" + prevHash
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

// VerifyAuditChain walks the audit_log table in id order and recomputes
// the chain. Returns the id of the first mismatching row, or 0 if the
// chain is intact end-to-end. Used by the operator-facing
// `shellyctl audit-verify` subcommand and by retention-test fixtures.
func (db *DB) VerifyAuditChain() (int64, error) {
	rows, err := db.sql.Query(
		`SELECT id, ts, level, message, request_id, risk_level, prev_hash
		 FROM audit_log ORDER BY id ASC`,
	)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	expectedPrev := ""
	for rows.Next() {
		var id int64
		var ts, level, msg, reqID, risk, prevHash string
		if err := rows.Scan(&id, &ts, &level, &msg, &reqID, &risk, &prevHash); err != nil {
			return 0, err
		}
		if prevHash != expectedPrev {
			return id, nil
		}
		expectedPrev = chainLink(ts, level, msg, reqID, risk, prevHash)
	}
	return 0, rows.Err()
}

// PruneAuditLogOlderThan deletes rows whose ts is strictly older than
// the cutoff. Uses a controlled bypass of the audit_log_no_delete
// trigger (via the __retention_bypass settings flag flipped inside a
// transaction). Returns the number of rows removed. S1 from the
// consolidated review — keeps the table from growing unboundedly on
// long-running operator deployments.
func (db *DB) PruneAuditLogOlderThan(cutoff time.Time) (int64, error) {
	tx, err := db.sql.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	// Flip the bypass flag — the trigger reads it from `settings`.
	if _, err := tx.Exec(
		`INSERT INTO settings(key, value) VALUES ('__retention_bypass', '1')
		 ON CONFLICT(key) DO UPDATE SET value='1'`,
	); err != nil {
		return 0, err
	}
	res, err := tx.Exec(`DELETE FROM audit_log WHERE ts < ?`, cutoff.UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	// Clear the bypass flag inside the same transaction so a crash
	// between flip and clear leaves the protection intact.
	if _, err := tx.Exec(
		`INSERT INTO settings(key, value) VALUES ('__retention_bypass', '0')
		 ON CONFLICT(key) DO UPDATE SET value='0'`,
	); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (db *DB) GetLogs(level, search string) ([]LogEntry, error) {
	return db.GetLogsFiltered(level, search, "")
}

// GetLogsFiltered extends GetLogs with the v0.1.10 risk_level column. Empty
// risk filters keep the prior behaviour. Recognised values: "low",
// "medium", "high"; anything else is ignored so a stale frontend bookmark
// with an unknown value doesn't break the query.
func (db *DB) GetLogsFiltered(level, search, risk string) ([]LogEntry, error) {
	query := `SELECT id, ts, level, message, request_id, risk_level FROM audit_log WHERE 1=1`
	args := []any{}
	if level != "" {
		query += ` AND level = ?`
		args = append(args, strings.ToUpper(level))
	}
	if search != "" {
		query += ` AND message LIKE ?`
		args = append(args, "%"+search+"%")
	}
	switch strings.ToLower(strings.TrimSpace(risk)) {
	case "low", "medium", "high":
		query += ` AND risk_level = ?`
		args = append(args, strings.ToLower(risk))
	}
	query += ` ORDER BY id DESC LIMIT 500`
	rows, err := db.sql.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LogEntry
	for rows.Next() {
		var entry LogEntry
		if err := rows.Scan(&entry.ID, &entry.TS, &entry.Level, &entry.Message, &entry.RequestID, &entry.RiskLevel); err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

func (db *DB) GetLogsForExport(level, search string, limit int) ([]LogEntry, error) {
	return db.GetLogsForExportFiltered(level, search, "", limit)
}

func (db *DB) GetLogsForExportFiltered(level, search, risk string, limit int) ([]LogEntry, error) {
	if limit <= 0 {
		limit = 100000
	}
	query := `SELECT id, ts, level, message, request_id, risk_level FROM audit_log WHERE 1=1`
	args := []any{}
	if level != "" {
		query += ` AND level = ?`
		args = append(args, strings.ToUpper(level))
	}
	if search != "" {
		query += ` AND message LIKE ?`
		args = append(args, "%"+search+"%")
	}
	switch strings.ToLower(strings.TrimSpace(risk)) {
	case "low", "medium", "high":
		query += ` AND risk_level = ?`
		args = append(args, strings.ToLower(risk))
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := db.sql.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LogEntry
	for rows.Next() {
		var entry LogEntry
		if err := rows.Scan(&entry.ID, &entry.TS, &entry.Level, &entry.Message, &entry.RequestID, &entry.RiskLevel); err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

// ClearLogs removes every audit_log row. Like PruneAuditLogOlderThan it
// must flip the __retention_bypass flag inside the transaction, otherwise
// the audit_log_no_delete trigger rejects the DELETE as append-only.
func (db *DB) ClearLogs() (int64, error) {
	tx, err := db.sql.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(
		`INSERT INTO settings(key, value) VALUES ('__retention_bypass', '1')
		 ON CONFLICT(key) DO UPDATE SET value='1'`,
	); err != nil {
		return 0, err
	}
	res, err := tx.Exec(`DELETE FROM audit_log`)
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(
		`INSERT INTO settings(key, value) VALUES ('__retention_bypass', '0')
		 ON CONFLICT(key) DO UPDATE SET value='0'`,
	); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	count, _ := res.RowsAffected()
	return count, nil
}
