package db

import (
	"testing"
	"time"
)

// TestAuditChainIntactAfterWrites locks in the S2 contract: every audit
// row's prev_hash equals the SHA-256 chain link of the previous row.
// VerifyAuditChain returns 0 (no mismatch) on a freshly-written chain.
func TestAuditChainIntactAfterWrites(t *testing.T) {
	db := openTestDB(t)
	for i := 0; i < 5; i++ {
		if err := db.AddLogWithAttrs("INFO", "test entry", "req-1", ""); err != nil {
			t.Fatalf("AddLogWithAttrs[%d] error = %v", i, err)
		}
	}
	mismatchID, err := db.VerifyAuditChain()
	if err != nil {
		t.Fatalf("VerifyAuditChain error = %v", err)
	}
	if mismatchID != 0 {
		t.Fatalf("expected intact chain, got mismatch at id=%d", mismatchID)
	}
}

// TestAuditChainDetectsTampering proves the chain detects an attacker
// (or an unlucky operator) modifying a row's message after-the-fact.
// The first detected mismatch is the row whose prev_hash no longer
// matches the recomputed link of the tampered predecessor.
func TestAuditChainDetectsTampering(t *testing.T) {
	db := openTestDB(t)
	for i := 0; i < 5; i++ {
		if err := db.AddLogWithAttrs("INFO", "row content", "", ""); err != nil {
			t.Fatalf("AddLogWithAttrs[%d] error = %v", i, err)
		}
	}
	// Tamper with row id=2 message — the chain link computed for row 3
	// at write time used the ORIGINAL row-2 content. Row 3's stored
	// prev_hash should now disagree with the recomputed link.
	// The append-only trigger blocks DELETE but not UPDATE; UPDATE is
	// exactly the tampering vector we want this to catch.
	if _, err := db.sql.Exec(`UPDATE audit_log SET message='TAMPERED' WHERE id=2`); err != nil {
		t.Fatalf("tamper UPDATE error = %v", err)
	}
	mismatchID, err := db.VerifyAuditChain()
	if err != nil {
		t.Fatalf("VerifyAuditChain error = %v", err)
	}
	if mismatchID == 0 {
		t.Fatalf("expected chain mismatch after tampering, got intact chain")
	}
}

// TestAppendOnlyTriggerBlocksDirectDelete is the S2 trigger guarantee:
// `DELETE FROM audit_log WHERE id=1` is refused unless the retention
// bypass flag has been flipped. The retention pruner flips it inside
// a transaction; ad-hoc DELETE attempts (e.g. an operator-MCP tool that
// forgets to use the bypass) fail with the trigger's RAISE(ABORT).
func TestAppendOnlyTriggerBlocksDirectDelete(t *testing.T) {
	db := openTestDB(t)
	if err := db.AddLogWithAttrs("INFO", "to-delete", "", ""); err != nil {
		t.Fatalf("AddLogWithAttrs error = %v", err)
	}
	_, err := db.sql.Exec(`DELETE FROM audit_log WHERE id=1`)
	if err == nil {
		t.Fatalf("expected delete to be blocked by trigger, but it succeeded")
	}
}

// TestPruneAuditLogOlderThanRespectsBypass proves the retention pruner
// can delete rows even with the trigger active — it flips the bypass
// flag inside its transaction. Newer rows must survive.
func TestPruneAuditLogOlderThanRespectsBypass(t *testing.T) {
	db := openTestDB(t)
	if err := db.AddLogWithAttrs("INFO", "old", "", ""); err != nil {
		t.Fatalf("AddLogWithAttrs error = %v", err)
	}
	// Backdate the row so PruneAuditLogOlderThan picks it up.
	if _, err := db.sql.Exec(`UPDATE audit_log SET ts = ? WHERE id = 1`,
		time.Now().AddDate(-1, 0, 0).UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("backdate UPDATE error = %v", err)
	}
	if err := db.AddLogWithAttrs("INFO", "new", "", ""); err != nil {
		t.Fatalf("AddLogWithAttrs error = %v", err)
	}
	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	n, err := db.PruneAuditLogOlderThan(cutoff)
	if err != nil {
		t.Fatalf("PruneAuditLogOlderThan error = %v", err)
	}
	if n != 1 {
		t.Fatalf("pruned count = %d, want 1", n)
	}
	// New row must still be there.
	logs, err := db.GetLogs("", "")
	if err != nil {
		t.Fatalf("GetLogs error = %v", err)
	}
	if len(logs) != 1 || logs[0].Message != "new" {
		t.Fatalf("expected only 'new' to survive, got %d rows", len(logs))
	}
	// And the trigger must still block ad-hoc deletes afterward (bypass
	// was cleared at the end of the prune transaction).
	if _, err := db.sql.Exec(`DELETE FROM audit_log WHERE id=2`); err == nil {
		t.Fatalf("trigger still allowing deletes after retention prune — bypass flag leaked")
	}
}

// openTestDB is defined in db_test.go — these tests reuse it.
