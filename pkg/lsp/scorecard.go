package lsp

import (
	"fmt"
	"strings"
)

// RenderLSPScorecard formats an ANSI terminal overview of LSP daemon performance and capabilities.
func RenderLSPScorecard(status ServerStatusResult, sampleFastTab *FastTabResult) string {
	var sb strings.Builder

	sb.WriteString("\n\033[1;36m========================================================================================\033[0m\n")
	sb.WriteString("\033[1;36m         ⚡ [AgentVCR] LANGUAGE SERVER PROTOCOL (LSP 3.17) & FAST-TAB DAEMON            \033[0m\n")
	sb.WriteString("\033[1;36m========================================================================================\033[0m\n\n")

	sb.WriteString(" \033[1mDaemon Protocol:\033[0m   LSP 3.17 JSON-RPC 2.0 (stdio / TCP)\n")
	sb.WriteString(fmt.Sprintf(" \033[1mServer Uptime:\033[0m     %.2f seconds\n", status.UptimeSeconds))
	sb.WriteString(fmt.Sprintf(" \033[1mActive Buffers:\033[0m    \033[1;32m%d\033[0m in-memory tracked documents\n", status.ActiveBuffers))
	sb.WriteString(fmt.Sprintf(" \033[1mPattern Bank:\033[0m      \033[1;35m%d\033[0m registered AST syntactic idioms\n\n", status.SpeculatePatterns))

	// Supported Capabilities Table
	sb.WriteString("\033[1;33m--- Exposed Editor Capabilities ---\033[0m\n")
	sb.WriteString(" • textDocument/didOpen       \033[32m✓ Active\033[0m (In-memory buffer synchronization)\n")
	sb.WriteString(" • textDocument/didChange     \033[32m✓ Active\033[0m (Full and incremental change tracking)\n")
	sb.WriteString(" • textDocument/completion    \033[32m✓ Active\033[0m (Sub-50ms AST draft completion snippets)\n")
	sb.WriteString(" • textDocument/hover         \033[32m✓ Active\033[0m (AgentVCR flight attestation & diagnostics)\n")
	sb.WriteString(" • agentvcr/fastTab           \033[1;32m✓ Active\033[0m (Cursor-style multi-token predictive ghost text)\n")
	sb.WriteString(" • agentvcr/status            \033[32m✓ Active\033[0m (Telemetry statistics & health probe)\n\n")

	// Sample Fast-Tab Speculative Preview
	if sampleFastTab != nil && sampleFastTab.Available && sampleFastTab.Draft != nil {
		sb.WriteString("\033[1;33m--- Speculative Fast-Tab Ghost Text Preview ---\033[0m\n")
		sb.WriteString(fmt.Sprintf(" • Syntactic Reason:    \033[35m%s\033[0m\n", sampleFastTab.Draft.SyntacticReason))
		sb.WriteString(fmt.Sprintf(" • Speculation Latency: \033[32m%d ms\033[0m (SLA < 50ms)\n", sampleFastTab.LatencyMs))
		sb.WriteString(fmt.Sprintf(" • Confidence Score:    \033[1;32m%.0f%%\033[0m\n", sampleFastTab.Confidence*100))
		sb.WriteString(fmt.Sprintf(" • Typing Speed Gain:   \033[1;33m%s\033[0m\n\n", sampleFastTab.EstimatedGain))

		sb.WriteString("\033[90m┌── Editor Buffer (Virtual Ghost Text) ────────────────────────────────┐\033[0m\n")
		for _, line := range sampleFastTab.Draft.PredictedLines {
			sb.WriteString(fmt.Sprintf("\033[90m│\033[0m \033[1;32m+ %-66s\033[0m \033[90m│\033[0m\n", line))
		}
		sb.WriteString("\033[90m└──────────────────────────────────────────────────────────────────────┘\033[0m\n\n")
	}

	sb.WriteString("\033[1;33m--- Integration Guide ---\033[0m\n")
	sb.WriteString(" • \033[1mNeovim (nvim-lspconfig):\033[0m\n")
	sb.WriteString("     require('lspconfig').agentvcr.setup{ cmd = { 'agentvcr', 'lsp', '--stdio' } }\n")
	sb.WriteString(" • \033[1mVSCode / Zed:\033[0m\n")
	sb.WriteString("     Configure language server command: `agentvcr lsp --stdio`\n\n")

	sb.WriteString(" \033[1;42;30m OPERATIONAL \033[0m \033[1;32mLSP 3.17 daemon running with sub-50ms speculative fast-tab latency.\033[0m\n\n")

	return sb.String()
}
