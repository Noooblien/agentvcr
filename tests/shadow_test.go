package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/shadow"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func createSampleSessions() (*trace.Session, *trace.Session) {
	now := time.Now()

	baseEvents := []trace.TraceEvent{
		{
			ID:        "evt-b0",
			Index:     0,
			Type:      trace.EventLLMRequest,
			Timestamp: now.Add(-5 * time.Minute),
			DurationMs: 120,
			Payload: map[string]interface{}{
				"prompt": "Analyze cluster memory usage and optimize pod allocation.",
				"tokens": 150,
			},
		},
		{
			ID:        "evt-b1",
			Index:     1,
			Type:      trace.EventToolCall,
			Timestamp: now.Add(-4 * time.Minute),
			DurationMs: 200,
			Payload: map[string]interface{}{
				"tool":    "k8s_describe_nodes",
				"command": "kubectl describe nodes",
				"tokens":  60,
			},
		},
		{
			ID:        "evt-b2",
			Index:     2,
			Type:      trace.EventLLMResponse,
			Timestamp: now.Add(-3 * time.Minute),
			DurationMs: 300,
			Payload: map[string]interface{}{
				"content": "Node memory pressure detected on worker-2. Recommending rolling restart.",
				"tokens":  95,
			},
		},
	}

	candEvents := []trace.TraceEvent{
		{
			ID:        "evt-c0",
			Index:     0,
			Type:      trace.EventLLMRequest,
			Timestamp: now.Add(-5 * time.Minute),
			DurationMs: 105,
			Payload: map[string]interface{}{
				"prompt": "Analyze cluster memory usage and optimize pod allocation.",
				"tokens": 150,
			},
		},
		{
			ID:        "evt-c1",
			Index:     1,
			Type:      trace.EventToolCall,
			Timestamp: now.Add(-4 * time.Minute),
			DurationMs: 190,
			Payload: map[string]interface{}{
				"tool":    "k8s_describe_nodes",
				"command": "kubectl describe nodes",
				"tokens":  55,
			},
		},
		{
			ID:        "evt-c2",
			Index:     2,
			Type:      trace.EventLLMResponse,
			Timestamp: now.Add(-3 * time.Minute),
			DurationMs: 260,
			Payload: map[string]interface{}{
				"content": "Memory pressure observed on node worker-2. Recommend rolling deployment restart.",
				"tokens":  90,
			},
		},
	}

	baseSession := &trace.Session{
		ID:        "sess-baseline-1",
		AgentName: "infra-agent-gpt4o",
		Events:    baseEvents,
	}

	candSession := &trace.Session{
		ID:        "sess-candidate-1",
		AgentName: "infra-agent-claude35-sonnet",
		Events:    candEvents,
	}

	return baseSession, candSession
}

func TestShadow_CompatibleCandidatePromoted(t *testing.T) {
	baseSession, candSession := createSampleSessions()

	baseModel := shadow.ModelCandidate{
		ID:             "gpt-4o",
		ModelName:      "GPT-4o",
		Provider:       "OpenAI",
		CostPer1kOutput: 0.015,
	}
	candModel := shadow.ModelCandidate{
		ID:             "claude-3-5-sonnet",
		ModelName:      "Claude-3.5-Sonnet",
		Provider:       "Anthropic",
		CostPer1kOutput: 0.012, // 20% cheaper
	}

	cfg := shadow.ShadowConfig{
		Mode:                 shadow.RoutingShadowOnly,
		MaxDriftThreshold:   0.35,
		MaxLatencyRatio:     1.30,
		MaxCostIncreaseRatio: 1.10,
		MaxAllowedErrorRate: 0.02,
		MinSampleCount:      1,
	}

	engine := shadow.NewShadowEngine(cfg, baseModel, candModel)
	comparison := engine.CompareTrajectories(baseSession, candSession)

	if !comparison.Passed {
		t.Fatalf("Expected comparison to pass, got rejections: %v", comparison.RejectionReasons)
	}
	if !comparison.PromotionRecommended {
		t.Fatalf("Expected promotion recommended")
	}
	if comparison.AverageDriftScore > 0.35 {
		t.Fatalf("Drift score too high: %.2f", comparison.AverageDriftScore)
	}
	if comparison.CostDeltaPercent >= 0.0 {
		t.Fatalf("Expected cost savings with cheaper candidate model")
	}

	report := engine.GenerateAuditReport()
	if report.Status != "PROMOTABLE" {
		t.Fatalf("Expected PROMOTABLE status, got: %s", report.Status)
	}
	if report.CircuitBreakerTripped {
		t.Fatalf("Did not expect circuit breaker to trip")
	}
}

