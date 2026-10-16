package drift

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// DetectorConfig configures behavioral drift detection sensitivity and entropy bounds.
type DetectorConfig struct {
	DriftThreshold        float64 // Jensen-Shannon divergence threshold (default 0.20)
	TransitionDiffThreshold float64 // Minimum delta to flag an individual transition (default 0.15)
	MaxEntropyDelta       float64 // Max allowed change in Shannon entropy (default 0.40)
}

// DefaultDetectorConfig returns production defaults for behavioral drift tracking.
func DefaultDetectorConfig() DetectorConfig {
	return DetectorConfig{
		DriftThreshold:          0.20,
		TransitionDiffThreshold: 0.15,
		MaxEntropyDelta:         0.40,
	}
}

// Detector evaluates cross-trajectory Markov transition divergence and synthesizes corrective policies.
type Detector struct {
	config DetectorConfig
}

// NewDetector initializes a new behavioral drift detector.
func NewDetector(cfg DetectorConfig) *Detector {
	return &Detector{config: cfg}
}

// BuildProfile computes the statistical Markov transition and tool probability distribution of a session.
func (d *Detector) BuildProfile(session *trace.Session) *TrajectoryProfile {
	profile := &TrajectoryProfile{
		SessionID:         session.ID,
		TotalEvents:       len(session.Events),
		ToolCounts:        make(map[string]int),
		ToolProbabilities: make(map[string]float64),
		TransitionMatrix:  make(map[string]map[string]float64),
	}

	if len(session.Events) == 0 {
		return profile
	}

	var stateSequence []string
	var totalDuration int64
	totalToolCalls := 0

	for _, ev := range session.Events {
		totalDuration += ev.DurationMs
		state := string(ev.Type)

		// Disambiguate tool call names for granular behavioral tracking
		if ev.Type == trace.EventToolCall {
			if toolName, ok := ev.Payload["name"].(string); ok && toolName != "" {
				state = "tool:" + toolName
			}
			profile.ToolCounts[state]++
			totalToolCalls++
		}
		stateSequence = append(stateSequence, state)
	}

	profile.AverageStepDurationMs = float64(totalDuration) / float64(len(session.Events))

	// Compute Tool Probabilities
	if totalToolCalls > 0 {
		for tool, count := range profile.ToolCounts {
			profile.ToolProbabilities[tool] = float64(count) / float64(totalToolCalls)
		}
	}

	// Compute Shannon Action Entropy
	profile.ActionEntropy = ComputeShannonEntropy(profile.ToolProbabilities)

	// Compute First-Order Markov Transition Counts
	transitionCounts := make(map[string]map[string]int)
	outgoingTotals := make(map[string]int)

	for i := 0; i < len(stateSequence)-1; i++ {
		from := stateSequence[i]
		to := stateSequence[i+1]

		if transitionCounts[from] == nil {
			transitionCounts[from] = make(map[string]int)
		}
		transitionCounts[from][to]++
		outgoingTotals[from]++
	}

	// Normalize into Transition Probability Matrix P(to | from)
	for from, toMap := range transitionCounts {
		profile.TransitionMatrix[from] = make(map[string]float64)
		total := float64(outgoingTotals[from])
		for to, count := range toMap {
			profile.TransitionMatrix[from][to] = float64(count) / total
		}
	}

	return profile
}

// AnalyzeDrift compares candidate trajectory behavior against a trusted baseline trajectory.
func (d *Detector) AnalyzeDrift(baseline, candidate *trace.Session) *DriftAnalysis {
	baseProfile := d.BuildProfile(baseline)
	candProfile := d.BuildProfile(candidate)

	// 1. Flatten all observed transitions into a joint state space
	allTransitions := make(map[string]bool)
	for from, toMap := range baseProfile.TransitionMatrix {
		for to := range toMap {
			allTransitions[from+"->"+to] = true
		}
	}
	for from, toMap := range candProfile.TransitionMatrix {
		for to := range toMap {
			allTransitions[from+"->"+to] = true
		}
	}

	baseDist := make(map[string]float64)
	candDist := make(map[string]float64)

	var driftedTransitions []TransitionDrift

	for key := range allTransitions {
		var from, to string
		_, _ = fmt.Sscanf(key, "%s->%s", &from, &to)
		// Fallback split if fmt.Sscanf struggles with custom runes
		parts := splitTransitionKey(key)
		from = parts[0]
		to = parts[1]

		pBase := 0.0
		if baseProfile.TransitionMatrix[from] != nil {
			pBase = baseProfile.TransitionMatrix[from][to]
		}
		pCand := 0.0
		if candProfile.TransitionMatrix[from] != nil {
			pCand = candProfile.TransitionMatrix[from][to]
		}

		baseDist[key] = pBase
		candDist[key] = pCand

		delta := pCand - pBase
		absDelta := math.Abs(delta)
		if absDelta >= d.config.TransitionDiffThreshold {
			driftedTransitions = append(driftedTransitions, TransitionDrift{
				FromState:    from,
				ToState:      to,
				BaselineProb: pBase,
				TargetProb:   pCand,
				ProbDelta:    delta,
				AnomalyScore: absDelta,
			})
		}
	}

	// Sort drifted transitions descending by severity
	sort.Slice(driftedTransitions, func(i, j int) bool {
		return driftedTransitions[i].AnomalyScore > driftedTransitions[j].AnomalyScore
	})

	// 2. Compute Jensen-Shannon Divergence
	jsDiv := ComputeJSDivergence(baseDist, candDist)
	entropyDelta := candProfile.ActionEntropy - baseProfile.ActionEntropy

	driftDetected := jsDiv >= d.config.DriftThreshold || math.Abs(entropyDelta) >= d.config.MaxEntropyDelta

	analysis := &DriftAnalysis{
		BaselineSessionID:  baseline.ID,
		TargetSessionID:    candidate.ID,
		JSDivergence:       jsDiv,
		EntropyDelta:       entropyDelta,
		DriftDetected:      driftDetected,
		ThresholdUsed:      d.config.DriftThreshold,
		DriftedTransitions: driftedTransitions,
		AnalyzedAt:         time.Now().UTC(),
	}

	// 3. Synthesize corrective policy if drift occurred
	if driftDetected {
		analysis.SynthesizedPolicy = d.SynthesizePolicy(analysis, baseProfile, candProfile)
	}

	return analysis
}

