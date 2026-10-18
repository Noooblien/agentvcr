package tests_test

import (
	"strings"
	"testing"

	"github.com/Noooblien/agentvcr/pkg/diff"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func TestSemanticDiff_IdenticalSessions(t *testing.T) {
	events := []trace.TraceEvent{
		{
			Index:      0,
			Type:       trace.EventLLMRequest,
			DurationMs: 100,
			Payload:    map[string]interface{}{"prompt": "Inspect database status"},
		},
		{
			Index:      1,
			Type:       trace.EventToolCall,
			DurationMs: 50,
			Payload:    map[string]interface{}{"name": "sql_query", "query": "SELECT 1;"},
		},
		{
			Index:      2,
			Type:       trace.EventToolResult,
			DurationMs: 10,
			Payload:    map[string]interface{}{"output": "1"},
		},
	}

	sessionA := &trace.Session{
		ID:        "session_a",
		AgentName: "db-agent",
		Events:    events,
	}
	sessionB := &trace.Session{
		ID:        "session_b",
		AgentName: "db-agent",
		Events:    events,
	}

	report := diff.AnalyzeSemanticDiff(sessionA, sessionB, diff.DefaultDiffOptions())

	if report.StabilityScore < 99.9 {
		t.Fatalf("expected 100%% stability for identical sessions, got %.1f%%", report.StabilityScore)
	}
	if report.Verdict != diff.VerdictPass {
		t.Fatalf("expected VerdictPass, got %s", report.Verdict)
	}
	if report.ExactMatches != 3 {
		t.Errorf("expected 3 exact matches, got %d", report.ExactMatches)
	}
	if report.Regressions != 0 || report.ParamDrifts != 0 {
		t.Errorf("expected 0 regressions and 0 drifts, got %d regressions, %d drifts", report.Regressions, report.ParamDrifts)
	}
}

func TestSemanticDiff_SemanticParaphrase(t *testing.T) {
	sessionA := &trace.Session{
		ID:        "session_a",
		AgentName: "writer",
		Events: []trace.TraceEvent{
			{
				Index: 0,
				Type:  trace.EventLLMRequest,
				Payload: map[string]interface{}{
					"prompt": "Summarize the quarterly financial earnings report",
				},
			},
			{
				Index: 1,
				Type:  trace.EventLLMResponse,
				Payload: map[string]interface{}{
					"content": "Revenue grew by 24% year-over-year while operating margins held firm at 31%.",
				},
			},
		},
	}

	sessionB := &trace.Session{
		ID:        "session_b",
		AgentName: "writer-v2",
		Events: []trace.TraceEvent{
			{
				Index: 0,
				Type:  trace.EventLLMRequest,
				Payload: map[string]interface{}{
					"prompt": "Summarize the quarterly financial earnings report",
				},
			},
			{
				Index: 1,
				Type:  trace.EventLLMResponse,
				Payload: map[string]interface{}{
					"content": "Quarterly revenue increased 24% YoY with solid operating margins maintained at 31%.",
				},
			},
		},
	}

	opts := diff.DefaultDiffOptions()
	report := diff.AnalyzeSemanticDiff(sessionA, sessionB, opts)

	if report.SemanticMatches != 1 {
		t.Errorf("expected 1 semantic match for paraphrased response, got %d", report.SemanticMatches)
	}
	if report.Verdict != diff.VerdictPass {
		t.Errorf("expected VerdictPass for paraphrased response within tolerance, got %s", report.Verdict)
	}
}

func TestSemanticDiff_ParamDrift(t *testing.T) {
	sessionA := &trace.Session{
		ID:        "session_base",
		AgentName: "executor",
		Events: []trace.TraceEvent{
			{
				Index:   0,
				Type:    trace.EventLLMRequest,
				Payload: map[string]interface{}{"prompt": "Read configuration"},
			},
			{
				Index: 1,
				Type:  trace.EventToolCall,
				Payload: map[string]interface{}{
					"name": "read_file",
					"path": "configs/production.yaml",
				},
			},
			{
				Index:   2,
				Type:    trace.EventToolResult,
				Payload: map[string]interface{}{"output": "env: production"},
			},
		},
	}

	sessionB := &trace.Session{
		ID:        "session_target",
		AgentName: "executor",
		Events: []trace.TraceEvent{
			{
				Index:   0,
				Type:    trace.EventLLMRequest,
				Payload: map[string]interface{}{"prompt": "Read configuration"},
			},
			{
				Index: 1,
				Type:  trace.EventToolCall,
				Payload: map[string]interface{}{
					"name": "read_file",
					"path": "configs/staging.yaml",
				},
			},
			{
				Index:   2,
				Type:    trace.EventToolResult,
				Payload: map[string]interface{}{"output": "env: production"},
			},
		},
	}

	report := diff.AnalyzeSemanticDiff(sessionA, sessionB, diff.DefaultDiffOptions())

	if report.ParamDrifts != 1 {
		t.Fatalf("expected 1 parameter drift, got %d", report.ParamDrifts)
	}
	if len(report.AlignedSteps[1].ParamDiffs) == 0 {
		t.Errorf("expected detailed param diffs explaining path change")
	}
	if !strings.Contains(report.AlignedSteps[1].ParamDiffs[0], "staging.yaml") {
		t.Errorf("expected staging.yaml mentioned in param diff: %v", report.AlignedSteps[1].ParamDiffs)
	}
	if report.Verdict != diff.VerdictWarn {
		t.Errorf("expected VerdictWarn for parameter drift, got %s", report.Verdict)
	}
}

func TestSemanticDiff_OutcomeRegression_And_Strict(t *testing.T) {
	base, regressedTarget := diff.GenerateDemoSessions(true)

	// 1. Regular analysis -> should catch outcome regression and FAIL
	report := diff.AnalyzeSemanticDiff(base, regressedTarget, diff.DefaultDiffOptions())

	if report.Regressions == 0 {
		t.Fatalf("expected at least 1 regression, got 0")
	}
	if report.Verdict != diff.VerdictFail {
		t.Errorf("expected VerdictFail due to error regression, got %s", report.Verdict)
	}

	// 2. Strict analysis -> should also FAIL
	strictOpts := diff.DefaultDiffOptions()
	strictOpts.Strict = true
	strictReport := diff.AnalyzeSemanticDiff(base, regressedTarget, strictOpts)
	if strictReport.Verdict != diff.VerdictFail {
		t.Errorf("expected VerdictFail in strict mode, got %s", strictReport.Verdict)
	}

	// 3. Clean target -> should PASS
	_, cleanTarget := diff.GenerateDemoSessions(false)
	cleanReport := diff.AnalyzeSemanticDiff(base, cleanTarget, diff.DefaultDiffOptions())
	if cleanReport.Regressions != 0 {
		t.Errorf("expected 0 regressions in clean target, got %d", cleanReport.Regressions)
	}
	if cleanReport.Verdict == diff.VerdictFail {
		t.Errorf("expected passing or warn verdict, got %s", cleanReport.Verdict)
	}
}

func TestSemanticDiff_ScorecardAndMermaid(t *testing.T) {
	base, target := diff.GenerateDemoSessions(true)
	report := diff.AnalyzeSemanticDiff(base, target, diff.DefaultDiffOptions())

	scorecard := diff.RenderDiffScorecard(report)
	if !strings.Contains(scorecard, "AgentVCR • Semantic Trajectory Diff") {
		t.Errorf("expected scorecard header not found")
	}
	if !strings.Contains(scorecard, "Stability Score:") {
		t.Errorf("expected stability score in scorecard")
	}
	if !strings.Contains(scorecard, "[VERDICT: FAIL]") {
		t.Errorf("expected verdict fail banner in scorecard")
	}

	mermaid := diff.RenderDiffMermaid(report)
	if !strings.Contains(mermaid, "flowchart TD") {
		t.Errorf("expected Mermaid flowchart header")
	}
	if !strings.Contains(mermaid, "subgraph Base") || !strings.Contains(mermaid, "subgraph Target") {
		t.Errorf("expected Base and Target subgraphs in Mermaid")
	}
}
