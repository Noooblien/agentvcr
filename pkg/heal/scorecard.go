package heal

import (
	"fmt"
	"strings"
)

// RenderLoopbackScorecard produces a rich ANSI terminal representation of browser log loopback healing.
func RenderLoopbackScorecard(session *LoopbackSession) string {
	var sb strings.Builder

	sb.WriteString("\n\033[1;36m========================================================================================\033[0m\n")
	sb.WriteString("\033[1;36m       🩺 [AgentVCR] IN-BROWSER CONSOLE LOOPBACK & ZERO-ERROR HOT-PATCH ENGINE          \033[0m\n")
	sb.WriteString("\033[1;36m========================================================================================\033[0m\n\n")

	sb.WriteString(fmt.Sprintf(" \033[1mSession ID:\033[0m         %s\n", session.SessionID))
	sb.WriteString(fmt.Sprintf(" \033[1mExecution Latency:\033[0m  %.2f ms | \033[1mIterations Run:\033[0m %d of %d max\n\n",
		session.DurationMs, session.IterationsRun, session.MaxIterations))

	// Metrics
	sb.WriteString("\033[1;33m--- Loopback Healing Metrics ---\033[0m\n")
	sb.WriteString(fmt.Sprintf(" • Initial Diagnostics:     \033[1;31m%d\033[0m\n", session.InitialErrorCount))
	sb.WriteString(fmt.Sprintf(" • Synthesized Hot-Patches: \033[1;35m%d\033[0m\n", len(session.SynthesizedPatches)))
	sb.WriteString(fmt.Sprintf(" • Remaining Unfixed:       \033[1;33m%d\033[0m\n\n", session.FinalErrorCount))

	// Diagnostics List
	if len(session.Diagnostics) > 0 {
		sb.WriteString("\033[1;33m--- Detected In-Browser / Bundler Diagnostics ---\033[0m\n")
		for _, d := range session.Diagnostics {
			sb.WriteString(fmt.Sprintf(" • \033[1;31m%-8s\033[0m [%s] %s\n", d.ID, d.Category, d.RawMessage))
			if d.TargetFile != "" {
				sb.WriteString(fmt.Sprintf("   \033[90mLocation:\033[0m %s:%d:%d\n", d.TargetFile, d.TargetLine, d.TargetColumn))
			}
			if d.SuggestedFix != "" {
				sb.WriteString(fmt.Sprintf("   \033[36mSuggested Fix:\033[0m %s\n", d.SuggestedFix))
			}
		}
		sb.WriteString("\n")
	}

	// Synthesized Patches Preview
	if len(session.SynthesizedPatches) > 0 {
		sb.WriteString("\033[1;33m--- Synthesized Autonomous Hot-Patches ---\033[0m\n")
		for _, p := range session.SynthesizedPatches {
			sb.WriteString(fmt.Sprintf(" • \033[1;32m%-11s\033[0m \033[1m%s\033[0m (Confidence: %.0f%%)\n",
				p.PatchID, p.TargetFile, p.Confidence*100))
			sb.WriteString(fmt.Sprintf("   \033[35mExplanation:\033[0m %s\n", p.Explanation))
			sb.WriteString("\033[90m┌── Unified Hot-Patch Diff ────────────────────────────────────────────┐\033[0m\n")
			diffLines := strings.Split(p.DiffSnippet, "\n")
			for _, dl := range diffLines {
				color := "\033[32m"
				if strings.HasPrefix(dl, "-") {
					color = "\033[31m"
				}
				sb.WriteString(fmt.Sprintf("\033[90m│\033[0m %s%-68s\033[0m \033[90m│\033[0m\n", color, dl))
			}
			sb.WriteString("\033[90m└──────────────────────────────────────────────────────────────────────┘\033[0m\n\n")
		}
	}

	// Verdict Banner
	sb.WriteString("\033[1;33m--- Self-Healing Loopback Verdict ---\033[0m\n")
	if session.ZeroErrorsAchieved {
		sb.WriteString(" \033[1;42;30m ZERO ERRORS ACHIEVED \033[0m \033[1;32mIn-browser console errors cleanly resolved to 0 via autonomous hot-patching.\033[0m\n\n")
	} else {
		sb.WriteString(fmt.Sprintf(" \033[1;41;37m ERRORS PERSIST \033[0m \033[1;31m%d diagnostics could not be automatically resolved within iteration budget.\033[0m\n\n", session.FinalErrorCount))
	}

	return sb.String()
}
