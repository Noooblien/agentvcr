package cdc

import (
	"fmt"
	"strings"
)

// RenderCDCScorecard formats a database CDC summary into an executive ANSI terminal scorecard.
func RenderCDCScorecard(summary *CDCSummary) string {
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString("========================================================================================\n")
	b.WriteString("       🗄️ [AgentVCR] DATABASE TRANSACTION FLIGHT RECORDER & CDC ROLLBACK JOURNAL        \n")
	b.WriteString("========================================================================================\n\n")

	b.WriteString(fmt.Sprintf(" Session ID:           %s\n", summary.SessionID))
	b.WriteString(fmt.Sprintf(" Transactions Audited: %d (Committed: %d, Aborted: %d)\n", summary.TotalTransactions, summary.CommittedCount, summary.AbortedCount))
	b.WriteString(fmt.Sprintf(" Row-Level Mutations:  %d rows affected across %d tables\n", summary.TotalRowsAffected, len(summary.TablesMutated)))

	if len(summary.TablesMutated) > 0 {
		b.WriteString(fmt.Sprintf(" Tables Modified:      %s\n", strings.Join(summary.TablesMutated, ", ")))
	}

	b.WriteString("\n--- Transaction Execution Journal ---\n")
	limit := 6
	if len(summary.Transactions) < limit {
		limit = len(summary.Transactions)
	}
	for i := 0; i < limit; i++ {
		tx := summary.Transactions[i]
		statusColor := "\033[32mCOMMITTED\033[0m"
		if tx.Status == TxAborted {
			statusColor = "\033[33mABORTED\033[0m"
		}
		alertBadge := ""
		if len(tx.SafetyViolations) > 0 {
			alertBadge = fmt.Sprintf(" \033[31m[⚠️ %d ALERTS]\033[0m", len(tx.SafetyViolations))
		}

		b.WriteString(fmt.Sprintf(" [Step %02d] %-14s (%-8s @ %-12s): %s (%d ms, %d rows)%s\n",
			tx.StepIndex,
			tx.TxID,
			tx.Engine,
			tx.DatabaseName,
			statusColor,
			tx.ExecutionLatencyMs,
			tx.RowsAffected,
			alertBadge,
		))

		if len(tx.RawStatements) > 0 {
			firstStmt := tx.RawStatements[0]
			b.WriteString(fmt.Sprintf("   SQL: %s\n", truncate(firstStmt, 75)))
		}
	}

	b.WriteString("\n--- Automated Inverse Compensating SQL Script (Preview) ---\n")
	scriptLines := strings.Split(strings.TrimSpace(summary.CompensatingScript), "\n")
	previewCount := 8
	if len(scriptLines) < previewCount {
		previewCount = len(scriptLines)
	}
	for i := 0; i < previewCount; i++ {
		b.WriteString(fmt.Sprintf("   %s\n", scriptLines[i]))
	}
	if len(scriptLines) > previewCount {
		b.WriteString(fmt.Sprintf("   ... [%d more inverse SQL statements generated]\n", len(scriptLines)-previewCount))
	}

	if len(summary.SafetyViolations) > 0 {
		b.WriteString("\n--- Database Safety & Schema Invariant Alerts ---\n")
		for _, v := range summary.SafetyViolations {
			b.WriteString(fmt.Sprintf(" ⚠️  %s\n", v))
		}
	}

	b.WriteString("\n--- Database Flight Recorder Verdict ---\n")
	if summary.PassedAudit {
		b.WriteString("  \033[32mPASS\033[0m  All transactions safely journaled. 100% deterministic compensating rollback verified.\n\n")
	} else {
		b.WriteString(fmt.Sprintf("  \033[31mFAIL\033[0m  %d safety invariant violations detected (destructive or unbounded SQL statements).\n\n", summary.DestructiveQueriesCount))
	}

	return b.String()
}
