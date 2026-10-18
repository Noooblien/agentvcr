package regress

import (
	"fmt"
	"strings"
	"time"
)

// RenderRegressionScorecard formats a single regression report for the terminal.
func RenderRegressionScorecard(rep *RegressionReport) string {
	var b strings.Builder

	verdictColor := "\033[1;32m" // Green
	if rep.Verdict != VerdictPass {
		verdictColor = "\033[1;31m" // Red
	}

	b.WriteString("\033[1;35m========================================================================================\033[0m\n")
	b.WriteString("  \033[1;37mAGENTVCR // GOLDEN TRAJECTORY REGRESSION ASSERTIONS ENGINE\033[0m\n")
	b.WriteString("\033[1;35m========================================================================================\033[0m\n")
	b.WriteString(fmt.Sprintf("  \033[1;33mTask Name:\033[0m           %s (%s)\n", rep.TaskName, rep.TaskID))
	b.WriteString(fmt.Sprintf("  \033[1;33mCandidate Session:\033[0m   %s\n", rep.CandidateSessionID))
	b.WriteString(fmt.Sprintf("  \033[1;33mGolden Baseline:\033[0m     %s\n", rep.GoldenSessionID))
	b.WriteString(fmt.Sprintf("  \033[1;33mEvaluation Status:\033[0m   %s[%s]\033[0m (Pass Rate: %.1f%%)\n",
		verdictColor, rep.Verdict, rep.PassRate))

	// Sequence Alignment Summary
	b.WriteString("  \033[1;34m----------------------------------------------------------------------------------------\033[0m\n")
	b.WriteString("  \033[1;37mSEQUENCE ALIGNMENT & EFFICIENCY METRICS:\033[0m\n")
	b.WriteString(fmt.Sprintf("  Tool Alignment Score:  %.1f%% (LCS: %d / Max: %d steps)\n",
		rep.Alignment.SimilarityScore*100, rep.Alignment.LCSLength, max(rep.Alignment.GoldenLength, rep.Alignment.CandidateLength)))

	tokenSign := "+"
	if rep.TokenDelta < 0 {
		tokenSign = ""
	}
	b.WriteString(fmt.Sprintf("  Token Consumption:     %s%d tokens (%s%.1f%% vs golden)\n",
		tokenSign, rep.TokenDelta, tokenSign, rep.TokenDeltaPercent))
	b.WriteString(fmt.Sprintf("  Latency Delta:         %+d ms\n", rep.DurationDeltaMs))

	if len(rep.Alignment.MissingTools) > 0 {
		b.WriteString(fmt.Sprintf("  \033[1;31mMissing Golden Tools:\033[0m  %v\n", rep.Alignment.MissingTools))
	}
	if len(rep.Alignment.ExtraneousTools) > 0 {
		b.WriteString(fmt.Sprintf("  \033[1;33mExtraneous Tools:\033[0m      %v\n", rep.Alignment.ExtraneousTools))
	}

	// Failures breakdown
	if len(rep.Failures) > 0 {
		b.WriteString("  \033[1;34m----------------------------------------------------------------------------------------\033[0m\n")
		b.WriteString("  \033[1;37mASSERTION FAILURES & REGRESSION DIAGNOSTICS:\033[0m\n")
		for i, f := range rep.Failures {
			sevColor := "\033[1;31m" // Red for Critical
			if f.Severity == SeverityMajor {
				sevColor = "\033[1;33m" // Yellow for Major
			} else if f.Severity == SeverityMinor {
				sevColor = "\033[1;36m" // Cyan for Minor
			}
			b.WriteString(fmt.Sprintf("  [%d] %s[%s]\033[0m %s\n", i+1, sevColor, f.Severity, f.RuleID))
			b.WriteString(fmt.Sprintf("      Violation:   %s\n", f.Message))
			b.WriteString(fmt.Sprintf("      Remediation: \033[0;37m%s\033[0m\n", f.Remediation))
		}
	} else {
		b.WriteString("  \033[1;32m✓ All golden trajectory assertions satisfied with zero semantic regressions.\033[0m\n")
	}

	b.WriteString("\033[1;35m========================================================================================\033[0m\n")
	return b.String()
}

