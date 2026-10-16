package heal

import (
	"time"
)

// PatchStrategy defines the type of autonomous correction applied to a failing trajectory step.
type PatchStrategy string

const (
	StrategySyntaxRepair       PatchStrategy = "syntax_repair"       // Fixes invalid JSON, broken brackets, escaping errors
	StrategySchemaCoercion     PatchStrategy = "schema_coercion"     // Fixes type mismatches, missing required parameters
	StrategyDependencyFallback PatchStrategy = "dependency_fallback" // Auto-injects missing packages, tools, or binary fallbacks
	StrategyPromptRefinement   PatchStrategy = "prompt_refinement"   // Injects explicit counter-instructions to bypass failure
	StrategyRetryWithBackoff   PatchStrategy = "retry_with_backoff"  // Adds exponential backoff & jitter for transient network faults
)

// TrajectoryFault represents a detected error or crash in an agent's execution sequence.
type TrajectoryFault struct {
	StepIndex    int           `json:"step_index"`
	FaultType    string        `json:"fault_type"`    // e.g. "json_syntax", "schema_mismatch", "missing_dependency", "command_failure"
	ErrorMessage string        `json:"error_message"` // Raw error message or stderr
	TargetField  string        `json:"target_field,omitempty"`
	Severity     string        `json:"severity"`      // "low", "medium", "critical"
	Strategy     PatchStrategy `json:"strategy"`
}

// ASTMutation describes a concrete modification to the trajectory event's payload or payload AST.
type ASTMutation struct {
	Path        string      `json:"path"`        // JSON path or AST node identifier
	OriginalVal interface{} `json:"original_val"`
	MutatedVal  interface{} `json:"mutated_val"`
	Description string      `json:"description"`
}

// HealPatch encapsulates a synthesized fix for a specific trajectory fault.
type HealPatch struct {
	PatchID       string        `json:"patch_id"`
	FaultIndex    int           `json:"fault_index"`
	Strategy      PatchStrategy `json:"strategy"`
	Mutations     []ASTMutation `json:"mutations"`
	Confidence    float64       `json:"confidence"` // 0.0 to 1.0
	Explanation   string        `json:"explanation"`
	OriginalEvent []byte        `json:"original_event,omitempty"`
	PatchedEvent  []byte        `json:"patched_event,omitempty"`
	CreatedAt     time.Time     `json:"created_at"`
}

// HealReport summarizes the autonomous healing diagnosis and synthesized patches for a session.
type HealReport struct {
	SessionID             string            `json:"session_id"`
	TotalEventsInspected  int               `json:"total_events_inspected"`
	FaultsDetected        int               `json:"faults_detected"`
	PatchesSynthesized    int               `json:"patches_synthesized"`
	PatchesApplied        int               `json:"patches_applied"`
	HealingSuccessRate    float64           `json:"healing_success_rate"`
	Faults                []TrajectoryFault `json:"faults"`
	Patches               []HealPatch       `json:"patches"`
	AutoAppliedCleanly    bool              `json:"auto_applied_cleanly"`
	EstimatedRecoveryTime int64             `json:"estimated_recovery_time_ms"`
	GeneratedAt           time.Time         `json:"generated_at"`
}
