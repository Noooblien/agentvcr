package chaos

import (
	"time"
)

// FaultType defines the taxonomy of injected execution failures.
type FaultType string

const (
	FaultToolTimeout       FaultType = "TOOL_TIMEOUT_HANG"
	FaultRateLimit         FaultType = "RATE_LIMIT_429"
	FaultUpstreamOutage    FaultType = "UPSTREAM_503_OUTAGE"
	FaultPayloadCorrupt    FaultType = "PAYLOAD_CORRUPT"
	FaultToolFlaky         FaultType = "TOOL_FLAKY_INTERMITTENT"
	FaultStateCorruption   FaultType = "STATE_CORRUPTION"
)

// AgentReaction characterizes how an agent handles an injected fault.
type AgentReaction string

const (
	ReactionRetryBackoff       AgentReaction = "RETRY_BACKOFF"
	ReactionFallbackTool       AgentReaction = "FALLBACK_TOOL"
	ReactionFatalCrash         AgentReaction = "FATAL_CRASH"
	ReactionUnhandledException AgentReaction = "UNHANDLED_EXCEPTION"
	ReactionGracefulDegrade    AgentReaction = "GRACEFUL_DEGRADE"
)

// ChaosStatus categorizes the overall resilience verdict.
type ChaosStatus string

const (
	StatusResilient       ChaosStatus = "RESILIENT"
	StatusDegraded        ChaosStatus = "DEGRADED"
	StatusCriticalFailure ChaosStatus = "CRITICAL_FAILURE"
)

// FaultRule configures the conditions and parameters for injecting a fault.
type FaultRule struct {
	ID             string        `json:"id"`
	TargetTool     string        `json:"target_tool,omitempty"` // Empty matches any tool
	TargetStep     int           `json:"target_step,omitempty"` // 0 matches any step
	Type           FaultType     `json:"type"`
	Probability    float64       `json:"probability"`      // 0.0 to 1.0
	InjectedDelay  time.Duration `json:"injected_delay"`   // Delay for timeout/hang
	MaxOccurrences int           `json:"max_occurrences"`  // Max times this rule can fire
	CustomMessage  string        `json:"custom_message,omitempty"`
}

// InjectedFaultEvent records a single occurrence of chaos injection during trajectory execution.
type InjectedFaultEvent struct {
	StepIndex        int           `json:"step_index"`
	ToolName         string        `json:"tool_name"`
	FaultType        FaultType     `json:"fault_type"`
	InjectedMessage  string        `json:"injected_message"`
	InjectedDelayMs  int64         `json:"injected_delay_ms"`
	Reaction         AgentReaction `json:"reaction"`
	Recovered        bool          `json:"recovered"`
	RecoveryTimeMs   int64         `json:"recovery_time_ms"`
	ExtraTokensSpent int           `json:"extra_tokens_spent"`
}

// ChaosExperimentConfig defines parameters for a chaos evaluation run.
type ChaosExperimentConfig struct {
	ExperimentName string      `json:"experiment_name"`
	SessionID      string      `json:"session_id"`
	Rules          []FaultRule `json:"rules"`
	ChaosSeed      int64       `json:"chaos_seed"`
	StrictMode     bool        `json:"strict_mode"`
	MaxRetries     int         `json:"max_retries"`
}

// ChaosExperimentReport aggregates empirical results and resilience telemetry.
type ChaosExperimentReport struct {
	ExperimentName         string               `json:"experiment_name"`
	SessionID              string               `json:"session_id"`
	AgentName              string               `json:"agent_name"`
	TotalStepsEvaluated    int                  `json:"total_steps_evaluated"`
	TotalFaultsInjected    int                  `json:"total_faults_injected"`
	TotalFaultsRecovered   int                  `json:"total_faults_recovered"`
	RecoveryRatePct        float64              `json:"recovery_rate_pct"`
	ResilienceScore        float64              `json:"resilience_score"` // 0.0 to 100.0
	Status                 ChaosStatus          `json:"status"`
	MeanTimeToRecoveryMs   float64              `json:"mean_time_to_recovery_ms"`
	TokenBlastRadiusTokens int                  `json:"token_blast_radius_tokens"`
	Passed                 bool                 `json:"passed"`
	Events                 []InjectedFaultEvent `json:"events"`
	Duration               time.Duration        `json:"duration"`
}

// ChaosBenchmarkSuite aggregates multiple chaos experiment scenarios.
type ChaosBenchmarkSuite struct {
	Timestamp            time.Time               `json:"timestamp"`
	TotalScenarios       int                     `json:"total_scenarios"`
	PassedScenarios      int                     `json:"passed_scenarios"`
	OverallResilience    float64                 `json:"overall_resilience"`
	TotalFaultsInjected  int                     `json:"total_faults_injected"`
	TotalFaultsRecovered int                     `json:"total_faults_recovered"`
	OverallRecoveryRate  float64                 `json:"overall_recovery_rate"`
	Reports              []ChaosExperimentReport `json:"reports"`
}
