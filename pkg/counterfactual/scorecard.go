package counterfactual

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// ANSI color escapes for terminal rendering.
const (
	colorReset   = "\033[0m"
	colorBold    = "\033[1m"
	colorRed     = "\033[1;31m"
	colorGreen   = "\033[1;32m"
	colorYellow  = "\033[1;33m"
	colorBlue    = "\033[1;34m"
	colorMagenta = "\033[1;35m"
	colorCyan    = "\033[1;36m"
	colorGray    = "\033[0;90m"
)

// RenderScorecard generates a terminal scorecard detailing the counterfactual trajectory comparison.
func RenderScorecard(comp *BranchComparison) string {
	var sb strings.Builder

	outcomeColor := colorGreen
	if comp.Outcome == OutcomeRegressionFailure || comp.Outcome == OutcomeRunawayDivergence {
		outcomeColor = colorRed
	} else if comp.Outcome == OutcomeAlternativeEquiv {
		outcomeColor = colorCyan
	}

	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("%s┌────────────────────────────────────────────────────────────────────────┐%s\n", colorMagenta, colorReset))
	sb.WriteString(fmt.Sprintf("%s│ %s%-70s%s%s │%s\n", colorMagenta, colorBold, "  AGENTVCR COUNTERFACTUAL WHAT-IF SIMULATION SCORECARD", colorReset, colorMagenta, colorReset))
	sb.WriteString(fmt.Sprintf("%s├────────────────────────────────────────────────────────────────────────┤%s\n", colorMagenta, colorReset))
	sb.WriteString(fmt.Sprintf("%s│%s Baseline Session: %-51s %s│%s\n", colorMagenta, colorReset, comp.SessionID, colorMagenta, colorReset))
	sb.WriteString(fmt.Sprintf("%s│%s What-If Branch:   %-51s %s│%s\n", colorMagenta, colorReset, truncate(comp.BranchID, 51), colorMagenta, colorReset))
	sb.WriteString(fmt.Sprintf("%s│%s Branch Step:      %-51s %s│%s\n", colorMagenta, colorReset, fmt.Sprintf("Step %d (Intervention Point)", comp.Mutation.BranchStep), colorMagenta, colorReset))
	sb.WriteString(fmt.Sprintf("%s│%s Mutation Type:    %-51s %s│%s\n", colorMagenta, colorReset, comp.Mutation.Type, colorMagenta, colorReset))
	sb.WriteString(fmt.Sprintf("%s│%s Outcome Verdict:  %s%-51s%s %s│%s\n", colorMagenta, colorReset, outcomeColor, comp.Outcome, colorReset, colorMagenta, colorReset))
	sb.WriteString(fmt.Sprintf("%s├────────────────────────────────────────────────────────────────────────┤%s\n", colorMagenta, colorReset))

	// Trajectory Step & Token Differences
	stepDiffStr := fmt.Sprintf("%d baseline -> %d branch (%+d steps)", comp.BaselineSteps, comp.BranchSteps, comp.StepDelta)
	sb.WriteString(fmt.Sprintf("%s│%s Trajectory Steps: %-51s %s│%s\n", colorMagenta, colorReset, stepDiffStr, colorMagenta, colorReset))

	tokenColor := colorGreen
	if comp.TokenDelta > 0 {
		tokenColor = colorRed
	}
	tokenDiffStr := fmt.Sprintf("%d -> %d (%s%+d tok%s, %.1f%% savings)", comp.BaselineTokens, comp.BranchTokens, tokenColor, comp.TokenDelta, colorReset, comp.TokenSavingsPct)
	sb.WriteString(fmt.Sprintf("%s│%s Token Variance:   %-60s %s│%s\n", colorMagenta, colorReset, tokenDiffStr, colorMagenta, colorReset))

	latDiffStr := fmt.Sprintf("%dms -> %dms (%+dms)", comp.BaselineLatencyMs, comp.BranchLatencyMs, comp.LatencyDeltaMs)
	sb.WriteString(fmt.Sprintf("%s│%s Latency Variance: %-51s %s│%s\n", colorMagenta, colorReset, latDiffStr, colorMagenta, colorReset))

	costDiffStr := fmt.Sprintf("$%.4f -> $%.4f (diff: $%.4f)", comp.CostBaselineUSD, comp.CostBranchUSD, comp.CostDeltaUSD)
	sb.WriteString(fmt.Sprintf("%s│%s Cost Modeling:    %-51s %s│%s\n", colorMagenta, colorReset, costDiffStr, colorMagenta, colorReset))

	sb.WriteString(fmt.Sprintf("%s│%s Path Divergence:  %-51s %s│%s\n", colorMagenta, colorReset, fmt.Sprintf("%.1f%% (Levenshtein dist: %d)", comp.PathDivergencePct, comp.PathEditDistance), colorMagenta, colorReset))
	sb.WriteString(fmt.Sprintf("%s├────────────────────────────────────────────────────────────────────────┤%s\n", colorMagenta, colorReset))
	sb.WriteString(fmt.Sprintf("%s│%s %sSynthesis Note:%s                                                         %s│%s\n", colorMagenta, colorReset, colorBold, colorReset, colorMagenta, colorReset))
	sb.WriteString(fmt.Sprintf("%s│%s   %-67s %s│%s\n", colorMagenta, colorGray, truncate(comp.SummaryNote, 67), colorMagenta, colorReset))
	sb.WriteString(fmt.Sprintf("%s└────────────────────────────────────────────────────────────────────────┘%s\n", colorMagenta, colorReset))

	return sb.String()
}

