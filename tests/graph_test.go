package tests

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Noooblien/agentvcr/pkg/graph"
)

func TestGraph_LinearExecutionAndStateReducers(t *testing.T) {
	builder := graph.NewStateGraph()
	builder.AddChannel("task", "", graph.ReducerOverwrite)
	builder.AddChannel("logs", []interface{}{}, graph.ReducerAppendList)
	builder.AddChannel("meta", map[string]interface{}{}, graph.ReducerMergeMap)

	builder.AddNode("planner", func(ctx *graph.GraphContext, state map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{
			"task": "build-api",
			"logs": []interface{}{"planner: decomposed task into endpoints"},
			"meta": map[string]interface{}{"priority": "high"},
		}, nil
	})

	builder.AddNode("executor", func(ctx *graph.GraphContext, state map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{
			"logs": []interface{}{"executor: implemented endpoints in Go"},
			"meta": map[string]interface{}{"lines_of_code": 250},
		}, nil
	})

	builder.SetEntryPoint("planner")
	builder.AddEdge("planner", "executor")
	builder.SetFinishPoint("executor")

	cg, err := builder.Compile(graph.DefaultGraphConfig())
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	trace, err := cg.Invoke("thread-linear-01", map[string]interface{}{"task": "init"})
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}

	if trace.Status != graph.StatusCompleted {
		t.Fatalf("expected status completed, got %s", trace.Status)
	}
	if len(trace.ExecutionPath) != 2 {
		t.Fatalf("expected 2 execution steps, got %d", len(trace.ExecutionPath))
	}
	if trace.FinalState["task"] != "build-api" {
		t.Fatalf("expected task build-api, got %v", trace.FinalState["task"])
	}

	logs, ok := trace.FinalState["logs"].([]interface{})
	if !ok || len(logs) != 2 {
		t.Fatalf("expected 2 log entries from ReducerAppendList, got %v", trace.FinalState["logs"])
	}

	meta, ok := trace.FinalState["meta"].(map[string]interface{})
	if !ok || meta["priority"] != "high" || meta["lines_of_code"] != 250 {
		t.Fatalf("expected merged metadata, got %v", meta)
	}
}

func TestGraph_ConditionalBranchingAndLoopCycle(t *testing.T) {
	builder := graph.NewStateGraph()
	builder.AddChannel("iteration", 0, graph.ReducerOverwrite)
	builder.AddChannel("codeQuality", 30, graph.ReducerOverwrite)
	builder.AddChannel("changelog", []interface{}{}, graph.ReducerAppendList)

	// Node: Coder
	builder.AddNode("coder", func(ctx *graph.GraphContext, state map[string]interface{}) (map[string]interface{}, error) {
		iter := state["iteration"].(int) + 1
		quality := state["codeQuality"].(int) + 25
		return map[string]interface{}{
			"iteration":   iter,
			"codeQuality": quality,
			"changelog":   []interface{}{fmt.Sprintf("coder: refined patch (attempt %d, quality %d)", iter, quality)},
		}, nil
	})

	// Node: Reviewer
	builder.AddNode("reviewer", func(ctx *graph.GraphContext, state map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{
			"changelog": []interface{}{"reviewer: evaluated test suite"},
		}, nil
	})

	// Node: Deployer
	builder.AddNode("deployer", func(ctx *graph.GraphContext, state map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{
			"changelog": []interface{}{"deployer: production rollout successful"},
		}, nil
	})

	builder.SetEntryPoint("coder")
	builder.AddEdge("coder", "reviewer")

	// Conditional routing from reviewer:
	// If codeQuality >= 80 -> "deploy", else "fix"
	builder.AddConditionalEdge("reviewer", func(ctx *graph.GraphContext, state map[string]interface{}) (string, error) {
		quality := state["codeQuality"].(int)
		if quality >= 80 {
			return "deploy", nil
		}
		return "fix", nil
	}, map[string]string{
		"deploy": "deployer",
		"fix":    "coder",
	})

	builder.SetFinishPoint("deployer")

	cg, err := builder.Compile(graph.DefaultGraphConfig())
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	trace, err := cg.Invoke("thread-loop-01", nil)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}

	if trace.Status != graph.StatusCompleted {
		t.Fatalf("expected completed status, got %s (err: %s)", trace.Status, trace.Error)
	}

	// Initial quality: 30.
	// Iter 1: coder -> 55, reviewer -> fix.
	// Iter 2: coder -> 80, reviewer -> deploy.
	// Iter 3: deployer -> END.
	// Path: coder -> reviewer -> coder -> reviewer -> deployer (5 steps)
	if len(trace.ExecutionPath) != 5 {
		t.Fatalf("expected 5 steps in cyclic path, got %d (%v)", len(trace.ExecutionPath), trace.ExecutionPath)
	}
	if trace.CycleCount != 2 {
		t.Fatalf("expected 2 loop cycles, got %d", trace.CycleCount)
	}
	if trace.FinalState["codeQuality"].(int) != 80 {
		t.Fatalf("expected final codeQuality 80, got %v", trace.FinalState["codeQuality"])
	}
}

