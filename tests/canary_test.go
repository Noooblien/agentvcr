package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/canary"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func TestCanary_ChaosLatencySpikeAndPayloadMutation(t *testing.T) {
	cfg := canary.CanaryConfig{
		ShadowPercentage:    100.0,
		ChaosRate:           1.0, // Force chaos on every step
		ChaosMode:           canary.ChaosLatencySpike,
		LatencyDelayMs:      300,
		MaxAllowedErrorRate: 0.10,
		MaxDivergenceRate:   0.20,
		AutoRollback:        true,
	}

	engine := canary.NewEngine(cfg)

	basePayload := map[string]interface{}{"prompt": "hello world"}
	baseDuration := int64(100)

	canaryPayload, canaryDur, chaos, err := engine.InjectChaos(basePayload, baseDuration)
	if err != nil {
		t.Fatalf("unexpected error during latency spike chaos: %v", err)
	}
	if chaos != canary.ChaosLatencySpike {
		t.Errorf("expected ChaosLatencySpike, got %s", chaos)
	}
	if canaryDur != 400 {
		t.Errorf("expected canary duration 400ms (100+300), got %d", canaryDur)
	}
	if canaryPayload["prompt"] != "hello world" {
		t.Errorf("latency spike should not alter prompt payload")
	}

	// Test Payload Corruption
	cfg.ChaosMode = canary.ChaosCorruptPayload
	engine = canary.NewEngine(cfg)
	corruptPayload, _, chaosCorrupt, _ := engine.InjectChaos(basePayload, baseDuration)
	if chaosCorrupt != canary.ChaosCorruptPayload {
		t.Errorf("expected ChaosCorruptPayload, got %s", chaosCorrupt)
	}
	if !strings.Contains(corruptPayload["prompt"].(string), "CHAOS_MUTATION") {
		t.Errorf("expected payload mutation, got %v", corruptPayload["prompt"])
	}
}

func TestCanary_CircuitBreakerAndAutoRollback(t *testing.T) {
	// Configure high error rate chaos (HTTP 502)
	cfg := canary.CanaryConfig{
		ShadowPercentage:    100.0,
		ChaosRate:           1.0, // Every step errors
		ChaosMode:           canary.ChaosHTTPError,
		MaxAllowedErrorRate: 0.10,
		MaxDivergenceRate:   0.10,
		AutoRollback:        true,
	}
	engine := canary.NewEngine(cfg)

	session := &trace.Session{
		ID:        "session_canary_test",
		AgentName: "code-agent",
		CreatedAt: time.Now(),
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventLLMRequest, DurationMs: 150, Payload: map[string]interface{}{"query": "build project"}},
			{Index: 2, Type: trace.EventToolCall, DurationMs: 200, Payload: map[string]interface{}{"tool": "bash"}},
		},
	}

	report := engine.ShadowSession(session)

	if report.TotalStepsShadowed != 2 {
		t.Fatalf("expected 2 shadowed steps, got %d", report.TotalStepsShadowed)
	}
	if report.CanaryErrorCount != 2 {
		t.Errorf("expected 2 canary errors, got %d", report.CanaryErrorCount)
	}
	if !report.CircuitBreakerTripped {
		t.Errorf("circuit breaker should trip due to 100%% error rate")
	}
	if !report.RollbackTriggered {
		t.Errorf("auto-rollback should be triggered")
	}
	if report.Passed {
		t.Errorf("failing canary report should have Passed = false")
	}

	scorecard := canary.RenderCanaryScorecard(report)
	if !strings.Contains(scorecard, "AGENTVCR CANARY TRAFFIC SHADOWING") {
		t.Errorf("scorecard missing header: %s", scorecard)
	}
	if !strings.Contains(scorecard, "[TRIGGERED]") {
		t.Errorf("scorecard missing rollback triggered banner: %s", scorecard)
	}
}

func TestCanary_HealthyShadowingPasses(t *testing.T) {
	// Configure zero chaos
	cfg := canary.CanaryConfig{
		ShadowPercentage:    100.0,
		ChaosRate:           0.0,
		ChaosMode:           canary.ChaosNone,
		MaxAllowedErrorRate: 0.05,
		MaxDivergenceRate:   0.10,
		AutoRollback:        true,
	}
	engine := canary.NewEngine(cfg)

	session := &trace.Session{
		ID:        "session_canary_healthy",
		AgentName: "code-agent",
		CreatedAt: time.Now(),
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventLLMRequest, DurationMs: 120, Payload: map[string]interface{}{"status": "ok"}},
		},
	}

	report := engine.ShadowSession(session)

	if !report.Passed {
		t.Errorf("healthy canary shadowing should pass: %+v", report)
	}
	if report.CircuitBreakerTripped {
		t.Errorf("circuit breaker should not trip on healthy canary")
	}
	if report.RollbackTriggered {
		t.Errorf("auto-rollback should not be triggered")
	}
}
