package tests_test

import (
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/metrics"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func TestComputeSessionMetrics(t *testing.T) {
	session := &trace.Session{
		ID:        "metrics_test_session",
		AgentName: "eval-agent",
		Events: []trace.TraceEvent{
			{
				Index:     0,
				Type:      trace.EventLLMRequest,
				Timestamp: time.Now(),
				Payload:   map[string]interface{}{"prompt": "test prompt"},
			},
			{
				Index:     1,
				Type:      trace.EventLLMResponse,
				Timestamp: time.Now(),
				DurationMs: 450,
				Payload: map[string]interface{}{
					"body": map[string]interface{}{
						"usage": map[string]interface{}{
							"prompt_tokens":     float64(100),
							"completion_tokens": float64(50),
							"total_tokens":      float64(150),
						},
					},
				},
			},
			{
				Index:     2,
				Type:      trace.EventToolCall,
				Timestamp: time.Now(),
				Payload:   map[string]interface{}{"name": "bash"},
			},
			{
				Index:     3,
				Type:      trace.EventToolResult,
				Timestamp: time.Now(),
				Payload:   map[string]interface{}{"status": "success"},
			},
		},
	}

	m := metrics.ComputeSessionMetrics(session)

	if m.TotalEvents != 4 {
		t.Errorf("expected 4 total events, got %d", m.TotalEvents)
	}
	if m.PromptTokens != 100 {
		t.Errorf("expected 100 prompt tokens, got %d", m.PromptTokens)
	}
	if m.CompletionTokens != 50 {
		t.Errorf("expected 50 completion tokens, got %d", m.CompletionTokens)
	}
	if m.TotalTokens != 150 {
		t.Errorf("expected 150 total tokens, got %d", m.TotalTokens)
	}
	if m.ToolCalls != 1 {
		t.Errorf("expected 1 tool call, got %d", m.ToolCalls)
	}
	if m.EstimatedCostUSD <= 0 {
		t.Errorf("expected positive cost estimation, got %f", m.EstimatedCostUSD)
	}
}
