package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/loopbreaker"
)

func TestIntegration_LoopBreaker_DemoSuite(t *testing.T) {
	cfg := loopbreaker.DefaultLoopConfig()
	summary := loopbreaker.RunDemoSuite(cfg)

	if summary.TotalStepsEvaluated < 15 {
		t.Fatalf("expected at least 15 evaluated steps in demo, got %d", summary.TotalStepsEvaluated)
	}

	if summary.CyclesDetected < 3 {
		t.Errorf("expected at least 3 cycles detected, got %d", summary.CyclesDetected)
	}

	if summary.TotalTokensSaved <= 0 {
		t.Errorf("expected tokens saved > 0, got %d", summary.TotalTokensSaved)
	}

	if summary.EstimatedSavingsUSD <= 0 {
		t.Errorf("expected cost savings > 0, got %.4f", summary.EstimatedSavingsUSD)
	}

	// Verify scorecard rendering
	scorecard := loopbreaker.RenderLoopScorecard(summary)
	if !strings.Contains(scorecard, "AGENTVCR REASONING CYCLE & LOOP BREAKER") {
		t.Errorf("scorecard missing header")
	}

	// Verify Mermaid state diagram rendering
	mermaid := loopbreaker.RenderLoopMermaid(summary.Detections)
	if !strings.Contains(mermaid, "stateDiagram-v2") {
		t.Errorf("mermaid missing stateDiagram-v2")
	}

	// Verify Markdown audit report rendering
	md := loopbreaker.RenderLoopMarkdownReport(summary)
	if !strings.Contains(md, "Executive Summary") {
		t.Errorf("markdown report missing Executive Summary")
	}
}

func TestIntegration_LoopBreaker_LiveTraceStreaming(t *testing.T) {
	cfg := loopbreaker.DefaultLoopConfig()
	breaker := loopbreaker.NewLoopBreaker(cfg)

	// Step 0-1: Initial healthy exploratory steps
	d0 := breaker.RecordStep(loopbreaker.StepRecord{
		StepIndex:      0,
		Timestamp:      time.Now(),
		ToolName:       "init_agent",
		ArgumentsJSON:  `{"goal": "debug API"}`,
		TokensConsumed: 100,
	})
	if d0.Detected {
		t.Errorf("step 0 should not trigger cycle")
	}

	d1 := breaker.RecordStep(loopbreaker.StepRecord{
		StepIndex:      1,
		Timestamp:      time.Now(),
		ToolName:       "get_endpoints",
		ArgumentsJSON:  `{"service": "auth"}`,
		TokensConsumed: 150,
	})
	if d1.Detected {
		t.Errorf("step 1 should not trigger cycle")
	}

	// Step 2-4: Identical failing tool calls
	var triggeredDetection *loopbreaker.CycleDetection
	for i := 2; i <= 4; i++ {
		det := breaker.RecordStep(loopbreaker.StepRecord{
			StepIndex:      i,
			Timestamp:      time.Now(),
			ToolName:       "request_auth_token",
			ArgumentsJSON:  `{"client_id": "agent-007", "scope": "admin"}`,
			OutputJSON:     `{"error": "Unauthorized: invalid client credentials"}`,
			IsError:        true,
			TokensConsumed: 300,
		})
		if det.Detected {
			triggeredDetection = det
		}
	}

	if triggeredDetection == nil || !triggeredDetection.Detected {
		t.Fatalf("expected cycle to be detected on repeating failing calls")
	}

	if triggeredDetection.Action != loopbreaker.ActionInjectSteering && triggeredDetection.Action != loopbreaker.ActionTripCircuitBreaker {
		t.Errorf("expected intervention action, got %s", triggeredDetection.Action)
	}

	if !strings.Contains(triggeredDetection.SteeringPrompt, "request_auth_token") {
		t.Errorf("steering prompt missing tool name")
	}

	// Agent heeds steering prompt and calls alternate tool
	dRecovered := breaker.RecordStep(loopbreaker.StepRecord{
		StepIndex:      5,
		Timestamp:      time.Now(),
		ToolName:       "refresh_cached_credentials",
		ArgumentsJSON:  `{"vault_path": "secrets/oauth"}`,
		TokensConsumed: 200,
	})
	if dRecovered.Detected {
		t.Errorf("recovered step with different tool should not be flagged as cycle")
	}
}