// SynthesizePolicy generates automated corrective guardrails to counter observed behavioral drift.
func (d *Detector) SynthesizePolicy(analysis *DriftAnalysis, baseProfile, candProfile *TrajectoryProfile) *SynthesizedGuardrailPolicy {
	rnd := make([]byte, 4)
	_, _ = rand.Read(rnd)
	policyID := "pol_drift_" + hex.EncodeToString(rnd)

	var restrictedTransitions []string
	toolCeilings := make(map[string]int)

	for _, dt := range analysis.DriftedTransitions {
		// If a transition probability increased drastically from 0 or low base, restrict it
		if dt.BaselineProb < 0.10 && dt.TargetProb > 0.30 {
			restrictedTransitions = append(restrictedTransitions, fmt.Sprintf("%s -> %s", dt.FromState, dt.ToState))
		}
	}

	// Cap tools that were over-indexed compared to baseline
	for tool, candCount := range candProfile.ToolCounts {
		baseCount := baseProfile.ToolCounts[tool]
		if candCount > (baseCount*2) && candCount > 3 {
			// Limit tool call frequency to baseline + 1 buffer
			toolCeilings[tool] = baseCount + 1
		}
	}

	mitigation := "Enforce Markov transition bounds and constrain runaway tool call frequencies to baseline profile."
	if len(restrictedTransitions) > 0 {
		mitigation = fmt.Sprintf("Prohibit anomalous transitions %v and cap over-indexed tool executions.", restrictedTransitions)
	}

	return &SynthesizedGuardrailPolicy{
		PolicyID:              policyID,
		GeneratedAt:           time.Now().UTC(),
		TargetSessionID:       analysis.TargetSessionID,
		RestrictedTransitions: restrictedTransitions,
		ToolFrequencyCeilings: toolCeilings,
		RecommendedPromptMod:  mitigation,
	}
}

// ComputeShannonEntropy calculates H(X) = -sum(p * log2(p))
func ComputeShannonEntropy(dist map[string]float64) float64 {
	entropy := 0.0
	for _, p := range dist {
		if p > 0.0 {
			entropy -= p * math.Log2(p)
		}
	}
	return entropy
}

// ComputeJSDivergence computes the symmetric Jensen-Shannon divergence between two distributions.
func ComputeJSDivergence(p, q map[string]float64) float64 {
	// 1. Calculate midpoint distribution M = 0.5 * (P + Q)
	allKeys := make(map[string]bool)
	for k := range p {
		allKeys[k] = true
	}
	for k := range q {
		allKeys[k] = true
	}

	if len(allKeys) == 0 {
		return 0.0
	}

	// Normalize distributions so sum(P) = 1 and sum(Q) = 1
	sumP, sumQ := 0.0, 0.0
	for k := range allKeys {
		sumP += p[k]
		sumQ += q[k]
	}
	if sumP == 0.0 && sumQ == 0.0 {
		return 0.0
	}

	pNorm := make(map[string]float64)
	qNorm := make(map[string]float64)
	m := make(map[string]float64)

	for k := range allKeys {
		valP := 0.0
		if sumP > 0 {
			valP = p[k] / sumP
		}
		valQ := 0.0
		if sumQ > 0 {
			valQ = q[k] / sumQ
		}
		pNorm[k] = valP
		qNorm[k] = valQ
		m[k] = 0.5 * (valP + valQ)
	}

	klPM := klDiv(pNorm, m)
	klQM := klDiv(qNorm, m)

	js := 0.5*klPM + 0.5*klQM
	if js < 0.0 {
		js = 0.0
	}
	if js > 1.0 {
		js = 1.0
	}
	return js
}

func klDiv(a, b map[string]float64) float64 {
	kl := 0.0
	for k, pa := range a {
		if pa > 0.0 {
			pb := b[k]
			if pb > 0.0 {
				kl += pa * math.Log2(pa/pb)
			}
		}
	}
	return kl
}

func splitTransitionKey(key string) [2]string {
	idx := 0
	for i := 0; i < len(key)-1; i++ {
		if key[i] == '-' && key[i+1] == '>' {
			idx = i
			break
		}
	}
	if idx > 0 {
		return [2]string{key[:idx], key[idx+2:]}
	}
	return [2]string{key, ""}
}
