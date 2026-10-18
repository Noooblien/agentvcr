package distill

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

// RenderDistillScorecard formats an ANSI terminal scorecard of trajectory distillation results.
func RenderDistillScorecard(report *DistillationReport) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("%s%s========================================================================================%s\n", ColorBold, ColorMagenta, ColorReset))
	sb.WriteString(fmt.Sprintf("%s%s  AGENTVCR // TRAJECTORY DISTILLATION & ALIGNMENT DATASET SCORECARD%s\n", ColorBold, ColorWhite, ColorReset))
	sb.WriteString(fmt.Sprintf("%s%s========================================================================================%s\n\n", ColorBold, ColorMagenta, ColorReset))

	statusStr := fmt.Sprintf("%s[PASS]%s", ColorGreen, ColorReset)
	if !report.Passed {
		statusStr = fmt.Sprintf("%s[BELOW_MARGIN]%s", ColorYellow, ColorReset)
	}

	sb.WriteString(fmt.Sprintf("  %sReport ID:%s           %s\n", ColorBold, ColorReset, report.ReportID))
	sb.WriteString(fmt.Sprintf("  %sTarget Format:%s       %s%s%s\n", ColorBold, ColorReset, ColorCyan, report.Format, ColorReset))
	sb.WriteString(fmt.Sprintf("  %sDistillation Gate:%s   %s (Min Delta Reward: %.2f | Actual Margin: %.2f)\n",
		ColorBold, ColorReset, statusStr, 0.20, report.AverageRewardMargin))
	sb.WriteString(fmt.Sprintf("  %sToken Economy:%s       Raw: %d tokens | Distilled: %d tokens (Compressed: %.1f%%)\n\n",
		ColorBold, ColorReset, report.RawTokensTotal, report.DistilledTokensTotal, report.CompressionRatio*100))

	// Synthesis Telemetry Table
	sb.WriteString(fmt.Sprintf("  %s%sDATASET SYNTHESIS TELEMETRY:%s\n", ColorBold, ColorWhite, ColorReset))
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("  %-28s %-16s %s\n", "METRIC", "QUANTITY", "SPECIFICATION"))
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("  %-28s %-16d %s\n", "Sessions Ingested", report.TotalSessionsScanned, "Multi-turn trace recordings"))
	sb.WriteString(fmt.Sprintf("  %-28s %-16d %s\n", "SFT Training Examples", report.SFTExamplesGenerated, "OpenAI / ShareGPT format rows"))
	sb.WriteString(fmt.Sprintf("  %-28s %-16d %s\n", "DPO Preference Pairs", report.DPOPairsGenerated, "Chosen vs Rejected pairs"))
	sb.WriteString(fmt.Sprintf("  %-28s %-16d %s\n", "Pruned / Denoised Turns", report.PrunedTurnsCount, "Noisy tool outputs & loop frames removed"))
	sb.WriteString(fmt.Sprintf("  %-28s %-16.2f %s\n", "Reward Margin (ΔR)", report.AverageRewardMargin, "Preference margin above baseline"))
	sb.WriteString(fmt.Sprintf("  %-28s %-16s %s\n", "Context Compression", fmt.Sprintf("%.1f%%", report.CompressionRatio*100), "Token storage savings ratio"))
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────────\n\n")

	// Sample DPO Pair Preview
	if report.SamplePrompt != "" {
		sb.WriteString(fmt.Sprintf("  %s%sSAMPLE DPO PREFERENCE PAIR PREVIEW:%s\n", ColorBold, ColorWhite, ColorReset))
		sb.WriteString(fmt.Sprintf("  %sPrompt:%s   \"%s\"\n", ColorBold, ColorReset, report.SamplePrompt))
		sb.WriteString(fmt.Sprintf("  %s%s[CHOSEN]%s   %s\n", ColorBold, ColorGreen, ColorReset, report.SampleChosenSummary))
		sb.WriteString(fmt.Sprintf("  %s%s[REJECTED]%s %s\n\n", ColorBold, ColorRed, ColorReset, report.SampleRejectedSummary))
	}

	sb.WriteString(fmt.Sprintf("%s%s========================================================================================%s\n", ColorBold, ColorMagenta, ColorReset))

	return sb.String()
}

// RenderDistillMermaid formats a Mermaid diagram depicting the distillation and alignment pipeline.
func RenderDistillMermaid(report *DistillationReport) string {
	var sb strings.Builder

	sb.WriteString("```mermaid\n")
	sb.WriteString("graph TD\n")
	sb.WriteString(fmt.Sprintf("    RAW[\"Raw Agent Sessions (%d Traces)\"] --> FILTER[\"Noise Filter & Tool Deduplication\"]\n", report.TotalSessionsScanned))
	sb.WriteString(fmt.Sprintf("    FILTER -->|Pruned %d Turns| EXTRACT[\"Trajectory Distiller\"]\n", report.PrunedTurnsCount))
	sb.WriteString(fmt.Sprintf("    EXTRACT -->|SFT Extraction| SFT[\"SFT Dataset (%d Examples)<br/>OpenAI JSONL\"]\n", report.SFTExamplesGenerated))
	sb.WriteString(fmt.Sprintf("    EXTRACT -->|Pairing & Divergence| DPO[\"DPO Preference Pairs (%d Pairs)<br/>Reward Margin ΔR: %.2f\"]\n", report.DPOPairsGenerated, report.AverageRewardMargin))
	sb.WriteString("    DPO --> HF[\"HuggingFace Alignment Format\"]\n")
	sb.WriteString("    SFT --> TUNE[\"LoRA / QLoRA Fine-Tuning Pipeline\"]\n")
	sb.WriteString("    HF --> TUNE\n")
	sb.WriteString("    style DPO fill:#d4edda,stroke:#333,stroke-width:2px\n")
	sb.WriteString("    style SFT fill:#cce5ff,stroke:#333,stroke-width:2px\n")
	sb.WriteString("```\n")

	return sb.String()
}
