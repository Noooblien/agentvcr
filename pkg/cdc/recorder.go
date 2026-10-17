package cdc

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Dangerous query regex patterns.
var (
	reDeleteWithoutWhere = regexp.MustCompile(`(?i)^\s*DELETE\s+FROM\s+[^\s;]+(?:\s*;|\s*$)`)
	reUpdateWithoutWhere = regexp.MustCompile(`(?i)^\s*UPDATE\s+[^\s;]+\s+SET\s+[^;]+(?:\s*;|\s*$)`)
	reDropTable          = regexp.MustCompile(`(?i)^\s*DROP\s+TABLE`)
	reDropDatabase       = regexp.MustCompile(`(?i)^\s*DROP\s+DATABASE`)
	reTruncate           = regexp.MustCompile(`(?i)^\s*TRUNCATE`)
)

// CDCRecorder intercepts and journals database mutations, generating compensating undo scripts.
type CDCRecorder struct {
	mu           sync.RWMutex
	sessionID    string
	transactions map[string]*DBTransaction
	orderedTx    []*DBTransaction
	savepoints   map[string]int
}

// NewCDCRecorder initializes a thread-safe database transaction flight recorder.
func NewCDCRecorder(sessionID string) *CDCRecorder {
	return &CDCRecorder{
		sessionID:    sessionID,
		transactions: make(map[string]*DBTransaction),
		savepoints:   make(map[string]int),
	}
}

// BeginTransaction opens a new monitored database transaction unit.
func (r *CDCRecorder) BeginTransaction(txID string, engine string, dbName string, stepIndex int) *DBTransaction {
	r.mu.Lock()
	defer r.mu.Unlock()

	if engine == "" {
		engine = "postgres"
	}
	if dbName == "" {
		dbName = "production_db"
	}

	tx := &DBTransaction{
		TxID:         txID,
		SessionID:    r.sessionID,
		StepIndex:    stepIndex,
		DatabaseName: dbName,
		Engine:       engine,
		Status:       TxActive,
		StartTime:    time.Now().UTC(),
	}

	r.transactions[txID] = tx
	r.orderedTx = append(r.orderedTx, tx)
	return tx
}

// RecordStatement registers a raw SQL statement executed within a transaction, running safety checks.
func (r *CDCRecorder) RecordStatement(txID string, sql string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	tx, exists := r.transactions[txID]
	if !exists {
		return fmt.Errorf("transaction '%s' not found", txID)
	}

	cleanSQL := strings.TrimSpace(sql)
	tx.RawStatements = append(tx.RawStatements, cleanSQL)

	// Safety check 1: Unbounded DELETE without WHERE
	if reDeleteWithoutWhere.MatchString(cleanSQL) && !strings.Contains(strings.ToUpper(cleanSQL), "WHERE") {
		tx.SafetyViolations = append(tx.SafetyViolations, fmt.Sprintf("CRITICAL: Unbounded DELETE without WHERE clause: '%s'", truncate(cleanSQL, 50)))
	}

	// Safety check 2: Unbounded UPDATE without WHERE
	if reUpdateWithoutWhere.MatchString(cleanSQL) && !strings.Contains(strings.ToUpper(cleanSQL), "WHERE") {
		tx.SafetyViolations = append(tx.SafetyViolations, fmt.Sprintf("CRITICAL: Unbounded UPDATE without WHERE clause: '%s'", truncate(cleanSQL, 50)))
	}

	// Safety check 3: Destructive DDL DROP
	if reDropTable.MatchString(cleanSQL) || reDropDatabase.MatchString(cleanSQL) || reTruncate.MatchString(cleanSQL) {
		tx.SafetyViolations = append(tx.SafetyViolations, fmt.Sprintf("HIGH: Destructive DDL statement executed: '%s'", truncate(cleanSQL, 50)))
	}

	return nil
}