func TestGraph_RecursionLimitEnforcement(t *testing.T) {
	builder := graph.NewStateGraph()
	builder.AddChannel("counter", 0, graph.ReducerOverwrite)

	builder.AddNode("ping", func(ctx *graph.GraphContext, state map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{"counter": state["counter"].(int) + 1}, nil
	})
	builder.AddNode("pong", func(ctx *graph.GraphContext, state map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{"counter": state["counter"].(int) + 1}, nil
	})

	builder.SetEntryPoint("ping")
	builder.AddEdge("ping", "pong")
	builder.AddEdge("pong", "ping") // Infinite cycle

	cfg := graph.GraphConfig{MaxRecursionLimit: 6}
	cg, err := builder.Compile(cfg)
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	trace, err := cg.Invoke("thread-recursion-01", nil)
	if err == nil {
		t.Fatalf("expected recursion limit error, got nil")
	}
	if trace.Status != graph.StatusRecursionLimitExceeded {
		t.Fatalf("expected StatusRecursionLimitExceeded, got %s", trace.Status)
	}
	if trace.TotalSteps != 6 {
		t.Fatalf("expected exactly 6 steps before limit cut-off, got %d", trace.TotalSteps)
	}
}

func TestGraph_HumanInTheLoopInterruptionAndResume(t *testing.T) {
	builder := graph.NewStateGraph()
	builder.AddChannel("approved", false, graph.ReducerOverwrite)
	builder.AddChannel("deployed", false, graph.ReducerOverwrite)

	builder.AddNode("staging", func(ctx *graph.GraphContext, state map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{"approved": false}, nil
	})

	builder.AddNode("prod_deployer", func(ctx *graph.GraphContext, state map[string]interface{}) (map[string]interface{}, error) {
		if !state["approved"].(bool) {
			return nil, fmt.Errorf("safety violation: deployment requires human approval")
		}
		return map[string]interface{}{"deployed": true}, nil
	})

	builder.SetEntryPoint("staging")
	builder.AddEdge("staging", "prod_deployer")
	builder.SetFinishPoint("prod_deployer")

	// Set InterruptBefore gate on prod_deployer
	cfg := graph.DefaultGraphConfig()
	cfg.InterruptBefore = []string{"prod_deployer"}

	cg, err := builder.Compile(cfg)
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	// 1. First run halts at interrupt gate
	trace1, err := cg.Invoke("thread-hitl-01", nil)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	if trace1.Status != graph.StatusInterrupted {
		t.Fatalf("expected StatusInterrupted, got %s", trace1.Status)
	}
	if trace1.Interrupt == nil || trace1.Interrupt.NodeName != "prod_deployer" {
		t.Fatalf("expected interrupt at prod_deployer, got %v", trace1.Interrupt)
	}

	// 2. Resume execution by injecting state approval
	lastCP := trace1.ActiveCheckpoint
	if lastCP == nil {
		t.Fatalf("expected active checkpoint on interrupted trace")
	}

	trace2, err := cg.Resume("thread-hitl-01", lastCP.CheckpointID, map[string]interface{}{
		"approved": true,
	})
	if err != nil {
		t.Fatalf("Resume failed: %v", err)
	}
	if trace2.Status != graph.StatusCompleted {
		t.Fatalf("expected resume to complete, got %s (err: %s)", trace2.Status, trace2.Error)
	}
	if trace2.FinalState["deployed"] != true {
		t.Fatalf("expected deployed == true in final state, got %v", trace2.FinalState["deployed"])
	}
}

