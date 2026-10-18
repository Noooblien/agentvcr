package shadow

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

// RenderShadowScorecard formats an ANSI terminal scorecard of the shadowing audit report.
func RenderShadowScorecard(report ShadowAuditReport) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("%s%s========================================================================================%s\n", ColorBold, ColorCyan, ColorReset))
	sb.WriteString(fmt.Sprintf("%s%s  AGENTVCR // CONTINUOUS MODEL SHADOWING & LIVE A/B DRIFT EVALUATION SCORECARD%s\n", ColorBold, ColorWhite, ColorReset))
	sb.WriteString(fmt.Sprintf("%s%s========================================================================================%s\n\n", ColorBold, ColorCyan, ColorReset))

	statusColor := ColorGreen
	switch report.Status {
	case "BLOCKED", "ROLLED_BACK":
		statusColor = ColorRed
	case "OBSERVING":
		statusColor = ColorYellow
	}

	sb.WriteString(fmt.Sprintf("  %sAudit ID:%s          %s\n", ColorBold, ColorReset, report.AuditID))
	sb.WriteString(fmt.Sprintf("  %sRouting Mode:%s      %s\n", ColorBold, ColorReset, report.Mode))
	sb.WriteString(fmt.Sprintf("  %sBaseline Model:%s    %s (%s)\n", ColorBold, ColorReset, report.BaselineModel.ModelName, report.BaselineModel.Provider))
	sb.WriteString(fmt.Sprintf("  %sCandidate Model:%s   %s (%s)\n", ColorBold, ColorReset, report.CandidateModel.ModelName, report.CandidateModel.Provider))
	sb.WriteString(fmt.Sprintf("  %sEvaluation Status:%s %s%s[%s]%s\n", ColorBold, ColorReset, ColorBold, statusColor, report.Status, ColorReset))

	if report.CircuitBreakerTripped {
		sb.WriteString(fmt.Sprintf("  %sCircuit Breaker:%s   %s%sTRIPPED: %s%s\n", ColorBold, ColorReset, ColorBold, ColorRed, report.CircuitBreakerReason, ColorReset))
	}

	costSymbol := "+"
	if report.GlobalCostDeltaPercent < 0 {
		costSymbol = ""
	}
	sb.WriteString(fmt.Sprintf("  %sScale & Metrics:%s   %d sessions, %d steps | Drift: %.2f | Latency Ratio: %.2fx | Cost Delta: %s%.1f%%\n\n",
		ColorBold, ColorReset, report.TotalSessionsShadowed, report.TotalStepsEvaluated,
		report.GlobalAverageDrift, report.GlobalLatencyRatio, costSymbol, report.GlobalCostDeltaPercent))

	// Step-Level Trajectory Comparison Breakdown
	sb.WriteString(fmt.Sprintf("  %s%sSTEP-LEVEL DIVERGENCE & TELEMETRY DELTAS:%s\n", ColorBold, ColorWhite, ColorReset))
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("  %-6s %-12s %-10s %-12s %-10s %-10s %s\n", "STEP", "TYPE", "DRIFT", "SEVERITY", "LATENCY", "TOKENS", "RATIONALE"))
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────────\n")

	for _, session := range report.Sessions {
		for _, step := range session.StepComparisons {
			sevColor := ColorGreen
			switch step.DriftSeverity {
			case SeverityModerate:
				sevColor = ColorYellow
			case SeverityHigh, SeverityCritical:
				sevColor = ColorRed
			}

			latStr := fmt.Sprintf("%+dms", step.LatencyDeltaMs)
			tokStr := fmt.Sprintf("%+d", step.TokenDelta)

			sb.WriteString(fmt.Sprintf("  %-6d %-12s %-10.2f %s%-12s%s %-10s %-10s %s\n",
				step.StepIndex, step.EventType, step.DriftScore, sevColor, step.DriftSeverity, ColorReset,
				latStr, tokStr, step.DriftRationale))
		}
	}

	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────────\n")

	// Recommendations
	if len(report.Recommendations) > 0 {
		sb.WriteString(fmt.Sprintf("\n  %s%sEXECUTIVE GATE RECOMMENDATIONS:%s\n", ColorBold, ColorWhite, ColorReset))
		for _, rec := range report.Recommendations {
			sb.WriteString(fmt.Sprintf("  • %s\n", rec))
		}
	}

	sb.WriteString(fmt.Sprintf("\n%s%s========================================================================================%s\n", ColorBold, ColorCyan, ColorReset))
	return sb.String()
}

// RenderShadowMermaid generates a Mermaid flowchart depicting baseline vs candidate comparison.
func RenderShadowMermaid(report ShadowAuditReport) string {
	var sb strings.Builder

	sb.WriteString("```mermaid\n")
	sb.WriteString("graph TD\n")
	sb.WriteString(fmt.Sprintf("    TRAFFIC[\"Live Agent Traffic (%s)\"] --> SPLIT{\"Shadow Router\"}\n", report.Mode))
	sb.WriteString(fmt.Sprintf("    SPLIT -->|Primary Path| BASE[\"Baseline: %s<br/>Latency: 1.0x | Cost: $%.4f\"]\n",
		report.BaselineModel.ModelName, report.BaselineModel.CostPer1kOutput))
	sb.WriteString(fmt.Sprintf("    SPLIT -.->|Shadow Mirror| CAND[\"Candidate: %s<br/>Latency: %.2fx | Cost: $%.4f\"]\n",
		report.CandidateModel.ModelName, report.GlobalLatencyRatio, report.CandidateModel.CostPer1kOutput))

	sb.WriteString("    BASE --> EVAL[\"Live Trajectory Comparator\"]\n")
	sb.WriteString("    CAND --> EVAL\n")

	gateColor := "#d4edda"
	gateNode := "GATE_PASS"
	if report.CircuitBreakerTripped {
		gateColor = "#f8d7da"
		gateNode = "GATE_FAIL"
	} else if report.Status == "OBSERVING" {
		gateColor = "#fff3cd"
		gateNode = "GATE_OBSERVING"
	}

	sb.WriteString(fmt.Sprintf("    EVAL --> %s[\"Status: %s<br/>Avg Drift: %.2f<br/>Cost Delta: %.1f%%\"]\n",
		gateNode, report.Status, report.GlobalAverageDrift, report.GlobalCostDeltaPercent))
	sb.WriteString(fmt.Sprintf("    style %s fill:%s,stroke:#333,stroke-width:2px\n", gateNode, gateColor))
	sb.WriteString("```\n")

	return sb.String()
}
