package tests

import (
	"strings"
	"testing"

	"github.com/Noooblien/agentvcr/pkg/counterfactual"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func TestIntegration_Counterfactual_DemoSuite(t *testing.T) {
	comparisons, summary, err := counterfactual.RunDemoSuite()
	if err != nil {
		t.Fatalf("failed to run counterfactual demo suite: %v", err)
	}

	if len(comparisons) != 4 {
		t.Fatalf("expected 4 scenario comparisons, got %d", len(comparisons))
	}

	// Scenario 1: 429 Rate Limit -> Cache Fallback Tool Substitution
	c1 := comparisons[0]
	if c1.Outcome != counterfactual.OutcomeResolvedSuccess {
		t.Errorf("scenario 1 should be OutcomeResolvedSuccess, got %v", c1.Outcome)
	}
	if !c1.ResolvedFailure {
		t.Errorf("scenario 1 should resolve failure")
	}

	// Scenario 2: SQL Error -> Argument Override Schema Correction
	c2 := comparisons[1]
	if c2.Outcome != counterfactual.OutcomeResolvedSuccess {
		t.Errorf("scenario 2 should be OutcomeResolvedSuccess, got %v", c2.Outcome)
	}
	if !c2.ResolvedFailure {
		t.Errorf("scenario 2 should resolve failure")
	}

	// Scenario 3: Runaway Search Loop -> Steering Prompt
	c3 := comparisons[2]
	if c3.TokenDelta >= 0 {
		t.Errorf("scenario 3 should save tokens, delta is %d", c3.TokenDelta)
	}

	// Verify all scorecards render cleanly
	for _, c := range comparisons {
		scorecard := counterfactual.RenderScorecard(c)
		if len(scorecard) == 0 || !strings.Contains(scorecard, "COUNTERFACTUAL WHAT-IF") {
			t.Errorf("scenario %s produced invalid scorecard", c.Mutation.ID)
		}
	}

	// Verify markdown and summary
	md := counterfactual.RenderMarkdownReport(comparisons, summary)
	if !strings.Contains(md, "Counterfactual What-If Trajectory Report") {
		t.Errorf("invalid markdown report header")
	}
}

func TestIntegration_Counterfactual_LiveSimulation(t *testing.T) {
	sim := counterfactual.NewSimulator()
	baseline := &trace.Session{
		ID:        "live_trace_session_99",
		AgentName: "agent-live-worker",
		Status:    "failed",
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventLLMRequest, Payload: map[string]interface{}{"tokens": 40}},
			{Index: 2, Type: trace.EventToolCall, Payload: map[string]interface{}{"tool": "unstable_remote_api", "tokens": 100}},
			{Index: 3, Type: trace.EventError, Payload: map[string]interface{}{"error": "connection reset", "tokens": 20}},
		},
	}

	mutation := counterfactual.CounterfactualMutation{
		ID:              "mut_live_fallback",
		Name:            "Live Fallback Intervention",
		BranchStep:      2,
		Type:            counterfactual.MutateToolSubstitution,
		TargetTool:      "unstable_remote_api",
		ReplacementTool: "stable_local_service",
	}

	branch, comp, err := sim.Simulate(baseline, mutation)
	if err != nil {
		t.Fatalf("Simulate failed: %v", err)
	}

	if branch.ParentID != baseline.ID {
		t.Errorf("expected branch ParentID %s, got %s", baseline.ID, branch.ParentID)
	}
	if branch.ForkAtStep != 2 {
		t.Errorf("expected branch ForkAtStep 2, got %d", branch.ForkAtStep)
	}
	if comp.Outcome != counterfactual.OutcomeResolvedSuccess {
		t.Errorf("expected OutcomeResolvedSuccess, got %v", comp.Outcome)
	}
	if !comp.ResolvedFailure {
		t.Errorf("expected ResolvedFailure to be true")
	}

	mermaid := counterfactual.RenderMermaid(comp, baseline, branch)
	if !strings.Contains(mermaid, "graph TD") {
		t.Errorf("expected valid Mermaid diagram")
	}
}
