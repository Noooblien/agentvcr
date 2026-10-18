package swarm

import (
	"fmt"
	"strings"
)

// ANSI terminal color escape codes
const (
	ColorReset   = "\033[0m"
	ColorBold    = "\033[1m"
	ColorDim     = "\033[2m"
	ColorRed     = "\033[31m"
	ColorGreen   = "\033[32m"
	ColorYellow  = "\033[33m"
	ColorBlue    = "\033[34m"
	ColorMagenta = "\033[35m"
	ColorCyan    = "\033[36m"
	ColorWhite   = "\033[37m"
)

// RenderSwarmScorecard formats an ANSI terminal scorecard of the swarm execution trajectory.
func RenderSwarmScorecard(report *SwarmReport) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("%s%s========================================================================================%s\n", ColorBold, ColorCyan, ColorReset))
	sb.WriteString(fmt.Sprintf("%s%s  AGENTVCR // MULTI-AGENT SWARM ORCHESTRATION & HANDOFF FLIGHT RECORDER%s\n", ColorBold, ColorWhite, ColorReset))
	sb.WriteString(fmt.Sprintf("%s%s========================================================================================%s\n\n", ColorBold, ColorCyan, ColorReset))

	statusColor := ColorGreen
	statusText := "HEALTHY / COMPLETED"
	if report.CycleDetected || report.CircuitBreakerFired {
		statusColor = ColorRed
		statusText = "CIRCUIT BREAKER FIRED (DEADLOCK CYCLE PREVENTED)"
	} else if report.Verdict != nil && !report.Verdict.ConsensusReached {
		statusColor = ColorYellow
		statusText = "DISSENSUS / NO CLEAR MAJORITY"
	}

	sb.WriteString(fmt.Sprintf("  %sSwarm ID:%s          %s\n", ColorBold, ColorReset, report.SwarmID))
	sb.WriteString(fmt.Sprintf("  %sTopology:%s          %s%s%s\n", ColorBold, ColorReset, ColorCyan, report.Topology, ColorReset))
	sb.WriteString(fmt.Sprintf("  %sSwarm Status:%s      %s%s[%s]%s\n", ColorBold, ColorReset, ColorBold, statusColor, statusText, ColorReset))
	sb.WriteString(fmt.Sprintf("  %sExecution Scale:%s   %d turns, %d handoffs, %d tokens across %d agents (Duration: %dms)\n",
		ColorBold, ColorReset, report.TotalTurns, report.TotalHandoffs, report.TotalTokens, len(report.UniqueAgentsInvolved), report.ExecutionDurationMs))

	if len(report.HandoffChain) > 1 {
		sb.WriteString(fmt.Sprintf("  %sHandoff Path:%s      %s\n", ColorBold, ColorReset, strings.Join(report.HandoffChain, " -> ")))
	}

	if report.CycleDetected {
		sb.WriteString(fmt.Sprintf("  %sCycle Hazard:%s      %s%s%s\n", ColorBold, ColorReset, ColorRed, report.CycleDetails, ColorReset))
	}

	// Debate Jury Details (if applicable)
	if report.Verdict != nil {
		sb.WriteString(fmt.Sprintf("\n  %s%sJURY CONSENSUS VERDICT:%s\n", ColorBold, ColorWhite, ColorReset))
		sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────────\n")
		vColor := ColorGreen
		if !report.Verdict.ConsensusReached {
			vColor = ColorYellow
		}
		sb.WriteString(fmt.Sprintf("  Winning Position:   %s%s[%s]%s (Confidence: %.2f/1.0)\n",
			ColorBold, vColor, report.Verdict.WinningPosition, ColorReset, report.Verdict.ConfidenceScore))
		sb.WriteString(fmt.Sprintf("  Total Ballots:      %d cast across jury\n", report.Verdict.TotalBallots))
		sb.WriteString(fmt.Sprintf("  Judgement Summary:  %s\n", report.Verdict.JudgementSummary))
		if len(report.Verdict.DissentingAgents) > 0 {
			sb.WriteString(fmt.Sprintf("  Dissenting Voices:  %s\n", strings.Join(report.Verdict.DissentingAgents, ", ")))
		}
		sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────────\n")
	}

	// Step-by-Step Turn Log Table
	sb.WriteString(fmt.Sprintf("\n  %s%sSWARM TURN TRAJECTORY LOG:%s\n", ColorBold, ColorWhite, ColorReset))
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("  %-6s %-16s %-14s %-8s %-18s %s\n", "TURN", "AGENT", "ROLE", "TOKENS", "HANDOFF TO", "OUTPUT SUMMARY"))
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────────\n")

	for _, turn := range report.TurnHistory {
		handoffDisp := turn.HandoffTarget
		if handoffDisp == "" {
			handoffDisp = "None (Terminal)"
		}

		outSnippet := strings.ReplaceAll(turn.OutputContent, "\n", " ")
		if len(outSnippet) > 32 {
			outSnippet = outSnippet[:29] + "..."
		}

		sb.WriteString(fmt.Sprintf("  %-6d %-16s %-14s %-8d %-18s %s\n",
			turn.TurnIndex, turn.AgentName, turn.Role, turn.TokensConsumed, handoffDisp, outSnippet))
	}

	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("%s%s========================================================================================%s\n", ColorBold, ColorCyan, ColorReset))

	return sb.String()
}

