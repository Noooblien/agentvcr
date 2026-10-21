package metrics

import (
	"time"
)

// GranularTokenBreakdown captures detailed prompt, completion, cache read/write, and reasoning token metrics.
type GranularTokenBreakdown struct {
	PromptTokens          int     `json:"prompt_tokens"`
	CompletionTokens      int     `json:"completion_tokens"`
	CachedPromptTokens    int     `json:"cached_prompt_tokens"`
	ReasoningTokens       int     `json:"reasoning_tokens"`
	TotalTokens           int     `json:"total_tokens"`
	CacheHitRatio         float64 `json:"cache_hit_ratio"`          // 0.0 to 1.0
	TokensPerSecond       float64 `json:"tokens_per_second"`
	EstimatedPromptCost   float64 `json:"estimated_prompt_cost_usd"`
	EstimatedCompCost     float64 `json:"estimated_comp_cost_usd"`
	EstimatedSavingsUSD   float64 `json:"estimated_savings_usd"`
	TotalEstimatedCostUSD float64 `json:"total_estimated_cost_usd"`
}

// ModelRateCard defines cost per 1M tokens for a specific frontier or local LLM.
type ModelRateCard struct {
	ModelID           string  `json:"model_id"`
	PromptPerMillion  float64 `json:"prompt_per_million_usd"`
	CompPerMillion    float64 `json:"comp_per_million_usd"`
	CachedPerMillion  float64 `json:"cached_per_million_usd"`
}

// StepTokenMetric captures token consumption at an individual step index.
type StepTokenMetric struct {
	StepIndex        int     `json:"step_index"`
	Model            string  `json:"model"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	CachedTokens     int     `json:"cached_tokens"`
	DurationMs       int64   `json:"duration_ms"`
	CostUSD          float64 `json:"cost_usd"`
}

// TokenAuditReport summarizes a trajectory's complete token lifecycle and efficiency.
type TokenAuditReport struct {
	SessionID            string                 `json:"session_id"`
	AgentName            string                 `json:"agent_name"`
	TotalSteps           int                    `json:"total_steps"`
	Breakdown            GranularTokenBreakdown `json:"breakdown"`
	StepMetrics          []StepTokenMetric      `json:"step_metrics"`
	TokenEfficiencyScore float64                `json:"token_efficiency_score"` // 0.0 - 100.0
	EfficiencyVerdict    string                 `json:"efficiency_verdict"`     // "OPTIMIZED", "NOMINAL", "TOKEN_BLOATED"
	AnalyzedAt           time.Time              `json:"analyzed_at"`
}

// DefaultModelRateCards returns industry-standard token pricing.
var DefaultModelRateCards = map[string]ModelRateCard{
	"claude-3-7-sonnet": {
		ModelID:          "claude-3-7-sonnet",
		PromptPerMillion: 3.00,
		CompPerMillion:   15.00,
		CachedPerMillion: 0.30,
	},
	"claude-3-5-sonnet": {
		ModelID:          "claude-3-5-sonnet",
		PromptPerMillion: 3.00,
		CompPerMillion:   15.00,
		CachedPerMillion: 0.30,
	},
	"gpt-4o": {
		ModelID:          "gpt-4o",
		PromptPerMillion: 2.50,
		CompPerMillion:   10.00,
		CachedPerMillion: 1.25,
	},
	"gemini-2.0-flash": {
		ModelID:          "gemini-2.0-flash",
		PromptPerMillion: 0.10,
		CompPerMillion:   0.40,
		CachedPerMillion: 0.025,
	},
	"default": {
		ModelID:          "default",
		PromptPerMillion: 2.50,
		CompPerMillion:   10.00,
		CachedPerMillion: 0.50,
	},
}
