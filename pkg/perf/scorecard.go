package perf

import (
	"fmt"
	"strings"
)

// ANSI color codes for terminal formatting
const (
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
	colorGreen  = "\033[32m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorBlue   = "\033[34m"
)

// RenderPerformanceScorecard outputs a rich ANSI terminal dashboard of latency benchmark results.
func RenderPerformanceScorecard(report *SuiteReport) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("\n%s%s══════════════════════════════════════════════════════════════════════════════════════%s\n", colorBold, colorCyan, colorReset))
	sb.WriteString(fmt.Sprintf("%s%s   ⚡ AGENTVCR SUB-MILLISECOND LATENCY & PERFORMANCE SCORECARD%s\n", colorBold, colorCyan, colorReset))
	sb.WriteString(fmt.Sprintf("%s%s══════════════════════════════════════════════════════════════════════════════════════%s\n", colorBold, colorCyan, colorReset))

	sb.WriteString(fmt.Sprintf("  %sTimestamp:%s  %s    %sEnvironment:%s %s (%d CPUs)\n",
		colorDim, colorReset, report.Timestamp.Format("2006-01-02 15:04:05 UTC"),
		colorDim, colorReset, report.GoVersion, report.NumCPU))
	sb.WriteString(fmt.Sprintf("  %sSuite Run:%s   %.2f ms total duration\n\n",
		colorDim, colorReset, report.TotalDurationMs))

	// Table Header
	sb.WriteString(fmt.Sprintf("  %s%-36s %8s %10s %8s %8s %8s %8s %8s%s\n",
		colorBold, "SUBSYSTEM TARGET", "OPS", "OPS/SEC", "P50(µs)", "P90(µs)", "P99(µs)", "SLA(µs)", "STATUS", colorReset))
	sb.WriteString(fmt.Sprintf("  %s%s%s\n",
		colorDim, strings.Repeat("─", 102), colorReset))

	for _, r := range report.Results {
		statusBadge := fmt.Sprintf("%s%sPASS%s", colorBold, colorGreen, colorReset)
		if !r.PassesSLA {
			statusBadge = fmt.Sprintf("%s%sFAIL%s", colorBold, colorRed, colorReset)
		}

		sb.WriteString(fmt.Sprintf("  %-36s %8d %10.0f %8.1f %8.1f %8.1f %8.1f %8s\n",
			r.Name,
			r.Percentiles.TotalOps,
			r.Percentiles.OpsPerSec,
			r.Percentiles.P50Us,
			r.Percentiles.P90Us,
			r.Percentiles.P99Us,
			r.SLAThresholdUs,
			statusBadge,
		))

		// Second line: memory & allocations details
		if r.Percentiles.AllocsPerOp > 0 || r.Percentiles.BytesPerOp > 0 {
			sb.WriteString(fmt.Sprintf("    %s↳ Memory Overhead: %d allocs/op, %d B/op | Min: %.1fµs, Max: %.1fµs, StdDev: %.1fµs%s\n",
				colorDim, r.Percentiles.AllocsPerOp, r.Percentiles.BytesPerOp, r.Percentiles.MinUs, r.Percentiles.MaxUs, r.Percentiles.StdDevUs, colorReset))
		}
	}

	sb.WriteString(fmt.Sprintf("  %s%s%s\n\n",
		colorDim, strings.Repeat("─", 102), colorReset))

	if report.OverallPassed {
		sb.WriteString(fmt.Sprintf("  %s%s✔ OVERALL STATUS: ALL PERFORMANCE & LATENCY SLAS SATISFIED%s\n",
			colorBold, colorGreen, colorReset))
	} else {
		sb.WriteString(fmt.Sprintf("  %s%s✘ OVERALL STATUS: LATENCY REGRESSION DETECTED (BREACHED SLA)%s\n",
			colorBold, colorRed, colorReset))
	}

	sb.WriteString(fmt.Sprintf("%s%s══════════════════════════════════════════════════════════════════════════════════════%s\n\n", colorBold, colorCyan, colorReset))

	return sb.String()
}
