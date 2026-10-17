package finops

import (
	"time"
)

// EnforcementAction specifies the consequence when an agent exceeds budget thresholds.
type EnforcementAction string

const (
	ActionAllow     EnforcementAction = "ALLOW"      // Normal operation within limits
	ActionWarn      EnforcementAction = "WARN"       // Approaching threshold (e.g. 80%)
	ActionThrottle  EnforcementAction = "THROTTLE"   // Inject synthetic latency / reduce concurrency
	ActionDowngrade EnforcementAction = "DOWNGRADE"  // Automatically route to cheaper model tier
	ActionHardKill  EnforcementAction = "HARD_KILL"  // Cutoff execution immediately to prevent runaway billing
)

// ModelPricing defines token rates per 1,000 tokens for precise real-time cost computation.
type ModelPricing struct {
	ModelName          string  `json:"model_name"`
	InputUSDPer1k      float64 `json:"input_usd_per_1k"`
	OutputUSDPer1k     float64 `json:"output_usd_per_1k"`
	CachedInputUSDPer1k float64 `json:"cached_input_usd_per_1k"`
}

// DefaultModelPricings returns real-world pricing models for major frontier LLMs.
func DefaultModelPricings() map[string]ModelPricing {
	return map[string]ModelPricing{
		"gpt-4o": {
			ModelName:           "gpt-4o",
			InputUSDPer1k:       0.0050,
			OutputUSDPer1k:      0.0150,
			CachedInputUSDPer1k: 0.0025,
		},
		"gpt-4o-mini": {
			ModelName:           "gpt-4o-mini",
			InputUSDPer1k:       0.00015,
			OutputUSDPer1k:      0.00060,
			CachedInputUSDPer1k: 0.000075,
		},
		"o1": {
			ModelName:           "o1",
			InputUSDPer1k:       0.0150,
			OutputUSDPer1k:      0.0600,
			CachedInputUSDPer1k: 0.0075,
		},
		"claude-3-7-sonnet": {
			ModelName:           "claude-3-7-sonnet",
			InputUSDPer1k:       0.0030,
			OutputUSDPer1k:      0.0150,
			CachedInputUSDPer1k: 0.0015,
		},
		"claude-3-5-haiku": {
			ModelName:           "claude-3-5-haiku",
			InputUSDPer1k:       0.0008,
			OutputUSDPer1k:      0.0040,
			CachedInputUSDPer1k: 0.0004,
		},
	}
}

// BudgetPolicy defines financial and token caps for an agent, tenant, or trajectory session.
type BudgetPolicy struct {
	PolicyID           string        `json:"policy_id"`
	AgentID            string        `json:"agent_id"`
	MaxCostUSD         float64       `json:"max_cost_usd"`          // Hard financial spend ceiling
	MaxTotalTokens     int64         `json:"max_total_tokens"`      // Total token ceiling
	MaxCostPerMinute   float64       `json:"max_cost_per_minute"`   // Velocity ceiling (spikes/runaway loops)
	WarnThresholdPct   float64       `json:"warn_threshold_pct"`    // e.g., 0.80 for 80% warning
	ThrottleThresholdPct float64     `json:"throttle_threshold_pct"`// e.g., 0.90 for throttling
	DowngradeModel     string        `json:"downgrade_model,omitempty"` // Fallback cheap model (e.g. gpt-4o-mini)
	EnforceHardLimit   bool          `json:"enforce_hard_limit"`
	SlidingWindow      time.Duration `json:"sliding_window"`
}

// DefaultBudgetPolicy provides standard defensive constraints for autonomous agent swarms.
func DefaultBudgetPolicy(agentID string, maxCostUSD float64) BudgetPolicy {
	if maxCostUSD <= 0 {
		maxCostUSD = 1.00 // $1.00 default session safety ceiling
	}
	return BudgetPolicy{
		PolicyID:             "defensive-finops-" + agentID,
		AgentID:              agentID,
		MaxCostUSD:           maxCostUSD,
		MaxTotalTokens:       500000,
		MaxCostPerMinute:     maxCostUSD * 0.50,
		WarnThresholdPct:     0.75,
		ThrottleThresholdPct: 0.90,
		DowngradeModel:       "gpt-4o-mini",
		EnforceHardLimit:     true,
		SlidingWindow:        time.Minute,
	}
}

// TokenUsageDelta records an individual consumption event.
type TokenUsageDelta struct {
	ModelName        string    `json:"model_name"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	CachedTokens     int       `json:"cached_tokens"`
	CostUSD          float64   `json:"cost_usd"`
	Timestamp        time.Time `json:"timestamp"`
}

// FinOpsTelemetry represents aggregate live accounting and quota burn-rate.
type FinOpsTelemetry struct {
	AgentID             string            `json:"agent_id"`
	CurrentAction       EnforcementAction `json:"current_action"`
	TotalCostUSD        float64           `json:"total_cost_usd"`
	RemainingBudgetUSD  float64           `json:"remaining_budget_usd"`
	BudgetUtilization   float64           `json:"budget_utilization_pct"` // 0.0 - 1.0+
	TotalPromptTokens   int64             `json:"total_prompt_tokens"`
	TotalCompletionTokens int64           `json:"total_completion_tokens"`
	TotalCachedTokens   int64             `json:"total_cached_tokens"`
	BurnRateUSDPerMin   float64           `json:"burn_rate_usd_per_min"`
	ActiveModelOverride string            `json:"active_model_override,omitempty"`
	CircuitTripped      bool              `json:"circuit_tripped"`
	HardKillReason      string            `json:"hard_kill_reason,omitempty"`
	TotalInvocations    int64             `json:"total_invocations"`
}

// FinOpsAuditReport summarizes global financial compliance across multi-agent swarms.
type FinOpsAuditReport struct {
	SystemID         string            `json:"system_id"`
	Timestamp        time.Time         `json:"timestamp"`
	TotalAgents      int               `json:"total_agents"`
	AllCompliant     bool              `json:"all_compliant"`
	TotalCostUSD     float64           `json:"total_cost_usd"`
	TotalTokens      int64             `json:"total_tokens"`
	TotalDollarsSaved float64          `json:"total_dollars_saved"` // Savings from downgrades & caching
	AgentsTelemetry  []FinOpsTelemetry `json:"agents_telemetry"`
}
