package actor

import (
	"fmt"
	"strings"
)

// RenderActorScorecard formats an ANSI terminal scorecard visualizing system health and telemetry.
func RenderActorScorecard(report *SystemAuditReport) string {
	var sb strings.Builder

	sb.WriteString("\033[1;35m========================================================================================\033[0m\n")
	sb.WriteString("  \033[1;37mAGENTVCR // DISTRIBUTED AGENT ACTOR MESH & SUPERVISION TREE SCORECARD\033[0m\n")
	sb.WriteString("\033[1;35m========================================================================================\033[0m\n\n")

	sb.WriteString(fmt.Sprintf("  \033[1;37mSystem ID:\033[0m        %s\n", report.SystemID))
	sb.WriteString(fmt.Sprintf("  \033[1;37mStarted At:\033[0m       %s\n", report.StartedAt.Format("2006-01-02 15:04:05 UTC")))

	resBadge := "\033[1;32m[PASS] ALL ACTORS RESILIENT & SUPERVISED\033[0m"
	if !report.AllResilient {
		resBadge = "\033[1;31m[WARN] ACTOR CRASHES ESCALATED OR TERMINATED\033[0m"
	}
	sb.WriteString(fmt.Sprintf("  \033[1;37mMesh Health:\033[0m      %s\n\n", resBadge))

	sb.WriteString("  \033[1;36mMESH TELEMETRY OVERVIEW:\033[0m\n")
	sb.WriteString(fmt.Sprintf("  • Total Actors:       \033[1;33m%d\033[0m (\033[1;32m%d active\033[0m)\n", report.TotalActors, report.ActiveActors))
	sb.WriteString(fmt.Sprintf("  • Total Messages:     \033[1;32m%d\033[0m\n", report.TotalMessages))
	sb.WriteString(fmt.Sprintf("  • Automatic Restarts: \033[1;35m%d\033[0m\n", report.TotalRestarts))
	sb.WriteString(fmt.Sprintf("  • Dead Letters:       \033[1;31m%d\033[0m\n", report.DeadLettersCount))

	sb.WriteString("\n  \033[1;36mACTOR STATUS & PROCESSOR TELEMETRY:\033[0m\n")
	sb.WriteString("  " + strings.Repeat("─", 84) + "\n")
	sb.WriteString(fmt.Sprintf("  %-20s %-16s %-12s %-10s %-10s %s\n",
		"ACTOR ID", "ROLE", "STATUS", "MSGS", "RESTARTS", "AVG LATENCY"))
	sb.WriteString("  " + strings.Repeat("─", 84) + "\n")

	for _, a := range report.Actors {
		statusColor := "\033[32m"
		if a.Status == StatusTerminated {
			statusColor = "\033[31m"
		} else if a.Status == StatusRestarting {
			statusColor = "\033[33m"
		}

		sb.WriteString(fmt.Sprintf("  %-20s %-16s %s%-12s\033[0m %-10d %-10d %.1f µs\n",
			a.ActorID, a.Role, statusColor, a.Status, a.ProcessedCount, a.RestartsCount, a.AvgLatencyUs))
	}
	sb.WriteString("  " + strings.Repeat("─", 84) + "\n")

	if len(report.DeadLetters) > 0 {
		sb.WriteString("\n  \033[1;31mRECENT DEAD LETTERS (UNROUTABLE / DROPPED):\033[0m\n")
		for i, dl := range report.DeadLetters {
			if i >= 5 {
				sb.WriteString(fmt.Sprintf("  ... and %d more dead letters\n", len(report.DeadLetters)-5))
				break
			}
			sb.WriteString(fmt.Sprintf("  • [%s] From: %s -> To: %s | Reason: %s\n",
				dl.Timestamp.Format("15:04:05.000"), dl.Message.Sender, dl.Message.Recipient, dl.Reason))
		}
	}

	sb.WriteString("\n\033[1;35m========================================================================================\033[0m\n\n")
	return sb.String()
}

// RenderMermaidSupervisionTree outputs a Mermaid graph representation of the supervision mesh.
func RenderMermaidSupervisionTree(sys *ActorSystem) string {
	sys.mu.RLock()
	defer sys.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString("```mermaid\ngraph TD\n")
	sb.WriteString(fmt.Sprintf("    SYS[\"System: %s\"] --> ROOT[\"Root Supervisors\"]\n", sys.id))

	for supID, sup := range sys.supervisors {
		strat := sup.Strategy()
		sb.WriteString(fmt.Sprintf("    ROOT --> SUP_%s[\"Supervisor: %s<br/>(%s)\"]\n", supID, supID, strat))
		for _, child := range sup.Children() {
			telem := child.Telemetry()
			sb.WriteString(fmt.Sprintf("    SUP_%s --> ACT_%s[\"Actor: %s<br/>[%s] Restarts: %d\"]\n",
				supID, child.id, child.id, telem.Status, telem.RestartsCount))
		}
	}

	sb.WriteString("```\n")
	return sb.String()
}
