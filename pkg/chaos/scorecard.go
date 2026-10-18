package chaos

import (
	"encoding/json"
	"fmt"
	"strings"
)

// RenderTerminalScorecard formats the benchmark suite into a colored ANSI terminal dashboard.
func RenderTerminalScorecard(suite *ChaosBenchmarkSuite) string {
	var b strings.Builder

	b.WriteString("\n\033[1;36m========================================================================================\033[0m\n")
	b.WriteString("   \033[1;37mAGENTVCR AUTONOMOUS AGENT CHAOS & FAULT INJECTION BENCHMARK SCORECARD\033[0m\n")
	b.WriteString("\033[1;36m========================================================================================\033[0m\n\n")

	b.WriteString(fmt.Sprintf("   \033[1mTotal Scenarios Evaluated:\033[0m  %d\n", suite.TotalScenarios))
	b.WriteString(fmt.Sprintf("   \033[1;32mPassed Scenarios:\033[0m           %d / %d (%.1f%%)\n",
		suite.PassedScenarios, suite.TotalScenarios, float64(suite.PassedScenarios)/float64(suite.TotalScenarios)*100.0))
	b.WriteString(fmt.Sprintf("   \033[1;33mFaults Injected / Recovered:\033[0m %d / %d (%.1f%% recovery)\n",
		suite.TotalFaultsInjected, suite.TotalFaultsRecovered, suite.OverallRecoveryRate))
	b.WriteString(fmt.Sprintf("   \033[1;35mOverall Resilience Score:\033[0m   %.1f / 100.0\n\n", suite.OverallResilience))

	b.WriteString("\033[1;34m--- EXPERIMENT SCENARIO BREAKDOWN ---\033[0m\n")
	b.WriteString(fmt.Sprintf("%-36s | %-6s | %-8s | %-9s | %-10s | %-10s | %-12s\n",
		"Scenario Name", "Steps", "Injected", "Recovered", "Recov %", "MTTR (ms)", "Status"))
	b.WriteString(strings.Repeat("-", 102) + "\n")

	for _, rep := range suite.Reports {
		statusColor := "\033[1;32m"
		if rep.Status == StatusDegraded {
			statusColor = "\033[1;33m"
		} else if rep.Status == StatusCriticalFailure {
			statusColor = "\033[1;31m"
		}

		b.WriteString(fmt.Sprintf("%-36s | %-6d | %-8d | %-9d | %-9.1f%% | %-10.1f | %s%-12s\033[0m\n",
			truncate(rep.ExperimentName, 36),
			rep.TotalStepsEvaluated,
			rep.TotalFaultsInjected,
			rep.TotalFaultsRecovered,
			rep.RecoveryRatePct,
			rep.MeanTimeToRecoveryMs,
			statusColor,
			rep.Status,
		))
	}
	b.WriteString(strings.Repeat("-", 102) + "\n\n")

	b.WriteString("\033[1;34m--- INJECTED FAULT EVENT AUDIT LOG ---\033[0m\n")
	b.WriteString(fmt.Sprintf("%-4s | %-16s | %-24s | %-20s | %-10s | %-10s\n",
		"Step", "Tool", "Fault Injected", "Agent Reaction", "Recovered", "Blast Radius"))
	b.WriteString(strings.Repeat("-", 98) + "\n")

	for _, rep := range suite.Reports {
		for _, ev := range rep.Events {
			recovBadge := "\033[1;32mYES\033[0m"
			if !ev.Recovered {
				recovBadge = "\033[1;31mNO\033[0m "
			}

			b.WriteString(fmt.Sprintf("%-4d | %-16s | %-24s | %-20s | %-19s | +%-4d tok\n",
				ev.StepIndex,
				truncate(ev.ToolName, 16),
				truncate(string(ev.FaultType), 24),
				truncate(string(ev.Reaction), 20),
				recovBadge,
				ev.ExtraTokensSpent,
			))
		}
	}
	b.WriteString(strings.Repeat("-", 98) + "\n\n")

	if suite.OverallResilience >= 75.0 {
		b.WriteString("\033[1;32m[PASS] Agent Resilience verified across chaos suite (score >= 75.0).\033[0m\n\n")
	} else {
		b.WriteString("\033[1;31m[FAIL] Agent Resilience below threshold (score < 75.0).\033[0m\n\n")
	}

	return b.String()
}

