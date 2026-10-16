package leaderboard

import (
	"time"
)

// AgentScorecard contains multidimensional benchmark evaluation metrics for a single agent session.
type AgentScorecard struct {
	SessionID          string  `json:"session_id"`
	AgentName          string  `json:"agent_name"`
	Model              string  `json:"model"`
	Version            string  `json:"version,omitempty"`
	Rank               int     `json:"rank"`
	SuccessRate        float64 `json:"success_rate"`         // 0.0 to 100.0%
	TotalEvents        int     `json:"total_events"`
	TotalTokens        int     `json:"total_tokens"`
	PromptTokens       int     `json:"prompt_tokens"`
	CompletionTokens   int     `json:"completion_tokens"`
	EstimatedCostUSD   float64 `json:"estimated_cost_usd"`
	LatencyP50Ms       int64   `json:"latency_p50_ms"`
	LatencyP95Ms       int64   `json:"latency_p95_ms"`
	TotalDurationMs    int64   `json:"total_duration_ms"`
	ToolCallCount      int     `json:"tool_call_count"`
	ToolErrorCount     int     `json:"tool_error_count"`
	ToolAccuracyRate   float64 `json:"tool_accuracy_rate"`   // 0.0 to 100.0%
	SafetyScore        float64 `json:"safety_score"`         // 0.0 to 100.0%
	CompositeScore     float64 `json:"composite_score"`      // 0.0 to 100.0% normalized benchmark index
}

// Leaderboard aggregates ranked evaluation scorecards across multiple agents and models.
type Leaderboard struct {
	SuiteName          string           `json:"suite_name"`
	GeneratedAt        time.Time        `json:"generated_at"`
	TotalEntries       int              `json:"total_entries"`
	TopAgent           string           `json:"top_agent"`
	AverageSuccessRate float64          `json:"average_success_rate"`
	AverageTokens      int              `json:"average_tokens"`
	AverageCostUSD     float64          `json:"average_cost_usd"`
	AverageDurationMs  int64            `json:"average_duration_ms"`
	Entries            []AgentScorecard `json:"entries"`
}

// GateConfig defines automated regression tolerance thresholds for CI/CD gates.
type GateConfig struct {
	MaxSuccessRateDropPercent float64 `json:"max_success_rate_drop_percent"` // e.g. 5.0 (allow up to 5% drop)
	MaxTokenCostIncreaseRatio float64 `json:"max_token_cost_increase_ratio"` // e.g. 0.25 (allow up to 25% inflation)
	MaxLatencyIncreaseMs      int64   `json:"max_latency_increase_ms"`       // e.g. 500ms
	MaxAllowedToolErrors      int     `json:"max_allowed_tool_errors"`       // e.g. 0
	RequireZeroSafetyBreaches bool    `json:"require_zero_safety_breaches"`
}

// DefaultGateConfig provides standard strict regression thresholds for production deployments.
func DefaultGateConfig() GateConfig {
	return GateConfig{
		MaxSuccessRateDropPercent: 3.0,
		MaxTokenCostIncreaseRatio: 0.20,
		MaxLatencyIncreaseMs:      400,
		MaxAllowedToolErrors:      0,
		RequireZeroSafetyBreaches: true,
	}
}

// RegressionDiff details head-to-head performance deltas between baseline and candidate trajectories.
type RegressionDiff struct {
	BaselineSessionID   string         `json:"baseline_session_id"`
	CandidateSessionID  string         `json:"candidate_session_id"`
	BaselineAgent       string         `json:"baseline_agent"`
	CandidateAgent      string         `json:"candidate_agent"`
	BaselineScorecard   AgentScorecard `json:"baseline_scorecard"`
	CandidateScorecard  AgentScorecard `json:"candidate_scorecard"`
	DeltaSuccessRate    float64        `json:"delta_success_rate"`
	DeltaTotalTokens    int            `json:"delta_total_tokens"`
	DeltaCostUSD        float64        `json:"delta_cost_usd"`
	DeltaDurationMs     int64          `json:"delta_duration_ms"`
	DeltaLatencyP95Ms   int64          `json:"delta_latency_p95_ms"`
	DeltaToolErrors     int            `json:"delta_tool_errors"`
	DeltaCompositeScore float64        `json:"delta_composite_score"`
	RegressionDetected  bool           `json:"regression_detected"`
	RegressionReasons   []string       `json:"regression_reasons"`
	PassGate            bool           `json:"pass_gate"`
}
