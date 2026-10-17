package finops

import (
	"fmt"
	"strings"
)

// RenderFinOpsScorecard renders an ANSI terminal dashboard visualizing multi-agent spend, utilization, and circuit breakers.
func RenderFinOpsScorecard(report *FinOpsAuditReport) string {
	var sb strings.Builder

	sb.WriteString("\033[1;32m========================================================================================\033[0m\n")
	sb.WriteString("  \033[1;37mAGENTVCR // FINOPS TOKEN BUDGET CIRCUIT BREAKER & RUNAWAY COST SCORECARD\033[0m\n")
	sb.WriteString("\033[1;32m========================================================================================\033[0m\n\n")

	sb.WriteString(fmt.Sprintf("  \033[1;37mSystem ID:\033[0m        %s\n", report.SystemID))
	sb.WriteString(fmt.Sprintf("  \033[1;37mTimestamp:\033[0m        %s\n", report.Timestamp.Format("2006-01-02 15:04:05 UTC")))

	healthBadge := "\033[1;32m[PASS] ALL AGENT BUDGETS HEALTHY & WITHIN SLA\033[0m"
	if !report.AllCompliant {
		healthBadge = "\033[1;31m[ALERT] RUNAWAY SPEND DETECTED // CIRCUIT BREAKER TRIPPED\033[0m"
	}
	sb.WriteString(fmt.Sprintf("  \033[1;37mFinOps Health:\033[0m    %s\n\n", healthBadge))

	sb.WriteString("  \033[1;33mPORTFOLIO FINANCIAL & TOKEN SUMMARY:\033[0m\n")
	sb.WriteString(fmt.Sprintf("  • Monitored Agents:   \033[1;37m%d\033[0m\n", report.TotalAgents))
	sb.WriteString(fmt.Sprintf("  • Total Incurred Spend:\033[1;37m$%.4f\033[0m\n", report.TotalCostUSD))
	sb.WriteString(fmt.Sprintf("  • Total Tokens Burned:\033[1;37m%d\033[0m\n", report.TotalTokens))
	sb.WriteString(fmt.Sprintf("  • Downgrade Savings:  \033[1;32m$%.4f saved via auto-downgrade\033[0m\n", report.TotalDollarsSaved))

	sb.WriteString("\n  \033[1;33mAGENT BUDGET UTILIZATION & CIRCUIT STATUS:\033[0m\n")
	sb.WriteString("  " + strings.Repeat("─", 84) + "\n")
	sb.WriteString(fmt.Sprintf("  %-18s %-12s %-10s %-12s %-12s %s\n",
		"AGENT ID", "STATUS", "SPENT", "REMAINING", "UTILIZATION", "BURN/MIN"))
	sb.WriteString("  " + strings.Repeat("─", 84) + "\n")

	for _, tel := range report.AgentsTelemetry {
		statusColor := "\033[32m" // Green
		if tel.CircuitTripped {
			statusColor = "\033[31m" // Red
		} else if tel.CurrentAction == ActionDowngrade {
			statusColor = "\033[35m" // Magenta
		} else if tel.CurrentAction == ActionThrottle || tel.CurrentAction == ActionWarn {
			statusColor = "\033[33m" // Yellow
		}

		statusLabel := string(tel.CurrentAction)
		if tel.CircuitTripped {
			statusLabel = "KILLED"
		}

		sb.WriteString(fmt.Sprintf("  %-18s %s%-12s\033[0m $%-9.4f $%-11.4f %-12s $%.4f/m\n",
			tel.AgentID,
			statusColor,
			statusLabel,
			tel.TotalCostUSD,
			tel.RemainingBudgetUSD,
			fmt.Sprintf("%.1f%%", tel.BudgetUtilization*100.0),
			tel.BurnRateUSDPerMin,
		))
	}
	sb.WriteString("  " + strings.Repeat("─", 84) + "\n")

	// Print kill reasons if any
	hasKills := false
	for _, tel := range report.AgentsTelemetry {
		if tel.CircuitTripped {
			if !hasKills {
				sb.WriteString("\n  \033[1;31mCIRCUIT BREAKER TRIP AUDIT LOG:\033[0m\n")
				hasKills = true
			}
			sb.WriteString(fmt.Sprintf("  • Agent \033[1;37m%s\033[0m: %s\n", tel.AgentID, tel.HardKillReason))
		}
	}

	sb.WriteString("\n\033[1;32m========================================================================================\033[0m\n")
	return sb.String()
}

// RenderFinOpsMermaid generates a Mermaid flowchart depicting agent budget limits and circuit breaker trips.
func RenderFinOpsMermaid(report *FinOpsAuditReport) string {
	var sb strings.Builder

	sb.WriteString("```mermaid\n")
	sb.WriteString("graph TD\n")
	sb.WriteString("    CLIENT[\"Agent Orchestration Swarm\"] --> GW[\"FinOps Guardrail Manager\"]\n")
	sb.WriteString("    GW --> PREFLIGHT{\"Pre-Flight Budget &<br/>Burn-Rate Check\"}\n\n")

	for _, tel := range report.AgentsTelemetry {
		nodeID := strings.ReplaceAll(tel.AgentID, "-", "_")
		if tel.CircuitTripped {
			sb.WriteString(fmt.Sprintf("    PREFLIGHT -->|\"Exceeded Budget\"| TRIP_%s[\"Agent: %s<br/>[CIRCUIT TRIPPED: KILLED]<br/>Spend: $%.4f\"]\n",
				nodeID, tel.AgentID, tel.TotalCostUSD))
			sb.WriteString(fmt.Sprintf("    style TRIP_%s fill:#f8d7da,stroke:#dc3545,stroke-width:2px\n", nodeID))
		} else if tel.CurrentAction == ActionDowngrade {
			sb.WriteString(fmt.Sprintf("    PREFLIGHT -->|\"Near Budget Limit\"| DOWN_%s[\"Agent: %s<br/>[AUTO-DOWNGRADE]<br/>Model: %s\"]\n",
				nodeID, tel.AgentID, tel.ActiveModelOverride))
			sb.WriteString(fmt.Sprintf("    style DOWN_%s fill:#e2e3e5,stroke:#6c757d,stroke-width:2px\n", nodeID))
		} else {
			sb.WriteString(fmt.Sprintf("    PREFLIGHT -->|\"Within SLA\"| OK_%s[\"Agent: %s<br/>[ALLOW: Healthy]<br/>Util: %.1f%%\"]\n",
				nodeID, tel.AgentID, tel.BudgetUtilization*100.0))
			sb.WriteString(fmt.Sprintf("    style OK_%s fill:#d4edda,stroke:#28a745,stroke-width:2px\n", nodeID))
		}
	}

	sb.WriteString("```\n")
	return sb.String()
}
