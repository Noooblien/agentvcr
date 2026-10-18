package mcp

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

// FuzzReport summarizes the outcome of an adversarial MCP fuzzing session.
type FuzzReport struct {
	ToolName        string            `json:"tool_name"`
	TotalMutations  int               `json:"total_mutations"`
	BlockedCount    int               `json:"blocked_count"`
	HandledCount    int               `json:"handled_count"`
	PanicCount      int               `json:"panic_count"`
	ResilienceScore float64           `json:"resilience_score"` // 0.0 - 100.0%
	Results         []FuzzTrialResult `json:"results"`
}

// RenderFuzzScorecard formats an ANSI terminal scorecard of fuzzing outcomes.
func RenderFuzzScorecard(report FuzzReport) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("%s%s========================================================================================%s\n", ColorBold, ColorCyan, ColorReset))
	sb.WriteString(fmt.Sprintf("%s%s  AGENTVCR // MODEL CONTEXT PROTOCOL (MCP) ADVERSARIAL SCHEMA FUZZ SCORECARD%s\n", ColorBold, ColorWhite, ColorReset))
	sb.WriteString(fmt.Sprintf("%s%s========================================================================================%s\n\n", ColorBold, ColorCyan, ColorReset))

	scoreColor := ColorGreen
	if report.ResilienceScore < 95.0 {
		scoreColor = ColorYellow
	}
	if report.ResilienceScore < 85.0 || report.PanicCount > 0 {
		scoreColor = ColorRed
	}

	sb.WriteString(fmt.Sprintf("  %sTarget Tool:%s         %s%s%s\n", ColorBold, ColorReset, ColorCyan, report.ToolName, ColorReset))
	sb.WriteString(fmt.Sprintf("  %sResilience Score:%s    %s%s%.1f%%%s\n", ColorBold, ColorReset, ColorBold, scoreColor, report.ResilienceScore, ColorReset))
	sb.WriteString(fmt.Sprintf("  %sTotal Mutations:%s     %d (Blocked/Defended: %s%d%s, Server Panics: %s%d%s)\n",
		ColorBold, ColorReset, report.TotalMutations, ColorGreen, report.BlockedCount, ColorReset, ColorRed, report.PanicCount, ColorReset))
	sb.WriteString(fmt.Sprintf("  %sHandled by Server:%s   %d\n\n", ColorBold, ColorReset, report.HandledCount))

	sb.WriteString(fmt.Sprintf("  %s%sADVERSARIAL MUTATION TRIAL RESULTS:%s\n", ColorBold, ColorWhite, ColorReset))
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("  %-16s %-20s %-16s %-12s %s\n", "CASE ID", "STRATEGY", "TARGET FIELD", "OUTCOME", "DEFENSE STATUS"))
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────\n")

	for _, res := range report.Results {
		outcome := fmt.Sprintf("%sPASS%s", ColorGreen, ColorReset)
		status := "Blocked by Validator"

		if res.ServerPanic {
			outcome = fmt.Sprintf("%sPANIC%s", ColorRed, ColorReset)
			status = "CRITICAL: Server Crashed"
		} else if !res.BlockedByVal && !res.ServerHandled {
			outcome = fmt.Sprintf("%sLEAK%s", ColorRed, ColorReset)
			status = "Undetected Adversarial Payload"
		} else if !res.BlockedByVal && res.ServerHandled {
			outcome = fmt.Sprintf("%sHANDLED%s", ColorYellow, ColorReset)
			status = "Sanitized by Handler"
		}

		sb.WriteString(fmt.Sprintf("  %-16s %-20s %-16s %-12s %s\n",
			res.CaseID, res.Strategy, res.TargetField, outcome, status))
	}
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────\n\n")

	sb.WriteString(fmt.Sprintf("%s%s========================================================================================%s\n", ColorBold, ColorCyan, ColorReset))
	return sb.String()
}

// RenderFuzzMermaid generates a Mermaid flowchart diagram depicting the fuzzing matrix.
func RenderFuzzMermaid(report FuzzReport) string {
	var sb strings.Builder

	sb.WriteString("```mermaid\n")
	sb.WriteString("graph TD\n")
	sb.WriteString(fmt.Sprintf("    SCHEMA[\"MCP Tool InputSchema: %s\"] --> FUZZER[\"Schema Mutation Fuzzer\"]\n", report.ToolName))
	sb.WriteString("    FUZZER --> T1[\"Type Confusion\"]\n")
	sb.WriteString("    FUZZER --> T2[\"Boundary Values\"]\n")
	sb.WriteString("    FUZZER --> T3[\"Unicode & BIDI Override\"]\n")
	sb.WriteString("    FUZZER --> T4[\"Prompt Injection\"]\n")
	sb.WriteString("    FUZZER --> T5[\"Buffer Overflow (16KB)\"]\n")
	sb.WriteString("    FUZZER --> T6[\"Null-Byte Traversal\"]\n")
	sb.WriteString("    FUZZER --> T7[\"Deep Nested Recursion\"]\n")
	sb.WriteString("    FUZZER --> T8[\"Missing Mandatory Fields\"]\n\n")

	sb.WriteString(fmt.Sprintf("    T1 --> VAL{\"Validator & Hypervisor\"}\n"))
	sb.WriteString(fmt.Sprintf("    T2 --> VAL\n"))
	sb.WriteString(fmt.Sprintf("    T3 --> VAL\n"))
	sb.WriteString(fmt.Sprintf("    T4 --> VAL\n"))
	sb.WriteString(fmt.Sprintf("    T5 --> VAL\n"))
	sb.WriteString(fmt.Sprintf("    T6 --> VAL\n"))
	sb.WriteString(fmt.Sprintf("    T7 --> VAL\n"))
	sb.WriteString(fmt.Sprintf("    T8 --> VAL\n\n"))

	sb.WriteString("    VAL -->|Valid & Blocked| DEFENDED[\"Defended: 100% Resilience\"]\n")
	if report.PanicCount > 0 {
		sb.WriteString("    VAL -->|Server Crash| CRASH[\"Panics: Security Violation\"]\n")
	}

	sb.WriteString("```\n")
	return sb.String()
}