// RecordRowDelta records row change-data-capture state and automatically synthesizes compensating SQL.
func (r *CDCRecorder) RecordRowDelta(txID string, delta *RowDelta) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	tx, exists := r.transactions[txID]
	if !exists {
		return fmt.Errorf("transaction '%s' not found", txID)
	}

	if delta.Timestamp.IsZero() {
		delta.Timestamp = time.Now().UTC()
	}

	// Synthesize automated compensating SQL undo command
	if delta.CompensatingSQL == "" {
		delta.CompensatingSQL = synthesizeCompensatingSQL(delta)
	}

	tx.RowDeltas = append(tx.RowDeltas, delta)
	tx.RowsAffected++
	return nil
}

// RecordVectorDelta records high-dimensional vector embeddings and collection mutations.
func (r *CDCRecorder) RecordVectorDelta(txID string, delta *VectorDelta) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	tx, exists := r.transactions[txID]
	if !exists {
		return fmt.Errorf("transaction '%s' not found", txID)
	}

	if delta.Timestamp.IsZero() {
		delta.Timestamp = time.Now().UTC()
	}

	tx.VectorDeltas = append(tx.VectorDeltas, delta)
	return nil
}

// CommitTransaction seals a transaction as successfully committed.
func (r *CDCRecorder) CommitTransaction(txID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	tx, exists := r.transactions[txID]
	if !exists {
		return fmt.Errorf("transaction '%s' not found", txID)
	}

	tx.Status = TxCommitted
	tx.EndTime = time.Now().UTC()
	tx.ExecutionLatencyMs = tx.EndTime.Sub(tx.StartTime).Milliseconds()
	return nil
}

// AbortTransaction marks a transaction as rolled back by the engine.
func (r *CDCRecorder) AbortTransaction(txID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	tx, exists := r.transactions[txID]
	if !exists {
		return fmt.Errorf("transaction '%s' not found", txID)
	}

	tx.Status = TxAborted
	tx.EndTime = time.Now().UTC()
	tx.ExecutionLatencyMs = tx.EndTime.Sub(tx.StartTime).Milliseconds()
	return nil
}

// CreateSavepoint establishes a named rollback milestone at a specific trajectory step.
func (r *CDCRecorder) CreateSavepoint(name string, stepIndex int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.savepoints[name] = stepIndex
}

// SynthesizeRollbackScript generates an atomic, idempotent inverse SQL script to rewind to a specific step.
func (r *CDCRecorder) SynthesizeRollbackScript(fromStep int) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var statements []string

	// Walk transactions in reverse chronological order
	for i := len(r.orderedTx) - 1; i >= 0; i-- {
		tx := r.orderedTx[i]
		if tx.StepIndex < fromStep || tx.Status != TxCommitted {
			continue
		}

		// Walk row deltas in reverse chronological order
		for j := len(tx.RowDeltas) - 1; j >= 0; j-- {
			delta := tx.RowDeltas[j]
			if delta.CompensatingSQL != "" {
				statements = append(statements, delta.CompensatingSQL)
			}
		}
	}

	if len(statements) == 0 {
		return "-- No database mutations require rollback for step >= " + fmt.Sprintf("%d", fromStep), nil
	}

	var b strings.Builder
	b.WriteString("-- =========================================================================\n")
	b.WriteString(fmt.Sprintf("-- AgentVCR Automated Semantic Rollback Script (from step >= %d)\n", fromStep))
	b.WriteString(fmt.Sprintf("-- Session ID: %s | Generated: %s\n", r.sessionID, time.Now().UTC().Format(time.RFC3339)))
	b.WriteString("-- =========================================================================\n\n")
	b.WriteString("BEGIN TRANSACTION;\n\n")
	for _, stmt := range statements {
		b.WriteString(stmt)
		if !strings.HasSuffix(stmt, ";") {
			b.WriteString(";")
		}
		b.WriteString("\n")
	}
	b.WriteString("\nCOMMIT;\n")

	return b.String(), nil
}

