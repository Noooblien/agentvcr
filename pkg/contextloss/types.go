package contextloss

import "time"

// FidelityTier indicates the factual and constraint preservation quality of a compacted context.
type FidelityTier string

const (
	TierHighFidelity    FidelityTier = "HIGH_FIDELITY"     // CDS >= 90.0: Flawless constraint & entity preservation
	TierModerateBleed   FidelityTier = "MODERATE_BLEED"    // 70.0 <= CDS < 90.0: Minor entity bleed, constraints intact
	TierCriticalLoss    FidelityTier = "CRITICAL_LOSS"     // CDS < 70.0: Constraint omission or negative inversion
)

// LossCategory classifies the nature of information degradation across context compaction.
type LossCategory string

const (
	LossEntityBleed         LossCategory = "ENTITY_BLEED"          // File paths, IDs, or URLs dropped
	LossConstraintInversion LossCategory = "CONSTRAINT_INVERSION"  // Negative prohibition inverted into positive action
	LossConstraintOmission  LossCategory = "CONSTRAINT_OMISSION"   // Prohibitions omitted entirely from summary
	LossGhostFact           LossCategory = "GHOST_FACT_INJECTION"  // Facts/claims hallucinated in compaction
	LossTemporalDistortion  LossCategory = "TEMPORAL_DISTORTION"   // Precedence causality inverted
)

// EntityFact captures a critical technical entity identified in the original trajectory.
type EntityFact struct {
	Type              string `json:"type"` // "path", "url", "commit", "identifier", "command"
	Value             string `json:"value"`
	FoundInCompacted  bool   `json:"found_in_compacted"`
}

// NegativeConstraint represents an explicit negative safety instruction (e.g., "never delete table").
type NegativeConstraint struct {
	OriginalText      string `json:"original_text"`
	ProhibitedAction  string `json:"prohibited_action"`
	StatusInCompacted string `json:"status_in_compacted"` // "RETAINED", "OMITTED", "INVERTED"
	Severity          string `json:"severity"`            // "CRITICAL", "HIGH", "MEDIUM"
}

// GhostFact represents an assertion or entity hallucinated in the compacted summary.
type GhostFact struct {
	Claim             string `json:"claim"`
	Entity            string `json:"entity"`
	Explanation       string `json:"explanation"`
	Severity          string `json:"severity"`
}

// CompactionAssessment records the multi-dimensional fidelity analysis of a compacted context.
type CompactionAssessment struct {
	SessionID                string                `json:"session_id"`
	OriginalTokens           int                   `json:"original_tokens"`
	CompactedTokens          int                   `json:"compacted_tokens"`
	CompressionRatio         float64               `json:"compression_ratio"`          // OriginalTokens / CompactedTokens
	ContextDegradationScore  float64               `json:"context_degradation_score"` // 0.0 to 100.0 (Higher = Better Fidelity)
	Tier                     FidelityTier          `json:"tier"`
	EntityRecall             float64               `json:"entity_recall"`              // 0.0 to 1.0
	ConstraintFidelity       float64               `json:"constraint_fidelity"`        // 0.0 to 1.0
	GhostPenalty             float64               `json:"ghost_penalty"`              // 0.0 to 1.0
	EntitiesAudited          []EntityFact          `json:"entities_audited"`
	ConstraintsAudited       []NegativeConstraint  `json:"constraints_audited"`
	GhostFacts               []GhostFact           `json:"ghost_facts"`
	Violations               []string              `json:"violations"`
	Duration                 time.Duration         `json:"duration"`
}

// ContextLossSummary aggregates compaction fidelity telemetry across multiple sessions.
type ContextLossSummary struct {
	TotalEvaluated       int                     `json:"total_evaluated"`
	HighFidelityCount    int                     `json:"high_fidelity_count"`
	ModerateBleedCount   int                     `json:"moderate_bleed_count"`
	CriticalLossCount    int                     `json:"critical_loss_count"`
	AverageCDS           float64                 `json:"average_cds"`
	AverageCompression   float64                 `json:"average_compression"`
	Assessments          []*CompactionAssessment `json:"assessments"`
	Duration             time.Duration           `json:"duration"`
}

// ContextLossConfig configures the scoring weights and constraint audit thresholds.
type ContextLossConfig struct {
	WeightEntityRecall     float64  `json:"weight_entity_recall"`     // Default: 0.35
	WeightConstraintFidelity float64 `json:"weight_constraint_fidelity"` // Default: 0.45
	WeightGhostPenalty     float64  `json:"weight_ghost_penalty"`     // Default: 0.20
	MinAcceptableCDS       float64  `json:"min_acceptable_cds"`       // Default: 75.0
	NegativeKeywords       []string `json:"negative_keywords"`
}

// DefaultContextLossConfig returns calibrated production defaults.
func DefaultContextLossConfig() ContextLossConfig {
	return ContextLossConfig{
		WeightEntityRecall:       0.40,
		WeightConstraintFidelity: 0.60,
		WeightGhostPenalty:       0.20,
		MinAcceptableCDS:         75.0,
		NegativeKeywords: []string{
			"never", "must not", "do not", "cannot", "prohibited", "forbidden", "disallow", "refrain",
		},
	}
}
