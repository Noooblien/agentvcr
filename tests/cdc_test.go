package tests

import (
	"strings"
	"testing"

	"github.com/Noooblien/agentvcr/pkg/cdc"
)

func TestCDC_TransactionLifecycleAndStatements(t *testing.T) {
	rec := cdc.NewCDCRecorder("test-session-cdc-01")

	tx := rec.BeginTransaction("tx-001", "postgres", "app_production", 1)
	if tx == nil || tx.Status != cdc.TxActive {
		t.Fatalf("expected active transaction, got %v", tx)
	}

	err := rec.RecordStatement("tx-001", "SELECT * FROM users WHERE active = true;")
	if err != nil {
		t.Fatalf("RecordStatement failed: %v", err)
	}

	err = rec.CommitTransaction("tx-001")
	if err != nil {
		t.Fatalf("CommitTransaction failed: %v", err)
	}

	if tx.Status != cdc.TxCommitted {
		t.Fatalf("expected committed status, got %s", tx.Status)
	}
	if len(tx.RawStatements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(tx.RawStatements))
	}
}

func TestCDC_DangerousStatementDetection(t *testing.T) {
	rec := cdc.NewCDCRecorder("test-session-safety")
	rec.BeginTransaction("tx-danger", "postgres", "app_production", 2)

	_ = rec.RecordStatement("tx-danger", "DELETE FROM accounts;")
	_ = rec.RecordStatement("tx-danger", "UPDATE credentials SET password_hash = 'reset';")
	_ = rec.RecordStatement("tx-danger", "DROP TABLE audit_logs;")

	tx := rec.BeginTransaction("tx-check", "postgres", "app_production", 3)
	_ = tx // keep reference

	summary := rec.FinalizeAudit(true)
	if summary.PassedAudit {
		t.Fatalf("expected audit to fail due to dangerous SQL queries")
	}
	if summary.DestructiveQueriesCount != 3 {
		t.Fatalf("expected 3 destructive queries detected, got %d", summary.DestructiveQueriesCount)
	}

	hasUnboundedDelete := false
	hasDropTable := false
	for _, v := range summary.SafetyViolations {
		if strings.Contains(v, "Unbounded DELETE") {
			hasUnboundedDelete = true
		}
		if strings.Contains(v, "Destructive DDL") {
			hasDropTable = true
		}
	}
	if !hasUnboundedDelete || !hasDropTable {
		t.Fatalf("expected unbounded delete and drop table violations, got %v", summary.SafetyViolations)
	}
}

func TestCDC_AutomaticCompensatingSQLSynthesis(t *testing.T) {
	rec := cdc.NewCDCRecorder("test-session-compensating")
	rec.BeginTransaction("tx-mutations", "postgres", "app_production", 1)

	// 1. INSERT compensation -> DELETE
	err := rec.RecordRowDelta("tx-mutations", &cdc.RowDelta{
		Table:           "customers",
		PrimaryKeyCol:   "id",
		PrimaryKeyValue: 101,
		Operation:       cdc.OpInsert,
		After:           map[string]interface{}{"id": 101, "name": "Alice"},
	})
	if err != nil {
		t.Fatalf("RecordRowDelta failed: %v", err)
	}

	// 2. DELETE compensation -> INSERT back
	err = rec.RecordRowDelta("tx-mutations", &cdc.RowDelta{
		Table:           "customers",
		PrimaryKeyCol:   "id",
		PrimaryKeyValue: 102,
		Operation:       cdc.OpDelete,
		Before:          map[string]interface{}{"id": 102, "name": "Bob O'Reilly", "active": true},
	})
	if err != nil {
		t.Fatalf("RecordRowDelta failed: %v", err)
	}

	// 3. UPDATE compensation -> UPDATE back to 'Before'
	err = rec.RecordRowDelta("tx-mutations", &cdc.RowDelta{
		Table:           "customers",
		PrimaryKeyCol:   "id",
		PrimaryKeyValue: 103,
		Operation:       cdc.OpUpdate,
		Before:          map[string]interface{}{"id": 103, "name": "Charlie", "tier": "gold"},
		After:           map[string]interface{}{"id": 103, "name": "Charlie", "tier": "platinum"},
	})
	if err != nil {
		t.Fatalf("RecordRowDelta failed: %v", err)
	}

	_ = rec.CommitTransaction("tx-mutations")

	summary := rec.FinalizeAudit(false)
	script := summary.CompensatingScript

	// Check 1: DELETE FROM customers WHERE id = 101;
	if !strings.Contains(script, "DELETE FROM customers WHERE id = 101;") {
		t.Fatalf("missing expected undo INSERT statement in script:\n%s", script)
	}

	// Check 2: INSERT INTO customers with escaped Bob O''Reilly
	if !strings.Contains(script, "Bob O''Reilly") || !strings.Contains(script, "INSERT INTO customers") {
		t.Fatalf("missing expected undo DELETE statement in script:\n%s", script)
	}

	// Check 3: UPDATE customers SET ... WHERE id = 103;
	if !strings.Contains(script, "UPDATE customers SET") || !strings.Contains(script, "WHERE id = 103;") {
		t.Fatalf("missing expected undo UPDATE statement in script:\n%s", script)
	}
}

