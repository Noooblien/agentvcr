package diff

import (
	"fmt"
	"strings"
)

// ANSI Color constants for terminal rendering
const (
	ansiReset      = "\033[0m"
	ansiBold       = "\033[1m"
	ansiDim        = "\033[2m"
	ansiGreen      = "\033[32m"
	ansiYellow     = "\033[33m"
	ansiRed        = "\033[31m"
	ansiCyan       = "\033[36m"
	ansiMagenta    = "\033[35m"
	ansiWhite      = "\033[37m"
	ansiBgGreen    = "\033[42m\033[30m"
	ansiBgRed      = "\033[41m\033[37m"
	ansiBgYellow   = "\033[43m\033[30m"
)

// RenderDiffScorecard formats the semantic diff report into an ANSI terminal scorecard.
func RenderDiffScorecard(report *SemanticDiffReport) string {
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("%s%s┌─────────────────────────────────────────────────────────────────────────────┐%s\n", ansiBold, ansiCyan, ansiReset))
	b.WriteString(fmt.Sprintf("%s%s│       AgentVCR • Semantic Trajectory Diff & Regression Shield Engine       │%s\n", ansiBold, ansiCyan, ansiReset))
	b.WriteString(fmt.Sprintf("%s%s└─────────────────────────────────────────────────────────────────────────────┘%s\n", ansiBold, ansiCyan, ansiReset))

	// Overview Metadata
	b.WriteString(fmt.Sprintf("  %sBase Session:%s   %-22s %s(%s, %d events)%s\n", ansiBold, ansiReset, report.BaseSessionID, ansiDim, report.BaseAgentName, report.TotalBaseEvents, ansiReset))
	b.WriteString(fmt.Sprintf("  %sTarget Session:%s %-22s %s(%s, %d events)%s\n", ansiBold, ansiReset, report.TargetSessionID, ansiDim, report.TargetAgentName, report.TotalTargetEvents, ansiReset))

	// Score / Metric Highlights
	scoreColor := ansiGreen
	if report.StabilityScore < 60.0 {
		scoreColor = ansiRed
	} else if report.StabilityScore < 80.0 {
		scoreColor = ansiYellow
	}

	b.WriteString(fmt.Sprintf("\n  %sStability Score:%s          %s%s%5.1f%%%s\n", ansiBold, ansiReset, ansiBold, scoreColor, report.StabilityScore, ansiReset))
	b.WriteString(fmt.Sprintf("  %sTool Sequence Sim:%s        %s%5.1f%%%s\n", ansiBold, ansiReset, ansiCyan, report.ToolSequenceSimilarity, ansiReset))
	b.WriteString(fmt.Sprintf("  %sAligned Steps:%s            %d (Matches: %s%d exact%s, %s%d semantic%s, %s%d drift%s, %s%d regressed%s)\n",
		ansiBold, ansiReset, len(report.AlignedSteps),
		ansiGreen, report.ExactMatches, ansiReset,
		ansiCyan, report.SemanticMatches, ansiReset,
		ansiYellow, report.ParamDrifts+report.ControlFlowDrifts, ansiReset,
		ansiRed, report.Regressions, ansiReset))

	b.WriteString(fmt.Sprintf("  %sResource Deltas:%s          Tokens: %+d | Cost: $%+.4f USD | Latency: %+d ms\n",
		ansiBold, ansiReset, report.TotalTokenDelta, report.CostDeltaUSD, report.DurationDeltaMs))

	// Step Alignment Table
	b.WriteString("\n  " + ansiBold + "Step-by-Step Trajectory Alignment & Deviation Analysis:" + ansiReset + "\n")
	b.WriteString("  ┌──────┬────────────────────────┬────────────────────────┬─────────────────────┬──────────────┐\n")
	b.WriteString("  │ STEP │ BASE TRAJECTORY        │ TARGET TRAJECTORY      │ CLASSIFICATION      │ STATUS       │\n")
	b.WriteString("  ├──────┼────────────────────────┼────────────────────────┼─────────────────────┼──────────────┤\n")

	for i, s := range report.AlignedSteps {
		baseDesc := "-"
		if s.StepIndexBase >= 0 {
			if s.BaseToolName != "" {
				baseDesc = fmt.Sprintf("tool:%s", truncate(s.BaseToolName, 17))
			} else {
				baseDesc = fmt.Sprintf("%s:%s", s.BaseType, truncate(s.BaseSummary, 12))
			}
		}

		targetDesc := "-"
		if s.StepIndexTarget >= 0 {
			if s.TargetToolName != "" {
				targetDesc = fmt.Sprintf("tool:%s", truncate(s.TargetToolName, 17))
			} else {
				targetDesc = fmt.Sprintf("%s:%s", s.TargetType, truncate(s.TargetSummary, 12))
			}
		}

		classBadge, statusBadge := formatClassificationBadges(s.Classification)

		b.WriteString(fmt.Sprintf("  │ %-4d │ %-22s │ %-22s │ %-19s │ %-12s │\n",
			i+1, baseDesc, targetDesc, classBadge, statusBadge))

		// Print parameter deltas if present
		for _, pd := range s.ParamDiffs {
			b.WriteString(fmt.Sprintf("  │      │ %s↳ %-66s%s │\n", ansiDim+ansiYellow, truncate(pd, 66), ansiReset))
		}
	}
	b.WriteString("  └──────┴────────────────────────┴────────────────────────┴─────────────────────┴──────────────┘\n")

	// Final Verdict Box
	var verdictBanner string
	switch report.Verdict {
	case VerdictPass:
		verdictBanner = fmt.Sprintf("  %s%s [VERDICT: PASS] %s %s%s\n", ansiBgGreen, ansiBold, ansiReset, ansiGreen, report.VerdictReason)
	case VerdictWarn:
		verdictBanner = fmt.Sprintf("  %s%s [VERDICT: WARN] %s %s%s\n", ansiBgYellow, ansiBold, ansiReset, ansiYellow, report.VerdictReason)
	case VerdictFail:
		verdictBanner = fmt.Sprintf("  %s%s [VERDICT: FAIL] %s %s%s\n", ansiBgRed, ansiBold, ansiReset, ansiRed, report.VerdictReason)
	}

	b.WriteString("\n" + verdictBanner + ansiReset + "\n")
	return b.String()
}

