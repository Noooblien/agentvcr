package sandbox

import (
	"fmt"
	"strings"
)

// RenderSandboxScorecard formats a rich ANSI terminal audit scorecard for the sandbox.
func RenderSandboxScorecard(report *SandboxAuditReport) string {
	var sb strings.Builder

	sb.WriteString("\033[1;35m========================================================================================\033[0m\n")
	sb.WriteString("  \033[1;37mAGENTVCR // DETERMINISTIC SANDBOX EXECUTION HYPERVISOR SCORECARD\033[0m\n")
	sb.WriteString("\033[1;35m========================================================================================\033[0m\n\n")

	// Status badge
	stateColor := "\033[1;32m" // Green
	if report.State == StateTerminated {
		stateColor = "\033[1;34m" // Blue
	} else if report.State == StateKilledOOM || report.State == StateKilledTimeout {
		stateColor = "\033[1;31m" // Red
	}
	sb.WriteString(fmt.Sprintf("  \033[1;37mContainer ID:\033[0m     %s\n", report.SandboxID))
	sb.WriteString(fmt.Sprintf("  \033[1;37mStatus:\033[0m           %s%s\033[0m\n", stateColor, report.State))
	sb.WriteString(fmt.Sprintf("  \033[1;37mStarted At:\033[0m       %s\n", report.StartedAt.Format("2006-01-02 15:04:05 UTC")))

	sb.WriteString("\n  \033[1;36mRESOURCE QUOTAS & RUNTIME TELEMETRY:\033[0m\n")
	sb.WriteString(fmt.Sprintf("  • Commands Executed:    \033[1;33m%d\033[0m\n", report.Stats.CommandsExecuted))
	sb.WriteString(fmt.Sprintf("  • Total Wall Time:      \033[1;32m%d ms\033[0m\n", report.Stats.TotalDurationMs))
	sb.WriteString(fmt.Sprintf("  • Total CPU Time:       \033[1;32m%d ms\033[0m\n", report.Stats.TotalCPUTimeMs))
	sb.WriteString(fmt.Sprintf("  • Peak RSS Memory:      \033[1;35m%.2f MB\033[0m\n", float64(report.Stats.PeakMemoryBytes)/(1024*1024)))
	sb.WriteString(fmt.Sprintf("  • Total Disk Written:   \033[1;35m%d bytes\033[0m\n", report.Stats.TotalDiskWritten))

	sb.WriteString("\n  \033[1;36mOVERLAYFS COPY-ON-WRITE (CoW) DELTA:\033[0m\n")
	sb.WriteString(fmt.Sprintf("  • Files Created:        \033[1;32m+%d\033[0m\n", report.Stats.FilesCreated))
	sb.WriteString(fmt.Sprintf("  • Files Modified:       \033[1;33m~%d\033[0m\n", report.Stats.FilesModified))
	sb.WriteString(fmt.Sprintf("  • Files Deleted:        \033[1;31m-%d\033[0m\n", report.Stats.FilesDeleted))

	sb.WriteString("\n  \033[1;36mCOMMAND EXECUTION TIMELINE:\033[0m\n")
	sb.WriteString("  " + strings.Repeat("─", 84) + "\n")
	sb.WriteString(fmt.Sprintf("  %-16s %-32s %-8s %-10s %s\n", "CMD ID", "COMMAND", "STATUS", "LATENCY", "VIOLATIONS"))
	sb.WriteString("  " + strings.Repeat("─", 84) + "\n")

	for _, cmd := range report.ExecHistory {
		cmdStr := cmd.CommandLine
		if len(cmdStr) > 30 {
			cmdStr = cmdStr[:27] + "..."
		}

		status := "\033[32mOK (0)\033[0m"
		if cmd.ExitCode != 0 {
			status = fmt.Sprintf("\033[31mFAIL (%d)\033[0m", cmd.ExitCode)
		}

		violStr := "\033[32mNone\033[0m"
		if len(cmd.Violations) > 0 {
			violStr = fmt.Sprintf("\033[1;31m%s (%s)\033[0m", cmd.Violations[0].Type, cmd.Violations[0].ActionTaken)
		}

		sb.WriteString(fmt.Sprintf("  %-16s %-32s %-18s %-10s %s\n",
			cmd.CommandID, cmdStr, status, fmt.Sprintf("%dms", cmd.DurationMs), violStr))
	}
	sb.WriteString("  " + strings.Repeat("─", 84) + "\n")

	// Security Compliance Summary
	sb.WriteString("\n  \033[1;36mSECURITY & COMPLIANCE EVALUATION:\033[0m\n")
	if report.ZeroEscapeRisk {
		sb.WriteString("  • Zero Host Escape Risk:       \033[1;32m[PASS] Zero privilege escalation or escape vectors\033[0m\n")
	} else {
		sb.WriteString("  • Zero Host Escape Risk:       \033[1;31m[FAIL] Escape attempts or security infractions detected\033[0m\n")
	}

	if report.CompliantWithSLA {
		sb.WriteString("  • Resource SLA Compliance:     \033[1;32m[PASS] Within CPU, memory, and disk boundaries\033[0m\n")
	} else {
		sb.WriteString("  • Resource SLA Compliance:     \033[1;31m[FAIL] Exceeded resource quota limits\033[0m\n")
	}

	scoreColor := "\033[1;32m"
	if report.SecurityScore < 80.0 {
		scoreColor = "\033[1;31m"
	} else if report.SecurityScore < 95.0 {
		scoreColor = "\033[1;33m"
	}
	sb.WriteString(fmt.Sprintf("  • Overall Security Score:      %s%.1f / 100.0\033[0m\n", scoreColor, report.SecurityScore))

	sb.WriteString("\n\033[1;35m========================================================================================\033[0m\n\n")
	return sb.String()
}
