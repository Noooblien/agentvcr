package sentry

import (
	"fmt"
	"strings"
)

// ANSI color codes.
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[1;31m"
	colorGreen  = "\033[1;32m"
	colorYellow = "\033[1;33m"
	colorCyan   = "\033[1;36m"
	colorGray   = "\033[0;90m"
	colorBold   = "\033[1m"
)

// RenderSentryScorecard generates an ANSI terminal dashboard summarizing the trajectory anomaly analysis.
func RenderSentryScorecard(verdict SentryVerdict, sessionID string) string {
	var sb strings.Builder

	statusBadge := fmt.Sprintf("%s[HEALTHY / PASS]%s", colorGreen, colorReset)
	if verdict.Action == ActionTripCircuitBreaker {
		statusBadge = fmt.Sprintf("%s[CIRCUIT BREAKER TRIPPED]%s", colorRed, colorReset)
	} else if verdict.Action == ActionThrottle {
		statusBadge = fmt.Sprintf("%s[TRAJECTORY THROTTLED]%s", colorYellow, colorReset)
	}

	riskColor := colorGreen
	if verdict.RiskScore >= 75.0 {
		riskColor = colorRed
	} else if verdict.RiskScore >= 40.0 {
		riskColor = colorYellow
	}

	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("%s╭────────────────────────────────────────────────────────────────────────────╮%s\n", colorCyan, colorReset))
	sb.WriteString(fmt.Sprintf("%s│%s  📡  %sAgentVCR Trajectory Anomaly Sentry & Circuit Breaker%s               %s│%s\n", colorCyan, colorReset, colorBold, colorReset, colorCyan, colorReset))
	sb.WriteString(fmt.Sprintf("%s├────────────────────────────────────────────────────────────────────────────┤%s\n", colorCyan, colorReset))
	sb.WriteString(fmt.Sprintf("│  %sSession ID:%s       %-30s  %sEvaluation:%s %-15s │\n", colorBold, colorReset, sessionID, colorBold, colorReset, fmt.Sprintf("%d µs", verdict.EvaluationDurUs)))
	sb.WriteString(fmt.Sprintf("│  %sTotal Steps:%s      %-30d  %sVerdict:%s    %-25s │\n", colorBold, colorReset, verdict.Metrics.TotalSteps, colorBold, colorReset, statusBadge))
	sb.WriteString(fmt.Sprintf("│  %sRisk Score:%s       %s%-6.1f / 100.0%s                %sAction:%s     %-16s │\n", colorBold, colorReset, riskColor, verdict.RiskScore, colorReset, colorBold, colorReset, verdict.Action))
	sb.WriteString(fmt.Sprintf("│  %sTotal Tokens:%s     %-30d  %sBurn Rate:%s  %-15s │\n", colorBold, colorReset, verdict.Metrics.TotalTokens, colorBold, colorReset, fmt.Sprintf("%.1f tok/s", verdict.Metrics.TokenBurnRate)))
	sb.WriteString(fmt.Sprintf("│  %sAvg Step Latency:%s %-30s  %sMax Latency:%s %-15s │\n", colorBold, colorReset, fmt.Sprintf("%.1f ms (σ=%.1f)", verdict.Metrics.AvgStepLatencyMs, verdict.Metrics.StdDevLatencyMs), colorBold, colorReset, fmt.Sprintf("%.1f ms", verdict.Metrics.MaxStepLatencyMs)))
	sb.WriteString(fmt.Sprintf("│  %sRepetition Index:%s %-30.2f  %sConsec Errors:%s %-14d │\n", colorBold, colorReset, verdict.Metrics.RepetitionIndex, colorBold, colorReset, verdict.Metrics.ConsecutiveErrors))
	sb.WriteString(fmt.Sprintf("│  %sSummary:%s          %-56s │\n", colorBold, colorReset, truncate(verdict.Summary, 56)))

	if len(verdict.Anomalies) > 0 {
		sb.WriteString(fmt.Sprintf("%s├────────────────────────────────────────────────────────────────────────────┤%s\n", colorCyan, colorReset))
		sb.WriteString(fmt.Sprintf("│  %sDetected Trajectory Anomalies (%d found):%s                                  │\n", colorRed, len(verdict.Anomalies), colorReset))
		for _, a := range verdict.Anomalies {
			sevColor := colorYellow
			if a.Severity == SeverityCritical {
				sevColor = colorRed
			}
			toolStr := ""
			if a.ToolName != "" {
				toolStr = fmt.Sprintf("[%s] ", a.ToolName)
			}
			sb.WriteString(fmt.Sprintf("│  %s[%-8s]%s Step %-3d %s%-45s │\n", sevColor, a.Severity, colorReset, a.StepIndex, toolStr, truncate(a.Message, 45)))
		}
	}

	sb.WriteString(fmt.Sprintf("%s╰────────────────────────────────────────────────────────────────────────────╯%s\n", colorCyan, colorReset))
	return sb.String()
}

