package microvm

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

// RenderMicroVMScorecard formats an ANSI terminal scorecard of the microVM execution and containment.
func RenderMicroVMScorecard(report *MicroVMReport) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("%s%s========================================================================================%s\n", ColorBold, ColorCyan, ColorReset))
	sb.WriteString(fmt.Sprintf("%s%s  AGENTVCR // EPHEMERAL FIRECRACKER MICROVM JAIL & SNAPSHOT HYPERVISOR%s\n", ColorBold, ColorWhite, ColorReset))
	sb.WriteString(fmt.Sprintf("%s%s========================================================================================%s\n\n", ColorBold, ColorCyan, ColorReset))

	statusColor := ColorGreen
	if report.Status == "BREACH_JAILED" {
		statusColor = ColorRed
	} else if report.Status == "RESTORED" {
		statusColor = ColorYellow
	}

	sb.WriteString(fmt.Sprintf("  %sMicroVM ID:%s         %s\n", ColorBold, ColorReset, report.VMID))
	sb.WriteString(fmt.Sprintf("  %sJail Containment:%s   %s%s[%s]%s\n", ColorBold, ColorReset, ColorBold, statusColor, report.Status, ColorReset))
	sb.WriteString(fmt.Sprintf("  %sCold Boot Latency:%s  %dms (Deterministic microVM kernel initialization)\n", ColorBold, ColorReset, report.BootLatencyMs))
	sb.WriteString(fmt.Sprintf("  %sSnapshot Telemetry:%s %d snapshots captured, %d time-travel restorations (Avg: %.1fms)\n",
		ColorBold, ColorReset, report.TotalSnapshots, report.TotalRestores, report.AverageRestoreMs))
	sb.WriteString(fmt.Sprintf("  %sBreach Defenses:%s    %d security containment traps tripped\n\n", ColorBold, ColorReset, report.BreachesDetected))

	if len(report.Violations) > 0 {
		sb.WriteString(fmt.Sprintf("  %s%sISOLATION VIOLATIONS INTERCEPTED & CONTAINED:%s\n", ColorBold, ColorRed, ColorReset))
		for _, v := range report.Violations {
			sb.WriteString(fmt.Sprintf("  %s• [BLOCKED]%s %s\n", ColorRed, ColorReset, v))
		}
		sb.WriteString("\n")
	}

	// Execution Command Table
	sb.WriteString(fmt.Sprintf("  %s%sEXECUTION LOG INSIDE CHROOT/SECCOMP JAIL:%s\n", ColorBold, ColorWhite, ColorReset))
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("  %-4s %-32s %-8s %-12s %s\n", "STEP", "COMMAND", "EXIT", "DURATION", "SECURITY STATUS"))
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────────\n")

	for i, res := range report.Executions {
		cmdStr := res.Command
		if len(cmdStr) > 30 {
			cmdStr = cmdStr[:27] + "..."
		}

		secStatus := fmt.Sprintf("%s[CONTAINED]%s", ColorGreen, ColorReset)
		if res.SecurityBreach {
			secStatus = fmt.Sprintf("%s%s[TRAPPED: %s]%s", ColorBold, ColorRed, res.ViolationType, ColorReset)
		}

		sb.WriteString(fmt.Sprintf("  %-4d %-32s %-8d %-12s %s\n",
			i+1, cmdStr, res.ExitCode, fmt.Sprintf("%dms", res.DurationMs), secStatus))
	}

	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("%s%s========================================================================================%s\n", ColorBold, ColorCyan, ColorReset))
	return sb.String()
}

// RenderMicroVMMermaid generates a Mermaid flowchart depicting the microVM containment boundary.
func RenderMicroVMMermaid(report *MicroVMReport) string {
	var sb strings.Builder

	sb.WriteString("```mermaid\n")
	sb.WriteString("graph TD\n")
	sb.WriteString(fmt.Sprintf("    HOST[\"Host OS / Antigravity Agent\"] -->|Spawn %dms| VM\n", report.BootLatencyMs))
	sb.WriteString(fmt.Sprintf("    subgraph VM [\"Firecracker MicroVM Jail: %s\"]\n", report.VMID))
	sb.WriteString("        SECCOMP[\"Seccomp-BPF Syscall Filter\"]\n")
	sb.WriteString("        CHROOT[\"Chroot Mount Namespace\"]\n")
	sb.WriteString("        NET[\"vhost-net Socket Containment\"]\n")
	sb.WriteString("    end\n")

	for i, res := range report.Executions {
		cmdNode := fmt.Sprintf("CMD_%d", i+1)
		cleanCmd := strings.ReplaceAll(res.Command, "\"", "'")
		if len(cleanCmd) > 24 {
			cleanCmd = cleanCmd[:21] + "..."
		}

		if res.SecurityBreach {
			sb.WriteString(fmt.Sprintf("    VM -->|Breach Attempt| %s[\"%s<br/>TRAPPED: %s\"]\n", cmdNode, cleanCmd, res.ViolationType))
			sb.WriteString(fmt.Sprintf("    style %s fill:#fbeae8,stroke:#dc3545,stroke-width:2px\n", cmdNode))
		} else {
			sb.WriteString(fmt.Sprintf("    VM -->|Nominal Exec| %s[\"%s<br/>Exit: 0 (%dms)\"]\n", cmdNode, cleanCmd, res.DurationMs))
			sb.WriteString(fmt.Sprintf("    style %s fill:#e6f4ea,stroke:#28a745,stroke-width:1px\n", cmdNode))
		}
	}

	if report.TotalSnapshots > 0 {
		sb.WriteString(fmt.Sprintf("    VM -.->|CoW Snapshot %.1fms| SNAP[\"%d Snapshots / %d Restores\"]\n",
			report.AverageRestoreMs, report.TotalSnapshots, report.TotalRestores))
		sb.WriteString("    style SNAP fill:#f1eefe,stroke:#6f42c1,stroke-width:2px\n")
	}

	sb.WriteString("```\n")
	return sb.String()
}
