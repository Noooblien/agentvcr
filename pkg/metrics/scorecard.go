package metrics

import (
	"fmt"
	"strings"
)

// ANSI terminal color escape codes.
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

// RenderTokenScorecard formats an ANSI terminal telemetry scorecard.
func RenderTokenScorecard(report *TokenAuditReport) string {
	var sb strings.Builder

	sb.WriteString("\n" + colorBold + colorBlue + "========================================================================================================\n" + colorReset)
	sb.WriteString(colorBold + colorWhite + "  AGENTVCR // GRANULAR TOKEN CONSUMPTION METRICS, PROMPT CACHE & COST AUDIT\n" + colorReset)
	sb.WriteString(colorBold + colorBlue + "========================================================================================================\n" + colorReset)

	verdictColor := colorGreen
	if report.EfficiencyVerdict == "TOKEN_BLOATED" {
		verdictColor = colorRed
	} else if report.EfficiencyVerdict == "NOMINAL" {
		verdictColor = colorYellow
	}

	sb.WriteString(fmt.Sprintf("  %sSession:%s %s  |  %sAgent:%s %s  |  %sSteps:%s %d  |  %sEfficiency Score:%s %.1f/100  |  %sVerdict:%s %s%s%s\n",
		colorBold, colorReset, report.SessionID,
		colorBold, colorReset, report.AgentName,
		colorBold, colorReset, report.TotalSteps,
		colorCyan, colorReset, report.TokenEfficiencyScore,
		colorBold, colorReset, verdictColor, report.EfficiencyVerdict, colorReset))
	sb.WriteString(colorGray + "--------------------------------------------------------------------------------------------------------\n" + colorReset)

	b := report.Breakdown
	sb.WriteString(fmt.Sprintf("  %sToken Totals:%s  Prompt: %s%d%s  |  Comp: %s%d%s  |  Cached Read: %s%d%s  |  Reasoning: %s%d%s  |  Total: %s%d%s\n",
		colorBold, colorReset,
		colorCyan, b.PromptTokens, colorReset,
		colorPurple, b.CompletionTokens, colorReset,
		colorGreen, b.CachedPromptTokens, colorReset,
		colorYellow, b.ReasoningTokens, colorReset,
		colorWhite, b.TotalTokens, colorReset))

	sb.WriteString(fmt.Sprintf("  %sFinancials:%s    Prompt Cost: $%.4f  |  Comp Cost: $%.4f  |  %sCache Savings: $%.4f%s  |  %sTotal Cost: $%.4f%s\n",
		colorBold, colorReset,
		b.EstimatedPromptCost,
		b.EstimatedCompCost,
		colorGreen, b.EstimatedSavingsUSD, colorReset,
		colorBold+colorYellow, b.TotalEstimatedCostUSD, colorReset))
	sb.WriteString(colorGray + "--------------------------------------------------------------------------------------------------------\n" + colorReset)

	sb.WriteString(fmt.Sprintf("%-8s %-20s %-14s %-14s %-14s %-12s %s\n",
		"STEP", "MODEL", "PROMPT TOK", "COMP TOK", "CACHED TOK", "LATENCY", "EST COST"))
	sb.WriteString(colorGray + "--------------------------------------------------------------------------------------------------------\n" + colorReset)

	for _, s := range report.StepMetrics {
		sb.WriteString(fmt.Sprintf("%-8d %-20s %-14d %-14d %-14d %-12s $%.5f\n",
			s.StepIndex,
			s.Model,
			s.PromptTokens,
			s.CompletionTokens,
			s.CachedTokens,
			fmt.Sprintf("%d ms", s.DurationMs),
			s.CostUSD))
	}

	sb.WriteString(colorBold + colorBlue + "========================================================================================================\n\n" + colorReset)
	return sb.String()
}

// RenderTokenMermaid outputs a Mermaid visualization of token burn progression.
func RenderTokenMermaid(report *TokenAuditReport) string {
	var sb strings.Builder

	sb.WriteString("```mermaid\n")
	sb.WriteString("flowchart LR\n")
	sb.WriteString("    Start([Session: " + report.SessionID + "]) --> StepBreakdown\n")
	sb.WriteString("    subgraph StepBreakdown [Token & Cost Attribution]\n")

	for i, s := range report.StepMetrics {
		nodeID := fmt.Sprintf("Step%d", s.StepIndex)
		sb.WriteString(fmt.Sprintf("        %s[\"Step %d (%s)<br/>%d Tokens | $%.4f\"]\n",
			nodeID, s.StepIndex, s.Model, s.PromptTokens+s.CompletionTokens, s.CostUSD))
		if i > 0 {
			prevNode := fmt.Sprintf("Step%d", report.StepMetrics[i-1].StepIndex)
			sb.WriteString(fmt.Sprintf("        %s --> %s\n", prevNode, nodeID))
		}
	}

	sb.WriteString("    end\n")
	sb.WriteString(fmt.Sprintf("    StepBreakdown --> Total[\"Total Token Burn: %d<br/>Net Cost: $%.4f | Cache Savings: $%.4f\"]\n",
		report.Breakdown.TotalTokens, report.Breakdown.TotalEstimatedCostUSD, report.Breakdown.EstimatedSavingsUSD))
	sb.WriteString("    style Total fill:#52c41a,stroke:#389e0d,stroke-width:2px\n")
	sb.WriteString("```\n")

	return sb.String()
}

// RenderTokenMarkdown formats a GitHub-flavored markdown report.
func RenderTokenMarkdown(report *TokenAuditReport) string {
	var sb strings.Builder

	sb.WriteString("## AgentVCR Granular Token Consumption & Cost Report\n\n")
	sb.WriteString(fmt.Sprintf("- **Session ID**: `%s`\n", report.SessionID))
	sb.WriteString(fmt.Sprintf("- **Agent**: `%s`\n", report.AgentName))
	sb.WriteString(fmt.Sprintf("- **Total Steps**: %d\n", report.TotalSteps))
	sb.WriteString(fmt.Sprintf("- **Total Tokens Burned**: `%d`\n", report.Breakdown.TotalTokens))
	sb.WriteString(fmt.Sprintf("- **Cache Hit Ratio**: `%.1f%%`\n", report.Breakdown.CacheHitRatio*100))
	sb.WriteString(fmt.Sprintf("- **Net Estimated Cost**: `$%.4f` (Saved: `$%.4f` via caching)\n",
		report.Breakdown.TotalEstimatedCostUSD, report.Breakdown.EstimatedSavingsUSD))
	sb.WriteString(fmt.Sprintf("- **Token Efficiency Score**: `%.1f / 100` (`%s`)\n\n",
		report.TokenEfficiencyScore, report.EfficiencyVerdict))

	sb.WriteString("| Step | Model | Prompt Tokens | Completion Tokens | Cached Tokens | Latency | Estimated Cost |\n")
	sb.WriteString("|:---|:---|:---|:---|:---|:---|:---|\n")

	for _, s := range report.StepMetrics {
		sb.WriteString(fmt.Sprintf("| `%d` | `%s` | %d | %d | %d | %d ms | `$%.5f` |\n",
			s.StepIndex, s.Model, s.PromptTokens, s.CompletionTokens, s.CachedTokens, s.DurationMs, s.CostUSD))
	}

	return sb.String()
}