func TestCDC_VectorDeltaTracking(t *testing.T) {
	rec := cdc.NewCDCRecorder("test-session-vector")
	rec.BeginTransaction("tx-vec-01", "chroma", "knowledge_base", 1)

	err := rec.RecordVectorDelta("tx-vec-01", &cdc.VectorDelta{
		Collection:     "doc_chunks",
		VectorID:       "vec-999",
		Dimensions:     4,
		Operation:      cdc.OpVectorUpsert,
		EmbeddingAfter: []float32{0.12, 0.45, -0.22, 0.98},
		MetadataAfter:  map[string]interface{}{"source": "api_spec.md"},
	})
	if err != nil {
		t.Fatalf("RecordVectorDelta failed: %v", err)
	}

	_ = rec.CommitTransaction("tx-vec-01")

	summary := rec.FinalizeAudit(false)
	if len(summary.Transactions[0].VectorDeltas) != 1 {
		t.Fatalf("expected 1 vector delta, got %d", len(summary.Transactions[0].VectorDeltas))
	}
	vec := summary.Transactions[0].VectorDeltas[0]
	if vec.Dimensions != 4 || vec.VectorID != "vec-999" {
		t.Fatalf("unexpected vector delta: %v", vec)
	}
}

func TestCDC_SynthesizeRollbackScriptFromStep(t *testing.T) {
	rec := cdc.NewCDCRecorder("test-session-rollback")

	// Step 1: Tx 1
	rec.BeginTransaction("tx-step1", "sqlite", "main.db", 1)
	_ = rec.RecordRowDelta("tx-step1", &cdc.RowDelta{
		Table:           "orders",
		PrimaryKeyCol:   "id",
		PrimaryKeyValue: 1,
		Operation:       cdc.OpInsert,
	})
	_ = rec.CommitTransaction("tx-step1")

	// Step 2: Tx 2
	rec.BeginTransaction("tx-step2", "sqlite", "main.db", 2)
	_ = rec.RecordRowDelta("tx-step2", &cdc.RowDelta{
		Table:           "orders",
		PrimaryKeyCol:   "id",
		PrimaryKeyValue: 2,
		Operation:       cdc.OpInsert,
	})
	_ = rec.CommitTransaction("tx-step2")

	// Step 3: Tx 3
	rec.BeginTransaction("tx-step3", "sqlite", "main.db", 3)
	_ = rec.RecordRowDelta("tx-step3", &cdc.RowDelta{
		Table:           "orders",
		PrimaryKeyCol:   "id",
		PrimaryKeyValue: 3,
		Operation:       cdc.OpInsert,
	})
	_ = rec.CommitTransaction("tx-step3")

	// Rollback only step >= 2 (Tx 3, then Tx 2)
	script, err := rec.SynthesizeRollbackScript(2)
	if err != nil {
		t.Fatalf("SynthesizeRollbackScript failed: %v", err)
	}

	if !strings.Contains(script, "DELETE FROM orders WHERE id = 3;") {
		t.Fatalf("expected rollback of step 3 order 3")
	}
	if !strings.Contains(script, "DELETE FROM orders WHERE id = 2;") {
		t.Fatalf("expected rollback of step 2 order 2")
	}
	if strings.Contains(script, "WHERE id = 1;") {
		t.Fatalf("step 1 should NOT be included in rollback script >= 2")
	}

	// Verify reverse order: order 3 should appear BEFORE order 2
	idx3 := strings.Index(script, "id = 3")
	idx2 := strings.Index(script, "id = 2")
	if idx3 > idx2 {
		t.Fatalf("expected reverse chronological rollback order (id=3 before id=2), got idx3=%d, idx2=%d", idx3, idx2)
	}
}

func TestCDC_ScorecardAndAudit(t *testing.T) {
	rec := cdc.NewCDCRecorder("test-session-scorecard")
	rec.BeginTransaction("tx-score", "duckdb", "analytics", 1)
	_ = rec.RecordStatement("tx-score", "INSERT INTO metrics VALUES ('cpu', 42);")
	_ = rec.RecordRowDelta("tx-score", &cdc.RowDelta{
		Table:           "metrics",
		PrimaryKeyCol:   "name",
		PrimaryKeyValue: "cpu",
		Operation:       cdc.OpInsert,
	})
	_ = rec.CommitTransaction("tx-score")

	summary := rec.FinalizeAudit(false)
	card := cdc.RenderCDCScorecard(summary)

	if !strings.Contains(card, "DATABASE TRANSACTION FLIGHT RECORDER & CDC ROLLBACK JOURNAL") {
		t.Fatalf("missing header in scorecard: %s", card)
	}
	if !strings.Contains(card, "PASS") {
		t.Fatalf("expected PASS in clean scorecard: %s", card)
	}
}
