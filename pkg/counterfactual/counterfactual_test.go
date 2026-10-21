package counterfactual

import (
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

func TestToolSubstitution(t *testing.T) {
	sim := NewSimulator()
	baseline := &trace.Session{
		ID:        "sess_base_test",
		AgentName: "agent-test",
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventLLMRequest, Payload: map[string]interface{}{"tokens": 50}},
			{Index: 2, Type: trace.EventToolCall, Payload: map[string]interface{}{"tool": "failed_tool", "tokens": 80}},
			{Index: 3, Type: trace.EventToolResult, Payload: map[string]interface{}{"status": "error", "tokens": 20}},
			{Index: 4, Type: trace.EventError, Payload: map[string]interface{}{"error": "crash", "tokens": 10}},
		},
	}

	mut := CounterfactualMutation{
		ID:              "mut_sub_test",
		Name:            "Substitute with cache",
		BranchStep:      2,
		Type:            MutateToolSubstitution,
		TargetTool:      "failed_tool",
		ReplacementTool: "cache_tool",
	}

	branch, comp, err := sim.Simulate(baseline, mut)
	if err != nil {
		t.Fatalf("Simulate failed: %v", err)
	}

	if branch.ForkAtStep != 2 {
		t.Errorf("expected ForkAtStep 2, got %d", branch.ForkAtStep)
	}
	if !comp.ResolvedFailure {
		t.Errorf("expected ResolvedFailure to be true")
	}
	if comp.Outcome != OutcomeResolvedSuccess {
		t.Errorf("expected OutcomeResolvedSuccess, got %v", comp.Outcome)
	}
}

func TestArgumentOverride(t *testing.T) {
	sim := NewSimulator()
	baseline := &trace.Session{
		ID:        "sess_arg_test",
		AgentName: "agent-test",
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventLLMRequest, Payload: map[string]interface{}{"tokens": 60}},
			{Index: 2, Type: trace.EventToolCall, Payload: map[string]interface{}{"tool": "sql", "tokens": 90}},
			{Index: 3, Type: trace.EventError, Payload: map[string]interface{}{"error": "syntax error", "tokens": 30}},
		},
	}

	mut := CounterfactualMutation{
		ID:         "mut_arg_test",
		BranchStep: 2,
		Type:       MutateArgumentOverride,
		TargetTool: "sql",
		ArgumentOverrides: map[string]interface{}{
			"query": "SELECT * FROM valid_table",
		},
	}

	branch, comp, err := sim.Simulate(baseline, mut)
	if err != nil {
		t.Fatalf("Simulate failed: %v", err)
	}

	if len(branch.Events) <= 2 {
		t.Errorf("expected branch to have events, got %d", len(branch.Events))
	}
	if comp.Outcome != OutcomeResolvedSuccess {
		t.Errorf("expected OutcomeResolvedSuccess, got %v", comp.Outcome)
	}
}

func TestLevenshteinDistance(t *testing.T) {
	seqA := []string{"read_file", "grep", "write_file"}
	seqB := []string{"read_file", "search", "write_file"}
	dist := computeLevenshtein(seqA, seqB)
	if dist != 1 {
		t.Errorf("expected distance 1, got %d", dist)
	}

	seqIdentical := []string{"a", "b", "c"}
	if d := computeLevenshtein(seqIdentical, seqIdentical); d != 0 {
		t.Errorf("expected distance 0 for identical sequences, got %d", d)
	}
}

func TestDemoSuiteExecution(t *testing.T) {
	comparisons, summary, err := RunDemoSuite()
	if err != nil {
		t.Fatalf("RunDemoSuite failed: %v", err)
	}

	if summary.TotalScenarios != 4 {
		t.Errorf("expected 4 scenarios, got %d", summary.TotalScenarios)
	}
	if len(comparisons) != 4 {
		t.Errorf("expected 4 comparisons, got %d", len(comparisons))
	}
	if summary.ResolvedSuccessCount < 2 {
		t.Errorf("expected at least 2 resolved successes, got %d", summary.ResolvedSuccessCount)
	}
	if summary.NetTokensSaved <= 0 {
		t.Errorf("expected positive net tokens saved, got %d", summary.NetTokensSaved)
	}

	// Verify scorecard formatting
	sc := RenderScorecard(comparisons[0])
	if !strings.Contains(sc, "COUNTERFACTUAL WHAT-IF") {
		t.Errorf("expected scorecard header")
	}

	// Verify Mermaid diagram
	scenarios := DefaultDemoScenarios()
	sim := NewSimulator()
	branch, _, _ := sim.Simulate(scenarios[0].Baseline, scenarios[0].Mutation)
	mermaid := RenderMermaid(comparisons[0], scenarios[0].Baseline, branch)
	if !strings.Contains(mermaid, "graph TD") || !strings.Contains(mermaid, "SplitNode") {
		t.Errorf("expected mermaid graph TD and SplitNode")
	}

	// Verify Markdown report
	md := RenderMarkdownReport(comparisons, summary)
	if !strings.Contains(md, "Counterfactual What-If Trajectory Report") {
		t.Errorf("expected markdown report title")
	}

	// Verify JSON output
	jsonStr, err := RenderJSON(comparisons)
	if err != nil || !strings.Contains(jsonStr, "whatif_cache_fallback") {
		t.Errorf("expected valid JSON string with scenario ID")
	}
}

func TestNilBaselineSession(t *testing.T) {
	sim := NewSimulator()
	_, _, err := sim.Simulate(nil, CounterfactualMutation{})
	if err == nil {
		t.Fatalf("expected error on nil baseline session")
	}
}

func TestTimeFormatDuration(t *testing.T) {
	now := time.Now().UTC()
	if now.IsZero() {
		t.Fatal("time should not be zero")
	}
}
