package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/causal"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func TestCausal_HealthyTrajectoryDAG(t *testing.T) {
	now := time.Now()
	session := &trace.Session{
		ID:        "healthy-session-dag",
		AgentName: "code-agent",
		CreatedAt: now,
		Events: []trace.TraceEvent{
			{
				ID:        "ev_1",
				Index:     1,
				Timestamp: now,
				Type:      trace.EventLLMRequest,
				Payload:   map[string]interface{}{"prompt": "List repository files"},
			},
			{
				ID:        "ev_2",
				Index:     2,
				Timestamp: now.Add(50 * time.Millisecond),
				Type:      trace.EventToolCall,
				Payload:   map[string]interface{}{"tool_name": "list_files", "args": map[string]interface{}{"path": "."}},
			},
			{
				ID:        "ev_3",
				Index:     3,
				Timestamp: now.Add(100 * time.Millisecond),
				Type:      trace.EventToolResult,
				Payload:   map[string]interface{}{"result": []string{"main.go", "go.mod"}},
			},
		},
	}

	engine := causal.NewEngine()
	dag, err := engine.SynthesizeDAG(session)
	if err != nil {
		t.Fatalf("SynthesizeDAG failed: %v", err)
	}

	if len(dag.Nodes) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(dag.Nodes))
	}
	if len(dag.Edges) < 2 {
		t.Errorf("expected at least 2 edges, got %d", len(dag.Edges))
	}

	report, err := engine.DiagnoseRootCauses(session, dag)
	if err != nil {
		t.Fatalf("DiagnoseRootCauses failed: %v", err)
	}

	if report.HasFailures {
		t.Errorf("expected no failures in healthy trajectory")
	}

	mermaid := causal.ExportMermaidDiagram(dag)
	if !strings.Contains(mermaid, "flowchart TD") {
		t.Errorf("expected mermaid flowchart syntax")
	}
}

func TestCausal_RootCauseDiagnosisAndRepair(t *testing.T) {
	now := time.Now()
	session := &trace.Session{
		ID:        "failing-session-dag",
		AgentName: "sql-agent",
		CreatedAt: now,
		Events: []trace.TraceEvent{
			{
				ID:        "step_1",
				Index:     1,
				Timestamp: now,
				Type:      trace.EventLLMRequest,
				Payload:   map[string]interface{}{"prompt": "Drop user table"},
			},
			{
				ID:        "step_2",
				Index:     2,
				Timestamp: now.Add(100 * time.Millisecond),
				Type:      trace.EventToolCall,
				Payload: map[string]interface{}{
					"tool_name": "database_exec",
					"params":    map[string]interface{}{"query": "DROP TABLE users CASCADE"},
				},
			},
			{
				ID:        "step_3",
				Index:     3,
				Timestamp: now.Add(200 * time.Millisecond),
				Type:      trace.EventError,
				Payload: map[string]interface{}{
					"error":  "Permission denied: operation forbidden in read-only environment",
					"status": "failed",
				},
			},
		},
	}

	engine := causal.NewEngine()
	dag, err := engine.SynthesizeDAG(session)
	if err != nil {
		t.Fatalf("SynthesizeDAG failed: %v", err)
	}

	report, err := engine.DiagnoseRootCauses(session, dag)
	if err != nil {
		t.Fatalf("DiagnoseRootCauses failed: %v", err)
	}

	if !report.HasFailures {
		t.Fatalf("expected failures to be diagnosed")
	}
	if len(report.RootCauses) != 1 {
		t.Fatalf("expected 1 root cause, got %d", len(report.RootCauses))
	}

	rc := report.RootCauses[0]
	if rc.FailureStep != 3 {
		t.Errorf("expected failure at step 3, got %d", rc.FailureStep)
	}
	if rc.RootCauseStep != 2 {
		t.Errorf("expected root cause at step 2, got %d", rc.RootCauseStep)
	}
	if rc.RootCauseType != "bad_tool_invocation" {
		t.Errorf("expected 'bad_tool_invocation', got '%s'", rc.RootCauseType)
	}

	// Verify automated repair patch generation
	if len(report.RecommendedPatches) != 1 {
		t.Fatalf("expected 1 repair patch, got %d", len(report.RecommendedPatches))
	}
	patch := report.RecommendedPatches[0]
	if patch.TargetStep != 2 {
		t.Errorf("expected patch target step 2, got %d", patch.TargetStep)
	}
	if patch.Action != "modify_args" {
		t.Errorf("expected 'modify_args' patch action, got '%s'", patch.Action)
	}

	scorecard := causal.RenderCausalScorecard(report)
	if !strings.Contains(scorecard, "Root-Cause Isolations") {
		t.Errorf("expected scorecard to render root cause details")
	}
}
