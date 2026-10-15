package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/fuzz"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func createSampleSession() *trace.Session {
	now := time.Now()
	return &trace.Session{
		ID:        "baseline-session",
		AgentName: "code-debugger",
		Status:    "completed",
		Events: []trace.TraceEvent{
			{
				Index:      1,
				Timestamp:  now,
				Type:       trace.EventLLMRequest,
				DurationMs: 12,
				Payload: map[string]interface{}{
					"model":  "gpt-4o",
					"prompt": "Find and fix bugs in file main.go",
				},
			},
			{
				Index:      2,
				Timestamp:  now.Add(1 * time.Second),
				Type:       trace.EventLLMResponse,
				DurationMs: 400,
				Payload: map[string]interface{}{
					"content": "I will inspect main.go",
				},
			},
			{
				Index:      3,
				Timestamp:  now.Add(2 * time.Second),
				Type:       trace.EventToolCall,
				DurationMs: 50,
				Payload: map[string]interface{}{
					"tool_name": "read_file",
					"path":      "main.go",
				},
			},
			{
				Index:      4,
				Timestamp:  now.Add(3 * time.Second),
				Type:       trace.EventToolResult,
				DurationMs: 65,
				Payload: map[string]interface{}{
					"status":  "success",
					"content": "package main\nfunc main() {}\n",
				},
			},
		},
	}
}

func TestFuzz_PromptInjection(t *testing.T) {
	harness := fuzz.NewFuzzHarness(42)
	session := createSampleSession()

	fuzzed, report, err := harness.FuzzSession(session, fuzz.StrategyPromptInjection)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fuzzed.ID == session.ID {
		t.Errorf("fuzzed session must have a unique ID")
	}
	if fuzzed.ParentID != session.ID {
		t.Errorf("expected parent ID %s, got %s", session.ID, fuzzed.ParentID)
	}

	prompt := fuzzed.Events[0].Payload["prompt"].(string)
	if !strings.Contains(prompt, "Find and fix bugs") {
		t.Errorf("original prompt content preserved: %s", prompt)
	}
	if fuzzed.Events[0].Payload["adversarial_injection"] == nil {
		t.Errorf("expected adversarial_injection field to be set")
	}

	if report.Strategy != fuzz.StrategyPromptInjection {
		t.Errorf("report strategy mismatch: %s", report.Strategy)
	}
}

func TestFuzz_ToolFault(t *testing.T) {
	harness := fuzz.NewFuzzHarness(42)
	session := createSampleSession()

	fuzzed, report, err := harness.FuzzSession(session, fuzz.StrategyToolFault)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Step 4 was ToolResult
	toolResultPayload := fuzzed.Events[3].Payload
	if toolResultPayload["error"] == nil {
		t.Errorf("expected fault injection in tool result, got: %+v", toolResultPayload)
	}
	if report.MutatedStep != 4 {
		t.Errorf("expected step 4 to be mutated, got %d", report.MutatedStep)
	}
}

func TestFuzz_SuiteGeneration(t *testing.T) {
	harness := fuzz.NewFuzzHarness(1337)
	session := createSampleSession()

	sessions, reports, err := harness.GenerateSuite(session, 4)
	if err != nil {
		t.Fatalf("failed to generate fuzz suite: %v", err)
	}

	if len(sessions) != 4 || len(reports) != 4 {
		t.Fatalf("expected 4 mutated sessions and reports, got %d and %d", len(sessions), len(reports))
	}

	// Verify each mutation has valid report
	for i, rep := range reports {
		if rep.FuzzedSessionID != sessions[i].ID {
			t.Errorf("report session ID mismatch: %s vs %s", rep.FuzzedSessionID, sessions[i].ID)
		}
		if rep.Description == "" {
			t.Errorf("report description should not be empty")
		}
	}
}
