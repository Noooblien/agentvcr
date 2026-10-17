package compact

import (
	"time"
)

// CompactorStrategy defines the aggressiveness of context compaction.
type CompactorStrategy string

const (
	StrategyLossless   CompactorStrategy = "lossless"   // Only deduplicate identical outputs & clean whitespace
	StrategyAggressive CompactorStrategy = "aggressive" // Collapse retries, prune stale reads, truncate verbose tools
	StrategyKVCache    CompactorStrategy = "kv-cache"   // Reorganize prompt prefixes to maximize upstream KV-cache hits
)

// CompactorConfig configures the compaction engine parameters.
type CompactorConfig struct {
	Strategy               CompactorStrategy `json:"strategy"`
	DeduplicateTools       bool              `json:"deduplicate_tools"`
	CollapseRetries        bool              `json:"collapse_retries"`
	PruneStaleReads        bool              `json:"prune_stale_reads"`
	OptimizeKVCache        bool              `json:"optimize_kv_cache"`
	MaxToolResultTokens    int               `json:"max_tool_result_tokens"`
	MinSavingsThresholdPct float64           `json:"min_savings_threshold_pct"` // E.g. 15.0%
}

// DefaultCompactorConfig returns recommended production defaults.
func DefaultCompactorConfig() CompactorConfig {
	return CompactorConfig{
		Strategy:               StrategyLossless,
		DeduplicateTools:       true,
		CollapseRetries:        true,
		PruneStaleReads:        true,
		OptimizeKVCache:        true,
		MaxToolResultTokens:    1024,
		MinSavingsThresholdPct: 10.0,
	}
}

// RedundancyAnomaly represents a detected source of token waste in the trajectory.
type RedundancyAnomaly struct {
	StepIndex    int    `json:"step_index"`
	EventType    string `json:"event_type"`
	ToolName     string `json:"tool_name,omitempty"`
	Category     string `json:"category"` // "DUPLICATE_TOOL", "RETRY_LOOP", "VERBOSE_OUTPUT", "KV_CACHE_JITTER"
	TokensWasted int    `json:"tokens_wasted"`
	Details      string `json:"details"`
}

// TokenProfile provides deep analytical breakdown of a session's token consumption.
type TokenProfile struct {
	SessionID             string              `json:"session_id"`
	TotalTokens           int                 `json:"total_tokens"`
	PromptTokens          int                 `json:"prompt_tokens"`
	CompletionTokens      int                 `json:"completion_tokens"`
	ToolPayloadTokens     int                 `json:"tool_payload_tokens"`
	RedundantTokens       int                 `json:"redundant_tokens"`
	WastePercentage       float64             `json:"waste_percentage"`
	EstimatedCostUSD      float64             `json:"estimated_cost_usd"`
	Anomalies             []RedundancyAnomaly `json:"anomalies"`
	KVCacheHitRatioEst    float64             `json:"kv_cache_hit_ratio_est"`
	AnalyzedAt            time.Time           `json:"analyzed_at"`
}

// CompactionReport summarizes the results and cost savings of a compaction run.
type CompactionReport struct {
	OriginalSessionID     string    `json:"original_session_id"`
	CompactedSessionID    string    `json:"compacted_session_id"`
	OriginalTokens        int       `json:"original_tokens"`
	CompactedTokens       int       `json:"compacted_tokens"`
	TokensSaved           int       `json:"tokens_saved"`
	ReductionPercent      float64   `json:"reduction_percent"`
	OriginalCostUSD       float64   `json:"original_cost_usd"`
	CompactedCostUSD      float64   `json:"compacted_cost_usd"`
	SavingsUSD            float64   `json:"savings_usd"`
	DuplicateToolsPruned  int       `json:"duplicate_tools_pruned"`
	RetriesCollapsed      int       `json:"retries_collapsed"`
	StaleReadsTrimmed     int       `json:"stale_reads_trimmed"`
	KVCachePrefixAligned  bool      `json:"kv_cache_prefix_aligned"`
	SemanticFidelityScore float64   `json:"semantic_fidelity_score"` // 0.0 to 100.0%
	MeetsThreshold        bool      `json:"meets_threshold"`
	Timestamp             time.Time `json:"timestamp"`
}
