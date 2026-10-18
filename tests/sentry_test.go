package tests

import (
	"strings"
	"testing"

	"github.com/Noooblien/agentvcr/pkg/sentry"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func TestSentry_NominalHealthyTrajectory(t *testing.T) {
	cfg := sentry.DefaultSentryConfig()
	engine := sentry.NewEngine(cfg)

	events := []trace.TraceEvent{
		{
			ID: "step_1", Index: 1, Type: trace.EventLLMRequest, DurationMs: 100,
			Payload: map[string]interface{}{"tokens": 50},
		},
		{
			ID: "step_2", Index: 2, Type: trace.EventToolCall, DurationMs: 50,
			Payload: map[string]interface{}{"tool_name": "list_dir"},
		},
		{
			ID: "step_3", Index: 3, Type: trace.EventToolResult, DurationMs: 30,
			Payload: map[string]interface{}{"tool_name": "list_dir", "content": "file1.go"},
		},
		{
			ID: "step_4", Index: 4, Type: trace.EventToolCall, DurationMs: 60,
			Payload: map[string]interface{}{"tool_name": "read_file"},
		},
		{
			ID: "step_5", Index: 5, Type: trace.EventLLMResponse, DurationMs: 120,
			Payload: map[string]interface{}{"tokens": 80, "content": "All done."},
		},
	}

	verdict := engine.AnalyzeTrajectory(events)
	if !verdict.Healthy {
		t.Fatalf("expected healthy verdict for nominal trajectory, got unhealthy: %+v", verdict)
	}
	if verdict.Action != sentry.ActionPass {
		t.Fatalf("expected ActionPass, got %s", verdict.Action)
	}
	if len(verdict.Anomalies) != 0 {
		t.Fatalf("expected 0 anomalies, got %d", len(verdict.Anomalies))
	}
	if verdict.RiskScore != 0.0 {
		t.Fatalf("expected 0 risk score, got %.1f", verdict.RiskScore)
	}
}

func TestSentry_ToolLoopDetection(t *testing.T) {
	cfg := sentry.DefaultSentryConfig()
	cfg.MaxToolRepetition = 3
	engine := sentry.NewEngine(cfg)

	events := []trace.TraceEvent{
		{ID: "e1", Index: 1, Type: trace.EventToolCall, Payload: map[string]interface{}{"tool_name": "grep_code"}},
		{ID: "e2", Index: 2, Type: trace.EventToolCall, Payload: map[string]interface{}{"tool_name": "grep_code"}},
		{ID: "e3", Index: 3, Type: trace.EventToolCall, Payload: map[string]interface{}{"tool_name": "grep_code"}},
		{ID: "e4", Index: 4, Type: trace.EventToolCall, Payload: map[string]interface{}{"tool_name": "grep_code"}},
	}

	verdict := engine.AnalyzeTrajectory(events)
	if verdict.Healthy {
		t.Fatalf("expected unhealthy verdict due to tool loop, got healthy")
	}

	foundLoop := false
	for _, a := range verdict.Anomalies {
		if a.Type == sentry.AnomalyToolLoop {
			foundLoop = true
			if a.Severity != sentry.SeverityHigh {
				t.Fatalf("expected SeverityHigh, got %s", a.Severity)
			}
			break
		}
	}
	if !foundLoop {
		t.Fatalf("expected AnomalyToolLoop in anomalies list, got: %+v", verdict.Anomalies)
	}
}

func TestSentry_BipartiteOscillation(t *testing.T) {
	cfg := sentry.DefaultSentryConfig()
	engine := sentry.NewEngine(cfg)

	// A -> B -> A -> B
	events := []trace.TraceEvent{
		{ID: "o1", Index: 1, Type: trace.EventToolCall, Payload: map[string]interface{}{"tool_name": "search"}},
		{ID: "o2", Index: 2, Type: trace.EventToolCall, Payload: map[string]interface{}{"tool_name": "read"}},
		{ID: "o3", Index: 3, Type: trace.EventToolCall, Payload: map[string]interface{}{"tool_name": "search"}},
		{ID: "o4", Index: 4, Type: trace.EventToolCall, Payload: map[string]interface{}{"tool_name": "read"}},
	}

	verdict := engine.AnalyzeTrajectory(events)
	foundOsc := false
	for _, a := range verdict.Anomalies {
		if a.Type == sentry.AnomalyOscillation {
			foundOsc = true
			break
		}
	}
	if !foundOsc {
		t.Fatalf("expected AnomalyOscillation to be flagged, got: %+v", verdict.Anomalies)
	}
}

func TestSentry_LatencyOutlierZScore(t *testing.T) {
	cfg := sentry.DefaultSentryConfig()
	cfg.LatencyZScoreThreshold = 2.5
	engine := sentry.NewEngine(cfg)

	// 9 steps with normal 100ms latency, 1 outlier step with 8000ms latency
	var events []trace.TraceEvent
	for i := 1; i <= 9; i++ {
		events = append(events, trace.TraceEvent{
			ID:         "normal",
			Index:      i,
			Type:       trace.EventToolCall,
			DurationMs: 100,
			Payload:    map[string]interface{}{"tool_name": "ping"},
		})
	}
	events = append(events, trace.TraceEvent{
		ID:         "outlier",
		Index:      10,
		Type:       trace.EventToolCall,
		DurationMs: 8000,
		Payload:    map[string]interface{}{"tool_name": "heavy_query"},
	})

	verdict := engine.AnalyzeTrajectory(events)
	foundSpike := false
	for _, a := range verdict.Anomalies {
		if a.Type == sentry.AnomalyLatencySpike {
			foundSpike = true
			if a.StepIndex != 10 {
				t.Fatalf("expected outlier at step 10, got %d", a.StepIndex)
			}
			break
		}
	}
	if !foundSpike {
		t.Fatalf("expected AnomalyLatencySpike to be detected, got: %+v", verdict.Anomalies)
	}
}

func TestSentry_TokenVelocityBurnRate(t *testing.T) {
	cfg := sentry.DefaultSentryConfig()
	cfg.MaxTokenVelocity = 200.0
	engine := sentry.NewEngine(cfg)

	events := []trace.TraceEvent{
		{
			ID:         "v1",
			Index:      1,
			Type:       trace.EventLLMResponse,
			DurationMs: 50, // 50ms duration
			Payload:    map[string]interface{}{"tokens": 2000}, // 2000 tokens in 50ms = 40,000 tok/sec
		},
	}

	verdict := engine.AnalyzeTrajectory(events)
	foundVel := false
	for _, a := range verdict.Anomalies {
		if a.Type == sentry.AnomalyTokenVelocity {
			foundVel = true
			if a.Severity != sentry.SeverityCritical {
				t.Fatalf("expected SeverityCritical, got %s", a.Severity)
			}
			break
		}
	}
	if !foundVel {
		t.Fatalf("expected AnomalyTokenVelocity, got: %+v", verdict.Anomalies)
	}
}

func TestSentry_ErrorCascadeStorm(t *testing.T) {
	cfg := sentry.DefaultSentryConfig()
	cfg.MaxConsecutiveErrors = 3
	engine := sentry.NewEngine(cfg)

	events := []trace.TraceEvent{
		{ID: "e1", Index: 1, Type: trace.EventError, Payload: map[string]interface{}{"error": "disk full"}},
		{ID: "e2", Index: 2, Type: trace.EventError, Payload: map[string]interface{}{"error": "permission denied"}},
		{ID: "e3", Index: 3, Type: trace.EventError, Payload: map[string]interface{}{"error": "connection reset"}},
	}

	verdict := engine.AnalyzeTrajectory(events)
	foundCascade := false
	for _, a := range verdict.Anomalies {
		if a.Type == sentry.AnomalyErrorCascade {
			foundCascade = true
			if a.Severity != sentry.SeverityCritical {
				t.Fatalf("expected SeverityCritical for error storm, got %s", a.Severity)
			}
			break
		}
	}
	if !foundCascade {
		t.Fatalf("expected AnomalyErrorCascade, got: %+v", verdict.Anomalies)
	}
	if verdict.Metrics.ConsecutiveErrors != 3 {
		t.Fatalf("expected 3 consecutive errors, got %d", verdict.Metrics.ConsecutiveErrors)
	}
}

func TestSentry_CircuitBreakerActions(t *testing.T) {
	cfg := sentry.DefaultSentryConfig()
	cfg.TripRiskThreshold = 60.0
	cfg.ThrottleRiskThreshold = 30.0
	engine := sentry.NewEngine(cfg)

	// Severe failure storm tripping circuit breaker
	events := []trace.TraceEvent{
		{ID: "e1", Index: 1, Type: trace.EventError, Payload: map[string]interface{}{"error": "err1"}},
		{ID: "e2", Index: 2, Type: trace.EventError, Payload: map[string]interface{}{"error": "err2"}},
		{ID: "e3", Index: 3, Type: trace.EventError, Payload: map[string]interface{}{"error": "err3"}}, // Critical (+35)
		{ID: "e4", Index: 4, Type: trace.EventToolCall, DurationMs: 50, Payload: map[string]interface{}{"tokens": 4000}}, // Critical (+35)
	}

	verdict := engine.AnalyzeTrajectory(events)
	if verdict.RiskScore < 60.0 {
		t.Fatalf("expected risk score >= 60.0, got %.1f", verdict.RiskScore)
	}
	if verdict.Action != sentry.ActionTripCircuitBreaker {
		t.Fatalf("expected ActionTripCircuitBreaker, got %s", verdict.Action)
	}
	if verdict.Healthy {
		t.Fatalf("expected Healthy false when circuit breaker tripped")
	}
}

func TestSentry_ScorecardAndMermaid(t *testing.T) {
	cfg := sentry.DefaultSentryConfig()
	engine := sentry.NewEngine(cfg)

	scenarios := sentry.GenerateDemoScenarios()
	if len(scenarios) != 4 {
		t.Fatalf("expected 4 demo scenarios, got %d", len(scenarios))
	}

	for _, sc := range scenarios {
		verdict := engine.AnalyzeTrajectory(sc.Events)
		card := sentry.RenderSentryScorecard(verdict, sc.SessionID)
		if !strings.Contains(card, "AgentVCR Trajectory Anomaly Sentry & Circuit Breaker") {
			t.Fatalf("card missing header: %s", card)
		}
		if !strings.Contains(card, sc.SessionID) {
			t.Fatalf("card missing session ID: %s", card)
		}

		mermaid := sentry.RenderSentryMermaid(verdict)
		if !strings.Contains(mermaid, "flowchart TD") {
			t.Fatalf("mermaid missing flowchart TD header: %s", mermaid)
		}
		if !strings.Contains(mermaid, "Sentry Anomaly Engine") {
			t.Fatalf("mermaid missing sentry engine node: %s", mermaid)
		}
	}
}

func TestSentry_EmptyTrajectory(t *testing.T) {
	cfg := sentry.DefaultSentryConfig()
	engine := sentry.NewEngine(cfg)

	verdict := engine.AnalyzeTrajectory(nil)
	if !verdict.Healthy {
		t.Fatalf("expected healthy for empty trajectory")
	}
	if verdict.Action != sentry.ActionPass {
		t.Fatalf("expected ActionPass, got %s", verdict.Action)
	}
}
