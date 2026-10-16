package canary

import (
	"time"
)

// ChaosType defines the failure mode injected into the candidate execution stream.
type ChaosType string

const (
	ChaosNone            ChaosType = "none"
	ChaosLatencySpike    ChaosType = "latency_spike"     // Artificially delays responses
	ChaosHTTPError       ChaosType = "http_error"        // Injects 500, 502, or 429 status codes
	ChaosCorruptPayload  ChaosType = "corrupt_payload"   // Mutates JSON keys or tool payloads
	ChaosToolFailure     ChaosType = "tool_failure"      // Injects simulated tool execution failure
	ChaosDropEvent       ChaosType = "drop_event"        // Simulates network drops or timeouts
)

// CanaryConfig holds traffic shadowing and chaos parameters.
type CanaryConfig struct {
	ShadowPercentage float64   `json:"shadow_percentage"` // 0.0 to 100.0% traffic mirrored
	ChaosRate        float64   `json:"chaos_rate"`        // Probability of chaos injection (0.0 to 1.0)
	ChaosMode        ChaosType `json:"chaos_mode"`        // Selected chaos pattern
	LatencyDelayMs   int64     `json:"latency_delay_ms"`  // Milliseconds to inject for LatencySpike
	MaxAllowedErrorRate float64 `json:"max_allowed_error_rate"` // Circuit breaker threshold (e.g. 0.05)
	MaxDivergenceRate  float64 `json:"max_divergence_rate"`  // Circuit breaker divergence ceiling
	AutoRollback     bool      `json:"auto_rollback"`     // Automatically trip circuit breaker and rollback
}

// DefaultCanaryConfig returns production defaults for canary shadowing.
func DefaultCanaryConfig() CanaryConfig {
	return CanaryConfig{
		ShadowPercentage:    100.0,
		ChaosRate:           0.20,
		ChaosMode:           ChaosLatencySpike,
		LatencyDelayMs:      250,
		MaxAllowedErrorRate: 0.10,
		MaxDivergenceRate:   0.15,
		AutoRollback:        true,
	}
}

// ShadowExecution records the side-by-side execution comparison of a single step.
type ShadowExecution struct {
	StepIndex        int         `json:"step_index"`
	BaselinePayload  string      `json:"baseline_payload"`
	CanaryPayload    string      `json:"canary_payload"`
	ChaosInjected    ChaosType   `json:"chaos_injected"`
	BaselineDuration int64       `json:"baseline_duration_ms"`
	CanaryDuration   int64       `json:"canary_duration_ms"`
	Diverged         bool        `json:"diverged"`
	CanaryError      bool        `json:"canary_error"`
	ErrorMessage     string      `json:"error_message,omitempty"`
	Timestamp        time.Time   `json:"timestamp"`
}

// CanaryReport details the holistic safety and performance evaluation of canary shadowing.
type CanaryReport struct {
	SessionID            string            `json:"session_id"`
	TotalStepsShadowed   int               `json:"total_steps_shadowed"`
	ChaosInjectedCount   int               `json:"chaos_injected_count"`
	DivergentStepsCount  int               `json:"divergent_steps_count"`
	CanaryErrorCount     int               `json:"canary_error_count"`
	DivergenceRate       float64           `json:"divergence_rate"`
	CanaryErrorRate      float64           `json:"canary_error_rate"`
	CircuitBreakerTripped bool             `json:"circuit_breaker_tripped"`
	RollbackTriggered    bool              `json:"rollback_triggered"`
	Executions           []ShadowExecution `json:"executions"`
	Recommendations      []string          `json:"recommendations"`
	Passed               bool              `json:"passed"`
}
