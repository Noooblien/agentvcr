package snapshot

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

// RenderCoWScorecard formats an ANSI terminal scorecard for CoW snapshot operations.
func RenderCoWScorecard(report *SnapshotDeltaReport) string {
	var sb strings.Builder

	sb.WriteString("\n" + colorBold + colorCyan + "========================================================================================================\n" + colorReset)
	sb.WriteString(colorBold + colorWhite + "  AGENTVCR // DOCKER / COW LAYERED FILESYSTEM SNAPSHOT & ROLLBACK AUDIT\n" + colorReset)
	sb.WriteString(colorBold + colorCyan + "========================================================================================================\n" + colorReset)

	sb.WriteString(fmt.Sprintf("  %sSession:%s %s  |  %sActive Layers:%s %d  |  %sDelta Bytes:%s %d B  |  %sRollback Latency:%s %s  |  %sStatus:%s %s%s%s\n",
		colorBold, colorReset, report.SessionID,
		colorBold, colorReset, report.TotalLayers,
		colorBold, colorReset, report.TotalBytesDelta,
		colorCyan, colorReset, report.RollbackLatency,
		colorBold, colorReset, colorGreen, report.Status, colorReset))
	sb.WriteString(colorGray + "--------------------------------------------------------------------------------------------------------\n" + colorReset)

	sb.WriteString(fmt.Sprintf("  %sChanges Tracked:%s  Created: %s%d%s  |  Modified: %s%d%s  |  Deleted (Whiteouts): %s%d%s\n",
		colorBold, colorReset,
		colorGreen, len(report.CreatedFiles), colorReset,
		colorYellow, len(report.ModifiedFiles), colorReset,
		colorRed, len(report.DeletedFiles), colorReset))
	sb.WriteString(colorGray + "--------------------------------------------------------------------------------------------------------\n" + colorReset)

	sb.WriteString(fmt.Sprintf("%-16s %-32s %s\n", "MUTATION TYPE", "RELATIVE PATH", "DETAILS"))
	sb.WriteString(colorGray + "--------------------------------------------------------------------------------------------------------\n" + colorReset)

	for _, f := range report.CreatedFiles {
		sb.WriteString(fmt.Sprintf("%s%-16s%s %-32s %s\n", colorGreen, "CREATED", colorReset, f, "New file in upper scratch layer"))
	}
	for _, f := range report.ModifiedFiles {
		sb.WriteString(fmt.Sprintf("%s%-16s%s %-32s %s\n", colorYellow, "MODIFIED", colorReset, f, "Copy-on-write upper layer override"))
	}
	for _, f := range report.DeletedFiles {
		sb.WriteString(fmt.Sprintf("%s%-16s%s %-32s %s\n", colorRed, "DELETED", colorReset, f, "OCI .wh. tombstone marker created"))
	}

	sb.WriteString(colorBold + colorCyan + "========================================================================================================\n\n" + colorReset)
	return sb.String()
}

// RenderCoWMermaid renders a Mermaid DAG of the OverlayFS layered architecture.
func RenderCoWMermaid(report *SnapshotDeltaReport) string {
	var sb strings.Builder

	sb.WriteString("```mermaid\n")
	sb.WriteString("flowchart TD\n")
	sb.WriteString("    BaseLayer[(\"Lower Base Layer (Read-Only Immutable Workspace)\")]\n")
	sb.WriteString("    UpperLayer[(\"Upper Scratch Layer (Read-Write Copy-on-Write Delta)\")]\n")
	sb.WriteString("    Workdir[(\"OverlayFS Workdir (Kernel Transaction Log)\")]\n")
	sb.WriteString("    MergedView[\"Merged Unified View (Agent Execution Container)\"]\n")

	sb.WriteString("    BaseLayer --> MergedView\n")
	sb.WriteString("    UpperLayer --> MergedView\n")
	sb.WriteString("    Workdir -.-> UpperLayer\n")

	if len(report.DeletedFiles) > 0 {
		sb.WriteString("    Tombstone[\"OCI Whiteout Marker (.wh.)\"] --> UpperLayer\n")
		sb.WriteString("    style Tombstone fill:#ff4d4f,stroke:#ff0000,stroke-width:2px\n")
	}

	sb.WriteString("    style BaseLayer fill:#1890ff,stroke:#096dd9,stroke-width:1px\n")
	sb.WriteString("    style UpperLayer fill:#52c41a,stroke:#389e0d,stroke-width:1px\n")
	sb.WriteString("    style MergedView fill:#722ed1,stroke:#531dab,stroke-width:2px\n")

	sb.WriteString("```\n")
	return sb.String()
}

// RenderCoWMarkdown formats a GitHub-flavored Markdown report.
func RenderCoWMarkdown(report *SnapshotDeltaReport) string {
	var sb strings.Builder

	sb.WriteString("## AgentVCR Layered CoW Filesystem Snapshot Report\n\n")
	sb.WriteString(fmt.Sprintf("- **Session ID**: `%s`\n", report.SessionID))
	sb.WriteString(fmt.Sprintf("- **Active Layers**: %d\n", report.TotalLayers))
	sb.WriteString(fmt.Sprintf("- **Rollback Latency**: `%s`\n", report.RollbackLatency))
	sb.WriteString(fmt.Sprintf("- **Total Delta Overhead**: `%d Bytes`\n", report.TotalBytesDelta))
	sb.WriteString(fmt.Sprintf("- **Status**: `%s`\n\n", report.Status))

	sb.WriteString("| Action | Path | Description |\n")
	sb.WriteString("|:---|:---|:---|\n")

	for _, f := range report.CreatedFiles {
		sb.WriteString(fmt.Sprintf("| `CREATED` | `%s` | New file created in upper scratch layer |\n", f))
	}
	for _, f := range report.ModifiedFiles {
		sb.WriteString(fmt.Sprintf("| `MODIFIED` | `%s` | File shadowed and modified in upper layer |\n", f))
	}
	for _, f := range report.DeletedFiles {
		sb.WriteString(fmt.Sprintf("| `DELETED` | `%s` | Marked deleted with OCI `.wh.` whiteout tombstone |\n", f))
	}

	return sb.String()
}
