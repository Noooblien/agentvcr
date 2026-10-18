package shadow

import (
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// TrafficRoutingMode defines how live requests are dispatched to candidate models/agents.
type TrafficRoutingMode string

const (
	RoutingShadowOnly    TrafficRoutingMode = "shadow_dark_launch" // Mirrored asynchronously, production never sees canary response
	RoutingABSplit       TrafficRoutingMode = "ab_traffic_split"   // Canary responses served to a fraction of live traffic
	RoutingCanaryRamping TrafficRoutingMode = "canary_ramping"     // Gradually increasing traffic percentage
)

// DriftSeverity classifies semantic/structural discrepancy between baseline and candidate trajectories.
type DriftSeverity string

const (
	SeverityNone     DriftSeverity = "none"
	SeverityLow      DriftSeverity = "low"
	SeverityModerate DriftSeverity = "moderate"
	SeverityHigh     DriftSeverity = "high"
	SeverityCritical DriftSeverity = "critical"
)

// ModelCandidate describes an LLM model or agent prompt version under test.
type ModelCandidate struct {
	ID             string            `json:"id"`
	ModelName      string            `json:"model_name"`
	Provider       string            `json:"provider"`
	CostPer1kInput float64           `json:"cost_per_1k_input"`
	CostPer1kOutput float64          `json:"cost_per_1k_output"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// ShadowConfig configures continuous shadowing and A/B drift evaluation.
type ShadowConfig struct {
	Mode                   TrafficRoutingMode `json:"mode"`
	CandidatePercentage    float64            `json:"candidate_percentage"`    // 0.0 - 100.0%
	MaxDriftThreshold      float64            `json:"max_drift_threshold"`      // e.g. 0.25 (semantic distance limit)
	MaxLatencyRatio        float64            `json:"max_latency_ratio"`        // e.g. 1.30 (candidate cannot be >30% slower)
	MaxCostIncreaseRatio   float64            `json:"max_cost_increase_ratio"`  // e.g. 1.20 (candidate cost cannot exceed baseline by >20%)
	MaxAllowedErrorRate    float64            `json:"max_allowed_error_rate"`   // e.g. 0.02 (2% error ceiling)
	MinSampleCount         int                `json:"min_sample_count"`         // Minimum comparisons required before promotion
	AutoRollbackOnBreach   bool               `json:"auto_rollback_on_breach"`  // Terminate candidate on circuit trip
}

// StepComparison captures side-by-side execution metrics for a single turn in a shadowed trajectory.
type StepComparison struct {
	StepIndex              int           `json:"step_index"`
	EventType              trace.EventType `json:"event_type"`
	BaselinePayload        string        `json:"baseline_payload"`
	CandidatePayload       string        `json:"candidate_payload"`
	BaselineLatencyMs      int64         `json:"baseline_latency_ms"`
	CandidateLatencyMs     int64         `json:"candidate_latency_ms"`
	LatencyDeltaMs         int64         `json:"latency_delta_ms"`
	BaselineTokens         int           `json:"baseline_tokens"`
	CandidateTokens        int           `json:"candidate_tokens"`
	TokenDelta             int           `json:"token_delta"`
	SemanticSimilarity     float64       `json:"semantic_similarity"` // 0.0 - 1.0 (Cosine/Jaccard semantic overlap)
	DriftScore             float64       `json:"drift_score"`         // 0.0 (identical) - 1.0 (complete drift)
	DriftSeverity          DriftSeverity `json:"drift_severity"`
	ToolCallDiverged       bool          `json:"tool_call_diverged"`
	BaselineError          bool          `json:"baseline_error"`
	CandidateError         bool          `json:"candidate_error"`
	DriftRationale         string        `json:"drift_rationale,omitempty"`
	Timestamp              time.Time     `json:"timestamp"`
}

// TrajectoryComparison represents an end-to-end evaluation between baseline and candidate model runs.
type TrajectoryComparison struct {
	SessionID              string           `json:"session_id"`
	BaselineModel          string           `json:"baseline_model"`
	CandidateModel         string           `json:"candidate_model"`
	TotalSteps             int              `json:"total_steps"`
	AverageSemanticScore   float64          `json:"average_semantic_score"`
	AverageDriftScore      float64          `json:"average_drift_score"`
	MaxStepDrift           float64          `json:"max_step_drift"`
	BaselineTotalLatencyMs int64            `json:"baseline_total_latency_ms"`
	CandidateTotalLatencyMs int64           `json:"candidate_total_latency_ms"`
	LatencyRatio           float64          `json:"latency_ratio"`
	BaselineTotalTokens    int              `json:"baseline_total_tokens"`
	CandidateTotalTokens   int              `json:"candidate_total_tokens"`
	TokenDeltaPercent      float64          `json:"token_delta_percent"`
	BaselineEstimatedCost  float64          `json:"baseline_estimated_cost"`
	CandidateEstimatedCost float64          `json:"candidate_estimated_cost"`
	CostDeltaPercent       float64          `json:"cost_delta_percent"`
	ToolDivergenceCount    int              `json:"tool_divergence_count"`
	StepComparisons        []StepComparison `json:"step_comparisons"`
	Passed                 bool             `json:"passed"`
	PromotionRecommended   bool             `json:"promotion_recommended"`
	RejectionReasons       []string         `json:"rejection_reasons,omitempty"`
}

// ShadowAuditReport provides an aggregated executive dashboard across shadowed fleet runs.
type ShadowAuditReport struct {
	AuditID                string                 `json:"audit_id"`
	Timestamp              time.Time              `json:"timestamp"`
	Mode                   TrafficRoutingMode     `json:"mode"`
	BaselineModel          ModelCandidate         `json:"baseline_model"`
	CandidateModel         ModelCandidate         `json:"candidate_model"`
	TotalSessionsShadowed  int                    `json:"total_sessions_shadowed"`
	TotalStepsEvaluated    int                    `json:"total_steps_evaluated"`
	GlobalAverageDrift     float64                `json:"global_average_drift"`
	GlobalLatencyRatio     float64                `json:"global_latency_ratio"`
	GlobalCostDeltaPercent float64                `json:"global_cost_delta_percent"`
	OverallErrorRate       float64                `json:"overall_error_rate"`
	CircuitBreakerTripped  bool                   `json:"circuit_breaker_tripped"`
	CircuitBreakerReason   string                 `json:"circuit_breaker_reason,omitempty"`
	Status                 string                 `json:"status"` // "PROMOTABLE", "OBSERVING", "BLOCKED", "ROLLED_BACK"
	Sessions               []TrajectoryComparison `json:"sessions"`
	Recommendations        []string               `json:"recommendations"`
}
