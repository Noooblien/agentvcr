package memory

import (
	"math"
	"time"
)

// DecayEngine evaluates exponential forgetting curves and handles retrieval reinforcement.
type DecayEngine struct {
	decayRate           float64
	minRetentionScore   float64
	accessReinforcement float64
}

// NewDecayEngine initializes an Ebbinghaus memory decay calculator.
func NewDecayEngine(cfg MemoryConfig) *DecayEngine {
	rate := cfg.DecayRate
	if rate <= 0 {
		rate = 0.05
	}
	minScore := cfg.MinRetentionScore
	if minScore <= 0 {
		minScore = 0.15
	}
	reinf := cfg.AccessReinforcement
	if reinf <= 0 {
		reinf = 0.25
	}

	return &DecayEngine{
		decayRate:           rate,
		minRetentionScore:   minScore,
		accessReinforcement: reinf,
	}
}

// RetentionScore computes current recall probability / retention score using Ebbinghaus curve:
// R(t) = (Stability * Importance) * exp(-lambda * delta_hours) + 0.15 * ln(1 + AccessCount)
// Semantic facts with high importance remain permanently durable, while trivial unaccessed episodes fade.
func (de *DecayEngine) RetentionScore(node *MemoryNode, now time.Time) float64 {
	if node == nil {
		return 0.0
	}

	// Permanent semantic facts with importance >= 0.9 have infinite retention baseline
	if node.Level == LevelSemantic && node.Importance >= 0.90 {
		return 1.0
	}

	delta := now.Sub(node.LastAccessedAt)
	if delta < 0 {
		delta = 0
	}
	deltaHours := delta.Hours()

	// Initial strength factor
	stability := node.Stability
	if stability <= 0 {
		stability = 1.0
	}
	importance := node.Importance
	if importance <= 0 {
		importance = 0.1
	}

	baseRetention := (stability * importance) * math.Exp(-de.decayRate*deltaHours)

	// Repetition & retrieval reinforcement bonus
	frequencyBonus := 0.10 * math.Log1p(float64(node.AccessCount))

	score := baseRetention + frequencyBonus
	if score > 1.0 {
		score = 1.0
	}
	if score < 0.0 {
		score = 0.0
	}

	return score
}

// Reinforce strengthens memory stability upon conscious retrieval / recall turn.
func (de *DecayEngine) Reinforce(node *MemoryNode, now time.Time) {
	if node == nil {
		return
	}
	node.AccessCount++
	node.LastAccessedAt = now
	// Increase stability with diminishing returns: S_new = S_old * (1 + reinforcement / sqrt(AccessCount))
	boost := de.accessReinforcement / math.Sqrt(float64(node.AccessCount))
	node.Stability += boost
}

// IsDecayed determines whether a memory node has decayed below the minimum retention threshold.
func (de *DecayEngine) IsDecayed(node *MemoryNode, now time.Time) bool {
	// Never prune high-importance semantic facts
	if node.Level == LevelSemantic && node.Importance >= 0.70 {
		return false
	}
	score := de.RetentionScore(node, now)
	return score < de.minRetentionScore
}
