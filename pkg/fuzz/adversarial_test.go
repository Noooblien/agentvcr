package fuzz

import (
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

func TestAdversarialEngine_CampaignExecution(t *testing.T) {
	cfg := DefaultFuzzEngineConfig()
	engine := NewAdversarialEngine(cfg)

	session := &trace.Session{
		ID:        "test-session-fuzz-01",
		AgentName: "TestAgent",
		Events: []trace.TraceEvent{
			{
				Index:      1,
				Type:       trace.EventLLMRequest,
				Payload:    map[string]interface{}{"prompt": "Hello agent"},
				DurationMs: 10,
				Timestamp:  time.Now(),
			},
			{
				Index:      2,
				Type:       trace.EventToolResult,
				Payload:    map[string]interface{}{"output": "tool executed"},
				DurationMs: 50,
				Timestamp:  time.Now(),
			},
		},
	}

	report := engine.RunCampaign(session)
	if report == nil {
		t.Fatalf("expected non-nil campaign report")
	}

	if report.TotalMutations == 0 {
		t.Fatalf("expected adversarial test cases to be generated")
	}

	if len(report.Cases) != report.TotalMutations {
		t.Errorf("mismatch between cases count and total mutations: %d vs %d", len(report.Cases), report.TotalMutations)
	}

	if report.AgentRobustnessScore < 0.0 || report.AgentRobustnessScore > 100.0 {
		t.Errorf("robustness score out of bounds: %.2f", report.AgentRobustnessScore)
	}
}

func TestAdversarialEngine_DemoSuiteAndScorecard(t *testing.T) {
	report := RunFuzzDemo()
	if report == nil {
		t.Fatalf("expected non-nil demo report")
	}

	scorecard := RenderFuzzScorecard(report)
	if !strings.Contains(scorecard, "ADVERSARIAL TRAJECTORY FUZZING") {
		t.Errorf("scorecard missing expected title header")
	}

	mermaid := RenderFuzzMermaid(report)
	if !strings.Contains(mermaid, "flowchart TD") {
		t.Errorf("mermaid missing flowchart directive")
	}

	md := RenderFuzzMarkdown(report)
	if !strings.Contains(md, "AgentVCR Adversarial Fuzzing & Robustness Audit Report") {
		t.Errorf("markdown report missing expected heading")
	}
}