func formatClassificationBadges(c DiffClassification) (string, string) {
	switch c {
	case DiffMatchExact:
		return "MATCH_EXACT", ansiGreen + "IDENTICAL" + ansiReset
	case DiffMatchSemantic:
		return "MATCH_SEMANTIC", ansiCyan + "CONGRUENT" + ansiReset
	case DiffParamDrift:
		return "PARAM_DRIFT", ansiYellow + "DRIFT" + ansiReset
	case DiffControlFlowDrift:
		return "FLOW_DRIFT", ansiYellow + "DIVERGED" + ansiReset
	case DiffStepInserted:
		return "STEP_INSERTED", ansiCyan + "INSERTED" + ansiReset
	case DiffStepDeleted:
		return "STEP_DELETED", ansiDim + "DELETED" + ansiReset
	case DiffOutcomeRegression:
		return "REGRESSION", ansiRed + "REGRESSED" + ansiReset
	case DiffErrorInduced:
		return "ERROR_INDUCED", ansiRed + "FAILED" + ansiReset
	default:
		return string(c), "UNKNOWN"
	}
}

func truncate(s string, maxLen int) string {
	if len(s) > maxLen {
		if maxLen > 3 {
			return s[:maxLen-3] + "..."
		}
		return s[:maxLen]
	}
	return s
}

// RenderDiffMermaid generates a Mermaid flowchart contrasting the execution sequence of both runs.
func RenderDiffMermaid(report *SemanticDiffReport) string {
	var b strings.Builder
	b.WriteString("```mermaid\n")
	b.WriteString("flowchart TD\n")
	b.WriteString("  subgraph Base[\"Baseline Trajectory: " + report.BaseSessionID + "\"]\n")

	var prevBase string
	for i, s := range report.AlignedSteps {
		if s.StepIndexBase >= 0 {
			nodeID := fmt.Sprintf("B%d", i)
			label := s.BaseToolName
			if label == "" {
				label = string(s.BaseType)
			}
			b.WriteString(fmt.Sprintf("    %s[\"%d: %s\"]\n", nodeID, s.StepIndexBase+1, label))
			if prevBase != "" {
				b.WriteString(fmt.Sprintf("    %s --> %s\n", prevBase, nodeID))
			}
			prevBase = nodeID
		}
	}
	b.WriteString("  end\n\n")

	b.WriteString("  subgraph Target[\"Candidate Trajectory: " + report.TargetSessionID + "\"]\n")
	var prevTarget string
	for i, s := range report.AlignedSteps {
		if s.StepIndexTarget >= 0 {
			nodeID := fmt.Sprintf("T%d", i)
			label := s.TargetToolName
			if label == "" {
				label = string(s.TargetType)
			}
			b.WriteString(fmt.Sprintf("    %s[\"%d: %s\"]\n", nodeID, s.StepIndexTarget+1, label))
			if prevTarget != "" {
				b.WriteString(fmt.Sprintf("    %s --> %s\n", prevTarget, nodeID))
			}
			prevTarget = nodeID
		}
	}
	b.WriteString("  end\n\n")

	// Cross-links for deviations
	b.WriteString("  %% Semantic Alignment & Deviation Links\n")
	for i, s := range report.AlignedSteps {
		if s.StepIndexBase >= 0 && s.StepIndexTarget >= 0 {
			switch s.Classification {
			case DiffMatchExact, DiffMatchSemantic:
				b.WriteString(fmt.Sprintf("  B%d -.-|\"Aligned\"| T%d\n", i, i))
			case DiffParamDrift:
				b.WriteString(fmt.Sprintf("  B%d x--x|\"Param Drift\"| T%d\n", i, i))
			case DiffControlFlowDrift:
				b.WriteString(fmt.Sprintf("  B%d x--x|\"Flow Diverged\"| T%d\n", i, i))
			case DiffOutcomeRegression, DiffErrorInduced:
				b.WriteString(fmt.Sprintf("  B%d ==>|\"REGRESSION\"| T%d\n", i, i))
			}
		}
	}

	b.WriteString("\n  %% Styling classes\n")
	b.WriteString("  classDef match fill:#e6f4ea,stroke:#34a853,stroke-width:2px;\n")
	b.WriteString("  classDef drift fill:#fef7e0,stroke:#f9ab00,stroke-width:2px;\n")
	b.WriteString("  classDef regression fill:#fce8e6,stroke:#ea4335,stroke-width:3px;\n")

	for i, s := range report.AlignedSteps {
		if s.StepIndexTarget >= 0 {
			nodeID := fmt.Sprintf("T%d", i)
			switch s.Classification {
			case DiffMatchExact, DiffMatchSemantic:
				b.WriteString(fmt.Sprintf("  class %s match;\n", nodeID))
			case DiffParamDrift, DiffControlFlowDrift, DiffStepInserted:
				b.WriteString(fmt.Sprintf("  class %s drift;\n", nodeID))
			case DiffOutcomeRegression, DiffErrorInduced:
				b.WriteString(fmt.Sprintf("  class %s regression;\n", nodeID))
			}
		}
	}

	b.WriteString("```\n")
	return b.String()
}
