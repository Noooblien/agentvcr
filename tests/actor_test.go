package tests

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/actor"
)

func TestActor_LifecycleAndTell(t *testing.T) {
	sys := actor.NewActorSystem("test-sys-01")
	defer sys.Shutdown()

	var counter int64

	echoBehavior := func(ctx *actor.ActorContext, msg actor.Message) error {
		atomic.AddInt64(&counter, 1)
		return nil
	}

	cell, err := sys.SpawnActor("echo-1", "worker", echoBehavior, actor.DefaultMailboxConfig(), nil)
	if err != nil {
		t.Fatalf("failed to spawn actor: %v", err)
	}

	for i := 0; i < 5; i++ {
		err := sys.Send(actor.Message{
			ID:        "msg",
			Sender:    "client",
			Recipient: "echo-1",
			Priority:  actor.PriorityNormal,
			Payload:   map[string]interface{}{"val": i},
		})
		if err != nil {
			t.Fatalf("send failed: %v", err)
		}
	}

	// Wait briefly for execution
	time.Sleep(30 * time.Millisecond)

	if atomic.LoadInt64(&counter) != 5 {
		t.Fatalf("expected 5 messages processed, got %d", atomic.LoadInt64(&counter))
	}

	telem := cell.Telemetry()
	if telem.ProcessedCount != 5 {
		t.Fatalf("expected telemetry processed count 5, got %d", telem.ProcessedCount)
	}
}

func TestActor_Ask_Synchronous(t *testing.T) {
	sys := actor.NewActorSystem("test-sys-ask")
	defer sys.Shutdown()

	responder := func(ctx *actor.ActorContext, msg actor.Message) error {
		reqNum, _ := msg.Payload["num"].(int)
		return ctx.Tell(ctx.Sender(), map[string]interface{}{
			"squared": reqNum * reqNum,
		})
	}

	_, _ = sys.SpawnActor("math-worker", "calculator", responder, actor.DefaultMailboxConfig(), nil)

	resp, err := sys.Ask("client", "math-worker", map[string]interface{}{"num": 7}, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("ask request failed: %v", err)
	}

	squared, ok := resp.Payload["squared"].(int)
	if !ok || squared != 49 {
		t.Fatalf("expected squared value 49, got %v", resp.Payload["squared"])
	}
}

func TestActor_Mailbox_PriorityAndDeadLetters(t *testing.T) {
	sys := actor.NewActorSystem("test-sys-mbox")
	defer sys.Shutdown()

	// 1. Dead letter when sending to non-existent actor
	err := sys.Send(actor.Message{
		ID:        "lost-1",
		Sender:    "client",
		Recipient: "phantom-actor",
		Priority:  actor.PriorityNormal,
	})
	if err == nil {
		t.Fatalf("expected error sending to phantom actor")
	}

	if sys.DeadLetters().Count() != 1 {
		t.Fatalf("expected 1 dead letter, got %d", sys.DeadLetters().Count())
	}

	// 2. Priority Ordering in Mailbox
	mbox := actor.NewMailbox(actor.MailboxConfig{Capacity: 10, DropPolicy: actor.DropToDeadLett}, sys.DeadLetters())
	mbox.Push(actor.Message{ID: "low-1", Priority: actor.PriorityLow})
	mbox.Push(actor.Message{ID: "high-1", Priority: actor.PriorityHigh})
	mbox.Push(actor.Message{ID: "normal-1", Priority: actor.PriorityNormal})

	first, _ := mbox.Pop()
	if first.ID != "high-1" {
		t.Fatalf("expected high priority message first, got %s", first.ID)
	}

	second, _ := mbox.Pop()
	if second.ID != "normal-1" {
		t.Fatalf("expected normal priority message second, got %s", second.ID)
	}

	third, _ := mbox.Pop()
	if third.ID != "low-1" {
		t.Fatalf("expected low priority message third, got %s", third.ID)
	}
}

func TestActor_Supervision_OneForOne(t *testing.T) {
	sys := actor.NewActorSystem("test-sys-sup-1")
	defer sys.Shutdown()

	sup := sys.SpawnSupervisor("sup-1", actor.StrategyOneForOne, actor.DefaultRestartPolicy())

	failBehavior := func(ctx *actor.ActorContext, msg actor.Message) error {
		if _, shouldCrash := msg.Payload["crash"]; shouldCrash {
			panic("intentional test panic")
		}
		return nil
	}

	healthyBehavior := func(ctx *actor.ActorContext, msg actor.Message) error {
		return nil
	}

	child1, _ := sys.SpawnActor("worker-flaky", "flaky", failBehavior, actor.DefaultMailboxConfig(), sup)
	child2, _ := sys.SpawnActor("worker-stable", "stable", healthyBehavior, actor.DefaultMailboxConfig(), sup)

	// Trigger panic on child1
	_ = sys.Send(actor.Message{
		ID:        "msg-crash",
		Recipient: "worker-flaky",
		Payload:   map[string]interface{}{"crash": true},
	})

	time.Sleep(50 * time.Millisecond)

	telem1 := child1.Telemetry()
	telem2 := child2.Telemetry()

	if telem1.RestartsCount != 1 {
		t.Fatalf("expected child1 to restart once, got %d", telem1.RestartsCount)
	}
	if telem2.RestartsCount != 0 {
		t.Fatalf("expected child2 not to restart under OneForOne, got %d", telem2.RestartsCount)
	}
}

