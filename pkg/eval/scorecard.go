package eval

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

// RenderJudgeScorecard formats an ANSI terminal scorecard of the rubric evaluation.
func RenderJudgeScorecard(report JudgeEvaluationReport) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("%s%s========================================================================================%s\n", ColorBold, ColorCyan, ColorReset))
	sb.WriteString(fmt.Sprintf("%s%s  AGENTVCR // LLM-AS-A-JUDGE TRAJECTORY RUBRIC EVALUATION SCORECARD%s\n", ColorBold, ColorWhite, ColorReset))
	sb.WriteString(fmt.Sprintf("%s%s========================================================================================%s\n\n", ColorBold, ColorCyan, ColorReset))

	gradeColor := ColorGreen
	switch report.OverallGrade {
	case GradeWarning:
		gradeColor = ColorYellow
	case GradeFail, GradeCriticalBreach:
		gradeColor = ColorRed
	}

	sb.WriteString(fmt.Sprintf("  %sSession ID:%s       %s\n", ColorBold, ColorReset, report.SessionID))
	sb.WriteString(fmt.Sprintf("  %sAgent Name:%s       %s\n", ColorBold, ColorReset, report.AgentName))
	sb.WriteString(fmt.Sprintf("  %sOverall Score:%s    %s%s%.2f / 10.0%s\n", ColorBold, ColorReset, ColorBold, gradeColor, report.WeightedScore, ColorReset))
	sb.WriteString(fmt.Sprintf("  %sJudgement Grade:%s  %s%s[%s]%s\n", ColorBold, ColorReset, ColorBold, gradeColor, report.OverallGrade, ColorReset))
	sb.WriteString(fmt.Sprintf("  %sTrajectory Scale:%s %d steps, %d cumulative tokens\n", ColorBold, ColorReset, report.TotalSteps, report.TotalTokens))
	sb.WriteString(fmt.Sprintf("  %sSummary:%s           %s\n\n", ColorBold, ColorReset, report.JudgementSummary))

	// Dimension Breakdown
	sb.WriteString(fmt.Sprintf("  %s%sRUBRIC DIMENSION AVERAGES:%s\n", ColorBold, ColorWhite, ColorReset))
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("  %-28s %-12s %s\n", "DIMENSION", "SCORE", "HEALTH STATUS"))
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────\n")

	for dim, score := range report.DimensionScores {
		status := fmt.Sprintf("%s[EXCELLENT]%s", ColorGreen, ColorReset)
		if score < 7.0 {
			status = fmt.Sprintf("%s[DEFICIENT]%s", ColorRed, ColorReset)
		} else if score < 8.5 {
			status = fmt.Sprintf("%s[ACCEPTABLE]%s", ColorYellow, ColorReset)
		}
		sb.WriteString(fmt.Sprintf("  %-28s %-12.1f %s\n", dim, score, status))
	}
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────\n\n")

	// Individual Criterion Breakdown
	sb.WriteString(fmt.Sprintf("  %s%sINDIVIDUAL CRITERIA BREAKDOWN (%d evaluated):%s\n", ColorBold, ColorWhite, len(report.CriterionResults), ColorReset))
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("  %-22s %-20s %-8s %-10s %s\n", "CRITERION ID", "DIMENSION", "SCORE", "GRADE", "RATIONALE"))
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────\n")

	for _, cr := range report.CriterionResults {
		gColor := ColorGreen
		if cr.Grade == GradeWarning {
			gColor = ColorYellow
		} else if cr.Grade == GradeFail || cr.Grade == GradeCriticalBreach {
			gColor = ColorRed
		}

		rationale := cr.Rationale
		if len(rationale) > 35 {
			rationale = rationale[:32] + "..."
		}

		sb.WriteString(fmt.Sprintf("  %-22s %-20s %-8.1f %s%-10s%s %s\n",
			cr.CriterionID, cr.Dimension, cr.Score, gColor, cr.Grade, ColorReset, rationale))
	}
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────\n\n")

	sb.WriteString(fmt.Sprintf("%s%s========================================================================================%s\n", ColorBold, ColorCyan, ColorReset))
	return sb.String()
}

// RenderJudgeMermaid generates a Mermaid radar/DAG diagram of the evaluation.
func RenderJudgeMermaid(report JudgeEvaluationReport) string {
	var sb strings.Builder

	sb.WriteString("```mermaid\n")
	sb.WriteString("graph TD\n")
	sb.WriteString(fmt.Sprintf("    SESSION[\"Session: %s<br/>Agent: %s\"] --> JUDGE[\"LLM-as-a-Judge Engine\"]\n", report.SessionID, report.AgentName))

	for dim, score := range report.DimensionScores {
		nodeID := strings.ReplaceAll(string(dim), "_", "")
		sb.WriteString(fmt.Sprintf("    JUDGE --> %s[\"%s<br/>Score: %.1f/10.0\"]\n", nodeID, dim, score))
	}

	resultNode := "RESULT_PASS"
	resultColor := "#d4edda"
	if !report.Passed {
		resultNode = "RESULT_FAIL"
		resultColor = "#f8d7da"
	}

	sb.WriteString(fmt.Sprintf("\n    JUDGE --> %s[\"Overall Grade: %s<br/>Score: %.2f/10.0\"]\n",
		resultNode, report.OverallGrade, report.WeightedScore))
	sb.WriteString(fmt.Sprintf("    style %s fill:%s,stroke:#333,stroke-width:2px\n", resultNode, resultColor))
	sb.WriteString("```\n")

	return sb.String()
}
