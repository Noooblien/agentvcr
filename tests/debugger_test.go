package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/debugger"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func createSampleDebugSession() *trace.Session {
	now := time.Now()
	return &trace.Session{
		ID:        "sess-test-debug",
		AgentName: "orchestrator-agent",
		CreatedAt: now.Add(-10 * time.Minute),
		Metadata: map[string]string{
			"task":     "test-distributed-cluster",
			"priority": "high",
			"retries":  "0",
		},
		Events: []trace.TraceEvent{
			{
				ID:        "ev-0",
				Index:     0,
				Timestamp: now.Add(-9 * time.Minute),
				Type:      trace.EventLLMRequest,
				Payload: map[string]interface{}{
					"prompt": "Evaluate system health and perform disk cleanup.",
					"tokens": 250,
				},
			},
			{
				ID:        "ev-1",
				Index:     1,
				Timestamp: now.Add(-8 * time.Minute),
				Type:      trace.EventToolCall,
				Payload: map[string]interface{}{
					"tool":    "disk_usage",
					"command": "df -h /var/log",
					"tokens":  50,
				},
			},
			{
				ID:        "ev-2",
				Index:     2,
				Timestamp: now.Add(-7 * time.Minute),
				Type:      trace.EventToolResult,
				Payload: map[string]interface{}{
					"used_percent": 92.5,
					"state_mutation": map[string]interface{}{
						"disk_full": true,
						"retries":   1,
					},
				},
			},
			{
				ID:        "ev-3",
				Index:     3,
				Timestamp: now.Add(-6 * time.Minute),
				Type:      trace.EventToolCall,
				Payload: map[string]interface{}{
					"tool":    "bash",
					"command": "rm -rf /var/log/stale.log",
					"tokens":  120,
				},
			},
			{
				ID:        "ev-4",
				Index:     4,
				Timestamp: now.Add(-5 * time.Minute),
				Type:      trace.EventToolResult,
				Payload: map[string]interface{}{
					"exit_code": 0,
					"state_mutation": map[string]interface{}{
						"disk_full": false,
						"retries":   2,
					},
				},
			},
			{
				ID:        "ev-5",
				Index:     5,
				Timestamp: now.Add(-4 * time.Minute),
				Type:      trace.EventLLMResponse,
				Payload: map[string]interface{}{
					"content": "Disk cleanup complete. Reclaimed 14GB.",
					"tokens":  180,
				},
			},
		},
	}
}

func TestDebugger_Breakpoint_StepAndTool(t *testing.T) {
	session := createSampleDebugSession()
	engine := debugger.NewDebuggerEngine(session)

	// Add step breakpoint at Step 1
	bp1ID := engine.AddBreakpoint(debugger.Breakpoint{
		Type:       debugger.BreakStep,
		StepNumber: 1,
	})

	// Add tool breakpoint on "bash"
	bpBashID := engine.AddBreakpoint(debugger.Breakpoint{
		Type:   debugger.BreakToolCall,
		Target: "bash",
	})

	// Run Continue: should hit Step 1 (ev-1)
	evt1, err := engine.Continue()
	if err != nil {
		t.Fatalf("Continue failed: %v", err)
	}
	if evt1.StopReason != debugger.ReasonBreakpointHit {
		t.Fatalf("Expected ReasonBreakpointHit, got: %s", evt1.StopReason)
	}
	if evt1.BreakpointID != bp1ID {
		t.Fatalf("Expected breakpoint %s, got: %s", bp1ID, evt1.BreakpointID)
	}
	if evt1.StepIndex != 1 {
		t.Fatalf("Expected hit at step 1, got: %d", evt1.StepIndex)
	}

	// Continue again: should hit "bash" tool call at Step 3 (ev-3)
	evt2, err := engine.Continue()
	if err != nil {
		t.Fatalf("Continue to bash failed: %v", err)
	}
	if evt2.StopReason != debugger.ReasonBreakpointHit {
		t.Fatalf("Expected ReasonBreakpointHit on bash, got: %s", evt2.StopReason)
	}
	if evt2.BreakpointID != bpBashID {
		t.Fatalf("Expected breakpoint %s, got: %s", bpBashID, evt2.BreakpointID)
	}
	if evt2.StepIndex != 3 {
		t.Fatalf("Expected hit at step 3, got: %d", evt2.StepIndex)
	}

	// Test disabling breakpoint
	enabled, err := engine.ToggleBreakpoint(bpBashID)
	if err != nil || enabled {
		t.Fatalf("Expected breakpoint to be disabled, got enabled=%v, err=%v", enabled, err)
	}

	// Test removing breakpoint
	removed := engine.RemoveBreakpoint(bp1ID)
	if !removed {
		t.Fatalf("Expected breakpoint removal to succeed")
	}
}

