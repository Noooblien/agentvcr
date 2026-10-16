package throttle

import (
	"time"
)

// ThrottlePolicy defines rate limits and latency SLA budgets for an agent run.
type ThrottlePolicy struct {
	MaxRPM              int     `json:"max_rpm"`                // Max requests per minute
	MaxTPM              int     `json:"max_tpm"`                // Max tokens per minute
	MaxCostUSDPerHour   float64 `json:"max_cost_usd_per_hour"`  // Max dollar burn rate per hour
	TotalLatencyBudgetMs int64  `json:"total_latency_budget_ms"` // Total trajectory wall-clock latency ceiling
	PerStepBudgetMs     int64   `json:"per_step_budget_ms"`     // Per-step execution latency ceiling
	BurstCapacity       int     `json:"burst_capacity"`         // Token bucket burst allowance
}

// DefaultThrottlePolicy provides sensible production defaults for autonomous agent orchestration.
func DefaultThrottlePolicy() ThrottlePolicy {
	return ThrottlePolicy{
		MaxRPM:               60,     // 1 req/sec
		MaxTPM:               100000, // 100k TPM
		MaxCostUSDPerHour:    25.0,   // $25/hr
		TotalLatencyBudgetMs: 30000,  // 30s total budget
		PerStepBudgetMs:      5000,   // 5s per step
		BurstCapacity:        10,
	}
}

// ThrottleDecision contains the real-time enforcement decision for a pending execution step.
type ThrottleDecision struct {
	Allowed         bool          `json:"allowed"`
	DelayRequired   time.Duration `json:"delay_required"`
	ThrottleReason  string        `json:"throttle_reason,omitempty"`
	RemainingRPM    int           `json:"remaining_rpm"`
	RemainingTPM    int           `json:"remaining_tpm"`
	BudgetBurnedMs  int64         `json:"budget_burned_ms"`
	BudgetRemainingMs int64       `json:"budget_remaining_ms"`
	ExhaustedBudget bool          `json:"exhausted_budget"`
}

// TokenBucket represents a thread-safe token bucket rate limiter.
type TokenBucket struct {
	Capacity     float64   `json:"capacity"`
	Tokens       float64   `json:"tokens"`
	RefillRate   float64   `json:"refill_rate"` // tokens added per second
	LastRefillAt time.Time `json:"last_refill_at"`
}

// LatencyAllocation models dynamic remaining budget distribution across remaining steps.
type LatencyAllocation struct {
	StepIndex        int   `json:"step_index"`
	TargetBudgetMs   int64 `json:"target_budget_ms"`
	ActualDurationMs int64 `json:"actual_duration_ms"`
	DeltaMs          int64 `json:"delta_ms"`
	RemainingTotalMs int64 `json:"remaining_total_ms"`
}

// ThrottleAuditReport provides post-hoc analytics on rate-limiting incidents and SLA breaches.
type ThrottleAuditReport struct {
	SessionID             string              `json:"session_id"`
	AgentName             string              `json:"agent_name"`
	TotalSteps            int                 `json:"total_steps"`
	TotalDurationMs       int64               `json:"total_duration_ms"`
	AllowedRequests       int                 `json:"allowed_requests"`
	ThrottledRequests     int                 `json:"throttled_requests"`
	TotalDelayIncurredMs  int64               `json:"total_delay_incurred_ms"`
	TokenQuotaExceeded    bool                `json:"token_quota_exceeded"`
	LatencySLABreached    bool                `json:"latency_sla_breached"`
	Allocations           []LatencyAllocation `json:"allocations"`
	Breaches              []string            `json:"breaches"`
	ComplianceScore       float64             `json:"compliance_score"` // 0.0 to 100.0%
}
