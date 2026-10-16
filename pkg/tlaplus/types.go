package tlaplus

import (
	"time"
)

// TemporalOperator defines the modal logic quantifier used in temporal formulas.
type TemporalOperator string

const (
	OpAlways     TemporalOperator = "ALWAYS ([]P)"
	OpEventually TemporalOperator = "EVENTUALLY (<>Q)"
	OpLeadsto    TemporalOperator = "LEADSTO (P ~> Q)"
	OpNext       TemporalOperator = "NEXT (O P)"
)

// StatePredicate defines an atomic evaluation condition over state variables.
type StatePredicate struct {
	Variable   string      `json:"variable"`
	Operator   string      `json:"operator"` // "eq", "neq", "lt", "lte", "gt", "gte", "contains"
	TargetVal  interface{} `json:"target_value"`
}

// TemporalInvariant represents a formal specification property to be model checked.
type TemporalInvariant struct {
	ID                string          `json:"id"`
	Name              string          `json:"name"`
	Description       string          `json:"description"`
	Operator          TemporalOperator `json:"operator"`
	TriggerPredicate  *StatePredicate `json:"trigger_predicate,omitempty"` // For Leadsto (P ~> Q)
	TargetPredicate   StatePredicate  `json:"target_predicate"`
	IsSafety          bool            `json:"is_safety"` // True = Safety ([]P), False = Liveness (<>Q)
}

// StateTransitionRule governs valid edge transitions in the agent's finite state machine.
type StateTransitionRule struct {
	FromState string `json:"from_state"`
	Action    string `json:"action"`
	ToState   string `json:"to_state"`
	Allowed   bool   `json:"allowed"`
}

// StateSnapshot captures the concrete state variables of an agent at a given discrete step.
type StateSnapshot struct {
	StepIndex   int                    `json:"step_index"`
	ActionTaken string                 `json:"action_taken"`
	CurrentState string                `json:"current_state"`
	Variables   map[string]interface{} `json:"variables"`
	Timestamp   time.Time              `json:"timestamp"`
}

// ModelCheckingViolation records a formal counterexample trace refuting a temporal property.
type ModelCheckingViolation struct {
	InvariantID         string          `json:"invariant_id"`
	InvariantName       string          `json:"invariant_name"`
	Operator            TemporalOperator `json:"operator"`
	StepIndex           int             `json:"step_index"`
	ViolatingState      StateSnapshot   `json:"violating_state"`
	CounterexampleTrace []StateSnapshot `json:"counterexample_trace"`
	Reason              string          `json:"reason"`
}

// ModelCheckingReport documents the complete formal verification results and generated TLA+ spec.
type ModelCheckingReport struct {
	SessionID            string                   `json:"session_id"`
	TotalStatesEvaluated int                      `json:"total_states_evaluated"`
	TransitionsChecked   int                      `json:"transitions_checked"`
	InvariantsChecked    int                      `json:"invariants_checked"`
	InvariantsSatisfied  int                      `json:"invariants_satisfied"`
	InvariantsViolated   int                      `json:"invariants_violated"`
	FormallyVerified     bool                     `json:"formally_verified"`
	GeneratedTLASpec     string                   `json:"generated_tla_spec"`
	Violations           []ModelCheckingViolation `json:"violations"`
	StateHistory         []StateSnapshot          `json:"state_history"`
	GeneratedAt          time.Time                `json:"generated_at"`
}
