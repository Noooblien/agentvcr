package drift

import (
	"fmt"
	"strings"
	"time"
)

// TransitionDrift records an individual action transition whose probability drifted significantly.
type TransitionDrift struct {
	FromState     string  `json:"from_state"`
	ToState       string  `json:"to_state"`
	BaselineProb  float64 `json:"baseline_prob"`
	TargetProb    float64 `json:"target_prob"`
	ProbDelta     float64 `json:"prob_delta"`
	AnomalyScore  float64 `json:"anomaly_score"`
}

// TrajectoryProfile captures the statistical fingerprint of an agent's execution behavior.
type TrajectoryProfile struct {
	SessionID             string                        `json:"session_id"`
	TotalEvents           int                           `json:"total_events"`
	ToolCounts            map[string]int                `json:"tool_counts"`
	ToolProbabilities     map[string]float64            `json:"tool_probabilities"`
	TransitionMatrix      map[string]map[string]float64 `json:"transition_matrix"` // P(state_{t+1} | state_t)
	ActionEntropy         float64                       `json:"action_entropy"`    // Shannon entropy of tool distribution
	AverageStepDurationMs float64                       `json:"avg_step_duration_ms"`
}

// SynthesizedGuardrailPolicy represents an automated corrective policy synthesized to counter drift.
type SynthesizedGuardrailPolicy struct {
	PolicyID              string            `json:"policy_id"`
	GeneratedAt           time.Time         `json:"generated_at"`
	TargetSessionID       string            `json:"target_session_id"`
	RestrictedTransitions []string          `json:"restricted_transitions"` // e.g. "tool_read -> tool_exec_shell"
	ToolFrequencyCeilings map[string]int    `json:"tool_frequency_ceilings"` // Max allowed calls per tool
	RecommendedPromptMod  string            `json:"recommended_prompt_mod"`
}

// DriftAnalysis details the behavioral divergence between baseline and candidate trajectories.
type DriftAnalysis struct {
	BaselineSessionID   string                      `json:"baseline_session_id"`
	TargetSessionID     string                      `json:"target_session_id"`
	JSDivergence        float64                     `json:"js_divergence"` // Jensen-Shannon Divergence [0.0, 1.0]
	EntropyDelta        float64                     `json:"entropy_delta"`
	DriftDetected       bool                        `json:"drift_detected"`
	ThresholdUsed       float64                     `json:"threshold_used"`
	DriftedTransitions  []TransitionDrift           `json:"drifted_transitions"`
	SynthesizedPolicy   *SynthesizedGuardrailPolicy `json:"synthesized_policy,omitempty"`
	AnalyzedAt          time.Time                   `json:"analyzed_at"`
}

// FormatSummary returns a concise summary of the drift audit.
func (da *DriftAnalysis) FormatSummary() string {
	if !da.DriftDetected {
		return fmt.Sprintf("Behavioral alignment verified (JS-Div: %.4f < %.4f). Zero drift detected.",
			da.JSDivergence, da.ThresholdUsed)
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("BEHAVIORAL DRIFT DETECTED: JS-Div %.4f (Threshold: %.4f), Entropy Delta: %+.4f\n",
		da.JSDivergence, da.ThresholdUsed, da.EntropyDelta))
	for _, dt := range da.DriftedTransitions {
		sb.WriteString(fmt.Sprintf("  • Transition %s -> %s shifted by %+.2f%% (Base: %.2f%%, Target: %.2f%%)\n",
			dt.FromState, dt.ToState, dt.ProbDelta*100, dt.BaselineProb*100, dt.TargetProb*100))
	}
	return sb.String()
}
