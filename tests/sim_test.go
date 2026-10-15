package tests

import (
	"context"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/sim"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func TestSynthesizeFromSession(t *testing.T) {
	now := time.Now()
	session := &trace.Session{
		ID:        "session-test-synthesize",
		AgentName: "code-agent",
		CreatedAt: now,
		Events: []trace.TraceEvent{
			{
				Index:      1,
				Timestamp:  now,
				Type:       trace.EventToolCall,
				DurationMs: 50,
				Payload: map[string]interface{}{
					"tool_name": "fetch_user",
					"params": map[string]interface{}{
						"user_id": "usr_1234",
					},
				},
			},
			{
				Index:      2,
				Timestamp:  now.Add(50 * time.Millisecond),
				Type:       trace.EventToolResult,
				DurationMs: 45,
				Payload: map[string]interface{}{
					"result": map[string]interface{}{
						"name":  "Alice",
						"role":  "engineer",
						"email": "alice@agentvcr.dev",
					},
				},
			},
		},
	}

	synth := sim.NewSynthesizer()
	rules, err := synth.SynthesizeFromSession(session)
	if err != nil {
		t.Fatalf("SynthesizeFromSession failed: %v", err)
	}

	if len(rules) != 1 {
		t.Fatalf("expected 1 synthesized rule, got %d", len(rules))
	}

	r := rules[0]
	if r.ToolName != "fetch_user" {
		t.Errorf("expected tool name 'fetch_user', got %s", r.ToolName)
	}
	if r.ArgMatch["user_id"] != "usr_1234" {
		t.Errorf("expected argMatch user_id 'usr_1234', got %v", r.ArgMatch["user_id"])
	}

	// Test JSON export & import
	jsonBytes, err := synth.ExportRulesJSON(rules)
	if err != nil {
		t.Fatalf("ExportRulesJSON failed: %v", err)
	}

	imported, err := synth.ImportRulesJSON(jsonBytes)
	if err != nil {
		t.Fatalf("ImportRulesJSON failed: %v", err)
	}
	if len(imported) != len(rules) {
		t.Fatalf("imported rule count mismatch: %d != %d", len(imported), len(rules))
	}
}

func TestSimulator_RuleExecutionAndStatefulOps(t *testing.T) {
	rules := []sim.MockRule{
		{
			ID:       "mock-weather-sf",
			ToolName: "get_weather",
			ArgMatch: map[string]interface{}{
				"city": "San Francisco",
			},
			Response: map[string]interface{}{
				"temp":      68,
				"condition": "Sunny",
			},
			Status:    "success",
			LatencyMs: 25,
		},
		{
			ID:       "mock-stateful-save",
			ToolName: "save_preference",
			StatefulOp: "set",
			StateKey:   "theme",
			Response:   "dark_mode",
			Status:     "success",
			LatencyMs:  15,
		},
		{
			ID:           "mock-faulty-api",
			ToolName:     "payment_gateway",
			Status:       "error",
			ErrorMessage: "service unavailable (503)",
			LatencyMs:    40,
		},
	}

	simulator := sim.NewSimulator(rules)
	ctx := context.Background()

	// 1. Test argument matching rule
	res, err := simulator.ExecuteTool(ctx, "get_weather", map[string]interface{}{"city": "San Francisco"})
	if err != nil {
		t.Fatalf("ExecuteTool error: %v", err)
	}
	if res.Status != "success" || res.RuleID != "mock-weather-sf" {
		t.Errorf("unexpected result: %+v", res)
	}

	// 2. Test stateful rule
	_, err = simulator.ExecuteTool(ctx, "save_preference", map[string]interface{}{"value": "custom_neon"})
	if err != nil {
		t.Fatalf("stateful set failed: %v", err)
	}
	val, ok := simulator.GetState().Get("theme")
	if !ok || val != "custom_neon" {
		t.Errorf("expected state 'custom_neon', got %v", val)
	}

	// 3. Test error injection rule
	resErr, err := simulator.ExecuteTool(ctx, "payment_gateway", map[string]interface{}{"amount": 100})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resErr.Status != "error" {
		t.Errorf("expected status 'error', got %s", resErr.Status)
	}
}

