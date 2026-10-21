package contextloss

import (
	"fmt"
	"strings"
)

// ANSI color escape sequences.
const (
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorPurple = "\033[35m"
	colorCyan   = "\033[36m"
	colorWhite  = "\033[37m"
	colorGray   = "\033[90m"
)

// RenderContextLossScorecard formats an ANSI terminal summary of compaction fidelity audits.
func RenderContextLossScorecard(summary *ContextLossSummary) string {
	var sb strings.Builder

	sb.WriteString("\n" + colorBold + colorPurple + "========================================================================================================\n" + colorReset)
	sb.WriteString(colorBold + colorWhite + "  AGENTVCR // CONTEXT WINDOW COMPACTION LOSS & CONSTRAINT FIDELITY PROFILER\n" + colorReset)
	sb.WriteString(colorBold + colorPurple + "========================================================================================================\n" + colorReset)

	sb.WriteString(fmt.Sprintf("  %sEvaluated:%s %d sessions  |  %sHigh Fidelity:%s %d  |  %sBleed:%s %d  |  %sCritical Loss:%s %d  |  %sAvg CDS:%s %.1f/100  |  %sAvg Comp:%s %.1fx\n",
		colorBold, colorReset, summary.TotalEvaluated,
		colorGreen, colorReset, summary.HighFidelityCount,
		colorYellow, colorReset, summary.ModerateBleedCount,
		colorRed, colorReset, summary.CriticalLossCount,
		colorCyan, colorReset, summary.AverageCDS,
		colorWhite, colorReset, summary.AverageCompression))
	sb.WriteString(colorGray + "--------------------------------------------------------------------------------------------------------\n" + colorReset)

	sb.WriteString(fmt.Sprintf("%-22s %-12s %-8s %-8s %-8s %-8s %-8s %s\n",
		"SESSION ID", "COMPRESSION", "ORIG", "COMP", "RECALL", "CNSTR", "CDS", "TIER"))
	sb.WriteString(colorGray + "--------------------------------------------------------------------------------------------------------\n" + colorReset)

	for _, a := range summary.Assessments {
		tierColor := colorGreen
		tierIcon := "✓"

		switch a.Tier {
		case TierHighFidelity:
			tierColor = colorGreen
			tierIcon = "★"
		case TierModerateBleed:
			tierColor = colorYellow
			tierIcon = "▲"
		case TierCriticalLoss:
			tierColor = colorRed
			tierIcon = "✗"
		}

		sb.WriteString(fmt.Sprintf("%-22s %5.1fx       %-8d %-8d %3.0f%%     %3.0f%%    %s%5.1f%s   %s%s %-14s%s\n",
			truncate(a.SessionID, 21),
			a.CompressionRatio,
			a.OriginalTokens,
			a.CompactedTokens,
			a.EntityRecall*100,
			a.ConstraintFidelity*100,
			colorBold+tierColor, a.ContextDegradationScore, colorReset,
			tierColor, tierIcon, a.Tier, colorReset))

		if len(a.Violations) > 0 {
			for _, v := range a.Violations {
				sb.WriteString(fmt.Sprintf("    %s↳ [ALERT]%s %s\n", colorRed, colorReset, v))
			}
		}
		if len(a.GhostFacts) > 0 {
			for _, g := range a.GhostFacts {
				sb.WriteString(fmt.Sprintf("    %s↳ [GHOST FACT]%s %s (%s)\n", colorPurple, colorReset, g.Claim, g.Explanation))
			}
		}
	}

	sb.WriteString(colorBold + colorPurple + "========================================================================================================\n" + colorReset)
	sb.WriteString(colorGray + "  Formula: CDS = (0.40*EntityRecall + 0.60*ConstraintFidelity) * (1 - GhostPenalty) * 100\n" + colorReset)

	return sb.String()
}

