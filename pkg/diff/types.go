package diff

import (
	"github.com/Noooblien/agentvcr/pkg/trace"
)

// DiffClassification defines the nature of alignment or deviation between two trajectory steps.
type DiffClassification string

const (
	DiffMatchExact         DiffClassification = "MATCH_EXACT"
	DiffMatchSemantic      DiffClassification = "MATCH_SEMANTIC"
	DiffParamDrift         DiffClassification = "PARAM_DRIFT"
	DiffControlFlowDrift   DiffClassification = "CONTROL_FLOW_DRIFT"
	DiffStepInserted       DiffClassification = "STEP_INSERTED"
	DiffStepDeleted        DiffClassification = "STEP_DELETED"
	DiffOutcomeRegression  DiffClassification = "OUTCOME_REGRESSION"
	DiffErrorInduced       DiffClassification = "ERROR_INDUCED"
)

// RegressionVerdict represents the overall stability and regression status of a candidate trajectory.
type RegressionVerdict string

const (
	VerdictPass RegressionVerdict = "PASS"
	VerdictWarn RegressionVerdict = "WARN"
	VerdictFail RegressionVerdict = "FAIL"
)

// StepDiff captures the side-by-side comparison of aligned trajectory steps.
type StepDiff struct {
	StepIndexBase    int                `json:"step_index_base"`    // -1 if inserted in target
	StepIndexTarget  int                `json:"step_index_target"`  // -1 if deleted in target
	BaseType         trace.EventType    `json:"base_type,omitempty"`
	TargetType       trace.EventType    `json:"target_type,omitempty"`
	BaseToolName     string             `json:"base_tool_name,omitempty"`
	TargetToolName   string             `json:"target_tool_name,omitempty"`
	Classification   DiffClassification `json:"classification"`
	BaseSummary      string             `json:"base_summary,omitempty"`
	TargetSummary    string             `json:"target_summary,omitempty"`
	ParamDiffs       []string           `json:"param_diffs,omitempty"`
	SimilarityScore  float64            `json:"similarity_score"` // 0.0 - 1.0
	LatencyDeltaMs   int64              `json:"latency_delta_ms"`
	TokenDelta       int                `json:"token_delta"`
}

// SemanticDiffReport summarizes the structural, semantic, and token differences between two runs.
type SemanticDiffReport struct {
	BaseSessionID          string            `json:"base_session_id"`
	TargetSessionID        string            `json:"target_session_id"`
	BaseAgentName          string            `json:"base_agent_name"`
	TargetAgentName        string            `json:"target_agent_name"`
	AlignedSteps           []StepDiff        `json:"aligned_steps"`
	TotalBaseEvents        int               `json:"total_base_events"`
	TotalTargetEvents      int               `json:"total_target_events"`
	ExactMatches           int               `json:"exact_matches"`
	SemanticMatches        int               `json:"semantic_matches"`
	ParamDrifts            int               `json:"param_drifts"`
	ControlFlowDrifts      int               `json:"control_flow_drifts"`
	Insertions             int               `json:"insertions"`
	Deletions              int               `json:"deletions"`
	Regressions            int               `json:"regressions"`
	StabilityScore         float64           `json:"stability_score"`          // 0.0 - 100.0%
	ToolSequenceSimilarity float64           `json:"tool_sequence_similarity"` // 0.0 - 100.0%
	TotalTokenDelta        int               `json:"total_token_delta"`
	CostDeltaUSD           float64           `json:"cost_delta_usd"`
	DurationDeltaMs        int64             `json:"duration_delta_ms"`
	Verdict                RegressionVerdict `json:"verdict"`
	VerdictReason          string            `json:"verdict_reason"`
}

// DiffOptions configures tolerance and strictness thresholds for trajectory comparison.
type DiffOptions struct {
	Tolerance               float64 `json:"tolerance"`                 // Minimum stability score (0.0 - 1.0), default 0.80
	IgnoreCosmeticParaphrase bool    `json:"ignore_cosmetic_paraphrase"` // Treat high semantic similarity as matching
	Strict                  bool    `json:"strict"`                    // Fail on any regression, error, or control flow deviation
	MaxTokenRegressionPct   float64 `json:"max_token_regression_pct"`  // Max allowable percentage token increase before failure
}

// DefaultDiffOptions returns industry-standard baseline comparison options.
func DefaultDiffOptions() DiffOptions {
	return DiffOptions{
		Tolerance:               0.80,
		IgnoreCosmeticParaphrase: true,
		Strict:                  false,
		MaxTokenRegressionPct:   25.0,
	}
}
