package metrics

import (
	"github.com/Noooblien/agentvcr/pkg/trace"
)

// SessionMetrics calculates aggregated execution metrics for an agent session.
type SessionMetrics struct {
	TotalEvents        int     `json:"total_events"`
	LLMRequests        int     `json:"llm_requests"`
	LLMResponses       int     `json:"llm_responses"`
	ToolCalls          int     `json:"tool_calls"`
	ToolResults        int     `json:"tool_results"`
	PromptTokens       int     `json:"prompt_tokens"`
	CompletionTokens   int     `json:"completion_tokens"`
	TotalTokens        int     `json:"total_tokens"`
	EstimatedCostUSD   float64 `json:"estimated_cost_usd"`
	TotalDurationMs    int64   `json:"total_duration_ms"`
}

// ComputeSessionMetrics analyzes a session's events to compute usage and latency metrics.
func ComputeSessionMetrics(session *trace.Session) SessionMetrics {
	m := SessionMetrics{
		TotalEvents: len(session.Events),
	}

	for _, evt := range session.Events {
		switch evt.Type {
		case trace.EventLLMRequest:
			m.LLMRequests++
		case trace.EventLLMResponse:
			m.LLMResponses++
			// Extract usage if present in response body
			if body, ok := evt.Payload["body"].(map[string]interface{}); ok {
				if usage, ok := body["usage"].(map[string]interface{}); ok {
					if prompt, ok := usage["prompt_tokens"].(float64); ok {
						m.PromptTokens += int(prompt)
					}
					if comp, ok := usage["completion_tokens"].(float64); ok {
						m.CompletionTokens += int(comp)
					}
					if tot, ok := usage["total_tokens"].(float64); ok {
						m.TotalTokens += int(tot)
					}
				}
			}
		case trace.EventToolCall:
			m.ToolCalls++
		case trace.EventToolResult:
			m.ToolResults++
		}

		if evt.DurationMs > 0 {
			m.TotalDurationMs += evt.DurationMs
		}
	}

	// Cost heuristic: standard blending rate ($2.50 / 1M prompt, $10.00 / 1M completion)
	promptCost := (float64(m.PromptTokens) / 1000000.0) * 2.50
	completionCost := (float64(m.CompletionTokens) / 1000000.0) * 10.00
	m.EstimatedCostUSD = promptCost + completionCost

	return m
}