// RenderSentryMermaid produces a Mermaid flowchart visualizing trajectory health triage.
func RenderSentryMermaid(verdict SentryVerdict) string {
	var sb strings.Builder

	sb.WriteString("```mermaid\n")
	sb.WriteString("flowchart TD\n")
	sb.WriteString("    %% AgentVCR Sentry Telemetry Triage\n")
	sb.WriteString("    INGEST[\"Trajectory Flight Recorder Stream<br/>(Steps: " + fmt.Sprintf("%d, Tokens: %d)", verdict.Metrics.TotalSteps, verdict.Metrics.TotalTokens) + "\"]\n")
	sb.WriteString("    INGEST --> SENTRY{\"Sentry Anomaly Engine\"}\n\n")

	sb.WriteString(fmt.Sprintf("    SENTRY -->|Score: %.1f/100| GAUGE[\"Risk Aggregation Engine\"]\n", verdict.RiskScore))

	if len(verdict.Anomalies) == 0 {
		sb.WriteString("    SENTRY -->|Zero Anomalies| PASS[\"ALLOW<br/>Execution Nominal\"]\n")
		sb.WriteString("    style PASS fill:#d4edda,stroke:#28a745,color:#155724\n")
	} else {
		for i, a := range verdict.Anomalies {
			nodeID := fmt.Sprintf("A%d", i)
			sb.WriteString(fmt.Sprintf("    SENTRY -->|%s| %s[\"%s<br/>Step %d: %s\"]\n", a.Severity, nodeID, a.Type, a.StepIndex, escapeMermaid(truncate(a.Message, 30))))
			if a.Severity == SeverityCritical {
				sb.WriteString(fmt.Sprintf("    style %s fill:#f8d7da,stroke:#dc3545,color:#721c24\n", nodeID))
			} else {
				sb.WriteString(fmt.Sprintf("    style %s fill:#fff3cd,stroke:#ffc107,color:#856404\n", nodeID))
			}
		}
	}

	switch verdict.Action {
	case ActionPass:
		sb.WriteString("    GAUGE --> ACTION[\"STATUS: ALLOW<br/>Normal Execution Granted\"]\n")
		sb.WriteString("    style ACTION fill:#d4edda,stroke:#28a745,color:#155724\n")
	case ActionThrottle:
		sb.WriteString("    GAUGE --> ACTION[\"STATUS: THROTTLE<br/>Velocity Decreased (Sub-critical Anomaly)\"]\n")
		sb.WriteString("    style ACTION fill:#fff3cd,stroke:#ffc107,color:#856404\n")
	case ActionTripCircuitBreaker:
		sb.WriteString("    GAUGE --> ACTION[\"STATUS: CIRCUIT BREAKER TRIPPED<br/>Execution Halted to Prevent Loss\"]\n")
		sb.WriteString("    style ACTION fill:#f8d7da,stroke:#dc3545,color:#721c24\n")
	}

	sb.WriteString("```\n")
	return sb.String()
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

func escapeMermaid(s string) string {
	s = strings.ReplaceAll(s, "\"", "'")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
