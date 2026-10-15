package tests

import (
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/eval"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func TestEvaluate_ToolCallCount(t *testing.T) {
	now := time.Now()
	session := &trace.Session{
		ID:        "eval-test-1",
		AgentName: "weather-agent",
		CreatedAt: now,
		UpdatedAt: now.Add(2 * time.Second),
		Events: []trace.TraceEvent{
			{
				Index:      1,
				Timestamp:  now,
				Type:       trace.EventToolCall,
				DurationMs: 120,
				Payload: map[string]interface{}{
					"tool_name": "fetch_weather",
					"params":    map[string]interface{}{"city": "San Francisco"},
				},
			},
			{
				Index:      2,
				Timestamp:  now.Add(1 * time.Second),
				Type:       trace.EventToolCall,
				DurationMs: 150,
				Payload: map[string]interface{}{
					"tool_name": "fetch_weather",
					"params":    map[string]interface{}{"city": "Tokyo"},
				},
			},
		},
	}

	rules := []eval.AssertionRule{
		{
			Type:     eval.RuleToolCallCount,
			ToolName: "fetch_weather",
			MinCount: 1,
			MaxCount: 3,
		},
		{
			Type:     eval.RuleToolCallCount,
			ToolName: "execute_code",
			MinCount: 0,
			MaxCount: 0,
		},
	}

	report, err := eval.Evaluate(session, rules)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !report.AllPassed {
		t.Errorf("expected all rules to pass, got failures: %+v", report.Results)
	}
	if report.Passed != 2 || report.Failed != 0 {
		t.Errorf("expected 2 passed 0 failed, got passed=%d failed=%d", report.Passed, report.Failed)
	}
}

func TestEvaluate_BudgetAndErrors(t *testing.T) {
	now := time.Now()
	session := &trace.Session{
		ID:        "eval-test-2",
		AgentName: "db-agent",
		CreatedAt: now,
		UpdatedAt: now.Add(500 * time.Millisecond),
		Events: []trace.TraceEvent{
			{
				Index:      1,
				Timestamp:  now,
				Type:       trace.EventToolResult,
				DurationMs: 85,
				Payload: map[string]interface{}{
					"status": "success",
					"rows":   10,
				},
			},
		},
	}

	rules := []eval.AssertionRule{
		{
			Type: eval.RuleNoErrors,
		},
		{
			Type:        eval.RuleMaxLatency,
			MaxDuration: "1s",
		},
		{
			Type:      eval.RulePayloadMatch,
			Substring: "success",
		},
	}

	report, err := eval.Evaluate(session, rules)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !report.AllPassed {
		t.Fatalf("expected all assertions to pass, failed: %+v", report.Results)
	}
}
