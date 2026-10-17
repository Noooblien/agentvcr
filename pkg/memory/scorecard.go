package memory

import (
	"fmt"
	"strings"
	"time"
)

// RenderMemoryScorecard produces an ANSI terminal dashboard visualizing memory consolidation and decay metrics.
func RenderMemoryScorecard(report *ConsolidationReport, decayEngine *DecayEngine, now time.Time) string {
	var sb strings.Builder

	sb.WriteString("\033[1;34m========================================================================================\033[0m\n")
	sb.WriteString("  \033[1;37mAGENTVCR // AUTONOMOUS HIERARCHICAL MEMORY CONSOLIDATION & RETENTION SCORECARD\033[0m\n")
	sb.WriteString("\033[1;34m========================================================================================\033[0m\n\n")

	sb.WriteString(fmt.Sprintf("  \033[1;37mSystem ID:\033[0m        %s\n", report.SystemID))
	sb.WriteString(fmt.Sprintf("  \033[1;37mTimestamp:\033[0m        %s\n", report.Timestamp.Format("2006-01-02 15:04:05 UTC")))

	healthBadge := "\033[1;32m[PASS] CONSOLIDATED // ZERO CONTEXT SPILLOVER\033[0m"
	if report.TokenSavingsRatio < 0.30 {
		healthBadge = "\033[1;33m[WARN] LOW COMPRESSION // RETENTION UNCONSOLIDATED\033[0m"
	}
	sb.WriteString(fmt.Sprintf("  \033[1;37mMemory Status:\033[0m    %s\n\n", healthBadge))

	sb.WriteString("  \033[1;36mMEMORY HIERARCHY & TOKEN REDUCTION TELEMETRY:\033[0m\n")
	sb.WriteString(fmt.Sprintf("  • Active Memory Nodes:       \033[1;33m%d\033[0m (%d reflections, %d semantic facts)\n",
		report.ActiveMemoryNodes, report.ConsolidatedReflection, report.PermanentSemanticFacts))
	sb.WriteString(fmt.Sprintf("  • Episodic Turns Buffered:   \033[1;37m%d\033[0m\n", report.TotalEpisodicBuffered))
	sb.WriteString(fmt.Sprintf("  • Pruned Decayed Nodes:      \033[1;31m%d\033[0m\n", report.PrunedDecayedNodes))
	sb.WriteString(fmt.Sprintf("  • Context Window Tokens:     \033[1;31m%d raw\033[0m -> \033[1;32m%d consolidated\033[0m\n",
		report.RawEpisodicTokens, report.ConsolidatedTokens))
	sb.WriteString(fmt.Sprintf("  • Prompt Context Savings:    \033[1;32m%.1f%% token reduction\033[0m\n",
		report.TokenSavingsRatio*100.0))

	sb.WriteString("\n  \033[1;36mTOP CONSOLIDATED MEMORIES (RANKED BY EBBINGHAUS RETENTION & IMPORTANCE):\033[0m\n")
	sb.WriteString("  " + strings.Repeat("─", 84) + "\n")
	sb.WriteString(fmt.Sprintf("  %-10s %-12s %-10s %-10s %-10s %s\n",
		"NODE ID", "LEVEL", "RETENTION", "IMPORTANCE", "ACCESSES", "CONTENT PREVIEW"))
	sb.WriteString("  " + strings.Repeat("─", 84) + "\n")

	for _, m := range report.TopMemories {
		retention := decayEngine.RetentionScore(m, now)
		retentionBadge := fmt.Sprintf("%.2f", retention)
		if retention >= 0.85 {
			retentionBadge = "\033[32m" + retentionBadge + "\033[0m"
		} else if retention >= 0.50 {
			retentionBadge = "\033[33m" + retentionBadge + "\033[0m"
		} else {
			retentionBadge = "\033[31m" + retentionBadge + "\033[0m"
		}

		preview := m.Content
		if len(preview) > 40 {
			preview = preview[:37] + "..."
		}

		sb.WriteString(fmt.Sprintf("  %-10s %-12s %-19s %-10.2f %-10d %s\n",
			m.ID, m.Level, retentionBadge, m.Importance, m.AccessCount, preview))
	}
	sb.WriteString("  " + strings.Repeat("─", 84) + "\n")

	sb.WriteString("\n\033[1;34m========================================================================================\033[0m\n")
	return sb.String()
}

// RenderMemoryMermaid creates a Mermaid flowchart depicting hierarchical memory clustering and semantic graph edges.
func RenderMemoryMermaid(engine *MemoryEngine) string {
	var sb strings.Builder

	sb.WriteString("```mermaid\n")
	sb.WriteString("graph TD\n")
	sb.WriteString("    RAW[\"Raw Episodic Stream<br/>(Actions, Tool Calls, Turns)\"] --> BUFFER[\"Episodic Working Buffer\"]\n")
	sb.WriteString("    BUFFER -->|\"Hierarchical Clustering\"| REF[\"Reflections Layer<br/>(Synthesized Insights)\"]\n")
	sb.WriteString("    REF -->|\"Entity / Predicate Extraction\"| SEM[\"Semantic Knowledge Triples<br/>(Permanent Core Memory)\"]\n")
	sb.WriteString("    REF -.->|\"Ebbinghaus Decay Score < 0.15\"| PRUNE[\"Decayed Nodes (Pruned)\"]\n")

	triples := engine.KnowledgeGraph().ListTriples()
	if len(triples) > 0 {
		sb.WriteString("\n    subgraph KnowledgeGraph[\"Semantic Knowledge Graph\"]\n")
		for i, t := range triples {
			if i >= 6 {
				break
			}
			subID := strings.ReplaceAll(t.Subject, " ", "_")
			objID := strings.ReplaceAll(t.Object, " ", "_")
			sb.WriteString(fmt.Sprintf("        KG_%s[\"%s\"] -->|\"%s\"| KG_%s[\"%s\"]\n",
				subID, t.Subject, t.Predicate, objID, t.Object))
		}
		sb.WriteString("    end\n")
		sb.WriteString("    SEM --> KnowledgeGraph\n")
	}

	sb.WriteString("```\n")
	return sb.String()
}