// RenderMermaid generates a Mermaid flowchart contrasting the baseline trunk with the counterfactual branch.
func RenderMermaid(comp *BranchComparison, baseline, branch *trace.Session) string {
	var sb strings.Builder

	sb.WriteString("```mermaid\ngraph TD\n")
	sb.WriteString("  classDef baseline fill:#1e293b,stroke:#475569,stroke-width:2px,color:#f8fafc;\n")
	sb.WriteString("  classDef branch fill:#064e3b,stroke:#10b981,stroke-width:2px,color:#f8fafc;\n")
	sb.WriteString("  classDef split fill:#831843,stroke:#f43f5e,stroke-width:3px,color:#f8fafc;\n")
	sb.WriteString("  classDef error fill:#7f1d1d,stroke:#ef4444,stroke-width:2px,color:#f8fafc;\n\n")

	sb.WriteString("  subgraph Baseline [Trunk Trajectory: " + comp.SessionID + "]\n")
	for i, ev := range baseline.Events {
		toolName := ev.Type
		if t, ok := ev.Payload["tool"].(string); ok && t != "" {
			toolName = trace.EventType(t)
		}
		nodeID := fmt.Sprintf("B_%d", i+1)
		nodeClass := "baseline"
		if ev.Type == trace.EventError || hasEventError(ev) {
			nodeClass = "error"
		}
		sb.WriteString(fmt.Sprintf("    %s[\"Step %d: %s\"]:::%s\n", nodeID, i+1, toolName, nodeClass))
		if i > 0 {
			prevNode := fmt.Sprintf("B_%d", i)
			sb.WriteString(fmt.Sprintf("    %s --> %s\n", prevNode, nodeID))
		}
	}
	sb.WriteString("  end\n\n")

	splitStep := comp.Mutation.BranchStep
	sb.WriteString(fmt.Sprintf("  subgraph WhatIf [Counterfactual Branch: %s]\n", comp.Mutation.ID))
	sb.WriteString(fmt.Sprintf("    SplitNode{\"Step %d: INTERVENTION<br/>[%s]\"}:::split\n", splitStep, comp.Mutation.Type))

	prevID := "SplitNode"
	for i := splitStep - 1; i < len(branch.Events); i++ {
		ev := branch.Events[i]
		toolName := ev.Type
		if t, ok := ev.Payload["tool"].(string); ok && t != "" {
			toolName = trace.EventType(t)
		}
		nodeID := fmt.Sprintf("CF_%d", i+1)
		sb.WriteString(fmt.Sprintf("    %s[\"Step %d: %s\"]:::branch\n", nodeID, i+1, toolName))
		sb.WriteString(fmt.Sprintf("    %s --> %s\n", prevID, nodeID))
		prevID = nodeID
	}
	sb.WriteString("  end\n\n")

	if splitStep > 1 {
		sb.WriteString(fmt.Sprintf("  B_%d --> SplitNode\n", splitStep-1))
	} else {
		sb.WriteString("  B_1 -.-> SplitNode\n")
	}

	sb.WriteString("```")
	return sb.String()
}