// RenderMermaidDiff builds a Mermaid flowchart contrasting golden vs candidate tool sequence paths.
func RenderMermaidDiff(spec *GoldenSpec, cand *CandidateExecution, rep *RegressionReport) string {
	var b strings.Builder
	b.WriteString("```mermaid\n")
	b.WriteString("flowchart TD\n")
	b.WriteString(fmt.Sprintf("    subgraph Golden [\"Golden Baseline: %s\"]\n", spec.TaskName))
	for i, t := range spec.ExpectedToolSequence {
		b.WriteString(fmt.Sprintf("        G%d[\"[%d] %s\"]:::golden\n", i+1, i+1, t))
		if i > 0 {
			b.WriteString(fmt.Sprintf("        G%d --> G%d\n", i, i+1))
		}
	}
	b.WriteString("    end\n\n")

	b.WriteString(fmt.Sprintf("    subgraph Candidate [\"Candidate Run: %s (%s)\"]\n", cand.SessionID, cand.AgentModel))
	for j, t := range cand.ExecutedTools {
		nodeID := fmt.Sprintf("C%d", j+1)
		style := ":::candNormal"
		for _, f := range spec.ForbiddenTools {
			if t == f {
				style = ":::candForbidden"
				break
			}
		}
		b.WriteString(fmt.Sprintf("        %s[\"[%d] %s\"]%s\n", nodeID, j+1, t, style))
		if j > 0 {
			b.WriteString(fmt.Sprintf("        C%d --> C%d\n", j, j+1))
		}
	}
	b.WriteString("    end\n\n")

	b.WriteString("    classDef golden fill:#2196f3,stroke:#0d47a1,stroke-width:2px,color:#fff;\n")
	b.WriteString("    classDef candNormal fill:#4caf50,stroke:#1b5e20,stroke-width:2px,color:#fff;\n")
	b.WriteString("    classDef candForbidden fill:#f44336,stroke:#b71c1c,stroke-width:3px,color:#fff;\n")
	b.WriteString("```\n")

	return b.String()
}

// RenderMarkdownReport produces a GitHub Actions CI-friendly summary report.
func RenderMarkdownReport(reports []*RegressionReport) string {
	var b strings.Builder

	b.WriteString("# AgentVCR // Continuous Semantic Regression Report\n\n")
	b.WriteString(fmt.Sprintf("**Execution Date:** %s  \n", time.Now().UTC().Format(time.RFC3339)))
	b.WriteString("**Evaluation Framework:** AgentVCR Golden Trajectory Assertions  \n\n")

	b.WriteString("## Summary Matrix\n\n")
	b.WriteString("| Task Name | Candidate Session | Verdict | Pass Rate | Tool LCS | Token Delta | Duration Delta | Failures |\n")
	b.WriteString("| :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- |\n")

	for _, r := range reports {
		verdictBadge := "✅ PASS"
		if r.Verdict != VerdictPass {
			verdictBadge = "❌ FAIL"
		}
		tokenSign := "+"
		if r.TokenDelta < 0 {
			tokenSign = ""
		}
		b.WriteString(fmt.Sprintf("| `%s` | `%s` | %s | %.1f%% | %.1f%% | %s%d (%s%.1f%%) | %+d ms | %d |\n",
			r.TaskName,
			r.CandidateSessionID,
			verdictBadge,
			r.PassRate,
			r.Alignment.SimilarityScore*100,
			tokenSign, r.TokenDelta, tokenSign, r.TokenDeltaPercent,
			r.DurationDeltaMs,
			len(r.Failures),
		))
	}

	b.WriteString("\n## Granular Assertion Failures\n\n")
	hasFailures := false
	for _, r := range reports {
		if len(r.Failures) > 0 {
			hasFailures = true
			b.WriteString(fmt.Sprintf("### Task: `%s` (`%s`)\n\n", r.TaskName, r.CandidateSessionID))
			b.WriteString("| Severity | Rule ID | Observed | Expected | Message |\n")
			b.WriteString("| :--- | :--- | :--- | :--- | :--- |\n")
			for _, f := range r.Failures {
				b.WriteString(fmt.Sprintf("| **%s** | `%s` | `%v` | `%v` | %s |\n",
					f.Severity, f.RuleID, f.Observed, f.Expected, f.Message))
			}
			b.WriteString("\n")
		}
	}

	if !hasFailures {
		b.WriteString("All golden trajectory test suites passed with zero semantic regressions.\n")
	}

	return b.String()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
