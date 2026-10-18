package watermark

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

// RenderWatermarkScorecard formats an ANSI terminal scorecard of the watermark audit forensics.
func RenderWatermarkScorecard(report *WatermarkAuditReport) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("%s%s========================================================================================%s\n", ColorBold, ColorCyan, ColorReset))
	sb.WriteString(fmt.Sprintf("%s%s  AGENTVCR // STEGANOGRAPHIC PROVENANCE WATERMARK & FINGERPRINT AUDIT%s\n", ColorBold, ColorWhite, ColorReset))
	sb.WriteString(fmt.Sprintf("%s%s========================================================================================%s\n\n", ColorBold, ColorCyan, ColorReset))

	statusColor := ColorGreen
	if report.Status == "TAMPERED" {
		statusColor = ColorRed
	} else if report.Status == "NOT_DETECTED" {
		statusColor = ColorYellow
	}

	sb.WriteString(fmt.Sprintf("  %sSession ID:%s         %s\n", ColorBold, ColorReset, report.SessionID))
	sb.WriteString(fmt.Sprintf("  %sStrategy:%s           %s\n", ColorBold, ColorReset, report.Strategy))
	sb.WriteString(fmt.Sprintf("  %sAttestation Status:%s %s%s[%s]%s (Integrity Score: %.1f/10.0)\n",
		ColorBold, ColorReset, ColorBold, statusColor, report.Status, ColorReset, report.OverallIntegrityScore))
	sb.WriteString(fmt.Sprintf("  %sExtraction Telemetry:%s %d turns scanned, %d watermarks extracted (Tampered: %v)\n\n",
		ColorBold, ColorReset, report.TotalTurnsScanned, report.WatermarksExtracted, !report.AttestationValid))

	// Step-by-Step Forensics Table
	sb.WriteString(fmt.Sprintf("  %s%sWATERMARK EXTRACTION & CRYPTOGRAPHIC VERIFICATION LOG:%s\n", ColorBold, ColorWhite, ColorReset))
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("  %-6s %-12s %-12s %-10s %-16s %s\n", "TURN", "STRATEGY", "DETECTED", "CONF", "HMAC SIGNATURE", "STATUS"))
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────────\n")

	for i, res := range report.Results {
		detStr := fmt.Sprintf("%s[YES]%s", ColorGreen, ColorReset)
		if !res.Detected {
			detStr = fmt.Sprintf("%s[NO]%s", ColorYellow, ColorReset)
		}

		statStr := fmt.Sprintf("%s[VALID]%s", ColorGreen, ColorReset)
		sigStr := "none"
		if res.Payload != nil {
			sigStr = res.Payload.Signature
		}

		if res.Tampered {
			statStr = fmt.Sprintf("%s[SIGNATURE_INVALID]%s", ColorRed, ColorReset)
		} else if !res.Detected {
			statStr = fmt.Sprintf("%s[NOT_FOUND]%s", ColorYellow, ColorReset)
		}

		sb.WriteString(fmt.Sprintf("  %-6d %-12s %-12s %-10.2f %-16s %s\n",
			i+1, res.Strategy, detStr, res.BitConfidence, sigStr, statStr))
	}

	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("%s%s========================================================================================%s\n", ColorBold, ColorCyan, ColorReset))

	return sb.String()
}

// RenderWatermarkMermaid formats a Mermaid diagram depicting the watermarking lifecycle.
func RenderWatermarkMermaid(report *WatermarkAuditReport) string {
	var sb strings.Builder

	sb.WriteString("```mermaid\n")
	sb.WriteString("graph TD\n")
	sb.WriteString(fmt.Sprintf("    SESSION[\"Agent Session: %s\"] --> ENGINE[\"Watermark Engine\"]\n", report.SessionID))
	sb.WriteString("    ENGINE -->|HMAC-SHA256 Sign| EMBED[\"Steganographic Embedder\"]\n")
	sb.WriteString(fmt.Sprintf("    EMBED -->|%s| OUT[\"Watermarked Text Stream\"]\n", report.Strategy))
	sb.WriteString("    OUT --> SCAN[\"Audit Forensics Scanner\"]\n")

	statusColor := "#d4edda"
	statusNode := "ATTESTATION_PASS"
	if report.Status == "TAMPERED" {
		statusColor = "#f8d7da"
		statusNode = "ATTESTATION_FAIL"
	} else if report.Status == "NOT_DETECTED" {
		statusColor = "#fff3cd"
		statusNode = "ATTESTATION_MISSING"
	}

	sb.WriteString(fmt.Sprintf("    SCAN --> %s[\"Status: %s<br/>Score: %.1f/10.0\"]\n", statusNode, report.Status, report.OverallIntegrityScore))
	sb.WriteString(fmt.Sprintf("    style %s fill:%s,stroke:#333,stroke-width:2px\n", statusNode, statusColor))
	sb.WriteString("```\n")

	return sb.String()
}
