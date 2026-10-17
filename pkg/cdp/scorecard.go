package cdp

import (
	"fmt"
	"strings"
)

// RenderCDPScorecard renders a comprehensive ANSI terminal scorecard for a CDP flight recording.
func RenderCDPScorecard(report *CDPAuditReport) string {
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString("========================================================================================\n")
	b.WriteString("       🌐 [AgentVCR] CHROME DEVTOOLS PROTOCOL (CDP) FLIGHT RECORDER & DOM JOURNAL       \n")
	b.WriteString("========================================================================================\n\n")

	b.WriteString(fmt.Sprintf(" Session ID:          %s\n", report.SessionID))
	b.WriteString(fmt.Sprintf(" Target Page:         %s\n", report.TargetURL))
	b.WriteString(fmt.Sprintf(" Interception Time:   %.2f s | Captured CDP Frames: %d\n", float64(report.DurationMs)/1000.0, report.TotalFramesCaptured))
	if report.PageLoadTimeMs > 0 {
		b.WriteString(fmt.Sprintf(" Page Load Latency:   %d ms\n", report.PageLoadTimeMs))
	}

	b.WriteString("\n--- Network Waterfall & HTTP Traffic (W3C HAR 1.2 Telemetry) ---\n")
	b.WriteString(fmt.Sprintf(" • Total Requests:       %d (%d failed)\n", report.TotalNetworkRequests, report.FailedNetworkRequests))
	b.WriteString(fmt.Sprintf(" • Transferred Payload:  %.2f KB\n", float64(report.TotalBytesTransferred)/1024.0))
	b.WriteString(fmt.Sprintf(" • Latency Percentiles:  P50 TTFB: %.1f ms | P95 TTFB: %.1f ms\n", report.P50TTFBMs, report.P95TTFBMs))
	b.WriteString(fmt.Sprintf(" • Error Status Codes:   4xx Client Errors: %d | 5xx Server Errors: %d\n", report.HTTP4xxCount, report.HTTP5xxCount))

	if len(report.Transactions) > 0 {
		b.WriteString("\n Key HTTP Transactions:\n")
		limit := 5
		if len(report.Transactions) < limit {
			limit = len(report.Transactions)
		}
		for i := 0; i < limit; i++ {
			tx := report.Transactions[i]
			statusStr := fmt.Sprintf("%d", tx.StatusCode)
			if tx.StatusCode == 0 && tx.Failed {
				statusStr = "FAIL"
			}
			b.WriteString(fmt.Sprintf("   [%4s] %-4s %-45s (%.1f ms, %d B)\n",
				tx.Method,
				statusStr,
				truncateString(tx.URL, 45),
				tx.Timing.TotalDurationMs,
				tx.ResponseSize,
			))
		}
	}

	b.WriteString("\n--- Virtual DOM Reconstruction & Mutation Journal ---\n")
	b.WriteString(fmt.Sprintf(" • Reconstructed Nodes:  %d elements\n", report.DOMNodeCount))
	b.WriteString(fmt.Sprintf(" • Real-Time Mutations:  %d DOM transitions captured\n", report.DOMMutationCount))
	b.WriteString(fmt.Sprintf(" • Agent Interactions:   %d user input events (clicks/keys)\n", report.UserInteractionsCount))

	b.WriteString("\n--- JavaScript Runtime Diagnostics & Console Stream ---\n")
	if len(report.ConsoleMessages) == 0 {
		b.WriteString(" • Console Logs:         Clean runtime stream (0 warnings, 0 exceptions)\n")
	} else {
		b.WriteString(fmt.Sprintf(" • Uncaught Exceptions:  %d runtime errors\n", report.ConsoleErrorsCount))
		for _, log := range report.ConsoleMessages {
			loc := ""
			if log.URL != "" {
				loc = fmt.Sprintf(" (%s:%d)", truncateString(log.URL, 25), log.Line)
			}
			b.WriteString(fmt.Sprintf("   [%-5s] %s%s\n", strings.ToUpper(log.Level), truncateString(log.Text, 60), loc))
		}
	}

	b.WriteString("\n--- Browser Agent SLA Verdict ---\n")
	if report.SLAPassed {
		b.WriteString("  \033[32mPASS\033[0m  Zero uncaught exceptions, latency within SLA, clean HTTP response codes.\n\n")
	} else {
		b.WriteString("  \033[31mFAIL\033[0m  Browser agent SLA violations detected:\n")
		for _, v := range report.SLAViolations {
			b.WriteString(fmt.Sprintf("   - %s\n", v))
		}
		b.WriteString("\n")
	}

	return b.String()
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
