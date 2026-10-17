package tests

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/a2a"
)

func TestA2A_Identity_GenerateAndVerify(t *testing.T) {
	im := a2a.NewIdentityManager()

	card, err := im.GenerateIdentity(
		"agent-1",
		"Test Agent 1",
		"Worker",
		"openbot://swarm/agent-1",
		[]a2a.Capability{
			{Name: "text_summarize", Description: "Summarizes documents"},
		},
	)
	if err != nil {
		t.Fatalf("GenerateIdentity failed: %v", err)
	}

	if card.AgentID != "agent-1" || card.PublicKey == "" {
		t.Fatalf("Invalid card generated: %+v", card)
	}

	env := a2a.MessageEnvelope{
		ID:             "env-1",
		ConversationID: "conv-1",
		Type:           a2a.TypeHandshake,
		SenderID:       "agent-1",
		RecipientID:    "agent-2",
		Timestamp:      time.Now(),
		Payload: map[string]interface{}{
			"hello": "world",
		},
	}

	if err := im.SignEnvelope(&env); err != nil {
		t.Fatalf("SignEnvelope failed: %v", err)
	}
	if env.Signature == "" {
		t.Fatalf("Envelope signature is empty")
	}

	if !im.VerifyEnvelope(&env) {
		t.Fatalf("VerifyEnvelope failed on valid signature")
	}

	// Tamper with payload
	env.Payload["hello"] = "tampered_world"
	if im.VerifyEnvelope(&env) {
		t.Fatalf("VerifyEnvelope should fail on tampered payload")
	}
}

func TestA2A_Router_SendAndDeliver(t *testing.T) {
	im := a2a.NewIdentityManager()
	router := a2a.NewRouter(im)

	inboxAlice := router.RegisterInbox("alice", 10)
	inboxBob := router.RegisterInbox("bob", 10)
	_ = inboxAlice

	env := a2a.MessageEnvelope{
		ID:             "msg-1",
		ConversationID: "conv-1",
		Type:           a2a.TypeHeartbeat,
		SenderID:       "alice",
		RecipientID:    "bob",
		Timestamp:      time.Now(),
		Payload: map[string]interface{}{
			"ping": "pong",
		},
	}

	if err := router.Send(env); err != nil {
		t.Fatalf("Router.Send failed: %v", err)
	}

	select {
	case received := <-inboxBob:
		if received.ID != "msg-1" || received.Hops != 1 {
			t.Fatalf("Unexpected message received: %+v", received)
		}
		if received.Checksum == "" {
			t.Fatalf("Expected non-empty checksum on envelope")
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("Timed out waiting for message in Bob's inbox")
	}

	// Test non-existent recipient
	badEnv := a2a.MessageEnvelope{
		ID:          "msg-bad",
		SenderID:    "alice",
		RecipientID: "unknown-agent",
	}
	if err := router.Send(badEnv); err == nil {
		t.Fatalf("Expected error sending to non-existent agent")
	}
}

func TestA2A_Router_DelegateAndReply(t *testing.T) {
	im := a2a.NewIdentityManager()
	router := a2a.NewRouter(im)

	inboxWorker := router.RegisterInbox("worker-1", 10)

	// Background worker routine
	go func() {
		msg := <-inboxWorker
		taskID, ok := msg.Payload["task_id"].(string)
		if !ok {
			return
		}
		res := a2a.DelegationResult{
			TaskID:     taskID,
			Status:     "done",
			Result:     "Code compilation verified",
			Evidence:   "binary generated successfully",
			DurationMs: 25,
		}
		_ = router.ReplyTask("worker-1", msg.SenderID, msg.ConversationID, taskID, res)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	task := a2a.DelegationTask{
		Goal:           "Compile binary",
		Capability:     "build",
		RequiresReply:  true,
		TimeoutSeconds: 2,
	}

	result, err := router.Delegate(ctx, "leader", "worker-1", "conv-delegation", task)
	if err != nil {
		t.Fatalf("Delegation failed: %v", err)
	}

	if result.Status != "done" {
		t.Fatalf("Expected status 'done', got: %s", result.Status)
	}
	if result.Result != "Code compilation verified" {
		t.Fatalf("Unexpected delegation result: %v", result.Result)
	}
}

func TestA2A_Interceptor_RecordTraceEvent(t *testing.T) {
	interceptor := a2a.NewInterceptor(nil)

	env := a2a.MessageEnvelope{
		ID:             "env-tap-1",
		ConversationID: "conv-tap",
		Type:           a2a.TypeDelegate,
		SenderID:       "agent-a",
		RecipientID:    "agent-b",
		Timestamp:      time.Now(),
		Payload: map[string]interface{}{
			"goal": "Process telemetry",
		},
		Checksum: "mock-hash",
	}

	event := interceptor.RecordEnvelope(env)
	if event.Type != "state_change" {
		t.Fatalf("Expected event type state_change, got: %s", event.Type)
	}

	sess := interceptor.Session()
	if len(sess.Events) != 1 {
		t.Fatalf("Expected 1 event recorded, got: %d", len(sess.Events))
	}
	if sess.Events[0].Payload["a2a_id"] != "env-tap-1" {
		t.Fatalf("Unexpected recorded payload: %+v", sess.Events[0].Payload)
	}
}

func TestA2A_ScorecardAndMermaid(t *testing.T) {
	im := a2a.NewIdentityManager()
	router := a2a.NewRouter(im)

	_, _ = im.GenerateIdentity("planner", "Task Planner", "Lead", "openbot://planner", nil)
	_, _ = im.GenerateIdentity("coder", "Code Writer", "Worker", "openbot://coder", nil)

	_ = router.RegisterInbox("coder", 5)

	env := a2a.MessageEnvelope{
		ID:             "msg-10",
		ConversationID: "conv-10",
		Type:           a2a.TypeDelegate,
		SenderID:       "planner",
		RecipientID:    "coder",
		Timestamp:      time.Now(),
		Payload: map[string]interface{}{
			"goal": "Write tests",
		},
	}
	_ = router.Send(env)

	report := router.Audit()
	scorecard := a2a.RenderA2AScorecard(report)
	if !strings.Contains(scorecard, "AGENTVCR // A2A AGENT PROTOCOL") {
		t.Fatalf("Scorecard missing header title")
	}
	if !strings.Contains(scorecard, "planner") || !strings.Contains(scorecard, "coder") {
		t.Fatalf("Scorecard missing registered agents")
	}

	mermaid := a2a.RenderA2AMermaid(report)
	if !strings.Contains(mermaid, "sequenceDiagram") || !strings.Contains(mermaid, "planner->>coder") {
		t.Fatalf("Mermaid sequence diagram missing expected syntax: %s", mermaid)
	}
}
