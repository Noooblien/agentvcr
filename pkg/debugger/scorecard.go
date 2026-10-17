package debugger

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ANSI color escape sequences
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

// RenderDebuggerScorecard produces an interactive ANSI terminal dashboard.
func RenderDebuggerScorecard(summary DebugSessionSummary) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("%s%s========================================================================================%s\n", ColorBold, ColorCyan, ColorReset))
	sb.WriteString(fmt.Sprintf("%s%s  AGENTVCR // INTERACTIVE TIME-TRAVEL AGENT DEBUGGER (DAP)%s\n", ColorBold, ColorWhite, ColorReset))
	sb.WriteString(fmt.Sprintf("%s%s========================================================================================%s\n\n", ColorBold, ColorCyan, ColorReset))

	// Status line
	statusColor := ColorYellow
	switch summary.State {
	case StateRunning:
		statusColor = ColorGreen
	case StatePaused:
		statusColor = ColorYellow
	case StateTerminated:
		statusColor = ColorBlue
	case StateFaulted:
		statusColor = ColorRed
	}

	sb.WriteString(fmt.Sprintf("  %sSession ID:%s       %s\n", ColorBold, ColorReset, summary.SessionID))
	sb.WriteString(fmt.Sprintf("  %sAgent Name:%s       %s\n", ColorBold, ColorReset, summary.AgentName))
	sb.WriteString(fmt.Sprintf("  %sExecution State:%s  %s%s [%s]%s\n", ColorBold, ColorReset, ColorBold, statusColor, summary.State, ColorReset))
	sb.WriteString(fmt.Sprintf("  %sProgram Counter:%s  Step %s%d%s of %d (%.1f%% completed)\n",
		ColorBold, ColorReset, ColorCyan, summary.PC, ColorReset, summary.TotalSteps,
		calcPct(summary.PC, summary.TotalSteps)))
	sb.WriteString(fmt.Sprintf("  %sLast Stop Reason:%s %s%s%s\n", ColorBold, ColorReset, ColorMagenta, summary.LastStopReason, ColorReset))
	sb.WriteString(fmt.Sprintf("  %sTotal Tokens:%s     %s%d%s\n\n", ColorBold, ColorReset, ColorGreen, summary.CumulativeTokens, ColorReset))

	// Current Event / Action
	sb.WriteString(fmt.Sprintf("  %s%sCURRENT EVENT AT STEP %d:%s\n", ColorBold, ColorWhite, summary.PC, ColorReset))
	if summary.CurrentEvent != nil {
		sb.WriteString(fmt.Sprintf("  • Type:      %s%s%s\n", ColorYellow, summary.CurrentEvent.Type, ColorReset))
		sb.WriteString(fmt.Sprintf("  • Tool/Action:%s\n", extractToolName(summary.CurrentEvent)))
		payloadPreview, _ := json.Marshal(summary.CurrentEvent.Payload)
		if len(payloadPreview) > 80 {
			payloadPreview = append(payloadPreview[:77], []byte("...")...)
		}
		sb.WriteString(fmt.Sprintf("  • Payload:   %s%s%s\n\n", ColorDim, string(payloadPreview), ColorReset))
	} else {
		sb.WriteString("  • (No active event; end of trace or uninitialized)\n\n")
	}

	// Active Call Stack
	sb.WriteString(fmt.Sprintf("  %s%sAGENT SWARM CALL STACK:%s\n", ColorBold, ColorWhite, ColorReset))
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("  %-6s %-18s %-20s %-12s %s\n", "LEVEL", "AGENT ID", "ROLE", "STEP", "CURRENT ACTION"))
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────\n")
	for i := len(summary.CallStack) - 1; i >= 0; i-- {
		frame := summary.CallStack[i]
		prefix := "  "
		if i == len(summary.CallStack)-1 {
			prefix = fmt.Sprintf("%s▶ %s", ColorCyan, ColorReset)
		}
		sb.WriteString(fmt.Sprintf("%s%-6d %-18s %-20s %-12d %s\n",
			prefix, frame.Level, frame.AgentID, frame.AgentRole, frame.StepIndex, frame.CurrentAction))
	}
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────\n\n")

	// Working Memory / State Variables
	sb.WriteString(fmt.Sprintf("  %s%sWORKING MEMORY VARIABLES (%d keys):%s\n", ColorBold, ColorWhite, len(summary.WorkingMemory), ColorReset))
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────\n")
	if len(summary.WorkingMemory) == 0 {
		sb.WriteString("  (working memory is empty)\n")
	} else {
		for k, v := range summary.WorkingMemory {
			valStr := fmt.Sprintf("%v", v)
			if len(valStr) > 50 {
				valStr = valStr[:47] + "..."
			}
			sb.WriteString(fmt.Sprintf("  • %-20s = %s%s%s\n", k, ColorGreen, valStr, ColorReset))
		}
	}
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────\n\n")

	// Active Breakpoints
	sb.WriteString(fmt.Sprintf("  %s%sBREAKPOINTS (%d registered):%s\n", ColorBold, ColorWhite, len(summary.ActiveBreakpoints), ColorReset))
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("  %-10s %-16s %-24s %-10s %s\n", "ID", "TYPE", "TARGET / CONDITION", "HITS", "STATUS"))
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────\n")
	if len(summary.ActiveBreakpoints) == 0 {
		sb.WriteString("  (no breakpoints registered)\n")
	} else {
		for _, bp := range summary.ActiveBreakpoints {
			cond := bp.Target
			if bp.Type == BreakCondition {
				cond = bp.ConditionExpr
			} else if bp.Type == BreakStep {
				cond = fmt.Sprintf("Step %d", bp.StepNumber)
			} else if bp.Type == BreakTokenThreshold {
				cond = fmt.Sprintf(">= %d tokens", bp.TokenLimit)
			}
			status := fmt.Sprintf("%sENABLED%s", ColorGreen, ColorReset)
			if !bp.Enabled {
				status = fmt.Sprintf("%sDISABLED%s", ColorDim, ColorReset)
			}
			sb.WriteString(fmt.Sprintf("  %-10s %-16s %-24s %-10d %s\n", bp.ID, bp.Type, cond, bp.HitCount, status))
		}
	}
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────\n\n")

	// Active Watchpoints
	sb.WriteString(fmt.Sprintf("  %s%sINVARIANT WATCHPOINTS (%d registered):%s\n", ColorBold, ColorWhite, len(summary.ActiveWatchpoints), ColorReset))
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("  %-10s %-18s %-18s %-16s %s\n", "ID", "VARIABLE", "PREDICATE", "CURRENT VALUE", "STATUS"))
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────\n")
	if len(summary.ActiveWatchpoints) == 0 {
		sb.WriteString("  (no watchpoints registered)\n")
	} else {
		for _, wp := range summary.ActiveWatchpoints {
			predicate := fmt.Sprintf("%s %v", wp.Operator, wp.ExpectedValue)
			status := fmt.Sprintf("%s[PASS]%s", ColorGreen, ColorReset)
			if wp.Violated {
				status = fmt.Sprintf("%s[VIOLATED (%d)]%s", ColorRed, wp.ViolationCount, ColorReset)
			}
			sb.WriteString(fmt.Sprintf("  %-10s %-18s %-18s %-16v %s\n",
				wp.ID, wp.VariableKey, predicate, wp.CurrentValue, status))
		}
	}
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────\n\n")

	// Control legend
	sb.WriteString(fmt.Sprintf("  %sDEBUG COMMANDS:%s [c] Continue  [n] StepOver  [s] StepInto  [b] StepBack (Time-Travel)  [m] Mutate  [r] Restart\n", ColorBold, ColorReset))
	sb.WriteString(fmt.Sprintf("%s%s========================================================================================%s\n", ColorBold, ColorCyan, ColorReset))

	return sb.String()
}

