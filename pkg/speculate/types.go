package speculate

import (
	"time"
)

// BranchStatus represents the state of a speculative execution branch.
type BranchStatus string

const (
	BranchPending    BranchStatus = "pending"
	BranchCommitted  BranchStatus = "committed"  // Prediction matched upstream ground truth
	BranchRolledBack BranchStatus = "rolled_back" // Misprediction detected; branch discarded
	BranchCached     BranchStatus = "cached"      // Preserved in warm cache for future trajectory branches
)

// SpeculativeStep represents an anticipated trajectory event executed speculatively.
type SpeculativeStep struct {
	StepIndex        int                    `json:"step_index"`
	PredictedType    string                 `json:"predicted_type"` // e.g. "tool_call", "llm_response"
	PredictedPayload map[string]interface{} `json:"predicted_payload"`
	ConfidenceScore  float64                `json:"confidence_score"` // 0.0 to 1.0
	WallClockSavedMs int64                  `json:"wall_clock_saved_ms"`
	Timestamp        time.Time              `json:"timestamp"`
}

// SpeculativeBranch models an execution path branched ahead of upstream completion.
type SpeculativeBranch struct {
	BranchID         string            `json:"branch_id"`
	SessionID        string            `json:"session_id"`
	BaseStepIndex    int               `json:"base_step_index"`
	Status           BranchStatus      `json:"status"`
	CreatedAt        time.Time         `json:"created_at"`
	ResolvedAt       time.Time         `json:"resolved_at,omitempty"`
	Steps            []SpeculativeStep `json:"steps"`
	BranchSnapshot   map[string]interface{} `json:"branch_snapshot,omitempty"`
	RollbackReason   string            `json:"rollback_reason,omitempty"`
	MispredictedStep int               `json:"mispredicted_step,omitempty"`
}

// CacheEntry caches high-probability predictive state branches keyed by historical trajectory context hash.
type CacheEntry struct {
	ContextHash     string          `json:"context_hash"`
	LastHitAt       time.Time       `json:"last_hit_at"`
	HitCount        int             `json:"hit_count"`
	PredictedAction string          `json:"predicted_action"`
	CachedPayload   map[string]interface{} `json:"cached_payload"`
	SuccessRate     float64         `json:"success_rate"`
}

// SpeculateConfig controls speculative execution and predictive cache parameters.
type SpeculateConfig struct {
	MinConfidenceThreshold float64 `json:"min_confidence_threshold"` // e.g. 0.70
	MaxSpeculativeDepth    int     `json:"max_speculative_depth"`    // max forward steps (e.g. 3)
	CacheCapacity          int     `json:"cache_capacity"`           // max entries in predictive cache
	AutoRollbackOnMismatch bool    `json:"auto_rollback_on_mismatch"`
}

// DefaultSpeculateConfig returns production-ready speculative execution defaults.
func DefaultSpeculateConfig() SpeculateConfig {
	return SpeculateConfig{
		MinConfidenceThreshold: 0.75,
		MaxSpeculativeDepth:    3,
		CacheCapacity:          1000,
		AutoRollbackOnMismatch: true,
	}
}

// SpeculateReport provides comprehensive analytics on speculative branch efficiency.
type SpeculateReport struct {
	SessionID              string               `json:"session_id"`
	TotalBranches          int                  `json:"total_branches"`
	CommittedBranches      int                  `json:"committed_branches"`
	RolledBackBranches     int                  `json:"rolled_back_branches"`
	CachedHits             int                  `json:"cached_hits"`
	CacheHitRatio          float64              `json:"cache_hit_ratio"`       // 0.0 to 100.0%
	BranchPredictionAccuracy float64            `json:"branch_prediction_accuracy"` // 0.0 to 100.0%
	NetWallClockSavedMs    int64                `json:"net_wall_clock_saved_ms"`
	Branches               []*SpeculativeBranch `json:"branches"`
}
