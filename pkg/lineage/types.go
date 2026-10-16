package lineage

import (
	"time"
)

// MutationType classifies the exact state mutation operation.
type MutationType string

const (
	MutationSet    MutationType = "set"    // Variable initialized or set
	MutationUpdate MutationType = "update" // Variable value mutated/updated
	MutationDelete MutationType = "delete" // Variable purged/removed
	MutationRead   MutationType = "read"   // Variable referenced/observed
)

// StateMutation represents a microsecond-precision mutation on a state variable.
type StateMutation struct {
	MutationID        string       `json:"mutation_id"`
	StepIndex         int          `json:"step_index"`
	Timestamp         time.Time    `json:"timestamp"`
	MicrosecondOffset int64        `json:"microsecond_offset"` // Offset in microseconds from session start
	Key               string       `json:"key"`
	Type              MutationType `json:"type"`
	OldValue          interface{}  `json:"old_value,omitempty"`
	NewValue          interface{}  `json:"new_value,omitempty"`
	Source            string       `json:"source"` // e.g. "state_change", "tool:file_write", "llm_response"
	EventID           string       `json:"event_id"`
	Actor             string       `json:"actor"`
	Dependencies      []string     `json:"dependencies,omitempty"` // Other variables or keys referenced during this mutation
}

// VariableLineage tracks the full ancestral provenance and history of an individual state key.
type VariableLineage struct {
	Key            string          `json:"key"`
	InitialStep    int             `json:"initial_step"`
	CreatedAt      time.Time       `json:"created_at"`
	LastModifiedAt time.Time       `json:"last_modified_at"`
	TotalMutations int             `json:"total_mutations"`
	ReadCount      int             `json:"read_count"`
	WriteCount     int             `json:"write_count"`
	CurrentValue   interface{}     `json:"current_value,omitempty"`
	History        []StateMutation `json:"history"`
	Dependencies   []string        `json:"dependencies"`
}

// LineageGraph models the directed provenance graph of ephemeral sandbox state mutations.
type LineageGraph struct {
	SessionID      string                      `json:"session_id"`
	TotalSteps     int                         `json:"total_steps"`
	TotalMutations int                         `json:"total_mutations"`
	UniqueKeys     int                         `json:"unique_keys"`
	Variables      map[string]*VariableLineage `json:"variables"`
	Timeline       []StateMutation             `json:"timeline"`
	SandboxedState map[string]interface{}      `json:"sandboxed_state"`
}

// LineageReport contains comprehensive inspection analytics, point-in-time state, and anomaly detections.
type LineageReport struct {
	SessionID             string                 `json:"session_id"`
	GeneratedAt           time.Time              `json:"generated_at"`
	Graph                 *LineageGraph          `json:"graph"`
	FilterKey             string                 `json:"filter_key,omitempty"`
	TargetStep            int                    `json:"target_step,omitempty"`
	SnapshotAtStep        map[string]interface{} `json:"snapshot_at_step,omitempty"`
	DivergenceRisks       []string               `json:"divergence_risks"`
	EphemeralSandboxValid bool                   `json:"ephemeral_sandbox_valid"`
}