func TestDebugger_ConditionalBreakpoint_ExpressionAndRegex(t *testing.T) {
	session := createSampleDebugSession()
	engine := debugger.NewDebuggerEngine(session)

	// Add complex expression breakpoint: tool == 'bash' && arg contains 'rm'
	engine.AddBreakpoint(debugger.Breakpoint{
		ID:            "bp-cond-rm",
		Type:          debugger.BreakCondition,
		ConditionExpr: "tool == 'bash' && arg contains 'rm'",
	})

	evt, err := engine.Continue()
	if err != nil {
		t.Fatalf("Continue failed: %v", err)
	}
	if evt.StopReason != debugger.ReasonBreakpointHit || evt.BreakpointID != "bp-cond-rm" {
		t.Fatalf("Expected conditional breakpoint bp-cond-rm to hit, got: %v (reason: %s)", evt.BreakpointID, evt.StopReason)
	}
	if evt.StepIndex != 3 {
		t.Fatalf("Expected step 3, got %d", evt.StepIndex)
	}

	// Test Token Threshold Breakpoint
	session2 := createSampleDebugSession()
	engine2 := debugger.NewDebuggerEngine(session2)

	engine2.AddBreakpoint(debugger.Breakpoint{
		ID:         "bp-tokens",
		Type:       debugger.BreakTokenThreshold,
		TokenLimit: 350,
	})

	evtTokens, err := engine2.Continue()
	if err != nil {
		t.Fatalf("Token continue failed: %v", err)
	}
	if evtTokens.StopReason != debugger.ReasonBreakpointHit || evtTokens.BreakpointID != "bp-tokens" {
		t.Fatalf("Expected bp-tokens to trigger, got %s / %s", evtTokens.BreakpointID, evtTokens.StopReason)
	}
}

func TestDebugger_InvariantWatchpoint_Violation(t *testing.T) {
	session := createSampleDebugSession()
	engine := debugger.NewDebuggerEngine(session)

	// Watchpoint: retries must not exceed 1 (fails at step 4 when retries becomes 2)
	wpID := engine.AddWatchpoint(debugger.Watchpoint{
		ID:            "wp-retry-limit",
		VariableKey:   "retries",
		Operator:      ">",
		ExpectedValue: 1, // Triggers violation when retries > 1
	})

	evt, err := engine.Continue()
	if err != nil {
		t.Fatalf("Continue failed: %v", err)
	}
	if evt.StopReason != debugger.ReasonWatchpointViolated {
		t.Fatalf("Expected ReasonWatchpointViolated, got: %s", evt.StopReason)
	}
	if evt.WatchpointID != wpID {
		t.Fatalf("Expected watchpoint %s to trigger, got: %s", wpID, evt.WatchpointID)
	}

	// Verify watchpoint state
	watchpoints := engine.GetWatchpoints()
	if len(watchpoints) != 1 || !watchpoints[0].Violated {
		t.Fatalf("Expected watchpoint to be marked violated, got: %+v", watchpoints)
	}
}

func TestDebugger_TimeTravel_StepBackAndMutate(t *testing.T) {
	session := createSampleDebugSession()
	engine := debugger.NewDebuggerEngine(session)

	// Step forward 3 times
	for i := 0; i < 3; i++ {
		_, err := engine.StepInto()
		if err != nil {
			t.Fatalf("StepInto failed at index %d: %v", i, err)
		}
	}

	summaryBefore := engine.Summary()
	if summaryBefore.PC != 3 {
		t.Fatalf("Expected PC=3, got: %d", summaryBefore.PC)
	}
	if summaryBefore.WorkingMemory["disk_full"] != true {
		t.Fatalf("Expected disk_full=true before rewind, got: %v", summaryBefore.WorkingMemory["disk_full"])
	}

	// Time-travel step back to Step 2
	backEvt, err := engine.StepBack()
	if err != nil {
		t.Fatalf("StepBack failed: %v", err)
	}
	if backEvt.StopReason != debugger.ReasonStepBackCompleted {
		t.Fatalf("Expected ReasonStepBackCompleted, got: %s", backEvt.StopReason)
	}

	summaryAfter := engine.Summary()
	if summaryAfter.PC != 2 {
		t.Fatalf("Expected PC=2 after StepBack, got: %d", summaryAfter.PC)
	}

	// Hot-patch working memory
	engine.MutateMemory("injected_override", "SAFE_MODE_ENABLED")
	summaryMutated := engine.Summary()
	if summaryMutated.WorkingMemory["injected_override"] != "SAFE_MODE_ENABLED" {
		t.Fatalf("Expected mutated memory to contain injected_override")
	}

	// Restart debugger
	errRestart := engine.Restart()
	if errRestart != nil {
		t.Fatalf("Restart failed: %v", errRestart)
	}
	summaryRestarted := engine.Summary()
	if summaryRestarted.PC != 0 {
		t.Fatalf("Expected PC=0 after restart, got: %d", summaryRestarted.PC)
	}
}

