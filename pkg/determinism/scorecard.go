package determinism

import (
	"encoding/json"
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

// RenderScorecard produces a clean ANSI terminal dashboard for the determinism benchmark.
func RenderScorecard(report *DeterminismReport) string {
	var sb strings.Builder

	statusBadge := fmt.Sprintf("%s[PASS / DETERMINISTIC]%s", colorGreen, colorReset)
	if report.Status == StatusBreach {
		statusBadge = fmt.Sprintf("%s[BREACH / DIVERGED]%s", colorRed, colorReset)
	} else if report.Status == StatusDegraded {
		statusBadge = fmt.Sprintf("%s[DEGRADED]%s", colorYellow, colorReset)
	}

	scoreColor := colorGreen
	if report.DeterminismScore < 70.0 {
		scoreColor = colorRed
	} else if report.DeterminismScore < 85.0 {
		scoreColor = colorYellow
	}

	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("%s╭────────────────────────────────────────────────────────────────────────────╮%s\n", colorCyan, colorReset))
	sb.WriteString(fmt.Sprintf("%s│%s  🎯  %sAgentVCR Trajectory Replay Determinism & Token Benchmark%s          %s│%s\n", colorCyan, colorReset, colorBold, colorReset, colorCyan, colorReset))
	sb.WriteString(fmt.Sprintf("%s├────────────────────────────────────────────────────────────────────────────┤%s\n", colorCyan, colorReset))
	sb.WriteString(fmt.Sprintf("│  %sBaseline ID:%s     %-28s  %sExecution:%s    %-12s │\n", colorBold, colorReset, truncateStr(report.BaselineID, 28), colorBold, colorReset, fmt.Sprintf("%d µs", report.ExecutionDurUs)))
	sb.WriteString(fmt.Sprintf("│  %sTarget ID:%s       %-28s  %sStatus:%s       %-22s │\n", colorBold, colorReset, truncateStr(report.TargetID, 28), colorBold, colorReset, statusBadge))
	sb.WriteString(fmt.Sprintf("│  %sDeterminism Score:%s %s%-6.1f / 100.0%s                 %sTool Parity:%s  %-12s │\n", colorBold, colorReset, scoreColor, report.DeterminismScore, colorReset, colorBold, colorReset, fmt.Sprintf("%.1f%%", report.Metrics.ToolParityPercent)))
	sb.WriteString(fmt.Sprintf("│  %sArgument Parity:%s   %-28s  %sChecksum Match:%s%-11s │\n", colorBold, colorReset, fmt.Sprintf("%.1f%%", report.Metrics.ArgumentParityPercent), colorBold, colorReset, fmt.Sprintf("%.1f%%", report.Metrics.ChecksumParityPercent)))
	sb.WriteString(fmt.Sprintf("│  %sToken Baseline:%s   %-28s  %sToken Replay:%s  %-12s │\n", colorBold, colorReset, fmt.Sprintf("%d tokens", report.Metrics.Tokens.BaselineTokens), colorBold, colorReset, fmt.Sprintf("%d tokens", report.Metrics.Tokens.TargetTokens)))
	sb.WriteString(fmt.Sprintf("│  %sToken Delta:%s      %-28s  %sBloat Ratio:%s   %-12s │\n", colorBold, colorReset, fmt.Sprintf("%+d tokens", report.Metrics.Tokens.DeltaTokens), colorBold, colorReset, fmt.Sprintf("%.2fx", report.Metrics.Tokens.BloatRatio)))
	sb.WriteString(fmt.Sprintf("│  %sReplay Latency P50:%s%-28s  %sP95 / P99:%s     %-12s │\n", colorBold, colorReset, fmt.Sprintf("%.1f ms", report.Metrics.Latency.P50Ms), colorBold, colorReset, fmt.Sprintf("%.1f / %.1f ms", report.Metrics.Latency.P95Ms, report.Metrics.Latency.P99Ms)))
	sb.WriteString(fmt.Sprintf("│  %sStep Jitter (σ):%s  %-28s  %sProxy Overhead:%s%-12s │\n", colorBold, colorReset, fmt.Sprintf("%.1f ms", report.Metrics.Latency.JitterMs), colorBold, colorReset, fmt.Sprintf("%d µs", report.Metrics.Latency.ProxyOverheadUs)))
	sb.WriteString(fmt.Sprintf("│  %sSummary:%s          %-56s │\n", colorBold, colorReset, truncateStr(report.Summary, 56)))

	if len(report.Divergences) > 0 {
		sb.WriteString(fmt.Sprintf("%s├────────────────────────────────────────────────────────────────────────────┤%s\n", colorCyan, colorReset))
		sb.WriteString(fmt.Sprintf("│  %sDivergence Breakdown (%d anomalies found):%s                                  │\n", colorRed, len(report.Divergences), colorReset))
		for _, d := range report.Divergences {
			sevColor := colorYellow
			if d.Severity == SeverityCritical {
				sevColor = colorRed
			}
			stepStr := fmt.Sprintf("Step %-3d", d.StepIndex)
			if d.StepIndex == 0 {
				stepStr = "Global  "
			}
			sb.WriteString(fmt.Sprintf("│  %s[%-8s]%s %s %-50s │\n", sevColor, d.Severity, colorReset, stepStr, truncateStr(d.Description, 50)))
		}
	}

	sb.WriteString(fmt.Sprintf("%s╰────────────────────────────────────────────────────────────────────────────╯%s\n", colorCyan, colorReset))
	return sb.String()
}

