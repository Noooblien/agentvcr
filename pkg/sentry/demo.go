package sentry

import (
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// DemoScenario wraps a synthetic trajectory scenario for sentry analysis demonstration.
type DemoScenario struct {
	Name        string
	Description string
	SessionID   string
	Events      []trace.TraceEvent
}

// GenerateDemoScenarios builds 4 curated real-world agent scenarios testing different anomaly failure modes.
func GenerateDemoScenarios() []DemoScenario {
	baseTime := time.Now().Add(-10 * time.Minute)

	// Scenario 1: Nominal Healthy Trajectory
	sc1Events := []trace.TraceEvent{
		{
			ID: "ev_1", Index: 1, Type: trace.EventLLMRequest, Timestamp: baseTime, DurationMs: 120,
			Payload: map[string]interface{}{"prompt": "Inspect codebase and fix bug", "tokens": 150},
		},
		{
			ID: "ev_2", Index: 2, Type: trace.EventToolCall, Timestamp: baseTime.Add(200 * time.Millisecond), DurationMs: 45,
			Payload: map[string]interface{}{"tool_name": "list_dir", "arguments": map[string]interface{}{"path": "."}},
		},
		{
			ID: "ev_3", Index: 3, Type: trace.EventToolResult, Timestamp: baseTime.Add(300 * time.Millisecond), DurationMs: 50,
			Payload: map[string]interface{}{"tool_name": "list_dir", "content": "main.go, go.mod, README.md"},
		},
		{
			ID: "ev_4", Index: 4, Type: trace.EventToolCall, Timestamp: baseTime.Add(400 * time.Millisecond), DurationMs: 80,
			Payload: map[string]interface{}{"tool_name": "read_file", "arguments": map[string]interface{}{"path": "main.go"}},
		},
		{
			ID: "ev_5", Index: 5, Type: trace.EventLLMResponse, Timestamp: baseTime.Add(600 * time.Millisecond), DurationMs: 180,
			Payload: map[string]interface{}{"content": "Fixed bug in main.go", "tokens": 200},
		},
	}

	// Scenario 2: Recursive Tool Repetition Loop (OWASP-LLM08)
	sc2Events := []trace.TraceEvent{
		{ID: "loop_1", Index: 1, Type: trace.EventToolCall, Timestamp: baseTime, DurationMs: 40, Payload: map[string]interface{}{"tool_name": "grep_code"}},
		{ID: "loop_2", Index: 2, Type: trace.EventToolCall, Timestamp: baseTime.Add(100 * time.Millisecond), DurationMs: 42, Payload: map[string]interface{}{"tool_name": "grep_code"}},
		{ID: "loop_3", Index: 3, Type: trace.EventToolCall, Timestamp: baseTime.Add(200 * time.Millisecond), DurationMs: 45, Payload: map[string]interface{}{"tool_name": "grep_code"}},
		{ID: "loop_4", Index: 4, Type: trace.EventToolCall, Timestamp: baseTime.Add(300 * time.Millisecond), DurationMs: 48, Payload: map[string]interface{}{"tool_name": "grep_code"}},
	}

	// Scenario 3: Token Velocity Spike & Runaway Generation
	sc3Events := []trace.TraceEvent{
		{
			ID: "burn_1", Index: 1, Type: trace.EventLLMRequest, Timestamp: baseTime, DurationMs: 50,
			Payload: map[string]interface{}{"tokens": 50},
		},
		{
			ID: "burn_2", Index: 2, Type: trace.EventLLMResponse, Timestamp: baseTime.Add(80 * time.Millisecond), DurationMs: 100,
			Payload: map[string]interface{}{"tokens": 4000, "content": "Runaway repeating text... "},
		},
	}

	// Scenario 4: Error Cascade Storm (Consecutive Unhandled Failures)
	sc4Events := []trace.TraceEvent{
		{
			ID: "err_1", Index: 1, Type: trace.EventToolCall, Timestamp: baseTime, DurationMs: 50,
			Payload: map[string]interface{}{"tool_name": "connect_db"},
		},
		{
			ID: "err_2", Index: 2, Type: trace.EventError, Timestamp: baseTime.Add(100 * time.Millisecond), DurationMs: 20,
			Payload: map[string]interface{}{"error": "connection refused: 5432"},
		},
		{
			ID: "err_3", Index: 3, Type: trace.EventError, Timestamp: baseTime.Add(200 * time.Millisecond), DurationMs: 25,
			Payload: map[string]interface{}{"error": "retry failed: connection timeout"},
		},
		{
			ID: "err_4", Index: 4, Type: trace.EventError, Timestamp: baseTime.Add(300 * time.Millisecond), DurationMs: 30,
			Payload: map[string]interface{}{"error": "fatal: database pool exhausted"},
		},
	}

	return []DemoScenario{
		{
			Name:        "Nominal Healthy Agent Trajectory",
			Description: "Clean trajectory with normal tool invocation cadence and valid tokens",
			SessionID:   "session-nominal-001",
			Events:      sc1Events,
		},
		{
			Name:        "Infinite Tool Loop & Repetition (OWASP-LLM08)",
			Description: "Agent trapped in a 4-step identical tool repetition cycle",
			SessionID:   "session-loop-002",
			Events:      sc2Events,
		},
		{
			Name:        "Token Velocity Spike & Runaway Generation",
			Description: "Runaway LLM response generating 4,000 tokens in 100ms",
			SessionID:   "session-tokenburn-003",
			Events:      sc3Events,
		},
		{
			Name:        "Critical Error Cascade Storm",
			Description: "Agent suffering 3 consecutive unhandled database failures without recovery",
			SessionID:   "session-errstorm-004",
			Events:      sc4Events,
		},
	}
}