func TestDebugger_StepOver_ToolCall(t *testing.T) {
	session := createSampleDebugSession()
	engine := debugger.NewDebuggerEngine(session)

	// StepInto event 0 (LLM request)
	_, _ = engine.StepInto()

	// Currently at Step 1 (tool_call: disk_usage)
	summaryAtTool := engine.Summary()
	if summaryAtTool.PC != 1 {
		t.Fatalf("Expected PC=1 at tool call, got %d", summaryAtTool.PC)
	}

	// StepOver: should advance across tool_call + tool_result to step 3
	evt, err := engine.StepOver()
	if err != nil {
		t.Fatalf("StepOver failed: %v", err)
	}
	if evt == nil {
		t.Fatalf("Expected non-nil event from StepOver")
	}

	summaryAfter := engine.Summary()
	if summaryAfter.PC < 2 {
		t.Fatalf("Expected StepOver to advance past tool call, got PC=%d", summaryAfter.PC)
	}
}

func TestDebugger_DAP_ProtocolFraming(t *testing.T) {
	adapter := debugger.NewDAPAdapter()

	rawReq := []byte(`{"seq": 1, "type": "request", "command": "setBreakpoints", "arguments": {"source": {"name": "agent.go"}}}`)
	req, err := debugger.ParseDAPRequest(rawReq)
	if err != nil {
		t.Fatalf("Failed to parse DAP request: %v", err)
	}
	if req.Command != "setBreakpoints" || req.Seq != 1 {
		t.Fatalf("Unexpected parsed request: %+v", req)
	}

	// Create response
	resp := adapter.CreateResponse(req, map[string]interface{}{"status": "ok"})
	if !resp.Success || resp.RequestSeq != 1 || resp.Type != debugger.DAPTypeResponse {
		t.Fatalf("Invalid response: %+v", resp)
	}

	// Create stopped event
	stopEvt := adapter.CreateStoppedEvent(debugger.ReasonBreakpointHit, "Hit breakpoint bp-1", 4)
	if stopEvt.Event != "stopped" || stopEvt.Type != debugger.DAPTypeEvent {
		t.Fatalf("Invalid stopped event: %+v", stopEvt)
	}
}

func TestDebugger_ScorecardAndMermaid(t *testing.T) {
	session := createSampleDebugSession()
	engine := debugger.NewDebuggerEngine(session)

	engine.AddBreakpoint(debugger.Breakpoint{
		ID:         "bp-1",
		Type:       debugger.BreakStep,
		StepNumber: 2,
	})
	engine.AddWatchpoint(debugger.Watchpoint{
		ID:            "wp-1",
		VariableKey:   "retries",
		Operator:      "<=",
		ExpectedValue: 3,
	})

	_, _ = engine.Continue()

	summary := engine.Summary()
	scorecard := debugger.RenderDebuggerScorecard(summary)
	if !strings.Contains(scorecard, "AGENTVCR // INTERACTIVE TIME-TRAVEL AGENT DEBUGGER") {
		t.Fatalf("Scorecard missing header title")
	}
	if !strings.Contains(scorecard, "BREAKPOINTS") || !strings.Contains(scorecard, "bp-1") {
		t.Fatalf("Scorecard missing breakpoint information")
	}

	mermaid := debugger.RenderDebuggerMermaid(summary)
	if !strings.Contains(mermaid, "graph TD") || !strings.Contains(mermaid, "Step 2") {
		t.Fatalf("Mermaid diagram missing graph definition or step nodes: %s", mermaid)
	}
}
