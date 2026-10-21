package deadlock

import (
	"fmt"
	"strings"
)

// ANSI terminal color escape constants.
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

// RenderDeadlockScorecard prints a high-contrast CLI dashboard of the swarm deadlock state.
func RenderDeadlockScorecard(assessment *DeadlockAssessment) string {
	var sb strings.Builder

	sb.WriteString("\n" + colorBold + colorRed + "========================================================================================================\n" + colorReset)
	sb.WriteString(colorBold + colorWhite + "  AGENTVCR // MULTI-AGENT DISTRIBUTED DEADLOCK, WAIT-FOR GRAPH & LIVELOCK RESOLVER\n" + colorReset)
	sb.WriteString(colorBold + colorRed + "========================================================================================================\n" + colorReset)

	statusColor := colorGreen
	if assessment.Status == "DEADLOCKED" {
		statusColor = colorRed
	} else if assessment.Status == "DEGRADED" {
		statusColor = colorYellow
	}

	sb.WriteString(fmt.Sprintf("  %sSwarm:%s %s  |  %sAgents:%s %d  |  %sDependencies:%s %d  |  %sDFS Metric:%s %.1f/100  |  %sStatus:%s %s%s%s\n",
		colorBold, colorReset, assessment.SwarmID,
		colorBold, colorReset, assessment.TotalAgents,
		colorBold, colorReset, assessment.TotalEdges,
		colorCyan, colorReset, assessment.DeadlockFreedomScore,
		colorBold, colorReset, statusColor, assessment.Status, colorReset))
	sb.WriteString(colorGray + "--------------------------------------------------------------------------------------------------------\n" + colorReset)

	if len(assessment.Cycles) == 0 {
		sb.WriteString(fmt.Sprintf("  %s✓ Acyclic Fluid Execution Verified.%s No circular waits, lock collisions, or orphaned handoffs.\n", colorGreen, colorReset))
	} else {
		sb.WriteString(fmt.Sprintf("%-14s %-20s %-10s %-12s %-24s %s\n",
			"CYCLE ID", "DEADLOCK TYPE", "SEVERITY", "STALL DURATION", "RESOLUTION STRATEGY", "RESOLUTION DIRECTIVE"))
		sb.WriteString(colorGray + "--------------------------------------------------------------------------------------------------------\n" + colorReset)

		for _, c := range assessment.Cycles {
			sevColor := colorYellow
			if c.Severity == "CRITICAL" {
				sevColor = colorRed
			}

			sb.WriteString(fmt.Sprintf("%-14s %-20s %s%-10s%s %-12s %-24s %s\n",
				c.CycleID,
				c.Type,
				sevColor, c.Severity, colorReset,
				c.StallDuration.String(),
				c.ProposedResolution,
				c.ResolutionAction))

			sb.WriteString(fmt.Sprintf("  %s↳ Wait-For Dependency Path:%s %s\n", colorCyan, colorReset, strings.Join(c.CyclePath, " ➔ ")))
			if len(c.InvolvedResources) > 0 {
				sb.WriteString(fmt.Sprintf("  %s↳ Contended Locks:%s %s\n", colorYellow, colorReset, strings.Join(c.InvolvedResources, ", ")))
			}
		}
	}

	sb.WriteString(colorBold + colorRed + "========================================================================================================\n\n" + colorReset)
	return sb.String()
}

// RenderDeadlockMermaid outputs a Mermaid directed flowchart of the Wait-For Graph (WFG).
func RenderDeadlockMermaid(assessment *DeadlockAssessment) string {
	var sb strings.Builder

	sb.WriteString("```mermaid\n")
	sb.WriteString("flowchart LR\n")

	if len(assessment.Cycles) == 0 {
		sb.WriteString("    Healthy([All Agents Running Fluidly - Acyclic WFG])\n")
	} else {
		for _, c := range assessment.Cycles {
			for i := 0; i < len(c.CyclePath)-1; i++ {
				u := c.CyclePath[i]
				v := c.CyclePath[i+1]
				sb.WriteString(fmt.Sprintf("    %s -->|Wait-For: %s| %s\n", sanitizeMermaidNode(u), c.Type, sanitizeMermaidNode(v)))
			}
			sb.WriteString(fmt.Sprintf("    style %s fill:#ff4d4f,stroke:#ff0000,stroke-width:2px\n", sanitizeMermaidNode(c.CyclePath[0])))
		}
	}

	sb.WriteString("```\n")
	return sb.String()
}

// RenderDeadlockMarkdown formats a GitHub-flavored Markdown report.
func RenderDeadlockMarkdown(assessment *DeadlockAssessment) string {
	var sb strings.Builder

	sb.WriteString("## AgentVCR Distributed Deadlock & Wait-For Graph Report\n\n")
	sb.WriteString(fmt.Sprintf("- **Swarm ID**: `%s`\n", assessment.SwarmID))
	sb.WriteString(fmt.Sprintf("- **Total Agents**: %d\n", assessment.TotalAgents))
	sb.WriteString(fmt.Sprintf("- **Deadlock Freedom Score (DFS)**: `%.1f / 100`\n", assessment.DeadlockFreedomScore))
	sb.WriteString(fmt.Sprintf("- **Health Status**: `%s`\n\n", assessment.Status))

	if len(assessment.Cycles) == 0 {
		sb.WriteString("✅ **No deadlocks or livelocks detected. Graph is acyclic.**\n")
	} else {
		sb.WriteString("| Cycle ID | Deadlock Type | Severity | Stall Duration | Proposed Resolution | Dependency Cycle Path |\n")
		sb.WriteString("|:---|:---|:---|:---|:---|:---|\n")

		for _, c := range assessment.Cycles {
			sb.WriteString(fmt.Sprintf("| `%s` | `%s` | `%s` | `%s` | `%s` | `%s` |\n",
				c.CycleID, c.Type, c.Severity, c.StallDuration, c.ProposedResolution, strings.Join(c.CyclePath, " -> ")))
		}
	}

	return sb.String()
}

func sanitizeMermaidNode(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "-", "_"), ".", "_")
}
