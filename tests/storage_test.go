package storage_test

import (
	"os"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/replay"
	"github.com/Noooblien/agentvcr/pkg/storage"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func TestTraceRecordAndFork(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agentvcr_test_*")
	if err != nil {
		t.Fatalf("failed to create temp test dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store, err := storage.NewStore(tempDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	session := &trace.Session{
		ID:        "test_session_1",
		AgentName: "code-architect",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Status:    "recording",
		Events: []trace.TraceEvent{
			{
				ID:        "evt_0",
				Index:     0,
				Timestamp: time.Now(),
				Type:      trace.EventLLMRequest,
				Payload:   map[string]interface{}{"prompt": "write a fibonacci function"},
			},
			{
				ID:        "evt_1",
				Index:     1,
				Timestamp: time.Now(),
				Type:      trace.EventLLMResponse,
				Payload:   map[string]interface{}{"content": "def fib(n): ..."},
			},
			{
				ID:        "evt_2",
				Index:     2,
				Timestamp: time.Now(),
				Type:      trace.EventToolCall,
				Payload:   map[string]interface{}{"cmd": "pytest test_fib.py"},
			},
		},
	}

	if err := store.SaveSession(session); err != nil {
		t.Fatalf("failed to save session: %v", err)
	}

	loaded, err := store.LoadSession("test_session_1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}

	if len(loaded.Events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(loaded.Events))
	}

	// Test Fork
	engine := replay.NewEngine(store)
	forked, err := engine.ForkSession("test_session_1", 2, "test_session_1_fork")
	if err != nil {
		t.Fatalf("failed to fork session: %v", err)
	}

	if len(forked.Events) != 2 {
		t.Fatalf("expected 2 events in forked session, got %d", len(forked.Events))
	}

	if forked.ForkAtStep != 2 {
		t.Fatalf("expected fork_at_step 2, got %d", forked.ForkAtStep)
	}
}