// RenderSwarmMermaid formats an interactive Mermaid graph topology of the swarm execution.
func RenderSwarmMermaid(report *SwarmReport) string {
	var sb strings.Builder

	sb.WriteString("```mermaid\n")
	if report.Topology == TopologyDebateJury {
		sb.WriteString("graph TD\n")
		sb.WriteString(fmt.Sprintf("    TOPIC[\"Debate Topic: %s\"] --> DEBATE\n", report.SwarmID))
		sb.WriteString("    subgraph DEBATE [\"Debate Jury Panel\"]\n")

		for _, turn := range report.TurnHistory {
			if turn.Role == RoleDebater {
				cleanID := strings.ReplaceAll(turn.AgentName, "-", "_")
				sb.WriteString(fmt.Sprintf("        %s[\"%s<br/>Tokens: %d\"]\n", cleanID, turn.AgentName, turn.TokensConsumed))
			}
		}
		sb.WriteString("    end\n")

		verdictClean := "JURY_VERDICT"
		vColor := "#d4edda"
		if report.Verdict != nil && !report.Verdict.ConsensusReached {
			vColor = "#fff3cd"
		}

		for _, turn := range report.TurnHistory {
			if turn.Role == RoleDebater {
				cleanID := strings.ReplaceAll(turn.AgentName, "-", "_")
				sb.WriteString(fmt.Sprintf("    %s -->|Ballot| %s\n", cleanID, verdictClean))
			}
		}

		verdictText := "Consensus Reached"
		if report.Verdict != nil {
			verdictText = fmt.Sprintf("Winner: %s<br/>Score: %.2f", report.Verdict.WinningPosition, report.Verdict.ConfidenceScore)
		}
		sb.WriteString(fmt.Sprintf("    %s[\"%s<br/>%s\"]\n", verdictClean, report.FinalAgent, verdictText))
		sb.WriteString(fmt.Sprintf("    style %s fill:%s,stroke:#333,stroke-width:2px\n", verdictClean, vColor))

	} else {
		// Handoff Topology
		sb.WriteString("graph LR\n")
		visitedNodes := make(map[string]bool)

		for i := 0; i < len(report.HandoffChain)-1; i++ {
			from := report.HandoffChain[i]
			to := report.HandoffChain[i+1]
			cleanFrom := strings.ReplaceAll(from, "-", "_")
			cleanTo := strings.ReplaceAll(to, "-", "_")

			visitedNodes[cleanFrom] = true
			visitedNodes[cleanTo] = true

			edgeStyle := "-->"
			if report.CycleDetected && i == len(report.HandoffChain)-2 {
				edgeStyle = "-.->|CYCLE DETECTED|"
			} else {
				edgeStyle = fmt.Sprintf("-->|Handoff #%d|", i+1)
			}

			sb.WriteString(fmt.Sprintf("    %s %s %s\n", cleanFrom, edgeStyle, cleanTo))
		}

		// Initial node styling
		if len(report.HandoffChain) > 0 {
			initClean := strings.ReplaceAll(report.HandoffChain[0], "-", "_")
			sb.WriteString(fmt.Sprintf("    style %s fill:#f1eefe,stroke:#6f42c1,stroke-width:2px\n", initClean))
		}

		// Terminal node styling
		if len(report.HandoffChain) > 1 {
			termClean := strings.ReplaceAll(report.HandoffChain[len(report.HandoffChain)-1], "-", "_")
			if report.CycleDetected {
				sb.WriteString(fmt.Sprintf("    style %s fill:#fbeae8,stroke:#dc3545,stroke-width:3px\n", termClean))
			} else {
				sb.WriteString(fmt.Sprintf("    style %s fill:#e6f4ea,stroke:#28a745,stroke-width:2px\n", termClean))
			}
		}
	}

	sb.WriteString("```\n")
	return sb.String()
}
