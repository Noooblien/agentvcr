package research

import (
	"fmt"
	"strings"
)

// RenderResearchScorecard produces a rich ANSI terminal representation of the research findings.
func RenderResearchScorecard(report *ResearchReport) string {
	var sb strings.Builder

	sb.WriteString("\n\033[1;36m========================================================================================\033[0m\n")
	sb.WriteString("\033[1;36m       🔍 [AgentVCR] RECURSIVE MULTI-AGENT RESEARCH & CITATION VERIFICATION ENGINE      \033[0m\n")
	sb.WriteString("\033[1;36m========================================================================================\033[0m\n\n")

	sb.WriteString(fmt.Sprintf(" \033[1mResearch Objective:\033[0m %s\n", report.Query))
	sb.WriteString(fmt.Sprintf(" \033[1mExecution Latency:\033[0m  %.2f ms | \033[1mGenerated:\033[0m %s\n\n",
		report.DurationMs, report.GeneratedAt.Format("2006-01-02 15:04:05 MST")))

	// Key Metrics Table
	sb.WriteString("\033[1;33m--- Research Tree Metrics ---\033[0m\n")
	sb.WriteString(fmt.Sprintf(" • Total Tree Nodes:        \033[1m%d\033[0m (Max Depth: %d)\n",
		report.Tree.TotalNodes, report.Tree.MaxDepth))
	sb.WriteString(fmt.Sprintf(" • Verified Claims:         \033[1;32m%d\033[0m\n", len(report.VerifiedClaims)))
	sb.WriteString(fmt.Sprintf(" • Pruned Hallucinations:   \033[1;31m%d\033[0m\n", len(report.PrunedClaims)))
	sb.WriteString(fmt.Sprintf(" • Cryptographic Citations: \033[1;35m%d\033[0m\n", len(report.Citations)))
	sb.WriteString(fmt.Sprintf(" • Detected Contradictions: \033[1;33m%d\033[0m\n", len(report.Contradictions)))

	// Confidence & Grounding Progress Bars
	groundingBar := renderProgressBar(report.GroundingRatePct)
	confBar := renderProgressBar(report.OverallConfidence * 100.0)
	sb.WriteString(fmt.Sprintf(" • Grounding Rate:          %s \033[1m%.1f%%\033[0m\n", groundingBar, report.GroundingRatePct))
	sb.WriteString(fmt.Sprintf(" • Overall Confidence:      %s \033[1m%.1f%%\033[0m\n\n", confBar, report.OverallConfidence*100.0))

	// Verified Claims
	sb.WriteString("\033[1;33m--- Verified Grounded Claims ---\033[0m\n")
	if len(report.VerifiedClaims) == 0 {
		sb.WriteString(" \033[90m(No verified propositions survived grounding audit)\033[0m\n")
	} else {
		for i, c := range report.VerifiedClaims {
			cits := strings.Join(c.CitationIDs, ", ")
			if cits == "" {
				cits = "none"
			}
			sb.WriteString(fmt.Sprintf(" \033[1;32m[%d]\033[0m %s\n", i+1, c.Statement))
			sb.WriteString(fmt.Sprintf("     \033[90mConfidence: %.0f%% | Grounding: %.0f%% | Citations: %s\033[0m\n",
				c.Confidence*100, c.GroundingScore*100, cits))
		}
	}
	sb.WriteString("\n")

	// Cryptographic Citations Table
	if len(report.Citations) > 0 {
		sb.WriteString("\033[1;33m--- Cryptographic Citations (SHA-256 Grounded) ---\033[0m\n")
		for _, cit := range report.Citations {
			shortHash := cit.ContentHash
			if len(shortHash) > 12 {
				shortHash = shortHash[:12] + "..."
			}
			sb.WriteString(fmt.Sprintf(" • \033[1;35m%-6s\033[0m [%-12s] %s (hash: %s)\n",
				cit.ID, cit.SourceType, cit.SourceURI, shortHash))
		}
		sb.WriteString("\n")
	}

	// Contradictions & Conflict Resolution
	if len(report.Contradictions) > 0 {
		sb.WriteString("\033[1;31m--- Contradictions & Resolution ---\033[0m\n")
		for i, con := range report.Contradictions {
			sb.WriteString(fmt.Sprintf(" ⚠️  \033[1mConflict #%d\033[0m [%s Severity]: %s\n",
				i+1, con.Severity, con.Description))
			sb.WriteString(fmt.Sprintf("    \033[36mResolution:\033[0m %s\n", con.Resolution))
		}
		sb.WriteString("\n")
	}

	// Final Status Badge
	sb.WriteString("\033[1;33m--- Verification Verdict ---\033[0m\n")
	if report.PassedAudit {
		sb.WriteString(" \033[1;42;30m PASS \033[0m \033[1;32mResearch propositions mathematically grounded with zero unresolved contradictions.\033[0m\n\n")
	} else {
		sb.WriteString(" \033[1;41;37m FLAGGED \033[0m \033[1;31mGrounding threshold failed or unresolvable contradictions detected.\033[0m\n\n")
	}

	return sb.String()
}

func renderProgressBar(pct float64) string {
	width := 20
	completed := int((pct / 100.0) * float64(width))
	if completed > width {
		completed = width
	}
	if completed < 0 {
		completed = 0
	}
	fill := strings.Repeat("█", completed)
	empty := strings.Repeat("░", width-completed)

	color := "\033[32m"
	if pct < 50.0 {
		color = "\033[31m"
	} else if pct < 75.0 {
		color = "\033[33m"
	}
	return fmt.Sprintf("%s[%s%s]\033[0m", color, fill, empty)
}
