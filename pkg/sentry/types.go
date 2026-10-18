package sentry

import (
	"time"
)

// AnomalyType categorizes the specific runtime trajectory deviation.
type AnomalyType string

const (
	AnomalyToolLoop          AnomalyType = "TOOL_REPETITION_LOOP"
	AnomalyTokenVelocity     AnomalyType = "TOKEN_VELOCITY_SPIKE"
	AnomalyLatencySpike      AnomalyType = "LATENCY_OUTLIER_BREACH"
	AnomalyErrorCascade      AnomalyType = "ERROR_CASCADE_STORM"
	AnomalyContextExplosion  AnomalyType = "CONTEXT_EXPLOSION_RUNAWAY"
	AnomalyOscillation       AnomalyType = "BIPARTITE_TOOL_OSCILLATION"
)

// Severity indicates the threat level of an detected anomaly.
type Severity string

const (
	SeverityLow      Severity = "LOW"
	SeverityMedium   Severity = "MEDIUM"
	SeverityHigh     Severity = "HIGH"
	SeverityCritical Severity = "CRITICAL"
)

// CircuitAction defines the automated circuit breaker defense mechanism.
type CircuitAction string

const (
	ActionPass                CircuitAction = "ALLOW"
	ActionThrottle            CircuitAction = "THROTTLE"
	ActionRequireHumanApproval CircuitAction = "REQUIRE_APPROVAL"
	ActionTripCircuitBreaker  CircuitAction = "TRIP_CIRCUIT_BREAKER"
)

// AnomalyEvent records an anomalous event detected during trajectory evaluation.
type AnomalyEvent struct {
	Type        AnomalyType `json:"type"`
	Severity    Severity    `json:"severity"`
	StepIndex   int         `json:"step_index"`
	ToolName    string      `json:"tool_name,omitempty"`
	Message     string      `json:"message"`
	ObservedVal float64     `json:"observed_val"`
	Threshold   float64     `json:"threshold"`
	Timestamp   time.Time   `json:"timestamp"`
}

// SentryMetrics captures aggregated runtime telemetry across a trajectory.
type SentryMetrics struct {
	TotalSteps         int     `json:"total_steps"`
	TotalTokens        int     `json:"total_tokens"`
	AvgStepLatencyMs   float64 `json:"avg_step_latency_ms"`
	MaxStepLatencyMs   float64 `json:"max_step_latency_ms"`
	StdDevLatencyMs    float64 `json:"stddev_latency_ms"`
	TokenBurnRate      float64 `json:"token_burn_rate"` // Tokens per second
	RepetitionIndex    float64 `json:"repetition_index"` // 0.0 to 1.0
	ErrorCascadeCount  int     `json:"error_cascade_count"`
	ConsecutiveErrors  int     `json:"consecutive_errors"`
}

// SentryVerdict represents the comprehensive health appraisal of a trajectory.
type SentryVerdict struct {
	Healthy         bool           `json:"healthy"`
	Action          CircuitAction  `json:"action"`
	RiskScore       float64        `json:"risk_score"` // 0.0 to 100.0
	Summary         string         `json:"summary"`
	Anomalies       []AnomalyEvent `json:"anomalies"`
	Metrics         SentryMetrics  `json:"metrics"`
	EvaluatedAt     time.Time      `json:"evaluated_at"`
	EvaluationDurUs int64          `json:"evaluation_dur_us"`
}

// SentryConfig configures anomaly thresholds and circuit-breaker tripping rules.
type SentryConfig struct {
	MaxToolRepetition      int     `json:"max_tool_repetition"`
	MaxOscillationCycles   int     `json:"max_oscillation_cycles"`
	MaxTokenVelocity       float64 `json:"max_token_velocity"`       // Tokens / sec
	LatencyZScoreThreshold float64 `json:"latency_zscore_threshold"` // Multiples of stddev
	MaxConsecutiveErrors   int     `json:"max_consecutive_errors"`
	ThrottleRiskThreshold  float64 `json:"throttle_risk_threshold"`  // e.g. 40.0
	TripRiskThreshold      float64 `json:"trip_risk_threshold"`      // e.g. 75.0
}

// DefaultSentryConfig returns standard enterprise-grade anomaly sentry guardrails.
func DefaultSentryConfig() SentryConfig {
	return SentryConfig{
		MaxToolRepetition:      3,
		MaxOscillationCycles:   2,
		MaxTokenVelocity:       250.0,
		LatencyZScoreThreshold: 2.8,
		MaxConsecutiveErrors:   3,
		ThrottleRiskThreshold:  40.0,
		TripRiskThreshold:      75.0,
	}
}
