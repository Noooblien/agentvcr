package perf

import (
	"fmt"
	"strings"
)

// RenderPerformanceMermaid formats benchmark results as a Mermaid flowchart visualization.
func RenderPerformanceMermaid(report *SuiteReport) string {
	var sb strings.Builder

	sb.WriteString("```mermaid\n")
	sb.WriteString("flowchart TD\n")
	sb.WriteString(fmt.Sprintf("    Suite([\"AgentVCR Micro-Benchmark Suite (%.1f ms)\"])\n", report.TotalDurationMs))

	for i, r := range report.Results {
		nodeID := fmt.Sprintf("Target%d", i)
		statusColor := "#52c41a"
		if !r.PassesSLA {
			statusColor = "#ff4d4f"
		}

		sb.WriteString(fmt.Sprintf("    Suite --> %s[\"%s<br/>P50: %.1fµs | P99: %.1fµs | %.0f ops/s\"]\n",
			nodeID, r.Name, r.Percentiles.P50Us, r.Percentiles.P99Us, r.Percentiles.OpsPerSec))
		sb.WriteString(fmt.Sprintf("    style %s fill:%s,stroke:#333,stroke-width:1px\n", nodeID, statusColor))
	}

	sb.WriteString("```\n")
	return sb.String()
}

// RenderPerformanceMarkdown formats benchmark results as a GitHub Actions markdown table report.
func RenderPerformanceMarkdown(report *SuiteReport) string {
	var sb strings.Builder

	sb.WriteString("## AgentVCR Sub-Millisecond Latency & Micro-Benchmark Audit Report\n\n")
	sb.WriteString(fmt.Sprintf("- **Timestamp**: `%s`\n", report.Timestamp.Format("2006-01-02 15:04:05 UTC")))
	sb.WriteString(fmt.Sprintf("- **Environment**: `%s (%d CPUs)`\n", report.GoVersion, report.NumCPU))
	sb.WriteString(fmt.Sprintf("- **Total Duration**: `%.2f ms`\n", report.TotalDurationMs))

	statusText := "✅ ALL SLAS SATISFIED"
	if !report.OverallPassed {
		statusText = "❌ LATENCY REGRESSION DETECTED"
	}
	sb.WriteString(fmt.Sprintf("- **Overall Verdict**: `%s`\n\n", statusText))

	sb.WriteString("| Subsystem Target | Operations | Ops/Sec | P50 (µs) | P90 (µs) | P99 (µs) | SLA Limit | Verdict |\n")
	sb.WriteString("|:---|:---|:---|:---|:---|:---|:---|:---|\n")

	for _, r := range report.Results {
		verdict := "✅ PASS"
		if !r.PassesSLA {
			verdict = "❌ FAIL"
		}
		sb.WriteString(fmt.Sprintf("| `%s` | %d | `%.0f` | `%.1f` | `%.1f` | `%.1f` | `%.1f µs` | %s |\n",
			r.Name, r.Percentiles.TotalOps, r.Percentiles.OpsPerSec,
			r.Percentiles.P50Us, r.Percentiles.P90Us, r.Percentiles.P99Us, r.SLAThresholdUs, verdict))
	}

	return sb.String()
}
