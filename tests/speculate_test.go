package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/speculate"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func TestSpeculate_CreateAndCommitBranch(t *testing.T) {
	engine := speculate.NewEngine(speculate.DefaultSpeculateConfig())

	snapshot := map[string]interface{}{
		"current_dir": "/app",
		"auth_state":  "valid",
	}

	branch, err := engine.CreateBranch("session-test-spec", 2, snapshot)
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	step1 := speculate.SpeculativeStep{
		StepIndex:        3,
		PredictedType:    string(trace.EventToolCall),
		PredictedPayload: map[string]interface{}{"tool_name": "list_files"},
		ConfidenceScore:  0.92,
		WallClockSavedMs: 450,
		Timestamp:        time.Now(),
	}

	if err := engine.AddSpeculativeStep(branch.BranchID, step1); err != nil {
		t.Fatalf("AddSpeculativeStep failed: %v", err)
	}

	// Ground truth actual events matching the speculative prediction
	actualEvents := []trace.TraceEvent{
		{
			Index:   3,
			Type:    trace.EventToolCall,
			Payload: map[string]interface{}{"tool_name": "list_files"},
		},
	}

	committed, err := engine.CommitOrRollback(branch.BranchID, actualEvents)
	if err != nil {
		t.Fatalf("CommitOrRollback failed: %v", err)
	}

	if !committed {
		t.Errorf("Expected branch to be committed, but was rolled back: %s", branch.RollbackReason)
	}
	if branch.Status != speculate.BranchCommitted {
		t.Errorf("Expected status BranchCommitted, got %s", branch.Status)
	}

	report := engine.GenerateSpeculateReport("session-test-spec")
	if report.CommittedBranches != 1 {
		t.Errorf("Expected 1 committed branch, got %d", report.CommittedBranches)
	}
	if report.NetWallClockSavedMs != 450 {
		t.Errorf("Expected 450ms saved, got %dms", report.NetWallClockSavedMs)
	}
}

func TestSpeculate_MispredictionRollback(t *testing.T) {
	engine := speculate.NewEngine(speculate.DefaultSpeculateConfig())

	branch, err := engine.CreateBranch("session-rollback", 1, nil)
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	step := speculate.SpeculativeStep{
		StepIndex:        2,
		PredictedType:    string(trace.EventToolCall),
		PredictedPayload: map[string]interface{}{"tool_name": "execute_bash"},
		ConfidenceScore:  0.80,
		WallClockSavedMs: 300,
	}

	_ = engine.AddSpeculativeStep(branch.BranchID, step)

	// Actual event diverged: agent chose to write file instead of execute bash
	actualEvents := []trace.TraceEvent{
		{
			Index:   2,
			Type:    trace.EventToolCall,
			Payload: map[string]interface{}{"tool_name": "write_file"},
		},
	}

	committed, err := engine.CommitOrRollback(branch.BranchID, actualEvents)
	if err != nil {
		t.Fatalf("CommitOrRollback failed: %v", err)
	}

	if committed {
		t.Errorf("Expected mispredicted branch to be rolled back")
	}
	if branch.Status != speculate.BranchRolledBack {
		t.Errorf("Expected BranchRolledBack status, got %s", branch.Status)
	}
	if !strings.Contains(branch.RollbackReason, "Tool call mismatch") {
		t.Errorf("Expected tool call mismatch rollback reason, got: %s", branch.RollbackReason)
	}
}

func TestSpeculate_PredictiveCacheAndScorecard(t *testing.T) {
	engine := speculate.NewEngine(speculate.DefaultSpeculateConfig())

	contextHash := "ctx_abc123"
	engine.CachePredictiveBranch(contextHash, "fetch_db", map[string]interface{}{"query": "SELECT 1"})

	entry, found := engine.LookupCache(contextHash)
	if !found {
		t.Fatalf("Expected predictive cache entry to be found")
	}
	if entry.PredictedAction != "fetch_db" {
		t.Errorf("Expected action 'fetch_db', got %s", entry.PredictedAction)
	}

	report := engine.GenerateSpeculateReport("")
	if report.CachedHits == 0 {
		t.Errorf("Expected positive cached hits count")
	}

	card := speculate.RenderSpeculateScorecard(report)
	if !strings.Contains(card, "AGENTVCR ADAPTIVE SPECULATIVE EXECUTION") {
		t.Errorf("Expected scorecard title, got:\n%s", card)
	}
}

func TestASTPredictor_GenerateDraftAndFastTab(t *testing.T) {
	predictor := speculate.NewASTPredictor(speculate.DefaultPredictorConfig())

	ctx := speculate.ASTContext{
		Language:        "go",
		CurrentFunction: "HandleRequest",
		LeadingLines: []string{
			"func HandleRequest(w http.ResponseWriter, r *http.Request) {",
			"\tdata, err := io.ReadAll(r.Body)",
		},
		IndentLevel: 1,
	}

	pos := speculate.CursorPosition{
		FilePath: "server.go",
		Line:     3,
		Column:   1,
	}

	draft, err := predictor.GenerateDraft(ctx, pos)
	if err != nil {
		t.Fatalf("GenerateDraft failed: %v", err)
	}

	if draft.Confidence < 0.70 {
		t.Errorf("expected high confidence score, got %f", draft.Confidence)
	}
	if len(draft.PredictedLines) < 2 {
		t.Fatalf("expected multi-line completion, got %d lines", len(draft.PredictedLines))
	}
	if !strings.Contains(draft.PredictedLines[0], "if err != nil") {
		t.Errorf("expected error checking idiom, got: %s", draft.PredictedLines[0])
	}

	card := speculate.RenderDraftCard(draft)
	if !strings.Contains(card, "Multi-Token AST Edit Draft") {
		t.Errorf("invalid draft card format: %s", card)
	}

	// Validate user accept keystrokes
	acceptedUserLines := []string{
		"\tif err != nil {",
		"\t\treturn nil, fmt.Errorf(\"failed: %w\", err)",
		"\t}",
	}

	res, err := predictor.ValidateDraft(draft.DraftID, acceptedUserLines)
	if err != nil {
		t.Fatalf("ValidateDraft failed: %v", err)
	}

	if !res.Accepted {
		t.Errorf("expected draft to be accepted with high overlap")
	}
	if res.TimeSavedMs <= 0 {
		t.Errorf("expected positive time saved, got %dms", res.TimeSavedMs)
	}
}

func TestASTPredictor_TypeScriptTryCatch(t *testing.T) {
	predictor := speculate.NewASTPredictor(speculate.DefaultPredictorConfig())

	ctx := speculate.ASTContext{
		Language: "typescript",
		LeadingLines: []string{
			"async function fetchData() {",
			"  try {",
		},
		IndentLevel: 1,
	}

	draft, err := predictor.GenerateDraft(ctx, speculate.CursorPosition{FilePath: "index.ts", Line: 3, Column: 1})
	if err != nil {
		t.Fatalf("GenerateDraft TS failed: %v", err)
	}

	if !strings.Contains(draft.PredictedLines[0], "} catch") {
		t.Errorf("expected catch block prediction, got: %s", draft.PredictedLines[0])
	}
}

