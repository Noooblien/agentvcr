package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/harness"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func TestHarness_ComputePassAtK(t *testing.T) {
	// Known mathematical cases for pass@k:
	// Case 1: n = 10, c = 10 (all correct) -> pass@1 = 1.0, pass@5 = 1.0
	if p := harness.ComputePassAtK(10, 10, 1); p != 1.0 {
		t.Errorf("Expected 1.0, got %f", p)
	}

	// Case 2: n = 10, c = 0 (none correct) -> pass@1 = 0.0, pass@5 = 0.0
	if p := harness.ComputePassAtK(10, 0, 1); p != 0.0 {
		t.Errorf("Expected 0.0, got %f", p)
	}

	// Case 3: n = 10, c = 1 -> pass@1 = 0.10
	if p := harness.ComputePassAtK(10, 1, 1); p < 0.099 || p > 0.101 {
		t.Errorf("Expected ~0.10, got %f", p)
	}

	// Case 4: n = 10, c = 5, k = 1 -> pass@1 = 0.50
	if p := harness.ComputePassAtK(10, 5, 1); p < 0.499 || p > 0.501 {
		t.Errorf("Expected ~0.50, got %f", p)
	}

	// Case 5: k > n -> returns 0.0
	if p := harness.ComputePassAtK(5, 3, 10); p != 0.0 {
		t.Errorf("Expected 0.0 for k > n, got %f", p)
	}
}

func TestHarness_TaskRegistrationAndEvaluation(t *testing.T) {
	cfg := harness.DefaultHarnessConfig()
	cfg.Standard = harness.BenchmarkSWEBench
	cfg.MinPassRateAt1 = 0.50
	engine := harness.NewEngine(cfg)

	task := harness.TaskSpec{
		TaskID:          "django__django-14016",
		Standard:        harness.BenchmarkSWEBench,
		Repository:      "django/django",
		ProblemStatement: "TypeError: 'dict_keys' object is not subscriptable in Q() expressions",
		FailToPassTests: []string{"test_q_dict_keys_subscript"},
		PassToPassTests: []string{"test_q_basic"},
		MaxStepsAllowed: 15,
	}
	engine.RegisterTask(task)

	retrieved, exists := engine.GetTask("django__django-14016")
	if !exists || retrieved.Repository != "django/django" {
		t.Fatalf("Task registration failed")
	}

	now := time.Now()

	// Trial 1: Successful patch resolution
	sess1 := &trace.Session{
		ID:        "trial-01",
		AgentName: "swe-agent-gpt4o",
		Status:    "completed",
		Events: []trace.TraceEvent{
			{
				ID: "e1-0", Index: 0, Type: trace.EventLLMRequest, Timestamp: now,
				Payload: map[string]interface{}{"prompt": "Fix dict_keys in Q expressions", "tokens": 150},
			},
			{
				ID: "e1-1", Index: 1, Type: trace.EventToolCall, Timestamp: now.Add(time.Second),
				Payload: map[string]interface{}{"tool": "patch", "patch": "diff --git a/django/db/models/query_utils.py b/django/db/models/query_utils.py\n+ list(keys)[0]"},
			},
			{
				ID: "e1-2", Index: 2, Type: trace.EventLLMResponse, Timestamp: now.Add(2 * time.Second),
				Payload: map[string]interface{}{"content": "Fixed Q object keys indexing by converting to list.", "tokens": 80},
			},
		},
	}

	// Trial 2: Successful patch resolution
	sess2 := &trace.Session{
		ID:        "trial-02",
		AgentName: "swe-agent-gpt4o",
		Status:    "completed",
		Events: []trace.TraceEvent{
			{
				ID: "e2-0", Index: 0, Type: trace.EventLLMRequest, Timestamp: now,
				Payload: map[string]interface{}{"prompt": "Fix dict_keys in Q expressions", "tokens": 140},
			},
			{
				ID: "e2-1", Index: 1, Type: trace.EventToolCall, Timestamp: now.Add(time.Second),
				Payload: map[string]interface{}{"tool": "patch", "patch": "diff --git a/django/db/models/query_utils.py\n+ [k for k in keys][0]"},
			},
			{
				ID: "e2-2", Index: 2, Type: trace.EventLLMResponse, Timestamp: now.Add(2 * time.Second),
				Payload: map[string]interface{}{"content": "Fixed Q keys indexing.", "tokens": 75},
			},
		},
	}

	// Trial 3: Failed with tool error
	sess3 := &trace.Session{
		ID:        "trial-03",
		AgentName: "swe-agent-gpt4o",
		Status:    "failed",
		Events: []trace.TraceEvent{
			{
				ID: "e3-0", Index: 0, Type: trace.EventLLMRequest, Timestamp: now,
				Payload: map[string]interface{}{"prompt": "Fix dict_keys in Q expressions", "tokens": 120},
			},
			{
				ID: "e3-1", Index: 1, Type: trace.EventError, Timestamp: now.Add(time.Second),
				Payload: map[string]interface{}{"error": "SyntaxError: invalid syntax", "is_error": true},
			},
		},
	}

	agg := engine.EvaluateTask(task, []*trace.Session{sess1, sess2, sess3})
	if agg.TotalTrials != 3 {
		t.Errorf("Expected 3 trials, got %d", agg.TotalTrials)
	}
	if agg.PassedTrials != 2 {
		t.Errorf("Expected 2 passed trials, got %d", agg.PassedTrials)
	}
	if agg.PassAt1 < 0.65 || agg.PassAt1 > 0.68 {
		t.Errorf("Expected Pass@1 ~0.667, got %f", agg.PassAt1)
	}
}

