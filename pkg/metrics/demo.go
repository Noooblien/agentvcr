package metrics

import (
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// RunTokenMetricsDemo executes a representative multi-turn agent audit demonstrating prompt caching, reasoning tokens, and step-by-step cost attribution.
func RunTokenMetricsDemo() *TokenAuditReport {
	now := time.Now()
	session := &trace.Session{
		ID:        "trace_token_audit_1760800000",
		AgentName: "Claude-Architect-Agent",
		Events: []trace.TraceEvent{
			{
				Index:      1,
				Timestamp:  now,
				Type:       trace.EventLLMRequest,
				DurationMs: 15,
				Payload: map[string]interface{}{
					"model":  "claude-3-7-sonnet",
					"prompt": "Analyze repository architecture and provide database migration strategy.",
				},
			},
			{
				Index:      2,
				Timestamp:  now.Add(200 * time.Millisecond),
				Type:       trace.EventLLMResponse,
				DurationMs: 380,
				Payload: map[string]interface{}{
					"usage": map[string]interface{}{
						"input_tokens":            float64(3400),
						"output_tokens":           float64(820),
						"cache_read_input_tokens": float64(2800),
						"completion_tokens_details": map[string]interface{}{
							"reasoning_tokens": float64(256),
						},
					},
				},
			},
			{
				Index:      3,
				Timestamp:  now.Add(1 * time.Second),
				Type:       trace.EventLLMRequest,
				DurationMs: 12,
				Payload: map[string]interface{}{
					"model":  "claude-3-7-sonnet",
					"prompt": "Synthesize SQL DDL schema for partition tables.",
				},
			},
			{
				Index:      4,
				Timestamp:  now.Add(1200 * time.Millisecond),
				Type:       trace.EventLLMResponse,
				DurationMs: 510,
				Payload: map[string]interface{}{
					"usage": map[string]interface{}{
						"input_tokens":            float64(4200),
						"output_tokens":           float64(1150),
						"cache_read_input_tokens": float64(3800),
						"completion_tokens_details": map[string]interface{}{
							"reasoning_tokens": float64(412),
						},
					},
				},
			},
		},
	}

	auditor := NewTokenAuditor()
	return auditor.AuditSession(session)
}