func TestShadow_SevereDriftTripsCircuitBreaker(t *testing.T) {
	baseSession, _ := createSampleSessions()

	// Divergent candidate with errors and tool deviation
	now := time.Now()
	candEvents := []trace.TraceEvent{
		{
			ID:        "evt-c0",
			Index:     0,
			Type:      trace.EventLLMRequest,
			Timestamp: now,
			DurationMs: 120,
			Payload: map[string]interface{}{
				"prompt": "Unrelated topic inquiry",
			},
		},
		{
			ID:        "evt-c1",
			Index:     1,
			Type:      trace.EventToolCall,
			Timestamp: now,
			DurationMs: 500, // Very slow
			Payload: map[string]interface{}{
				"tool": "drop_all_tables", // Tool divergence
			},
		},
		{
			ID:        "evt-c2",
			Index:     2,
			Type:      trace.EventError,
			Timestamp: now,
			DurationMs: 600,
			Payload: map[string]interface{}{
				"error": "Unhandled runtime exception in candidate model",
			},
		},
	}
	candSession := &trace.Session{
		ID:        "sess-candidate-divergent",
		AgentName: "divergent-agent",
		Events:    candEvents,
	}

	baseModel := shadow.ModelCandidate{ModelName: "GPT-4o"}
	candModel := shadow.ModelCandidate{ModelName: "Experimental-LLM"}

	cfg := shadow.ShadowConfig{
		Mode:                 shadow.RoutingABSplit,
		MaxDriftThreshold:   0.25,
		MaxLatencyRatio:     1.20,
		MaxAllowedErrorRate: 0.05,
		MinSampleCount:      1,
	}

	engine := shadow.NewShadowEngine(cfg, baseModel, candModel)
	comparison := engine.CompareTrajectories(baseSession, candSession)

	if comparison.Passed {
		t.Fatalf("Expected comparison to FAIL due to tool divergence and error")
	}
	if comparison.PromotionRecommended {
		t.Fatalf("Promotion should NOT be recommended")
	}

	report := engine.GenerateAuditReport()
	if !report.CircuitBreakerTripped {
		t.Fatalf("Expected circuit breaker to trip due to error rate and drift")
	}
}

func TestShadow_ScorecardAndMermaid(t *testing.T) {
	baseSession, candSession := createSampleSessions()

	baseModel := shadow.ModelCandidate{ModelName: "GPT-4o", Provider: "OpenAI", CostPer1kOutput: 0.015}
	candModel := shadow.ModelCandidate{ModelName: "Llama-3.3-70B", Provider: "Meta", CostPer1kOutput: 0.003}

	cfg := shadow.ShadowConfig{
		Mode:               shadow.RoutingShadowOnly,
		MaxDriftThreshold: 0.40,
		MinSampleCount:    1,
	}

	engine := shadow.NewShadowEngine(cfg, baseModel, candModel)
	_ = engine.CompareTrajectories(baseSession, candSession)
	report := engine.GenerateAuditReport()

	scorecard := shadow.RenderShadowScorecard(report)
	if !strings.Contains(scorecard, "AGENTVCR // CONTINUOUS MODEL SHADOWING") {
		t.Fatalf("Scorecard missing header: %s", scorecard)
	}
	if !strings.Contains(scorecard, "STEP-LEVEL DIVERGENCE") {
		t.Fatalf("Scorecard missing divergence breakdown: %s", scorecard)
	}

	mermaid := shadow.RenderShadowMermaid(report)
	if !strings.Contains(mermaid, "graph TD") || !strings.Contains(mermaid, "Shadow Router") {
		t.Fatalf("Mermaid diagram missing expected structure: %s", mermaid)
	}
}
