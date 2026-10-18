package harness

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

// RenderHarnessScorecard formats an ANSI terminal scorecard of benchmark pass@k results.
func RenderHarnessScorecard(report *BenchmarkSummaryReport) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("%s%s========================================================================================%s\n", ColorBold, ColorCyan, ColorReset))
	sb.WriteString(fmt.Sprintf("%s%s  AGENTVCR // STANDARDIZED BENCHMARK HARNESS & PASS@K LEADERBOARD%s\n", ColorBold, ColorWhite, ColorReset))
	sb.WriteString(fmt.Sprintf("%s%s========================================================================================%s\n\n", ColorBold, ColorCyan, ColorReset))

	gateColor := ColorGreen
	gateStatus := "[PASS]"
	if !report.GatePassed {
		gateColor = ColorRed
		gateStatus = "[FAIL]"
	}

	sb.WriteString(fmt.Sprintf("  %sSuite Title:%s        %s\n", ColorBold, ColorReset, report.SuiteName))
	sb.WriteString(fmt.Sprintf("  %sStandard:%s           %s\n", ColorBold, ColorReset, report.Standard))
	sb.WriteString(fmt.Sprintf("  %sQuality Gate:%s       %s%s%s (Min Pass@1 Required: 40.0%%)\n", ColorBold, ColorReset, gateColor, gateStatus, ColorReset))
	sb.WriteString(fmt.Sprintf("  %sAggregate Metrics:%s  Pass@1: %s%.1f%%%s | Pass@5: %s%.1f%%%s | Pass@10: %s%.1f%%%s\n",
		ColorBold, ColorReset,
		ColorGreen, report.MeanPassAt1*100, ColorReset,
		ColorCyan, report.MeanPassAt5*100, ColorReset,
		ColorMagenta, report.MeanPassAt10*100, ColorReset))
	sb.WriteString(fmt.Sprintf("  %sResource Totals:%s    %d tasks evaluated (%d trials run) | %d tokens | $%.4f USD | Avg Latency: %d ms\n\n",
		ColorBold, ColorReset, report.TotalTasks, report.TotalTrialsRun, report.TotalTokensConsumed, report.TotalCostUSD, report.AvgTaskDurationMs))

	// Task Breakdown Table
	sb.WriteString(fmt.Sprintf("  %s%sTASK-LEVEL PASS@K VERIFICATION MATRIX:%s\n", ColorBold, ColorWhite, ColorReset))
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("  %-20s %-10s %-10s %-10s %-12s %-12s %s\n", "TASK ID", "PASS/TOTAL", "PASS@1", "PASS@5", "AVG TOKENS", "AVG LATENCY", "STATUS"))
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────────\n")

	for _, agg := range report.TaskAggregates {
		statColor := ColorGreen
		statStr := "RESOLVED"
		if agg.PassedTrials == 0 {
			statColor = ColorRed
			statStr = "UNRESOLVED"
		} else if agg.PassedTrials < agg.TotalTrials {
			statColor = ColorYellow
			statStr = "PARTIAL"
		}

		sb.WriteString(fmt.Sprintf("  %-20s %-10s %-10s %-10s %-12d %-12s %s%s%s\n",
			agg.TaskID,
			fmt.Sprintf("%d/%d", agg.PassedTrials, agg.TotalTrials),
			fmt.Sprintf("%.1f%%", agg.PassAt1*100),
			fmt.Sprintf("%.1f%%", agg.PassAt5*100),
			agg.AvgTokens,
			fmt.Sprintf("%dms", agg.AvgLatencyMs),
			statColor, statStr, ColorReset))
	}

	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("%s%s========================================================================================%s\n", ColorBold, ColorCyan, ColorReset))

	return sb.String()
}

// RenderHarnessMermaid formats a Mermaid diagram depicting the pass@k evaluation pipeline and outcomes.
func RenderHarnessMermaid(report *BenchmarkSummaryReport) string {
	var sb strings.Builder

	sb.WriteString("```mermaid\n")
	sb.WriteString("graph TD\n")
	sb.WriteString(fmt.Sprintf("    SUITE[\"Benchmark Suite: %s<br/>(%s)\"] --> HARNESS[\"AgentVCR Evaluation Harness\"]\n", report.SuiteName, report.Standard))
	sb.WriteString("    HARNESS --> DISPATCH[\"Task Trial Dispatcher (N Samples)\"]\n")

	resolvedPercent := 0.0
	if report.TotalTasks > 0 {
		resolvedPercent = (float64(report.ResolvedTasksCount) / float64(report.TotalTasks)) * 100.0
	}

	sb.WriteString(fmt.Sprintf("    DISPATCH --> PASS1[\"Pass@1: %.1f%%<br/>Single-Attempt Accuracy\"]\n", report.MeanPassAt1*100))
	sb.WriteString(fmt.Sprintf("    DISPATCH --> PASS5[\"Pass@5: %.1f%%<br/>Multi-Sample Coverage\"]\n", report.MeanPassAt5*100))
	sb.WriteString(fmt.Sprintf("    PASS1 --> GATE{\"Quality Gate<br/>Min 40%%\"}\n"))

	gateClass := "GATE_PASS"
	gateColor := "#d4edda"
	if !report.GatePassed {
		gateClass = "GATE_FAIL"
		gateColor = "#f8d7da"
	}

	sb.WriteString(fmt.Sprintf("    GATE --> %s[\"Status: %s<br/>Tasks Resolved: %d/%d (%.1f%%)\"]\n",
		gateClass,
		map[bool]string{true: "PASSED", false: "FAILED"}[report.GatePassed],
		report.ResolvedTasksCount, report.TotalTasks, resolvedPercent))
	sb.WriteString(fmt.Sprintf("    style %s fill:%s,stroke:#333,stroke-width:2px\n", gateClass, gateColor))
	sb.WriteString("```\n")

	return sb.String()
}