// RenderRouterScorecard formats an ANSI terminal scorecard of MCP multi-server routing telemetry.
func RenderRouterScorecard(report *RouterAuditReport) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("%s%s========================================================================================%s\n", ColorBold, ColorMagenta, ColorReset))
	sb.WriteString(fmt.Sprintf("%s%s  AGENTVCR // MODEL CONTEXT PROTOCOL (MCP) ROUTER & LOAD BALANCER SCORECARD%s\n", ColorBold, ColorWhite, ColorReset))
	sb.WriteString(fmt.Sprintf("%s%s========================================================================================%s\n\n", ColorBold, ColorMagenta, ColorReset))

	statusColor := ColorGreen
	statusStr := "[OPTIMAL]"
	if report.HealthyUpstreams < report.TotalUpstreams {
		statusColor = ColorYellow
		statusStr = "[DEGRADED]"
	}
	if report.HealthyUpstreams == 0 {
		statusColor = ColorRed
		statusStr = "[CRITICAL_OUTAGE]"
	}

	sb.WriteString(fmt.Sprintf("  %sRouting Policy:%s      %s%s%s\n", ColorBold, ColorReset, ColorCyan, report.Policy, ColorReset))
	sb.WriteString(fmt.Sprintf("  %sCluster Health:%s      %s%s%s (%d/%d Upstream Servers Available)\n",
		ColorBold, ColorReset, statusColor, statusStr, ColorReset, report.HealthyUpstreams, report.TotalUpstreams))
	sb.WriteString(fmt.Sprintf("  %sTraffic Telemetry:%s   %d Total Calls Routed | %d Failures Auto-Recovered\n\n",
		ColorBold, ColorReset, report.TotalRequestsRouted, report.FailedRequestsTotal))

	// Upstream Server Breakdown Table
	sb.WriteString(fmt.Sprintf("  %s%sUPSTREAM MCP SERVER INSTANCE POOL:%s\n", ColorBold, ColorWhite, ColorReset))
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("  %-20s %-12s %-12s %-10s %-10s %-10s %s\n", "SERVER ID", "CIRCUIT", "HEALTH", "ACTIVE", "ROUTED", "FAILED", "AVG LATENCY"))
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────────\n")

	for _, srv := range report.Servers {
		circColor := ColorGreen
		if srv.CircuitState == CircuitOpen {
			circColor = ColorRed
		} else if srv.CircuitState == CircuitHalfOpen {
			circColor = ColorYellow
		}

		healthStr := fmt.Sprintf("%sONLINE%s", ColorGreen, ColorReset)
		if !srv.Healthy {
			healthStr = fmt.Sprintf("%sOFFLINE%s", ColorRed, ColorReset)
		}

		sb.WriteString(fmt.Sprintf("  %-20s %s%-12s%s %-12s %-10d %-10d %-10d %.1fms\n",
			srv.ID,
			circColor, srv.CircuitState, ColorReset,
			healthStr,
			srv.ActiveRequests,
			srv.TotalRequests,
			srv.FailedRequests,
			srv.AverageRTTMs,
		))
	}

	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("%s%s========================================================================================%s\n", ColorBold, ColorMagenta, ColorReset))

	return sb.String()
}

// RenderRouterMermaid formats a Mermaid diagram depicting the MCP multi-server routing topology.
func RenderRouterMermaid(report *RouterAuditReport) string {
	var sb strings.Builder

	sb.WriteString("```mermaid\n")
	sb.WriteString("graph TD\n")
	sb.WriteString("    CLIENT[\"Agent Runtime (MCP Client)\"] --> ROUTER[\"AgentVCR MCP Router & Load Balancer\"]\n")
	sb.WriteString(fmt.Sprintf("    ROUTER -->|Policy: %s| POOL{\"Upstream Server Pool\"}\n", report.Policy))

	for i, srv := range report.Servers {
		nodeID := fmt.Sprintf("SRV%d", i+1)
		circClass := "fill:#d4edda,stroke:#28a745,stroke-width:2px"
		if srv.CircuitState == CircuitOpen {
			circClass = "fill:#f8d7da,stroke:#dc3545,stroke-width:2px"
		} else if srv.CircuitState == CircuitHalfOpen {
			circClass = "fill:#fff3cd,stroke:#ffc107,stroke-width:2px"
		}

		sb.WriteString(fmt.Sprintf("    POOL --> %s[\"%s<br/>(%s) | Circuit: %s<br/>Routed: %d | Avg: %.1fms\"]\n",
			nodeID, srv.ID, srv.Name, srv.CircuitState, srv.TotalRequests, srv.AverageRTTMs))
		sb.WriteString(fmt.Sprintf("    style %s %s\n", nodeID, circClass))
	}

	sb.WriteString("```\n")
	return sb.String()
}