func TestHarness_RunBenchmarkSuiteAndScorecard(t *testing.T) {
	cfg := harness.DefaultHarnessConfig()
	cfg.Standard = harness.BenchmarkHumanEval
	cfg.MinPassRateAt1 = 0.50
	engine := harness.NewEngine(cfg)

	now := time.Now()
	task1 := harness.TaskSpec{
		TaskID:          "HumanEval/0",
		Standard:        harness.BenchmarkHumanEval,
		ProblemStatement: "Check if in given list of numbers, any two numbers are closer to each other than given threshold.",
		FailToPassTests: []string{"test_has_close_elements"},
	}
	task2 := harness.TaskSpec{
		TaskID:          "HumanEval/1",
		Standard:        harness.BenchmarkHumanEval,
		ProblemStatement: "Separate a string of parenthetical groups into separate strings.",
		FailToPassTests: []string{"test_separate_paren_groups"},
	}
	engine.RegisterTask(task1)
	engine.RegisterTask(task2)

	sessPass := &trace.Session{
		ID: "pass-sess", AgentName: "agent-eval", Status: "completed",
		Events: []trace.TraceEvent{
			{ID: "p-0", Type: trace.EventLLMRequest, Timestamp: now, Payload: map[string]interface{}{"prompt": "Solve task", "tokens": 100}},
			{ID: "p-1", Type: trace.EventLLMResponse, Timestamp: now.Add(time.Second), Payload: map[string]interface{}{"content": "def solution(): return True", "tokens": 80}},
		},
	}
	sessFail := &trace.Session{
		ID: "fail-sess", AgentName: "agent-eval", Status: "failed",
		Events: []trace.TraceEvent{
			{ID: "f-0", Type: trace.EventError, Timestamp: now, Payload: map[string]interface{}{"error": "Timeout"}},
		},
	}

	trials := map[string][]*trace.Session{
		"HumanEval/0": {sessPass, sessPass},
		"HumanEval/1": {sessPass, sessFail},
	}

	report := engine.RunBenchmark(trials)
	if report.TotalTasks != 2 {
		t.Errorf("Expected 2 tasks, got %d", report.TotalTasks)
	}
	if !report.GatePassed {
		t.Errorf("Expected benchmark gate to pass (Mean Pass@1: %f)", report.MeanPassAt1)
	}

	scorecard := harness.RenderHarnessScorecard(report)
	if !strings.Contains(scorecard, "STANDARDIZED BENCHMARK HARNESS") {
		t.Errorf("Scorecard header missing: %s", scorecard)
	}

	mermaid := harness.RenderHarnessMermaid(report)
	if !strings.Contains(mermaid, "graph TD") {
		t.Errorf("Mermaid chart missing: %s", mermaid)
	}
}
