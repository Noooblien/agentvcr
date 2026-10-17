package graph

import (
	"encoding/json"
	"fmt"
	"strings"
)

// RenderGraphScorecard formats an execution trace into a rich ANSI terminal report.
func RenderGraphScorecard(trace *ExecutionTrace) string {
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString("========================================================================================\n")
	b.WriteString("       ⚡ [AgentVCR] STATEGRAPH WORKFLOW & DETERMINISTIC TIME-TRAVEL ENGINE             \n")
	b.WriteString("========================================================================================\n\n")

	b.WriteString(fmt.Sprintf(" Thread ID:           %s\n", trace.ThreadID))
	b.WriteString(fmt.Sprintf(" Lifecycle Status:    %s\n", formatStatus(trace.Status)))
	b.WriteString(fmt.Sprintf(" Steps Executed:      %d | Wall-clock Duration: %d ms | Loop Cycles: %d\n", trace.TotalSteps, trace.TotalDurationMs, trace.CycleCount))

	if trace.ActiveCheckpoint != nil {
		b.WriteString(fmt.Sprintf(" Active Checkpoint:   %s (Step %d)\n", trace.ActiveCheckpoint.CheckpointID, trace.ActiveCheckpoint.Step))
	}

	b.WriteString("\n--- Execution Path Traversal ---\n ")
	b.WriteString("START")
	for _, node := range trace.ExecutionPath {
		b.WriteString(fmt.Sprintf(" -> \033[1;36m[%s]\033[0m", node))
	}
	if trace.Status == StatusCompleted {
		b.WriteString(" -> END\n")
	} else if trace.Status == StatusInterrupted {
		b.WriteString(fmt.Sprintf(" -> \033[1;33m[PAUSED @ %s]\033[0m\n", trace.Interrupt.NodeName))
	} else {
		b.WriteString(" -> \033[1;31m[ABORTED]\033[0m\n")
	}

	b.WriteString("\n--- Final State Channel Snapshot ---\n")
	for k, v := range trace.FinalState {
		valBytes, _ := json.Marshal(v)
		valStr := string(valBytes)
		if len(valStr) > 65 {
			valStr = valStr[:62] + "..."
		}
		b.WriteString(fmt.Sprintf(" • %-22s: %s\n", k, valStr))
	}

	b.WriteString("\n--- Checkpoint Journal & Step Latency ---\n")
	for _, cp := range trace.Checkpoints {
		nextStr := "END"
		if len(cp.NextNodes) > 0 {
			nextStr = strings.Join(cp.NextNodes, ", ")
		}
		b.WriteString(fmt.Sprintf(" [Step %02d] %-18s -> Next: %-15s (%3d ms) | CP: %s\n",
			cp.Step,
			cp.NodeName,
			nextStr,
			cp.DurationMs,
			truncateString(cp.CheckpointID, 25),
		))
	}

	if trace.Interrupt != nil {
		b.WriteString("\n--- Human-in-the-Loop Interrupt Gate ---\n")
		b.WriteString(fmt.Sprintf(" ⚠️  Execution halted by '%s' gate on node '%s'\n", trace.Interrupt.Type, trace.Interrupt.NodeName))
		b.WriteString(fmt.Sprintf("    Interrupt ID:   %s\n", trace.Interrupt.InterruptID))
		b.WriteString("    Action:         Ready for human review or state injection via 'agentvcr graph -resume'\n")
	}

	b.WriteString("\n--- Graph Invariant & Execution Verdict ---\n")
	switch trace.Status {
	case StatusCompleted:
		b.WriteString("  \033[32mPASS\033[0m  StateGraph executed deterministically to completion with zero invariant violations.\n\n")
	case StatusInterrupted:
		b.WriteString("  \033[33mHALTED\033[0m  Execution cleanly suspended at configured Human-in-the-Loop interrupt boundary.\n\n")
	case StatusRecursionLimitExceeded:
		b.WriteString(fmt.Sprintf("  \033[31mFAIL\033[0m  %s (potential infinite loop cycle detected).\n\n", trace.Error))
	default:
		b.WriteString(fmt.Sprintf("  \033[31mFAIL\033[0m  Graph execution encountered error: %s\n\n", trace.Error))
	}

	return b.String()
}

func formatStatus(s GraphStatus) string {
	switch s {
	case StatusCompleted:
		return "\033[32mCOMPLETED\033[0m"
	case StatusInterrupted:
		return "\033[33mINTERRUPTED (Human-in-the-Loop)\033[0m"
	case StatusRecursionLimitExceeded:
		return "\033[31mRECURSION_LIMIT_EXCEEDED\033[0m"
	case StatusFailed:
		return "\033[31mFAILED\033[0m"
	default:
		return string(s)
	}
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
