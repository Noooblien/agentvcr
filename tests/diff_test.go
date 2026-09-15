package tests_test

import (
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/diff"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func TestCompareIdenticalSessions(t *testing.T) {
	sessionA := &trace.Session{
		ID:        "session_a",
		AgentName: "bot",
		Events: []trace.TraceEvent{
			{Index: 0, Type: trace.EventLLMRequest, Payload: map[string]interface{}{"msg": "hello"}},
			{Index: 1, Type: trace.EventLLMResponse, Payload: map[string]interface{}{"msg": "hi"}},
		},
	}
	sessionB := &trace.Session{
		ID:        "session_b",
		AgentName: "bot",
		Events: []trace.TraceEvent{
			{Index: 0, Type: trace.EventLLMRequest, Payload: map[string]interface{}{"msg": "hello"}},
			{Index: 1, Type: trace.EventLLMResponse, Payload: map[string]interface{}{"msg": "hi"}},
		},
	}

	res := diff.CompareSessions(sessionA, sessionB)
	if res.DivergenceStep != -1 {
		t.Errorf("expected no divergence (-1), got %d", res.DivergenceStep)
	}
}

func TestCompareDivergentSessions(t *testing.T) {
	now := time.Now()
	sessionBase := &trace.Session{
		ID:        "base_run",
		AgentName: "code-gen",
		Events: []trace.TraceEvent{
			{Index: 0, Type: trace.EventLLMRequest, Timestamp: now, Payload: map[string]interface{}{"p": "write test"}},
			{Index: 1, Type: trace.EventToolCall, Timestamp: now, Payload: map[string]interface{}{"cmd": "rm -rf /"}},
		},
	}

	sessionFork := &trace.Session{
		ID:        "fork_run",
		AgentName: "code-gen",
		Events: []trace.TraceEvent{
			{Index: 0, Type: trace.EventLLMRequest, Timestamp: now, Payload: map[string]interface{}{"p": "write test"}},
			{Index: 1, Type: trace.EventToolCall, Timestamp: now, Payload: map[string]interface{}{"cmd": "go test ./..."}},
		},
	}

	res := diff.CompareSessions(sessionBase, sessionFork)
	if res.DivergenceStep != 1 {
		t.Fatalf("expected divergence at step 1, got %d", res.DivergenceStep)
	}

	if res.DivergentDetails == nil || res.DivergentDetails.StepIndex != 1 {
		t.Errorf("expected detailed divergence report at step 1")
	}
}
