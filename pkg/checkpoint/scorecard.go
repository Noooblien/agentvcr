package checkpoint

import (
	"fmt"
	"strings"
)

// RenderCheckpointScorecard prints an ANSI terminal report visualizing checkpoint health and space savings.
func RenderCheckpointScorecard(report *CheckpointAuditReport) string {
	var sb strings.Builder

	sb.WriteString("\033[1;36m========================================================================================\033[0m\n")
	sb.WriteString("  \033[1;37mAGENTVCR // SESSION CHECKPOINT HIBERNATION & RESTORATION SCORECARD\033[0m\n")
	sb.WriteString("\033[1;36m========================================================================================\033[0m\n\n")

	sb.WriteString(fmt.Sprintf("  \033[1;37mSystem ID:\033[0m          %s\n", report.SystemID))
	sb.WriteString(fmt.Sprintf("  \033[1;37mTimestamp:\033[0m          %s\n", report.Timestamp.Format("2006-01-02 15:04:05 UTC")))

	healthBadge := "\033[1;32m[PASS] ALL CHECKPOINTS VERIFIED & RESTORABLE\033[0m"
	if !report.IntegrityVerified {
		healthBadge = "\033[1;31m[FAIL] INTEGRITY COMPROMISED // SHA-256 HASH MISMATCH\033[0m"
	}
	sb.WriteString(fmt.Sprintf("  \033[1;37mIntegrity Status:\033[0m   %s\n\n", healthBadge))

	sb.WriteString("  \033[1;33mCOMPRESSION & STORAGE EFFICIENCY:\033[0m\n")
	sb.WriteString(fmt.Sprintf("  • Total Images:        \033[1;37m%d\033[0m (%d hibernated, %d active/restored)\n",
		report.TotalImages, report.TotalHibernated, report.TotalRestored))
	sb.WriteString(fmt.Sprintf("  • Uncompressed Volume: \033[1;31m%d bytes\033[0m (%.2f KB)\n",
		report.TotalRawBytes, float64(report.TotalRawBytes)/1024.0))
	sb.WriteString(fmt.Sprintf("  • Compressed Storage:  \033[1;32m%d bytes\033[0m (%.2f KB)\n",
		report.TotalCompressedBytes, float64(report.TotalCompressedBytes)/1024.0))
	sb.WriteString(fmt.Sprintf("  • Delta Storage Savings:\033[1;32m%.1f%% space saved\033[0m\n",
		report.CompressionRatio*100.0))

	sb.WriteString("\n  \033[1;33mCHECKPOINT IMAGES & RESTORATION LINEAGE:\033[0m\n")
	sb.WriteString("  " + strings.Repeat("─", 84) + "\n")
	sb.WriteString(fmt.Sprintf("  %-18s %-12s %-6s %-12s %-14s %s\n",
		"IMAGE ID", "STATE", "STEP", "RAW BYTES", "COMPRESSED", "INTEGRITY HASH"))
	sb.WriteString("  " + strings.Repeat("─", 84) + "\n")

	for _, img := range report.Images {
		stateColor := "\033[32m" // Green
		if img.State == StateHibernated {
			stateColor = "\033[33m" // Yellow
		} else if img.State == StateCorrupted {
			stateColor = "\033[31m" // Red
		}

		hashPrefix := img.SHA256Hash
		if len(hashPrefix) > 12 {
			hashPrefix = hashPrefix[:12] + "..."
		}

		sb.WriteString(fmt.Sprintf("  %-18s %s%-12s\033[0m %-6d %-12d %-14d %s\n",
			img.ImageID,
			stateColor,
			img.State,
			img.StepIndex,
			img.UncompressedSize,
			img.CompressedSize,
			hashPrefix,
		))
	}
	sb.WriteString("  " + strings.Repeat("─", 84) + "\n")

	sb.WriteString("\n\033[1;36m========================================================================================\033[0m\n")
	return sb.String()
}

// RenderCheckpointMermaid exports a Mermaid diagram of the freeze, delta, and restoration flow.
func RenderCheckpointMermaid(report *CheckpointAuditReport) string {
	var sb strings.Builder

	sb.WriteString("```mermaid\n")
	sb.WriteString("graph TD\n")
	sb.WriteString("    AGENT[\"Active Agent Swarm Runtime\"] --> FREEZE[\"Freeze Memory & Tool Cursors\"]\n")
	sb.WriteString("    FREEZE --> DELTA{\"Incremental Delta<br/>Compression\"}\n")
	sb.WriteString("    DELTA --> DISK[(\"Encrypted Gzip Disk Archive<br/>.vcr_checkpoints\")]\n")
	sb.WriteString("    DISK --> VERIFY{\"SHA-256 Integrity Check\"}\n")
	sb.WriteString("    VERIFY -->|Valid| THAW[\"Thaw & Hot-Revive State\"]\n")
	sb.WriteString("    THAW --> RESUME[\"Resume Agent Execution at Step N\"]\n\n")

	for i, img := range report.Images {
		if i >= 4 {
			break
		}
		cleanID := strings.ReplaceAll(img.ImageID, "-", "_")
		sb.WriteString(fmt.Sprintf("    DISK -.-> CKPT_%s[\"Checkpoint: %s<br/>Step: %d | State: %s\"]\n",
			cleanID, img.ImageID, img.StepIndex, img.State))
	}

	sb.WriteString("```\n")
	return sb.String()
}
