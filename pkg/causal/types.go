package causal

import "github.com/Noooblien/agentvcr/pkg/trace"

// EdgeType classifies the causal relationship between two trajectory nodes.
type EdgeType string

const (
	EdgeDataDependency  EdgeType = "data_dependency"  // Output or state from A was consumed as input/prompt to B
	EdgeToolSequence    EdgeType = "tool_sequence"    // Direct operational step sequence
	EdgeErrorTrigger    EdgeType = "error_trigger"    // Event A introduced malformed state/bad argument that triggered error B
	EdgeStateMutation   EdgeType = "state_mutation"   // Event A mutated an environment state key
	EdgeContextSpill    EdgeType = "context_spill"    // Event A polluted context window leading to hallucination/drift
)

// CausalNode represents a step in the trajectory enriched with causal dependencies.
type CausalNode struct {
	ID          string                 `json:"id"`
	StepIndex   int                    `json:"step_index"`
	EventType   trace.EventType        `json:"event_type"`
	Summary     string                 `json:"summary"`
	ToolName    string                 `json:"tool_name,omitempty"`
	Inputs      []string               `json:"inputs,omitempty"`      // Tokens, parameters, or state keys read
	Outputs     []string               `json:"outputs,omitempty"`     // Tokens, outputs, or state keys produced
	IsError     bool                   `json:"is_error"`
	Payload     map[string]interface{} `json:"payload,omitempty"`
}

// CausalEdge represents a directed dependency link in the DAG.
type CausalEdge struct {
	FromID      string   `json:"from_id"`
	ToID        string   `json:"to_id"`
	Type        EdgeType `json:"type"`
	Description string   `json:"description"`
	Confidence  float64  `json:"confidence"` // 0.0 - 1.0
}

// CausalDAG is a directed acyclic graph capturing the end-to-end causal chain of an agent trajectory.
type CausalDAG struct {
	SessionID   string       `json:"session_id"`
	Nodes       []CausalNode `json:"nodes"`
	Edges       []CausalEdge `json:"edges"`
}

// RootCauseAnalysis isolates the failure origin and traces the propagating blast radius.
type RootCauseAnalysis struct {
	SessionID          string   `json:"session_id"`
	FailureStep        int      `json:"failure_step"`
	FailureEventID     string   `json:"failure_event_id"`
	RootCauseStep      int      `json:"root_cause_step"`
	RootCauseNodeID    string   `json:"root_cause_node_id"`
	RootCauseType      string   `json:"root_cause_type"` // "bad_parameter", "unhandled_tool_error", "hallucination", "state_corruption"
	Description        string   `json:"description"`
	CausalChain        []int    `json:"causal_chain"` // Sequence of step indices from root cause to failure
	BlastRadiusSteps   []int    `json:"blast_radius_steps"`
}

// RepairPatch specifies an automated candidate patch to heal the failure during trajectory branching (`fork`).
type RepairPatch struct {
	PatchID          string                 `json:"patch_id"`
	TargetStep       int                    `json:"target_step"`
	Action           string                 `json:"action"` // "modify_args", "mock_tool_result", "inject_guardrail", "retry"
	Reason           string                 `json:"reason"`
	ModifiedPayload  map[string]interface{} `json:"modified_payload"`
	SuggestedPrompt  string                 `json:"suggested_prompt,omitempty"`
}

// CausalReport encapsulates the full causal analysis, root cause diagnosis, and recommended repairs.
type CausalReport struct {
	SessionID          string            `json:"session_id"`
	TotalNodes         int               `json:"total_nodes"`
	TotalEdges         int               `json:"total_edges"`
	HasFailures        bool              `json:"has_failures"`
	RootCauses         []RootCauseAnalysis `json:"root_causes"`
	RecommendedPatches []RepairPatch     `json:"recommended_patches"`
	DAG                *CausalDAG        `json:"dag"`
}