// RenderDebuggerMermaid generates a Mermaid flowchart of the execution trajectory and debugger state.
func RenderDebuggerMermaid(summary DebugSessionSummary) string {
	var sb strings.Builder

	sb.WriteString("```mermaid\n")
	sb.WriteString("graph TD\n")
	sb.WriteString("    START([\"Session Start: " + summary.SessionID + "\"]) --> STEP_0[\"Step 0: Initialize\"]\n")

	prev := "STEP_0"
	for i := 1; i < summary.TotalSteps; i++ {
		nodeID := fmt.Sprintf("STEP_%d", i)
		nodeLabel := fmt.Sprintf("Step %d", i)

		if i == summary.PC {
			nodeLabel = fmt.Sprintf("Step %d [ACTIVE PC]<br/>%s", i, summary.LastStopReason)
		}

		sb.WriteString(fmt.Sprintf("    %s --> %s[\"%s\"]\n", prev, nodeID, nodeLabel))
		prev = nodeID
	}

	sb.WriteString(fmt.Sprintf("    %s --> END([\"End of Trajectory\"])\n\n", prev))

	// Annotate active PC and Breakpoints
	if summary.PC < summary.TotalSteps {
		sb.WriteString(fmt.Sprintf("    style STEP_%d fill:#ffd700,stroke:#333,stroke-width:3px\n", summary.PC))
	}
	for _, bp := range summary.ActiveBreakpoints {
		if bp.Type == BreakStep && bp.StepNumber < summary.TotalSteps {
			sb.WriteString(fmt.Sprintf("    style STEP_%d fill:#ff6b6b,stroke:#b22222,stroke-width:2px\n", bp.StepNumber))
		}
	}

	if summary.CanStepBack {
		sb.WriteString(fmt.Sprintf("    STEP_%d -.->|\"Time-Travel Rewind\"| STEP_%d\n", summary.PC, summary.PC-1))
	}

	sb.WriteString("```\n")
	return sb.String()
}

func calcPct(cur, total int) float64 {
	if total <= 0 {
		return 0.0
	}
	return (float64(cur) / float64(total)) * 100.0
}
