package tests

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/drift"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func createMockTrajectory(id string, toolSequence []string) *trace.Session {
	var events []trace.TraceEvent
	now := time.Now()

	for i, tool := range toolSequence {
		// LLM Request -> Tool Call -> Tool Result -> LLM Response
		events = append(events, trace.TraceEvent{
			ID:         fmt.Sprintf("evt_%d", i*4),
			Index:      i * 4,
			Timestamp:  now.Add(time.Duration(i*100) * time.Millisecond),
			Type:       trace.EventLLMRequest,
			DurationMs: 40,
			Payload:    map[string]interface{}{"prompt": "run step"},
		})
		events = append(events, trace.TraceEvent{
			ID:         fmt.Sprintf("evt_%d", i*4+1),
			Index:      i*4 + 1,
			Timestamp:  now.Add(time.Duration(i*100+40) * time.Millisecond),
			Type:       trace.EventToolCall,
			DurationMs: 20,
			Payload:    map[string]interface{}{"name": tool},
		})
		events = append(events, trace.TraceEvent{
			ID:         fmt.Sprintf("evt_%d", i*4+2),
			Index:      i*4 + 2,
			Timestamp:  now.Add(time.Duration(i*100+60) * time.Millisecond),
			Type:       trace.EventToolResult,
			DurationMs: 10,
			Payload:    map[string]interface{}{"output": "ok"},
		})
		events = append(events, trace.TraceEvent{
			ID:         fmt.Sprintf("evt_%d", i*4+3),
			Index:      i*4 + 3,
			Timestamp:  now.Add(time.Duration(i*100+70) * time.Millisecond),
			Type:       trace.EventLLMResponse,
			DurationMs: 30,
			Payload:    map[string]interface{}{"content": "step completed"},
		})
	}

	return &trace.Session{
		ID:        id,
		AgentName: "mock_agent",
		CreatedAt: now,
		Events:    events,
	}
}

func TestDrift_IdenticalSessionsZeroDivergence(t *testing.T) {
	detector := drift.NewDetector(drift.DefaultDetectorConfig())

	tools := []string{"search_web", "parse_html", "summarize", "save_db"}
	s1 := createMockTrajectory("sess_base", tools)
	s2 := createMockTrajectory("sess_cand", tools)

	analysis := detector.AnalyzeDrift(s1, s2)

	if analysis.DriftDetected {
		t.Fatalf("expected zero drift for identical trajectories, got drift detected")
	}
	if analysis.JSDivergence > 0.0001 {
		t.Errorf("expected JS divergence ~ 0.0, got %f", analysis.JSDivergence)
	}
	if len(analysis.DriftedTransitions) != 0 {
		t.Errorf("expected 0 drifted transitions, got %d", len(analysis.DriftedTransitions))
	}
}

func TestDrift_SubtleShiftWithinTolerance(t *testing.T) {
	detector := drift.NewDetector(drift.DefaultDetectorConfig())

	baseTools := []string{"search_web", "parse_html", "summarize", "save_db", "search_web", "parse_html"}
	candTools := []string{"search_web", "parse_html", "summarize", "save_db", "search_web", "summarize"}

	s1 := createMockTrajectory("sess_base", baseTools)
	s2 := createMockTrajectory("sess_cand", candTools)

	analysis := detector.AnalyzeDrift(s1, s2)

	if analysis.DriftDetected {
		t.Errorf("subtle shift should have remained within tolerance, but flagged drift (JS: %f)", analysis.JSDivergence)
	}
}

func TestDrift_SignificantDivergenceAndPolicySynthesis(t *testing.T) {
	detector := drift.NewDetector(drift.DefaultDetectorConfig())

	// Baseline uses conservative read tools
	baseTools := []string{"read_file", "search_docs", "read_file", "search_docs", "read_file"}
	// Candidate abruptly mutates into heavy shell executions and deletions
	candTools := []string{"exec_shell", "delete_file", "exec_shell", "delete_file", "exec_shell", "delete_file"}

	s1 := createMockTrajectory("sess_safe", baseTools)
	s2 := createMockTrajectory("sess_anomalous", candTools)

	analysis := detector.AnalyzeDrift(s1, s2)

	if !analysis.DriftDetected {
		t.Fatalf("expected drift to be detected for completely divergent tool behaviors")
	}
	if analysis.JSDivergence < 0.20 {
		t.Errorf("expected high JS divergence >= 0.20, got %f", analysis.JSDivergence)
	}
	if analysis.SynthesizedPolicy == nil {
		t.Fatalf("expected synthesized corrective policy to be generated")
	}

	policy := analysis.SynthesizedPolicy
	if len(policy.ToolFrequencyCeilings) == 0 && len(policy.RestrictedTransitions) == 0 {
		t.Errorf("synthesized policy has neither ceilings nor restricted transitions")
	}

	scorecard := drift.RenderDriftScorecard(analysis)
	if !strings.Contains(scorecard, "DRIFT DETECTED") {
		t.Errorf("expected scorecard to indicate DRIFT DETECTED")
	}
	if !strings.Contains(scorecard, "SYNTHESIZED CORRECTIVE GUARDRAIL POLICY") {
		t.Errorf("expected scorecard to show synthesized policy")
	}
}
