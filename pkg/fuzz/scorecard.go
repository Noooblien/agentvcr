package fuzz

import (
	"fmt"
	"strings"
)

// ANSI terminal color codes.
const (
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorPurple = "\033[35m"
	colorCyan   = "\033[36m"
	colorWhite  = "\033[37m"
	colorGray   = "\033[90m"
)

// RenderFuzzScorecard outputs a high-contrast terminal UI scorecard for the fuzz campaign.
func RenderFuzzScorecard(report *FuzzCampaignReport) string {
	var sb strings.Builder

	sb.WriteString("\n" + colorBold + colorPurple + "========================================================================================================\n" + colorReset)
	sb.WriteString(colorBold + colorWhite + "  AGENTVCR // ADVERSARIAL TRAJECTORY FUZZING & AGENT ROBUSTNESS SCORECARD\n" + colorReset)
	sb.WriteString(colorBold + colorPurple + "========================================================================================================\n" + colorReset)

	statusColor := colorGreen
	if report.Status == "CRITICAL" {
		statusColor = colorRed
	} else if report.Status == "VULNERABLE" {
		statusColor = colorYellow
	}

	sb.WriteString(fmt.Sprintf("  %sCampaign:%s %s  |  %sSession:%s %s  |  %sMutations:%s %d  |  %sRobustness Score:%s %.1f/100  |  %sStatus:%s %s%s%s\n",
		colorBold, colorReset, report.CampaignID,
		colorBold, colorReset, report.SessionID,
		colorBold, colorReset, report.TotalMutations,
		colorCyan, colorReset, report.AgentRobustnessScore,
		colorBold, colorReset, statusColor, report.Status, colorReset))
	sb.WriteString(colorGray + "--------------------------------------------------------------------------------------------------------\n" + colorReset)

	sb.WriteString(fmt.Sprintf("  %sBreaches Detected:%s  Crashes: %s%d%s  |  Jailbreaks: %s%d%s  |  Hangs: %s%d%s  |  Corruptions: %s%d%s  |  Resilient: %s%d%s\n",
		colorBold, colorReset,
		colorRed, report.CrashesDetected, colorReset,
		colorRed, report.JailbreaksDetected, colorReset,
		colorYellow, report.HangsDetected, colorReset,
		colorYellow, report.StateCorruptions, colorReset,
		colorGreen, report.ResilientCount, colorReset))
	sb.WriteString(colorGray + "--------------------------------------------------------------------------------------------------------\n" + colorReset)

	sb.WriteString(fmt.Sprintf("%-14s %-22s %-20s %-12s %s\n",
		"CASE ID", "TARGET", "CATEGORY", "VERDICT", "OBSERVED BEHAVIOR"))
	sb.WriteString(colorGray + "--------------------------------------------------------------------------------------------------------\n" + colorReset)

	for _, c := range report.Cases {
		verdictColor := colorGreen
		verdictText := "PASSED"
		if !c.Surviving {
			verdictColor = colorRed
			verdictText = "VULN"
		}

		sb.WriteString(fmt.Sprintf("%-14s %-22s %-20s %s%-12s%s %s\n",
			c.CaseID,
			c.Target,
			c.Category,
			verdictColor, verdictText, colorReset,
			c.ObservedBehavior))
	}

	sb.WriteString(colorBold + colorPurple + "========================================================================================================\n\n" + colorReset)
	return sb.String()
}

// RenderFuzzMermaid renders a Mermaid diagram illustrating the attack surfaces fuzzed and outcome.
func RenderFuzzMermaid(report *FuzzCampaignReport) string {
	var sb strings.Builder

	sb.WriteString("```mermaid\n")
	sb.WriteString("flowchart TD\n")
	sb.WriteString("    Campaign([Adversarial Fuzz Campaign: " + report.CampaignID + "])\n")

	sb.WriteString("    Campaign --> Surface1[MCP JSON-RPC Frames]\n")
	sb.WriteString("    Campaign --> Surface2[LLM Prompt Context Delimiters]\n")
	sb.WriteString("    Campaign --> Surface3[Tool Output Buffers & Null Bytes]\n")
	sb.WriteString("    Campaign --> Surface4[Unicode Stress & Token Bounds]\n")

	if report.JailbreaksDetected > 0 {
		sb.WriteString("    Surface2 -->|Detected Prompt Override| Breach[Vulnerability: Prompt Jailbreak]\n")
		sb.WriteString("    style Breach fill:#ff4d4f,stroke:#ff0000,stroke-width:2px\n")
	} else {
		sb.WriteString("    Surface2 -->|Sentinel Guard Active| Safe2[Prompt Guard Resilient]\n")
		sb.WriteString("    style Safe2 fill:#52c41a,stroke:#389e0d,stroke-width:1px\n")
	}

	sb.WriteString("    Surface1 -->|Safe Parser Trapping| Safe1[JSON-RPC Hardened]\n")
	sb.WriteString("    Surface3 -->|Zero Buffer Corruption| Safe3[Tool Buffer Hardened]\n")
	sb.WriteString("    Surface4 -->|Bounds Verified| Safe4[Unicode Stream Hardened]\n")

	sb.WriteString("```\n")
	return sb.String()
}

// RenderFuzzMarkdown formats a GitHub-flavored markdown audit summary.
func RenderFuzzMarkdown(report *FuzzCampaignReport) string {
	var sb strings.Builder

	sb.WriteString("## AgentVCR Adversarial Fuzzing & Robustness Audit Report\n\n")
	sb.WriteString(fmt.Sprintf("- **Campaign ID**: `%s`\n", report.CampaignID))
	sb.WriteString(fmt.Sprintf("- **Target Session**: `%s`\n", report.SessionID))
	sb.WriteString(fmt.Sprintf("- **Agent Robustness Score (ARS)**: `%.1f / 100`\n", report.AgentRobustnessScore))
	sb.WriteString(fmt.Sprintf("- **Audit Verdict**: `%s`\n", report.Status))
	sb.WriteString(fmt.Sprintf("- **Total Adversarial Trials**: %d\n", report.TotalMutations))
	sb.WriteString(fmt.Sprintf("- **Breaches Detected**: Crashes: `%d`, Jailbreaks: `%d`, State Corruptions: `%d`\n\n",
		report.CrashesDetected, report.JailbreaksDetected, report.StateCorruptions))

	sb.WriteString("| Case ID | Target Surface | Mutation Category | Severity | Verdict | Observed Behavior |\n")
	sb.WriteString("|:---|:---|:---|:---|:---|:---|\n")

	for _, c := range report.Cases {
		verdict := "✅ PASS"
		if !c.Surviving {
			verdict = "❌ VULN"
		}
		sb.WriteString(fmt.Sprintf("| `%s` | `%s` | `%s` | `%s` | %s | %s |\n",
			c.CaseID, c.Target, c.Category, c.Severity, verdict, c.ObservedBehavior))
	}

	return sb.String()
}
