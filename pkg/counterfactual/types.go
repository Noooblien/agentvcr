package counterfactual

import (
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// MutationType classifies the counterfactual intervention injected at the branch step.
type MutationType string

const (
	// MutateToolSubstitution redirects an expensive or failing tool invocation to an alternative fallback tool.
	MutateToolSubstitution MutationType = "TOOL_SUBSTITUTION"
	// MutateArgumentOverride modifies the arguments passed to a tool to correct errors or supply missing keys.
	MutateArgumentOverride MutationType = "ARGUMENT_OVERRIDE"
	// MutateResponseMock injects synthetic, cached, or simulated response data in place of upstream results.
	MutateResponseMock MutationType = "RESPONSE_MOCK"
	// MutateSteeringPrompt injects a human-in-the-loop guidance or system prompt correction into the context.
	MutateSteeringPrompt MutationType = "STEERING_PROMPT"
)

// BranchOutcome categorizes how the counterfactual branch resolved compared to baseline.
type BranchOutcome string

const (
	OutcomeResolvedSuccess      BranchOutcome = "RESOLVED_SUCCESS"       // Fixes baseline failure, achieves goal
	OutcomeAlternativeEquiv     BranchOutcome = "ALTERNATIVE_EQUIVALENCE" // Different path, same successful end state
	OutcomeRegressionFailure    BranchOutcome = "REGRESSION_FAILURE"     // Introduced new error or degraded outcome
	OutcomeRunawayDivergence    BranchOutcome = "RUNAWAY_DIVERGENCE"     // Trajectory expanded into loops or runaway tokens
)

// CounterfactualMutation defines an atomic what-if hypothesis to inject into an agent trajectory.
type CounterfactualMutation struct {
	ID                string                 `json:"id"`
	Name              string                 `json:"name"`
	Description       string                 `json:"description"`
	BranchStep        int                    `json:"branch_step"` // 1-indexed step in the trajectory to intervene
	Type              MutationType           `json:"type"`
	TargetTool        string                 `json:"target_tool,omitempty"`
	ReplacementTool   string                 `json:"replacement_tool,omitempty"`
	ArgumentOverrides map[string]interface{} `json:"argument_overrides,omitempty"`
	MockResponse      map[string]interface{} `json:"mock_response,omitempty"`
	SteeringPrompt    string                 `json:"steering_prompt,omitempty"`
}

// BranchComparison captures the comparative telemetry between baseline and counterfactual trajectory.
type BranchComparison struct {
	SessionID         string                 `json:"session_id"`
	BranchID          string                 `json:"branch_id"`
	Mutation          CounterfactualMutation `json:"mutation"`
	BaselineSteps     int                    `json:"baseline_steps"`
	BranchSteps       int                    `json:"branch_steps"`
	StepDelta         int                    `json:"step_delta"`          // branch - baseline
	PathEditDistance  int                    `json:"path_edit_distance"`  // Levenshtein distance on tool names
	PathDivergencePct float64                `json:"path_divergence_pct"` // % of steps diverging after split
	BaselineTokens    int                    `json:"baseline_tokens"`
	BranchTokens      int                    `json:"branch_tokens"`
	TokenDelta        int                    `json:"token_delta"`         // branch - baseline (negative = saved)
	TokenSavingsPct   float64                `json:"token_savings_pct"`
	BaselineLatencyMs int64                  `json:"baseline_latency_ms"`
	BranchLatencyMs   int64                  `json:"branch_latency_ms"`
	LatencyDeltaMs    int64                  `json:"latency_delta_ms"`
	CostBaselineUSD   float64                `json:"cost_baseline_usd"`
	CostBranchUSD     float64                `json:"cost_branch_usd"`
	CostDeltaUSD      float64                `json:"cost_delta_usd"`
	Outcome           BranchOutcome          `json:"outcome"`
	ResolvedFailure   bool                   `json:"resolved_failure"`
	SummaryNote       string                 `json:"summary_note"`
	CreatedAt         time.Time              `json:"created_at"`
}

// SimulationSummary holds aggregate findings across multiple counterfactual what-if experiments.
type SimulationSummary struct {
	TotalScenarios         int                 `json:"total_scenarios"`
	ResolvedSuccessCount   int                 `json:"resolved_success_count"`
	AlternativeEquivCount  int                 `json:"alternative_equiv_count"`
	RegressionCount        int                 `json:"regression_count"`
	NetTokensSaved         int                 `json:"net_tokens_saved"`
	AverageTokenSavingsPct float64             `json:"average_token_savings_pct"`
	NetCostSavingsUSD      float64             `json:"net_cost_savings_usd"`
	NetLatencySavedMs      int64               `json:"net_latency_saved_ms"`
	Comparisons            []*BranchComparison `json:"comparisons"`
	Duration               time.Duration       `json:"duration"`
}

// DemoScenario models an end-to-end baseline trajectory and its counterfactual hypothesis.
type DemoScenario struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Baseline    *trace.Session         `json:"baseline"`
	Mutation    CounterfactualMutation `json:"mutation"`
}