func TestActor_Supervision_OneForAll(t *testing.T) {
	sys := actor.NewActorSystem("test-sys-sup-all")
	defer sys.Shutdown()

	sup := sys.SpawnSupervisor("sup-all", actor.StrategyOneForAll, actor.DefaultRestartPolicy())

	failBehavior := func(ctx *actor.ActorContext, msg actor.Message) error {
		if _, crash := msg.Payload["crash"]; crash {
			return errors.New("simulated error")
		}
		return nil
	}
	noopBehavior := func(ctx *actor.ActorContext, msg actor.Message) error { return nil }

	child1, _ := sys.SpawnActor("child-a", "worker", failBehavior, actor.DefaultMailboxConfig(), sup)
	child2, _ := sys.SpawnActor("child-b", "worker", noopBehavior, actor.DefaultMailboxConfig(), sup)

	_ = sys.Send(actor.Message{
		ID:        "msg-err",
		Recipient: "child-a",
		Payload:   map[string]interface{}{"crash": true},
	})

	time.Sleep(50 * time.Millisecond)

	telem1 := child1.Telemetry()
	telem2 := child2.Telemetry()

	if telem1.RestartsCount != 1 || telem2.RestartsCount != 1 {
		t.Fatalf("expected both children to restart under OneForAll, got %d and %d", telem1.RestartsCount, telem2.RestartsCount)
	}
}

func TestActor_Supervision_RestForOne(t *testing.T) {
	sys := actor.NewActorSystem("test-sys-rest")
	defer sys.Shutdown()

	sup := sys.SpawnSupervisor("sup-rest", actor.StrategyRestForOne, actor.DefaultRestartPolicy())

	noop := func(ctx *actor.ActorContext, msg actor.Message) error {
		if _, crash := msg.Payload["crash"]; crash {
			return errors.New("boom")
		}
		return nil
	}

	c1, _ := sys.SpawnActor("actor-1", "worker", noop, actor.DefaultMailboxConfig(), sup)
	c2, _ := sys.SpawnActor("actor-2", "worker", noop, actor.DefaultMailboxConfig(), sup)
	c3, _ := sys.SpawnActor("actor-3", "worker", noop, actor.DefaultMailboxConfig(), sup)

	// Crash actor-2: actor-2 and actor-3 should restart, actor-1 should NOT restart
	_ = sys.Send(actor.Message{Recipient: "actor-2", Payload: map[string]interface{}{"crash": true}})

	time.Sleep(50 * time.Millisecond)

	if c1.Telemetry().RestartsCount != 0 {
		t.Fatalf("actor-1 should NOT restart")
	}
	if c2.Telemetry().RestartsCount != 1 {
		t.Fatalf("actor-2 should restart once")
	}
	if c3.Telemetry().RestartsCount != 1 {
		t.Fatalf("actor-3 should restart once under RestForOne")
	}
}

func TestActor_Supervision_Escalation(t *testing.T) {
	sys := actor.NewActorSystem("test-sys-esc")
	defer sys.Shutdown()

	policy := actor.RestartPolicy{
		MaxRestarts: 2,
		TimeWindow:  5 * time.Second,
		Backoff:     1 * time.Millisecond,
	}
	sup := sys.SpawnSupervisor("sup-esc", actor.StrategyOneForOne, policy)

	crasher := func(ctx *actor.ActorContext, msg actor.Message) error {
		return errors.New("persistent fault")
	}

	cell, _ := sys.SpawnActor("flaky-max", "worker", crasher, actor.DefaultMailboxConfig(), sup)

	// Trigger 3 failures in a row (exceeding MaxRestarts = 2)
	for i := 0; i < 3; i++ {
		_ = sys.Send(actor.Message{Recipient: "flaky-max", Payload: map[string]interface{}{"run": i}})
		time.Sleep(10 * time.Millisecond)
	}

	if !sup.Escalated() {
		t.Fatalf("supervisor should have escalated failure")
	}

	telem := cell.Telemetry()
	if telem.Status != actor.StatusTerminated {
		t.Fatalf("actor should be terminated after exceeding restart budget, got %s", telem.Status)
	}
}

func TestActor_ScorecardAndMermaid(t *testing.T) {
	sys := actor.NewActorSystem("sys-audit")
	defer sys.Shutdown()

	sup := sys.SpawnSupervisor("root-sup", actor.StrategyOneForOne, actor.DefaultRestartPolicy())
	_, _ = sys.SpawnActor("lead-agent", "orchestrator", func(ctx *actor.ActorContext, msg actor.Message) error { return nil }, actor.DefaultMailboxConfig(), sup)

	report := sys.Audit()
	if report.TotalActors != 1 {
		t.Fatalf("expected 1 total actor, got %d", report.TotalActors)
	}

	scorecard := actor.RenderActorScorecard(report)
	if !strings.Contains(scorecard, "AGENTVCR // DISTRIBUTED AGENT ACTOR MESH") {
		t.Fatalf("scorecard missing header")
	}

	mermaid := actor.RenderMermaidSupervisionTree(sys)
	if !strings.Contains(mermaid, "graph TD") || !strings.Contains(mermaid, "lead-agent") {
		t.Fatalf("mermaid diagram missing actor node: %s", mermaid)
	}
}
