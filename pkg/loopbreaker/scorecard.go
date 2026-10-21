package loopbreaker

import (
	"fmt"
	"strings"
	"time"
)

// ANSI color escapes for terminal scorecard rendering.
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

// RenderLoopScorecard renders a high-visibility terminal scorecard of reasoning cycle interventions.
func RenderLoopScorecard(summary LoopBreakerSummary) string {
	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("%s┌────────────────────────────────────────────────────────────────────────┐%s\n", colorCyan, colorReset))
	sb.WriteString(fmt.Sprintf("%s│ %s%-70s%s%s │%s\n", colorCyan, colorBold, "  AGENTVCR REASONING CYCLE & LOOP BREAKER SCORECARD", colorReset, colorCyan, colorReset))
	sb.WriteString(fmt.Sprintf("%s├────────────────────────────────────────────────────────────────────────┤%s\n", colorCyan, colorReset))
	sb.WriteString(fmt.Sprintf("%s│%s Total Trajectory Steps Evaluated: %-37d %s│%s\n", colorCyan, colorReset, summary.TotalStepsEvaluated, colorCyan, colorReset))
	sb.WriteString(fmt.Sprintf("%s│%s Cycles Detected & Preempted:     %s%-37d%s %s│%s\n", colorCyan, colorReset, colorYellow, summary.CyclesDetected, colorReset, colorCyan, colorReset))
	sb.WriteString(fmt.Sprintf("%s│%s Steering Prompts Synthesized:    %s%-37d%s %s│%s\n", colorCyan, colorReset, colorBlue, summary.SteeringInterventions, colorReset, colorCyan, colorReset))
	sb.WriteString(fmt.Sprintf("%s│%s Circuit Breakers Tripped:        %s%-37d%s %s│%s\n", colorCyan, colorReset, colorRed, summary.CircuitBreakersTripped, colorReset, colorCyan, colorReset))
	sb.WriteString(fmt.Sprintf("%s│%s Redundant Tokens Preempted:      %s%-37d%s %s│%s\n", colorCyan, colorReset, colorGreen, summary.TotalTokensSaved, colorReset, colorCyan, colorReset))
	sb.WriteString(fmt.Sprintf("%s│%s Estimated Cost Savings:          %s$%-36.4f%s %s│%s\n", colorCyan, colorReset, colorGreen, summary.EstimatedSavingsUSD, colorReset, colorCyan, colorReset))
	sb.WriteString(fmt.Sprintf("%s└────────────────────────────────────────────────────────────────────────┘%s\n\n", colorCyan, colorReset))

	// Detailed Detection Breakdown
	sb.WriteString(fmt.Sprintf("%s%s%-6s %-30s %-8s %-8s %-18s %-12s%s\n",
		colorBold, colorBlue, "STEP", "PATTERN", "PERIOD", "REPEATS", "ACTION", "SAVED TOK", colorReset))
	sb.WriteString(fmt.Sprintf("%s%s%s\n", colorGray, strings.Repeat("─", 88), colorReset))

	for _, d := range summary.Detections {
		actColor := colorGreen
		switch d.Action {
		case ActionWarn:
			actColor = colorYellow
		case ActionInjectSteering:
			actColor = colorBlue
		case ActionTripCircuitBreaker:
			actColor = colorRed
		}

		sb.WriteString(fmt.Sprintf("%-6d %-30s %-8d %-8d %s%-18s%s %-12d\n",
			d.TriggerStepIndex,
			truncate(string(d.Pattern), 29),
			d.Period,
			d.RecurrenceCount,
			actColor, d.Action, colorReset,
			d.TokensWasted,
		))

		sb.WriteString(fmt.Sprintf("  %s└─ Sequence: [%s] (Similarity: %.1f%%, Confidence: %.2f)%s\n",
			colorGray, strings.Join(d.RepeatingTools, " -> "), d.ArgumentSimilarity*100, d.Confidence, colorReset))
	}
	sb.WriteString("\n")

	return sb.String()
}

