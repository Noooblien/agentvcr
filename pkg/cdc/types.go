package cdc

import (
	"encoding/json"
	"fmt"
	"time"
)

// OperationType classifies the database mutation kind.
type OperationType string

const (
	OpInsert       OperationType = "INSERT"
	OpUpdate       OperationType = "UPDATE"
	OpDelete       OperationType = "DELETE"
	OpCreateTable  OperationType = "CREATE_TABLE"
	OpDropTable    OperationType = "DROP_TABLE"
	OpAlterTable   OperationType = "ALTER_TABLE"
	OpTruncate     OperationType = "TRUNCATE"
	OpVectorUpsert OperationType = "VECTOR_UPSERT"
	OpVectorDelete OperationType = "VECTOR_DELETE"
)

// TxStatus tracks transaction lifecycle.
type TxStatus string

const (
	TxActive     TxStatus = "ACTIVE"
	TxCommitted  TxStatus = "COMMITTED"
	TxAborted    TxStatus = "ABORTED"
	TxRolledBack TxStatus = "ROLLED_BACK"
)

// RowDelta captures granular change-data-capture state before and after mutation.
type RowDelta struct {
	Table           string                 `json:"table"`
	PrimaryKeyCol   string                 `json:"primaryKeyCol"`
	PrimaryKeyValue interface{}            `json:"primaryKeyValue"`
	Operation       OperationType          `json:"operation"`
	Before          map[string]interface{} `json:"before,omitempty"`
	After           map[string]interface{} `json:"after,omitempty"`
	CompensatingSQL string                 `json:"compensatingSql"`
	Timestamp       time.Time              `json:"timestamp"`
}

// VectorDelta tracks high-dimensional vector embeddings and collection mutations.
type VectorDelta struct {
	Collection      string                 `json:"collection"`
	VectorID        string                 `json:"vectorId"`
	Dimensions      int                    `json:"dimensions"`
	Operation       OperationType          `json:"operation"`
	EmbeddingBefore []float32              `json:"embeddingBefore,omitempty"`
	EmbeddingAfter  []float32              `json:"embeddingAfter,omitempty"`
	MetadataBefore  map[string]interface{} `json:"metadataBefore,omitempty"`
	MetadataAfter   map[string]interface{} `json:"metadataAfter,omitempty"`
	Timestamp       time.Time              `json:"timestamp"`
}

// DBTransaction represents an atomic database transaction captured in the flight trace.
type DBTransaction struct {
	TxID               string          `json:"txId"`
	SessionID          string          `json:"sessionId"`
	StepIndex          int             `json:"stepIndex"`
	DatabaseName       string          `json:"databaseName"`
	Engine             string          `json:"engine"` // "postgres", "sqlite", "duckdb", "chroma"
	Status             TxStatus        `json:"status"`
	RawStatements      []string        `json:"rawStatements"`
	RowDeltas          []*RowDelta     `json:"rowDeltas"`
	VectorDeltas       []*VectorDelta  `json:"vectorDeltas"`
	RowsAffected       int             `json:"rowsAffected"`
	ExecutionLatencyMs int64           `json:"executionLatencyMs"`
	SafetyViolations   []string        `json:"safetyViolations,omitempty"`
	StartTime          time.Time       `json:"startTime"`
	EndTime            time.Time       `json:"endTime"`
}

// CDCSummary reports aggregated transaction metrics and rollback viability.
type CDCSummary struct {
	SessionID                string                    `json:"sessionId"`
	TotalTransactions        int                       `json:"totalTransactions"`
	CommittedCount           int                       `json:"committedCount"`
	AbortedCount             int                       `json:"abortedCount"`
	TotalRowsAffected        int                       `json:"totalRowsAffected"`
	TablesMutated            []string                  `json:"tablesMutated"`
	DestructiveQueriesCount  int                       `json:"destructiveQueriesCount"`
	SafeRollbackPossible     bool                      `json:"safeRollbackPossible"`
	CompensatingScript       string                    `json:"compensatingScript"`
	PassedAudit              bool                      `json:"passedAudit"`
	SafetyViolations         []string                  `json:"safetyViolations"`
	Transactions             []*DBTransaction          `json:"transactions"`
}

// FormatDeltaJSON formats delta payload cleanly.
func FormatDeltaJSON(delta *RowDelta) string {
	b, err := json.Marshal(delta)
	if err != nil {
		return fmt.Sprintf("%v", delta)
	}
	return string(b)
}