// RenderContextLossMermaid creates a Mermaid flowchart tracking compaction fidelity and entity retention.
func RenderContextLossMermaid(summary *ContextLossSummary) string {
	var sb strings.Builder
	sb.WriteString("```mermaid\nflowchart TD\n")
	sb.WriteString("    subgraph Input [Original Long-Horizon Trajectory]\n")
	sb.WriteString("        Orig[\"Full Transcript Tokens\"] --> Compactor[\"Context Compaction Stage\"]\n")
	sb.WriteString("    end\n\n")

	sb.WriteString("    subgraph Audit [Information Bleed Audits]\n")
	sb.WriteString("        Compactor --> Recall[\"1. Technical Entity Recall (35%)\"]\n")
	sb.WriteString("        Compactor --> Cnstr[\"2. Negative Constraint Fidelity (45%)\"]\n")
	sb.WriteString("        Compactor --> Ghost[\"3. Hallucinated Ghost Facts (20% Penalty)\"]\n")
	sb.WriteString("    end\n\n")

	sb.WriteString("    subgraph Synthesis [Context Degradation Score (CDS)]\n")
	sb.WriteString("        Recall --> CDS[\"Composite Degradation Score (0-100)\"]\n")
	sb.WriteString("        Cnstr --> CDS\n")
	sb.WriteString("        Ghost --> CDS\n")
	sb.WriteString("    end\n\n")

	sb.WriteString("    subgraph Verdicts [Session Degradation Classifications]\n")
	for i, a := range summary.Assessments {
		nodeID := fmt.Sprintf("Sess_%d", i+1)
		label := fmt.Sprintf("%s (CDS: %.1f | %s)", a.SessionID, a.ContextDegradationScore, a.Tier)
		if a.Tier == TierHighFidelity {
			sb.WriteString(fmt.Sprintf("        CDS --> %s[\"🌟 %s\"]\n", nodeID, label))
		} else if a.Tier == TierModerateBleed {
			sb.WriteString(fmt.Sprintf("        CDS --> %s[\"⚠️ %s\"]\n", nodeID, label))
		} else {
			sb.WriteString(fmt.Sprintf("        CDS --> %s[\"🚨 %s\"]\n", nodeID, label))
		}
	}
	sb.WriteString("    end\n```")
	return sb.String()
}

// RenderContextLossMarkdown creates a GitHub Markdown report table for CI/CD and PR verification.
func RenderContextLossMarkdown(summary *ContextLossSummary) string {
	var sb strings.Builder

	sb.WriteString("### Context Window Compaction Loss & Constraint Recall Report\n\n")
	sb.WriteString(fmt.Sprintf("**Evaluated**: %d sessions | **High Fidelity**: %d | **Moderate Bleed**: %d | **Critical Loss**: %d | **Average CDS**: `%.1f/100` | **Average Compression**: `%.1fx`\n\n",
		summary.TotalEvaluated, summary.HighFidelityCount, summary.ModerateBleedCount, summary.CriticalLossCount, summary.AverageCDS, summary.AverageCompression))

	sb.WriteString("| Session ID | Compression | Orig Tokens | Compacted Tokens | Entity Recall | Constraint Fidelity | Degradation Score (CDS) | Fidelity Tier |\n")
	sb.WriteString("| :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- |\n")

	for _, a := range summary.Assessments {
		tierBadge := fmt.Sprintf("`%s`", a.Tier)
		if a.Tier == TierHighFidelity {
			tierBadge = "🌟 **HIGH_FIDELITY**"
		} else if a.Tier == TierCriticalLoss {
			tierBadge = "🚨 **CRITICAL_LOSS**"
		}

		sb.WriteString(fmt.Sprintf("| `%s` | `%.1fx` | `%d` | `%d` | `%.0f%%` | `%.0f%%` | **`%.1f/100`** | %s |\n",
			a.SessionID, a.CompressionRatio, a.OriginalTokens, a.CompactedTokens, a.EntityRecall*100, a.ConstraintFidelity*100, a.ContextDegradationScore, tierBadge))
	}

	return sb.String()
}

func truncate(s string, max int) string {
	if len(s) > max {
		return s[:max-2] + ".."
	}
	return s
}