// RenderMarkdownReport produces a GitHub-Flavored Markdown summary report.
func RenderMarkdownReport(comparisons []*BranchComparison, summary *SimulationSummary) string {
	var sb strings.Builder

	sb.WriteString("# 🔀 AgentVCR Counterfactual What-If Trajectory Report\n\n")
	sb.WriteString(fmt.Sprintf("**Generated At:** `%s`  \n", time.Now().UTC().Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("**Total Scenarios Evaluated:** `%d`  \n", summary.TotalScenarios))
	sb.WriteString(fmt.Sprintf("**Resolved Failures:** `%d` (`%.1f%%`)  \n", summary.ResolvedSuccessCount, float64(summary.ResolvedSuccessCount)/float64(summary.TotalScenarios)*100.0))
	sb.WriteString(fmt.Sprintf("**Net Tokens Saved:** `%d` (Avg `%.1f%%` savings)  \n", summary.NetTokensSaved, summary.AverageTokenSavingsPct))
	sb.WriteString(fmt.Sprintf("**Net Cost Savings:** `$%.4f USD`  \n\n", summary.NetCostSavingsUSD))

	sb.WriteString("## 📊 What-If Experiment Matrix\n\n")
	sb.WriteString("| Scenario | Intervention | Step | Outcome | Step $\\Delta$ | Token $\\Delta$ | Latency $\\Delta$ | Cost Impact |\n")
	sb.WriteString("|:---|:---|:---:|:---|:---:|:---:|:---:|:---:|\n")

	for _, c := range comparisons {
		outcomeBadge := "✅ " + string(c.Outcome)
		if c.Outcome == OutcomeRegressionFailure || c.Outcome == OutcomeRunawayDivergence {
			outcomeBadge = "❌ " + string(c.Outcome)
		}
		costImpact := fmt.Sprintf("-$%.4f", -c.CostDeltaUSD)
		if c.CostDeltaUSD > 0 {
			costImpact = fmt.Sprintf("+$%.4f", c.CostDeltaUSD)
		}
		sb.WriteString(fmt.Sprintf("| **%s** | `%s` | `%d` | %s | `%+d` | `%+d` (%.1f%%) | `%+d ms` | `%s` |\n",
			c.Mutation.Name, c.Mutation.Type, c.Mutation.BranchStep, outcomeBadge,
			c.StepDelta, c.TokenDelta, c.TokenSavingsPct, c.LatencyDeltaMs, costImpact))
	}
	sb.WriteString("\n")

	sb.WriteString("## 🔬 Case Analysis & Synthesis Notes\n\n")
	for _, c := range comparisons {
		sb.WriteString(fmt.Sprintf("### %s (`%s`)\n", c.Mutation.Name, c.Mutation.ID))
		sb.WriteString(fmt.Sprintf("- **Hypothesis:** %s\n", c.Mutation.Description))
		sb.WriteString(fmt.Sprintf("- **Synthesis Finding:** %s\n", c.SummaryNote))
		sb.WriteString(fmt.Sprintf("- **Path Divergence:** `%.1f%%` (Levenshtein distance: `%d`)\n\n", c.PathDivergencePct, c.PathEditDistance))
	}

	return sb.String()
}

// RenderJSON serializes comparisons to formatted JSON.
func RenderJSON(comparisons []*BranchComparison) (string, error) {
	bytes, err := json.MarshalIndent(comparisons, "", "  ")
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func hasEventError(ev trace.TraceEvent) bool {
	if status, ok := ev.Payload["status"].(string); ok && (status == "error" || status == "failed") {
		return true
	}
	if code, ok := ev.Payload["status_code"].(int); ok && code >= 400 {
		return true
	}
	return false
}