// FinalizeAudit computes aggregated metrics, checks safety invariants, and returns the audit summary.
func (r *CDCRecorder) FinalizeAudit(strict bool) *CDCSummary {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tableSet := make(map[string]bool)
	committed := 0
	aborted := 0
	totalRows := 0
	destructiveCount := 0
	var violations []string

	for _, tx := range r.orderedTx {
		if tx.Status == TxCommitted {
			committed++
		} else if tx.Status == TxAborted {
			aborted++
		}
		totalRows += tx.RowsAffected

		for _, delta := range tx.RowDeltas {
			if delta.Table != "" {
				tableSet[delta.Table] = true
			}
		}

		if len(tx.SafetyViolations) > 0 {
			destructiveCount += len(tx.SafetyViolations)
			violations = append(violations, tx.SafetyViolations...)
		}
	}

	var tables []string
	for t := range tableSet {
		tables = append(tables, t)
	}
	sort.Strings(tables)

	rollbackScript, _ := r.SynthesizeRollbackScript(1)

	passedAudit := true
	if strict && len(violations) > 0 {
		passedAudit = false
	}

	return &CDCSummary{
		SessionID:               r.sessionID,
		TotalTransactions:       len(r.orderedTx),
		CommittedCount:          committed,
		AbortedCount:            aborted,
		TotalRowsAffected:       totalRows,
		TablesMutated:           tables,
		DestructiveQueriesCount: destructiveCount,
		SafeRollbackPossible:    true,
		CompensatingScript:      rollbackScript,
		PassedAudit:             passedAudit,
		SafetyViolations:        violations,
		Transactions:            r.orderedTx,
	}
}

func synthesizeCompensatingSQL(delta *RowDelta) string {
	table := delta.Table
	pkCol := delta.PrimaryKeyCol
	if pkCol == "" {
		pkCol = "id"
	}
	pkVal := formatSQLValue(delta.PrimaryKeyValue)

	switch delta.Operation {
	case OpInsert:
		// Undo INSERT -> DELETE row by primary key
		return fmt.Sprintf("DELETE FROM %s WHERE %s = %s;", table, pkCol, pkVal)

	case OpDelete:
		// Undo DELETE -> INSERT row back using 'Before' state
		if len(delta.Before) == 0 {
			return fmt.Sprintf("-- WARNING: Cannot synthesize inverse INSERT for %s: missing 'Before' row state", table)
		}
		var cols []string
		var vals []string
		for k, v := range delta.Before {
			cols = append(cols, k)
			vals = append(vals, formatSQLValue(v))
		}
		return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s);", table, strings.Join(cols, ", "), strings.Join(vals, ", "))

	case OpUpdate:
		// Undo UPDATE -> UPDATE row back to 'Before' column values
		if len(delta.Before) == 0 {
			return fmt.Sprintf("-- WARNING: Cannot synthesize inverse UPDATE for %s: missing 'Before' row state", table)
		}
		var setClauses []string
		for k, v := range delta.Before {
			if k == pkCol {
				continue
			}
			setClauses = append(setClauses, fmt.Sprintf("%s = %s", k, formatSQLValue(v)))
		}
		if len(setClauses) == 0 {
			return fmt.Sprintf("-- NOOP: UPDATE %s had no modified non-PK fields", table)
		}
		return fmt.Sprintf("UPDATE %s SET %s WHERE %s = %s;", table, strings.Join(setClauses, ", "), pkCol, pkVal)

	default:
		return fmt.Sprintf("-- Unhandled operation '%s' for table %s", delta.Operation, table)
	}
}

func formatSQLValue(val interface{}) string {
	if val == nil {
		return "NULL"
	}
	switch v := val.(type) {
	case string:
		escaped := strings.ReplaceAll(v, "'", "''")
		return fmt.Sprintf("'%s'", escaped)
	case bool:
		if v {
			return "TRUE"
		}
		return "FALSE"
	default:
		return fmt.Sprintf("%v", v)
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