func TestSimulator_BuiltinProviders(t *testing.T) {
	simulator := sim.NewSimulator(nil)
	ctx := context.Background()

	// 1. Built-in bash_sandbox
	resBash, err := simulator.ExecuteTool(ctx, "bash_sandbox", map[string]interface{}{"cmd": "echo test_payload"})
	if err != nil {
		t.Fatalf("bash execution error: %v", err)
	}
	payload, ok := resBash.Response.(map[string]interface{})
	if !ok || payload["stdout"] != "test_payload" {
		t.Errorf("expected stdout 'test_payload', got %v", payload["stdout"])
	}

	// 2. Built-in memory_kv
	_, _ = simulator.ExecuteTool(ctx, "memory_kv", map[string]interface{}{
		"action": "set",
		"key":    "session_token",
		"value":  "xyz_999",
	})
	resGet, _ := simulator.ExecuteTool(ctx, "memory_kv", map[string]interface{}{
		"action": "get",
		"key":    "session_token",
	})
	getPayload := resGet.Response.(map[string]interface{})
	if getPayload["value"] != "xyz_999" {
		t.Errorf("expected value 'xyz_999', got %v", getPayload["value"])
	}

	// 3. Built-in mock_search
	resSearch, err := simulator.ExecuteTool(ctx, "mock_search", map[string]interface{}{"query": "AgentVCR offline replay"})
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if resSearch.Status != "success" {
		t.Errorf("expected search success, got %s", resSearch.Status)
	}
}

func TestSimulator_SimulateSession(t *testing.T) {
	now := time.Now()
	baseSession := &trace.Session{
		ID:        "base-trajectory-1",
		AgentName: "dev-agent",
		CreatedAt: now,
		Events: []trace.TraceEvent{
			{
				Index:      1,
				Timestamp:  now,
				Type:       trace.EventLLMRequest,
				DurationMs: 100,
				Payload: map[string]interface{}{
					"prompt": "Inspect the repository and run whoami",
				},
				Checksum: "chk1",
			},
			{
				Index:      2,
				Timestamp:  now.Add(100 * time.Millisecond),
				Type:       trace.EventToolCall,
				DurationMs: 50,
				Payload: map[string]interface{}{
					"tool_name": "bash_sandbox",
					"params":    map[string]interface{}{"command": "whoami"},
				},
				Checksum: "chk2",
			},
			{
				Index:      3,
				Timestamp:  now.Add(150 * time.Millisecond),
				Type:       trace.EventToolResult,
				DurationMs: 50,
				Payload: map[string]interface{}{
					"output": "root", // Live result to be superseded by offline mock
				},
				Checksum: "chk3",
			},
		},
	}

	simulator := sim.NewSimulator(nil)
	simSession, report, err := simulator.SimulateSession(baseSession, "sim-trajectory-1")
	if err != nil {
		t.Fatalf("SimulateSession failed: %v", err)
	}

	if report.ToolCallsSimulated != 1 {
		t.Errorf("expected 1 simulated tool call, got %d", report.ToolCallsSimulated)
	}
	if len(simSession.Events) != 3 {
		t.Fatalf("expected 3 events in simulated session, got %d", len(simSession.Events))
	}

	// Verify the synthesized result
	lastEvent := simSession.Events[2]
	if lastEvent.Type != trace.EventToolResult {
		t.Fatalf("expected EventToolResult, got %s", lastEvent.Type)
	}
	if lastEvent.Payload["is_synthetic"] != true {
		t.Errorf("expected is_synthetic=true, got %v", lastEvent.Payload["is_synthetic"])
	}
	if lastEvent.Checksum == "" {
		t.Errorf("expected non-empty checksum")
	}
}