// RenderLoopMermaid produces a Mermaid state diagram visualizing repeating cycles and intervention checkpoints.
func RenderLoopMermaid(detections []CycleDetection) string {
	var sb strings.Builder

	sb.WriteString("```mermaid\n")
	sb.WriteString("stateDiagram-v2\n")
	sb.WriteString("    direction LR\n\n")

	seenNodes := make(map[string]bool)
	seenEdges := make(map[string]bool)

	for _, d := range detections {
		tools := d.RepeatingTools
		if len(tools) == 1 {
			tool := sanitizeMermaidNode(tools[0])
			edge := fmt.Sprintf("%s-->%s", tool, tool)
			if !seenEdges[edge] {
				seenEdges[edge] = true
				sb.WriteString(fmt.Sprintf("    %s --> %s: [CYCLE LOOP: %d repeats] %s\n", tool, tool, d.RecurrenceCount, d.Action))
			}
		} else {
			for i := 0; i < len(tools); i++ {
				from := sanitizeMermaidNode(tools[i])
				to := sanitizeMermaidNode(tools[(i+1)%len(tools)])
				edge := fmt.Sprintf("%s-->%s", from, to)
				if !seenEdges[edge] {
					seenEdges[edge] = true
					if i == len(tools)-1 {
						sb.WriteString(fmt.Sprintf("    %s --> %s: [CYCLE TRIP: %s]\n", from, to, d.Action))
					} else {
						sb.WriteString(fmt.Sprintf("    %s --> %s\n", from, to))
					}
				}
				seenNodes[from] = true
				seenNodes[to] = true
			}
		}
	}

	sb.WriteString("```\n")
	return sb.String()
}

// RenderLoopMarkdownReport produces a GitHub-flavored markdown report of loop breaker interventions.
func RenderLoopMarkdownReport(summary LoopBreakerSummary) string {
	var sb strings.Builder

	sb.WriteString("# AgentVCR Reasoning Cycle & Loop Breaker Audit Report\n\n")
	sb.WriteString(fmt.Sprintf("*Generated at: %s*\n\n", time.Now().UTC().Format(time.RFC3339)))

	sb.WriteString("## Executive Summary\n\n")
	sb.WriteString("| Metric | Value |\n")
	sb.WriteString("| :--- | :--- |\n")
	sb.WriteString(fmt.Sprintf("| **Total Trajectory Steps Evaluated** | `%d` |\n", summary.TotalStepsEvaluated))
	sb.WriteString(fmt.Sprintf("| **Cycles Detected & Preempted** | `%d` |\n", summary.CyclesDetected))
	sb.WriteString(fmt.Sprintf("| **Steering Prompts Injected** | `%d` |\n", summary.SteeringInterventions))
	sb.WriteString(fmt.Sprintf("| **Circuit Breakers Tripped** | `%d` |\n", summary.CircuitBreakersTripped))
	sb.WriteString(fmt.Sprintf("| **Tokens Saved** | `%d` tokens |\n", summary.TotalTokensSaved))
	sb.WriteString(fmt.Sprintf("| **Estimated Cost Savings** | `+$%.4f USD` |\n\n", summary.EstimatedSavingsUSD))

	sb.WriteString("## Cycle Interventions Breakdown\n\n")
	sb.WriteString("| Step | Pattern | Period | Recurrence | Action | Arg Similarity | Confidence | Tokens Saved |\n")
	sb.WriteString("| :---: | :--- | :---: | :---: | :---: | :---: | :---: | :---: |\n")

	for _, d := range summary.Detections {
		sb.WriteString(fmt.Sprintf("| `%d` | `%s` | `%d` | `%d` | **%s** | `%.1f%%` | `%.2f` | `+%d` |\n",
			d.TriggerStepIndex,
			d.Pattern,
			d.Period,
			d.RecurrenceCount,
			d.Action,
			d.ArgumentSimilarity*100,
			d.Confidence,
			d.TokensWasted,
		))
	}

	sb.WriteString("\n## Synthesized Steering Directives\n\n")
	for _, d := range summary.Detections {
		if d.SteeringPrompt != "" {
			sb.WriteString(fmt.Sprintf("### Step %d Intervention (`%s`)\n\n", d.TriggerStepIndex, d.Pattern))
			sb.WriteString("```text\n")
			sb.WriteString(d.SteeringPrompt)
			sb.WriteString("\n```\n\n")
		}
	}

	return sb.String()
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen < 3 {
		return s[:maxLen]
	}
	return s[:maxLen-2] + ".."
}

func sanitizeMermaidNode(s string) string {
	replacer := strings.NewReplacer("-", "_", " ", "_", ".", "_", ":", "_", "/", "_")
	return replacer.Replace(s)
}