// RenderMarkdown creates a formatted Markdown report table suitable for CI/CD status comments.
func RenderMarkdown(report *DeterminismReport) string {
	var sb strings.Builder

	statusIcon := "✅ PASS"
	if report.Status == StatusBreach {
		statusIcon = "❌ BREACH"
	} else if report.Status == StatusDegraded {
		statusIcon = "⚠️ DEGRADED"
	}

	sb.WriteString("### 🎯 AgentVCR Trajectory Determinism & Token Efficiency Benchmark\n\n")
	sb.WriteString(fmt.Sprintf("**Status:** %s | **Determinism Score:** `%.1f / 100.0` | **Evaluation Time:** `%d µs`\n\n",
		statusIcon, report.DeterminismScore, report.ExecutionDurUs))

	sb.WriteString("| Metric | Baseline | Target / Replay | Empirical Delta / Result |\n")
	sb.WriteString("| :--- | :--- | :--- | :--- |\n")
	sb.WriteString(fmt.Sprintf("| **Tool Call Parity** | %d steps | %d steps | `%.1f%%` |\n",
		report.Metrics.TotalBaselineSteps, report.Metrics.TotalTargetSteps, report.Metrics.ToolParityPercent))
	sb.WriteString(fmt.Sprintf("| **Tool Argument Parity** | 100.0%% | - | `%.1f%%` |\n", report.Metrics.ArgumentParityPercent))
	sb.WriteString(fmt.Sprintf("| **Total Tokens** | %d tokens | %d tokens | `%+d tokens (%.2fx bloat)` |\n",
		report.Metrics.Tokens.BaselineTokens, report.Metrics.Tokens.TargetTokens, report.Metrics.Tokens.DeltaTokens, report.Metrics.Tokens.BloatRatio))
	sb.WriteString(fmt.Sprintf("| **Replay Latency (P50 / P95)** | - | `%.1f ms / %.1f ms` | Jitter: `±%.1f ms` |\n",
		report.Metrics.Latency.P50Ms, report.Metrics.Latency.P95Ms, report.Metrics.Latency.JitterMs))
	sb.WriteString(fmt.Sprintf("| **Proxy Interception Overhead** | - | - | `%d µs / step` |\n\n", report.Metrics.Latency.ProxyOverheadUs))

	if len(report.Divergences) > 0 {
		sb.WriteString("#### ⚠️ Divergence Registry\n\n")
		sb.WriteString("| Step | Severity | Kind | Description |\n")
		sb.WriteString("| :--- | :--- | :--- | :--- |\n")
		for _, d := range report.Divergences {
			sb.WriteString(fmt.Sprintf("| %d | `%s` | `%s` | %s |\n", d.StepIndex, d.Severity, d.Kind, d.Description))
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("> **Benchmark Summary:** %s\n", report.Summary))
	return sb.String()
}

// RenderJSON serializes the determinism report into formatted JSON.
func RenderJSON(report *DeterminismReport) (string, error) {
	bytes, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}