// RenderMarkdownReport formats the suite into a GitHub Flavored Markdown document.
func RenderMarkdownReport(suite *ChaosBenchmarkSuite) string {
	var b strings.Builder

	b.WriteString("### AgentVCR Chaos & Fault Injection Benchmark Report\n\n")
	b.WriteString(fmt.Sprintf("- **Total Scenarios Evaluated:** `%d`\n", suite.TotalScenarios))
	b.WriteString(fmt.Sprintf("- **Passed Scenarios:** `%d` (%.1f%%)\n", suite.PassedScenarios, float64(suite.PassedScenarios)/float64(suite.TotalScenarios)*100.0))
	b.WriteString(fmt.Sprintf("- **Overall Resilience Score:** `%.1f / 100.0`\n", suite.OverallResilience))
	b.WriteString(fmt.Sprintf("- **Fault Recovery Rate:** `%.1f%%` (%d / %d recovered)\n\n",
		suite.OverallRecoveryRate, suite.TotalFaultsRecovered, suite.TotalFaultsInjected))

	b.WriteString("| Experiment Scenario | Steps | Injected | Recovered | Recovery % | MTTR (ms) | Blast Radius | Resilience Score | Status |\n")
	b.WriteString("|:--------------------|:-----:|:--------:|:---------:|:----------:|:---------:|:------------:|:----------------:|:------:|\n")

	for _, rep := range suite.Reports {
		statusIcon := "✅"
		if rep.Status == StatusDegraded {
			statusIcon = "⚠️"
		} else if rep.Status == StatusCriticalFailure {
			statusIcon = "❌"
		}

		b.WriteString(fmt.Sprintf("| %s | %d | %d | %d | %.1f%% | %.1f ms | +%d tok | `%.1f/100` | %s %s |\n",
			rep.ExperimentName,
			rep.TotalStepsEvaluated,
			rep.TotalFaultsInjected,
			rep.TotalFaultsRecovered,
			rep.RecoveryRatePct,
			rep.MeanTimeToRecoveryMs,
			rep.TokenBlastRadiusTokens,
			rep.ResilienceScore,
			statusIcon,
			rep.Status,
		))
	}

	b.WriteString("\n#### Fault Archetypes Evaluated\n\n")
	b.WriteString("1. **Rate Limit 429 Storm**: Downstream tool HTTP 429 responses; validates exponential backoff with jitter.\n")
	b.WriteString("2. **Downstream Tool Timeout**: 3,000ms delay injection; validates client deadline propagation and fallback tool paths.\n")
	b.WriteString("3. **Intermittent Transport Flakiness**: Connection reset by peer and corrupted JSON frames; validates retry resilience.\n")
	b.WriteString("4. **Cascading Upstream Outage**: 503 Service Unavailable and schema drift; validates graceful degradation.\n\n")

	b.WriteString(RenderChaosMermaid(suite))
	return b.String()
}

// RenderChaosMermaid generates a Mermaid flowchart sequence visualizing the chaos injection and recovery path.
func RenderChaosMermaid(suite *ChaosBenchmarkSuite) string {
	var b strings.Builder

	b.WriteString("```mermaid\n")
	b.WriteString("sequenceDiagram\n")
	b.WriteString("    autonumber\n")
	b.WriteString("    actor Agent as Autonomous Agent\n")
	b.WriteString("    participant VCR as AgentVCR Flight Recorder\n")
	b.WriteString("    participant Chaos as Chaos Injection Engine\n")
	b.WriteString("    participant Tool as Upstream MCP Tool / API\n\n")
	b.WriteString("    Agent->>VCR: ToolCall(web_search, query)\n")
	b.WriteString("    VCR->>Chaos: Intercept Frame & Match Fault Rules\n")
	b.WriteString("    alt Injected HTTP 429 Rate Limit\n")
	b.WriteString("        Chaos-->>Agent: 429 Rate Limit Exceeded (Retry-After: 2s)\n")
	b.WriteString("        Agent->>Agent: Exponential Jitter Backoff (Wait 2000ms)\n")
	b.WriteString("        Agent->>VCR: Retry ToolCall(web_search, query)\n")
	b.WriteString("        VCR->>Tool: Live Forwarding\n")
	b.WriteString("        Tool-->>Agent: 200 OK (Recovered)\n")
	b.WriteString("    else Injected 3000ms Tool Timeout\n")
	b.WriteString("        Chaos-->>Agent: Timeout Deadline Exceeded (3000ms)\n")
	b.WriteString("        Agent->>VCR: Fallback ToolCall(bash_exec, query)\n")
	b.WriteString("        VCR->>Tool: Live Forwarding\n")
	b.WriteString("        Tool-->>Agent: 200 OK (Fallback Recovered)\n")
	b.WriteString("    end\n")
	b.WriteString("```\n")

	return b.String()
}

// RenderJSON serializes the suite into formatted JSON.
func RenderJSON(suite *ChaosBenchmarkSuite) (string, error) {
	bytes, err := json.MarshalIndent(suite, "", "  ")
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
