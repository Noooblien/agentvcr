package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/heal"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func TestHeal_PristineTrajectoryZeroFaults(t *testing.T) {
	session := &trace.Session{
		ID:        "session_heal_pristine",
		AgentName: "healthy-agent",
		CreatedAt: time.Now().UTC(),
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventLLMRequest, Payload: map[string]interface{}{"prompt": "hello"}},
			{Index: 2, Type: trace.EventLLMResponse, Payload: map[string]interface{}{"content": "world"}},
		},
	}

	healer := heal.NewHealer(true)
	healedSession, report, err := healer.HealSession(session)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.FaultsDetected != 0 {
		t.Errorf("expected 0 faults, got %d", report.FaultsDetected)
	}
	if report.PatchesSynthesized != 0 {
		t.Errorf("expected 0 patches, got %d", report.PatchesSynthesized)
	}
	if !report.AutoAppliedCleanly {
		t.Errorf("expected clean report")
	}
	if len(healedSession.Events) != 2 {
		t.Errorf("expected 2 events in healed session")
	}
}

func TestHeal_SyntaxErrorAndMalformedJSONRepair(t *testing.T) {
	session := &trace.Session{
		ID:        "session_heal_syntax",
		AgentName: "buggy-json-agent",
		CreatedAt: time.Now().UTC(),
		Events: []trace.TraceEvent{
			{
				Index: 1,
				Type:  trace.EventToolCall,
				Payload: map[string]interface{}{
					"tool":      "calculator",
					"arguments": "{'expression': '2+2',}", // Invalid JSON: single quotes and trailing comma
				},
			},
		},
	}

	healer := heal.NewHealer(true)
	healedSession, report, err := healer.HealSession(session)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.FaultsDetected != 1 {
		t.Fatalf("expected 1 fault, got %d", report.FaultsDetected)
	}
	if report.Faults[0].Strategy != heal.StrategySyntaxRepair {
		t.Errorf("expected StrategySyntaxRepair, got %s", report.Faults[0].Strategy)
	}
	if report.PatchesSynthesized != 1 {
		t.Errorf("expected 1 patch, got %d", report.PatchesSynthesized)
	}

	repairedArgs := healedSession.Events[0].Payload["arguments"].(string)
	if !strings.Contains(repairedArgs, "\"expression\"") {
		t.Errorf("expected double-quoted keys, got: %s", repairedArgs)
	}
}

func TestHeal_ToolFailureAndDependencyBootstrap(t *testing.T) {
	session := &trace.Session{
		ID:        "session_heal_missing_dep",
		AgentName: "bash-agent",
		CreatedAt: time.Now().UTC(),
		Events: []trace.TraceEvent{
			{
				Index: 1,
				Type:  trace.EventToolCall,
				Payload: map[string]interface{}{
					"tool":    "bash",
					"command": "python3 script.py",
				},
			},
			{
				Index: 2,
				Type:  trace.EventToolResult,
				Payload: map[string]interface{}{
					"success":   false,
					"exit_code": float64(127),
					"stderr":    "bash: python3: ModuleNotFoundError: No module named 'setuptools'",
				},
			},
			{
				Index: 3,
				Type:  trace.EventError,
				Payload: map[string]interface{}{
					"error": "script crashed due to missing module",
				},
			},
		},
	}

	healer := heal.NewHealer(true)
	healedSession, report, err := healer.HealSession(session)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.FaultsDetected != 2 {
		t.Errorf("expected 2 faults, got %d", report.FaultsDetected)
	}

	// Verify EventError was transformed into resolved state change
	lastEv := healedSession.Events[2]
	if lastEv.Type != trace.EventStateChange {
		t.Errorf("expected EventStateChange after healing, got: %s", lastEv.Type)
	}
	if lastEv.Payload["healed_status"] != "resolved_by_patch" {
		t.Errorf("expected resolved_by_patch status")
	}
}

func TestHeal_ScorecardRendering(t *testing.T) {
	session := &trace.Session{
		ID:        "session_heal_card",
		AgentName: "scorecard-agent",
		CreatedAt: time.Now().UTC(),
		Events: []trace.TraceEvent{
			{
				Index: 1,
				Type:  trace.EventError,
				Payload: map[string]interface{}{
					"error": "connection timeout after 30000ms",
				},
			},
		},
	}

	healer := heal.NewHealer(true)
	_, report, err := healer.HealSession(session)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	card := heal.RenderHealScorecard(report)
	if !strings.Contains(card, "AGENTVCR AUTONOMOUS TRAJECTORY PATCH SYNTHESIZER") {
		t.Errorf("expected title in scorecard")
	}
	if !strings.Contains(card, "session_heal_card") {
		t.Errorf("expected session ID in scorecard")
	}
	if !strings.Contains(card, "retry_with_backoff") {
		t.Errorf("expected retry strategy in scorecard")
	}
}
