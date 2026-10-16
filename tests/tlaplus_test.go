package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/tlaplus"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func TestTLAPlus_CompliantSession_FormallyVerified(t *testing.T) {
	session := &trace.Session{
		ID:        "session_tla_compliant",
		CreatedAt: time.Now(),
		Events: []trace.TraceEvent{
			{
				ID:    "ev_1",
				Index: 1,
				Type:  trace.EventToolCall,
				Payload: map[string]interface{}{
					"tool_name": "read_data",
					"args":      "config.json",
				},
			},
			{
				ID:    "ev_2",
				Index: 2,
				Type:  trace.EventLLMResponse,
				Payload: map[string]interface{}{
					"content": "Data processed successfully.",
				},
			},
		},
	}

	checker := tlaplus.DefaultSafetySuite()
	report, err := checker.VerifySession(session)
	if err != nil {
		t.Fatalf("VerifySession failed: %v", err)
	}

	if !report.FormallyVerified {
		t.Errorf("Expected session to be formally verified, but got violations: %v", report.Violations)
	}
	if report.InvariantsViolated != 0 {
		t.Errorf("Expected 0 violations, got %d", report.InvariantsViolated)
	}
	if !strings.Contains(report.GeneratedTLASpec, "MODULE AgentVCR_Trace") {
		t.Errorf("Expected valid TLA+ specification to be synthesized")
	}
}

func TestTLAPlus_RetryExceeded_SafetyViolation(t *testing.T) {
	session := &trace.Session{
		ID:        "session_tla_retry_fail",
		CreatedAt: time.Now(),
		Events: []trace.TraceEvent{
			{ID: "ev_1", Index: 1, Type: trace.EventError, Payload: map[string]interface{}{"err": "fail 1"}},
			{ID: "ev_2", Index: 2, Type: trace.EventError, Payload: map[string]interface{}{"err": "fail 2"}},
			{ID: "ev_3", Index: 3, Type: trace.EventError, Payload: map[string]interface{}{"err": "fail 3"}},
			{ID: "ev_4", Index: 4, Type: trace.EventError, Payload: map[string]interface{}{"err": "fail 4"}},
		},
	}

	checker := tlaplus.DefaultSafetySuite()
	report, err := checker.VerifySession(session)
	if err != nil {
		t.Fatalf("VerifySession failed: %v", err)
	}

	if report.FormallyVerified {
		t.Errorf("Expected session to fail verification due to retry threshold")
	}

	foundRetryViolation := false
	for _, v := range report.Violations {
		if v.InvariantID == "INV-BOUNDED-RETRIES" {
			foundRetryViolation = true
			if len(v.CounterexampleTrace) == 0 {
				t.Errorf("Expected counterexample trace in violation")
			}
		}
	}

	if !foundRetryViolation {
		t.Errorf("Expected INV-BOUNDED-RETRIES violation in report")
	}
}

func TestTLAPlus_Leadsto_UnreleasedLockViolation(t *testing.T) {
	session := &trace.Session{
		ID:        "session_tla_lock_deadlock",
		CreatedAt: time.Now(),
		Events: []trace.TraceEvent{
			{
				ID:    "ev_1",
				Index: 1,
				Type:  trace.EventToolCall,
				Payload: map[string]interface{}{
					"tool_name": "acquire_lock",
					"resource":  "db_mutex",
				},
			},
		},
	}

	// Create custom checker where lock is not automatically freed at exit to test leadsto refutation
	invariants := []tlaplus.TemporalInvariant{
		{
			ID:          "INV-TEST-LEADSTO-FAIL",
			Name:        "Lock Release Required",
			Operator:    tlaplus.OpLeadsto,
			TriggerPredicate: &tlaplus.StatePredicate{
				Variable:  "active_tool",
				Operator:  "eq",
				TargetVal: "acquire_lock",
			},
			TargetPredicate: tlaplus.StatePredicate{
				Variable:  "active_tool",
				Operator:  "eq",
				TargetVal: "release_lock",
			},
		},
	}
	checker := tlaplus.NewModelChecker(invariants, nil)
	report, err := checker.VerifySession(session)
	if err != nil {
		t.Fatalf("VerifySession failed: %v", err)
	}

	if report.FormallyVerified {
		t.Errorf("Expected leadsto invariant violation when release_lock never called")
	}
}

func TestTLAPlus_ScorecardRendering(t *testing.T) {
	session := &trace.Session{
		ID:        "session_scorecard_tla",
		CreatedAt: time.Now(),
		Events: []trace.TraceEvent{
			{ID: "ev_1", Index: 1, Type: trace.EventToolCall, Payload: map[string]interface{}{"tool": "compute"}},
		},
	}

	checker := tlaplus.DefaultSafetySuite()
	report, err := checker.VerifySession(session)
	if err != nil {
		t.Fatalf("VerifySession failed: %v", err)
	}

	scorecard := tlaplus.RenderTLACheckScorecard(report)
	if !strings.Contains(scorecard, "AGENTVCR FORMALLY VERIFIED STATE-MACHINE TEMPORAL LOGIC MODEL CHECKER") {
		t.Errorf("Scorecard missing header")
	}
	if !strings.Contains(scorecard, "FORMAL TLA+ SPECIFICATION PREVIEW") {
		t.Errorf("Scorecard missing TLA spec preview")
	}
}
