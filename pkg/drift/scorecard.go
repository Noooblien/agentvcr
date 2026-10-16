package drift

import (
	"fmt"
	"strings"
)

// RenderDriftScorecard renders an executive terminal scorecard of behavioral drift.
func RenderDriftScorecard(analysis *DriftAnalysis) string {
	var sb strings.Builder

	sb.WriteString("\033[1;35m========================================================================================\033[0m\n")
	sb.WriteString("  \033[1;37mAGENTVCR // AUTONOMOUS BEHAVIORAL DRIFT & ENTROPY-GUIDED POLICY AUDIT\033[0m\n")
	sb.WriteString("\033[1;35m========================================================================================\033[0m\n")
	sb.WriteString(fmt.Sprintf("  \033[1;33mBaseline Session ID:\033[0m          %s\n", analysis.BaselineSessionID))
	sb.WriteString(fmt.Sprintf("  \033[1;33mCandidate Session ID:\033[0m         %s\n", analysis.TargetSessionID))
	sb.WriteString(fmt.Sprintf("  \033[1;33mJensen-Shannon Divergence:\033[0m    %.4f (Threshold: %.4f)\n", analysis.JSDivergence, analysis.ThresholdUsed))
	sb.WriteString(fmt.Sprintf("  \033[1;33mAction Entropy Delta:\033[0m         %+.4f bits\n", analysis.EntropyDelta))

	if !analysis.DriftDetected {
		sb.WriteString("  \033[1;33mAlignment Verification:\033[0m       \033[1;32m[PASS] CONSERVATIVE BEHAVIOR PRESERVED\033[0m\n")
	} else {
		sb.WriteString("  \033[1;33mAlignment Verification:\033[0m       \033[1;31m[DRIFT DETECTED] BEHAVIORAL PATTERN DIVERGED\033[0m\n")
	}

	sb.WriteString("  \033[1;34m----------------------------------------------------------------------------------------\033[0m\n")

	if len(analysis.DriftedTransitions) == 0 {
		sb.WriteString("  \033[1;32m✓ All Markov transition probabilities remain within tight statistical tolerance.\033[0m\n")
	} else {
		sb.WriteString("  \033[1;31mSTATISTICALLY SIGNIFICANT DRIFTED TRANSITIONS:\033[0m\n")
		for i, dt := range analysis.DriftedTransitions {
			deltaColor := "\033[1;31m"
			if dt.ProbDelta < 0 {
				deltaColor = "\033[1;34m"
			}
			sb.WriteString(fmt.Sprintf("    [%d] %s -> %s\n", i+1, dt.FromState, dt.ToState))
			sb.WriteString(fmt.Sprintf("        Base Prob: %.1f%%  |  Target Prob: %.1f%%  |  Delta: %s%+.1f%%\033[0m (Anomaly: %.3f)\n",
				dt.BaselineProb*100, dt.TargetProb*100, deltaColor, dt.ProbDelta*100, dt.AnomalyScore))
		}
	}

	if analysis.SynthesizedPolicy != nil {
		p := analysis.SynthesizedPolicy
		sb.WriteString("  \033[1;34m----------------------------------------------------------------------------------------\033[0m\n")
		sb.WriteString(fmt.Sprintf("  \033[1;33mSYNTHESIZED CORRECTIVE GUARDRAIL POLICY:\033[0m \033[1;32m%s\033[0m\n", p.PolicyID))
		if len(p.RestrictedTransitions) > 0 {
			sb.WriteString(fmt.Sprintf("    • Prohibited Transitions:  %v\n", p.RestrictedTransitions))
		}
		if len(p.ToolFrequencyCeilings) > 0 {
			sb.WriteString(fmt.Sprintf("    • Dynamic Tool Ceilings:   %v\n", p.ToolFrequencyCeilings))
		}
		sb.WriteString(fmt.Sprintf("    • Recommended Directive:   %s\n", p.RecommendedPromptMod))
	}

	sb.WriteString("\033[1;35m========================================================================================\033[0m\n")
	return sb.String()
}