func TestGraph_TimeTravelCheckpointRollback(t *testing.T) {
	builder := graph.NewStateGraph()
	builder.AddChannel("val", 0, graph.ReducerOverwrite)

	builder.AddNode("stepA", func(ctx *graph.GraphContext, state map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{"val": 10}, nil
	})
	builder.AddNode("stepB", func(ctx *graph.GraphContext, state map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{"val": state["val"].(int) * 2}, nil
	})
	builder.AddNode("stepC", func(ctx *graph.GraphContext, state map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{"val": state["val"].(int) + 5}, nil
	})

	builder.SetEntryPoint("stepA")
	builder.AddEdge("stepA", "stepB")
	builder.AddEdge("stepB", "stepC")
	builder.SetFinishPoint("stepC")

	cg, _ := builder.Compile(graph.DefaultGraphConfig())
	trace1, err := cg.Invoke("thread-tt-01", nil)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}

	// Normal run: 10 -> 20 -> 25
	if trace1.FinalState["val"] != 25 {
		t.Fatalf("expected 25, got %v", trace1.FinalState["val"])
	}

	// Find step 1 checkpoint (after stepA, before stepB)
	var cpA *graph.Checkpoint
	for _, cp := range trace1.Checkpoints {
		if cp.NodeName == "stepA" {
			cpA = cp
			break
		}
	}
	if cpA == nil {
		t.Fatalf("stepA checkpoint not found")
	}

	// Time-travel branch: mutate val to 50 at stepA, re-run forward
	traceBranch, err := cg.Resume("thread-tt-01", cpA.CheckpointID, map[string]interface{}{
		"val": 50,
	})
	if err != nil {
		t.Fatalf("Resume branch failed: %v", err)
	}

	// Branched run: 50 -> 100 -> 105
	if traceBranch.FinalState["val"] != 105 {
		t.Fatalf("expected branched val 105, got %v", traceBranch.FinalState["val"])
	}
}

func TestGraph_MermaidExportAndScorecard(t *testing.T) {
	builder := graph.NewStateGraph()
	builder.AddChannel("status", "init", graph.ReducerOverwrite)

	builder.AddNode("agent1", func(ctx *graph.GraphContext, state map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{"status": "ready"}, nil
	})
	builder.SetEntryPoint("agent1")
	builder.SetFinishPoint("agent1")

	cg, _ := builder.Compile(graph.DefaultGraphConfig())
	trace, _ := cg.Invoke("thread-mermaid", nil)

	mermaid := cg.ExportMermaid(trace)
	if !strings.Contains(mermaid, "flowchart TD") || !strings.Contains(mermaid, "agent1") {
		t.Fatalf("unexpected mermaid output: %s", mermaid)
	}

	scorecard := graph.RenderGraphScorecard(trace)
	if !strings.Contains(scorecard, "STATEGRAPH WORKFLOW & DETERMINISTIC TIME-TRAVEL ENGINE") {
		t.Fatalf("unexpected scorecard: %s", scorecard)
	}
	if !strings.Contains(scorecard, "PASS") {
		t.Fatalf("expected PASS in scorecard: %s", scorecard)
	}
}
