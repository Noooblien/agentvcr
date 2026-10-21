package triage

import (
	"fmt"
	"strings"
)

const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiCyan   = "\033[36m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiRed    = "\033[31m"
	ansiPurple = "\033[35m"
	ansiDim    = "\033[2m"
)

// RenderTriageScorecard formats a diagnostic triage report as an ANSI terminal scorecard.
func RenderTriageScorecard(r *TriageReport) string {
	var sb strings.Builder

	sevColor := ansiGreen
	switch r.OverallSeverity {
	case SeverityCritical:
		sevColor = ansiRed
	case SeverityHigh:
		sevColor = ansiPurple
	case SeverityMedium:
		sevColor = ansiYellow
	case SeverityLow:
		sevColor = ansiCyan
	}

	sb.WriteString(fmt.Sprintf("%s%s=== AGENTVCR AUTOMATED ROOT-CAUSE TRIAGE SCORECARD ===%s\n", ansiBold, ansiCyan, ansiReset))
	sb.WriteString(fmt.Sprintf("Session ID:      %s%s%s (Agent: %s)\n", ansiBold, r.SessionID, ansiReset, r.AgentName))
	sb.WriteString(fmt.Sprintf("Analysis Time:   %s (%d µs)\n", r.AnalyzedAt.Format("15:04:05.000Z07:00"), r.ExecutionTimeUs))
	sb.WriteString(fmt.Sprintf("Trace Events:    %d total | %s%d failed%s\n",
		r.TotalEvents, sevColor, r.FailedEvents, ansiReset))
	sb.WriteString(fmt.Sprintf("Overall Status:  %s%s[%s - %s]%s\n",
		ansiBold, sevColor, r.OverallSeverity, r.PrimaryCategory, ansiReset))
	sb.WriteString(strings.Repeat("─", 82) + "\n")

	if r.IsCleanRun || len(r.Findings) == 0 {
		sb.WriteString(fmt.Sprintf("%s%s✔ CLEAN TRAJECTORY:%s No unhandled faults, loops, or schema errors detected.\n", ansiBold, ansiGreen, ansiReset))
		sb.WriteString(strings.Repeat("─", 82) + "\n")
		return sb.String()
	}

	// Output Findings
	sb.WriteString(fmt.Sprintf("%s%sDIAGNOSED FAULT FINDINGS (%d):%s\n", ansiBold, ansiCyan, len(r.Findings), ansiReset))
	for idx, f := range r.Findings {
		badgeColor := ansiYellow
		if f.Severity == SeverityCritical {
			badgeColor = ansiRed
		} else if f.Severity == SeverityHigh {
			badgeColor = ansiPurple
		}

		sb.WriteString(fmt.Sprintf("\n[%d] %s%s[%s / %s]%s (Confidence: %.1f%%)\n",
			idx+1, badgeColor, ansiBold, f.Severity, f.Category, ansiReset, f.Confidence*100.0))
		sb.WriteString(fmt.Sprintf("    Inciting Step:   #%d (Event: %s, Tool: %s)\n",
			f.IncitingStepIndex, f.IncitingEventID, f.FailedToolName))
		sb.WriteString(fmt.Sprintf("    Error Message:   %s%s%s\n", ansiDim, f.ErrorMessage, ansiReset))
		sb.WriteString(fmt.Sprintf("    Root Cause:      %s%s%s\n", ansiBold, f.RootCauseSummary, ansiReset))
		sb.WriteString(fmt.Sprintf("    Causal Steps:    %v\n", f.CausalChainSteps))

		// Remediation details
		sb.WriteString(fmt.Sprintf("    %s↳ Remediation:%s  %s%s%s (Strategy: %s)\n",
			ansiGreen, ansiReset, ansiBold, f.Remediation.PatchDescription, ansiReset, f.Remediation.Strategy))
		if f.Remediation.CorrectivePrompt != "" {
			sb.WriteString(fmt.Sprintf("      Corrective Prompt: \"%s%s%s\"\n",
				ansiDim, f.Remediation.CorrectivePrompt, ansiReset))
		}
	}
	sb.WriteString("\n" + strings.Repeat("─", 82) + "\n")

	return sb.String()
}

// RenderTriageMermaid generates a Mermaid flowchart mapping the backward causal fault propagation.
func RenderTriageMermaid(r *TriageReport) string {
	var sb strings.Builder

	sb.WriteString("```mermaid\n")
	sb.WriteString("flowchart TD\n")
	sb.WriteString(fmt.Sprintf("    subgraph Trace [Execution Trace: %s]\n", r.SessionID))

	for idx, f := range r.Findings {
		faultNodeID := fmt.Sprintf("F%d", idx)
		sb.WriteString(fmt.Sprintf("        %s[\"Step #%d: %s\"] --> %sError[\"Error: %s\"]\n",
			faultNodeID, f.IncitingStepIndex, f.FailedToolName, faultNodeID, f.Category))
		sb.WriteString(fmt.Sprintf("        %sError --> %sRem[\"Fix: %s\"]\n",
			faultNodeID, faultNodeID, f.Remediation.Strategy))
	}

	sb.WriteString("    end\n")
	sb.WriteString("```\n")

	return sb.String()
}

// RenderMarkdownReport produces a GitHub Flavored Markdown report summarizing triage across sessions.
func RenderMarkdownReport(reports []*TriageReport) string {
	var sb strings.Builder

	sb.WriteString("# AgentVCR Automated Root-Cause Triage & Remediation Report\n\n")
	sb.WriteString("This report summarizes the backward causal slicing, fault diagnosis, and automated patch recommendations for analyzed agent execution traces.\n\n")

	sb.WriteString("## Triage Evaluation Summary\n\n")
	sb.WriteString("| Session ID | Agent | Total Events | Failed | Primary Fault | Severity | Status | Top Remediation |\n")
	sb.WriteString("| :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- |\n")

	for _, r := range reports {
		topRem := "None"
		if len(r.Remediations) > 0 {
			topRem = fmt.Sprintf("`%s` (%s)", r.Remediations[0].Strategy, r.Remediations[0].PatchDescription)
		}

		statusBadge := "🟢 **PASS**"
		if !r.IsCleanRun {
			statusBadge = fmt.Sprintf("🔴 **%s**", r.OverallSeverity)
		}

		sb.WriteString(fmt.Sprintf("| `%s` | `%s` | %d | %d | `%s` | %s | %s | %s |\n",
			r.SessionID,
			r.AgentName,
			r.TotalEvents,
			r.FailedEvents,
			r.PrimaryCategory,
			r.OverallSeverity,
			statusBadge,
			topRem,
		))
	}

	sb.WriteString("\n## Diagnosed Fault Details\n\n")
	for _, r := range reports {
		if r.IsCleanRun || len(r.Findings) == 0 {
			continue
		}

		sb.WriteString(fmt.Sprintf("### Session: `%s` (%s)\n\n", r.SessionID, r.AgentName))
		sb.WriteString("| Step # | Tool | Category | Severity | Root Cause Summary | Recommended Remediation |\n")
		sb.WriteString("| :--- | :--- | :--- | :--- | :--- | :--- |\n")

		for _, f := range r.Findings {
			sb.WriteString(fmt.Sprintf("| `%d` | `%s` | `%s` | **%s** | %s | *%s* |\n",
				f.IncitingStepIndex,
				f.FailedToolName,
				f.Category,
				f.Severity,
				f.RootCauseSummary,
				f.Remediation.PatchDescription,
			))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
