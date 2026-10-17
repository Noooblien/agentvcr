package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/memory"
)

func TestMemory_EpisodicBuffering(t *testing.T) {
	cfg := memory.DefaultMemoryConfig()
	engine := memory.NewMemoryEngine("test-system", cfg)

	turn1 := &memory.EpisodicTurn{
		Role:       "user",
		Content:    "Initialize payment engine",
		Importance: 0.8,
	}
	engine.RecordEpisode(turn1)

	if turn1.ID == "" {
		t.Fatalf("Expected auto-generated ID for episodic turn")
	}
	if turn1.Timestamp.IsZero() {
		t.Fatalf("Expected auto-assigned timestamp for episodic turn")
	}
}

func TestMemory_EbbinghausDecay_AndReinforcement(t *testing.T) {
	cfg := memory.DefaultMemoryConfig()
	cfg.DecayRate = 0.10 // 10% decay rate per hour
	decayEng := memory.NewDecayEngine(cfg)

	now := time.Now()
	node := &memory.MemoryNode{
		ID:             "mem-test-decay",
		Level:          memory.LevelReflection,
		Content:        "Ephemeral intermediate test artifact",
		Importance:     0.5,
		Stability:      1.0,
		CreatedAt:      now.Add(-20 * time.Hour),
		LastAccessedAt: now.Add(-20 * time.Hour),
		AccessCount:    0,
	}

	// 1. Retention after 20 hours of inactivity
	scoreBefore := decayEng.RetentionScore(node, now)
	if scoreBefore > 0.20 {
		t.Fatalf("Expected decayed retention score (<0.20), got: %.3f", scoreBefore)
	}

	// 2. Reinforcement
	decayEng.Reinforce(node, now)
	if node.AccessCount != 1 {
		t.Fatalf("Expected AccessCount 1, got %d", node.AccessCount)
	}
	if node.Stability <= 1.0 {
		t.Fatalf("Expected stability increase after reinforcement, got: %.3f", node.Stability)
	}

	// Score immediately after reinforcement at 'now'
	scoreAfter := decayEng.RetentionScore(node, now)
	if scoreAfter <= scoreBefore {
		t.Fatalf("Expected higher retention score after reinforcement (before: %.3f, after: %.3f)", scoreBefore, scoreAfter)
	}
}

func TestMemory_HierarchicalConsolidation(t *testing.T) {
	cfg := memory.DefaultMemoryConfig()
	cfg.ConsolidationBatchSize = 3
	engine := memory.NewMemoryEngine("test-consolidation", cfg)

	now := time.Now()

	// Ingest 6 turns (2 batches of 3)
	for i := 1; i <= 6; i++ {
		engine.RecordEpisode(&memory.EpisodicTurn{
			Role:        "assistant",
			Action:      "compile step with extensive AST traversal and type checking",
			Observation: "compiled pkg/core/runtime.go successfully with zero diagnostic warnings and zero memory leaks",
			Importance:  0.3,
			Timestamp:   now,
		})
	}

	report := engine.Consolidate(now)

	if report.ConsolidatedReflection != 2 {
		t.Fatalf("Expected 2 consolidated reflections from 6 episodic turns, got: %d", report.ConsolidatedReflection)
	}
	if report.TotalEpisodicBuffered != 0 {
		t.Fatalf("Expected episodic buffer to be drained into reflections, got: %d", report.TotalEpisodicBuffered)
	}
	if report.TokenSavingsRatio < 0.20 {
		t.Fatalf("Expected positive token savings ratio, got: %.2f", report.TokenSavingsRatio)
	}
}

func TestMemory_SemanticTripleExtraction_AndContradiction(t *testing.T) {
	kg := memory.NewSemanticKnowledgeGraph()
	now := time.Now()

	// 1. Add initial user preference
	contr1, _ := kg.AddOrUpdateTriple("User", "prefers", "PostgreSQL", 0.9, now)
	if contr1 {
		t.Fatalf("Initial triple should not be a contradiction")
	}

	triple1, ok := kg.GetTriple("User", "prefers")
	if !ok || triple1.Object != "PostgreSQL" {
		t.Fatalf("Expected PostgreSQL triple stored")
	}

	// 2. Add conflicting preference later
	later := now.Add(2 * time.Hour)
	contr2, diff := kg.AddOrUpdateTriple("User", "prefers", "CockroachDB", 0.95, later)
	if !contr2 {
		t.Fatalf("Expected contradiction resolution flag to be true")
	}
	if !strings.Contains(diff, "replaced 'PostgreSQL' with 'CockroachDB'") {
		t.Fatalf("Unexpected diff string: %s", diff)
	}

	// Verify updated state
	triple2, ok := kg.GetTriple("User", "prefers")
	if !ok || triple2.Object != "CockroachDB" {
		t.Fatalf("Expected updated triple CockroachDB, got: %v", triple2)
	}
}

func TestMemory_RecallTokenBudget(t *testing.T) {
	cfg := memory.DefaultMemoryConfig()
	engine := memory.NewMemoryEngine("test-recall", cfg)

	now := time.Now()

	// Ingest turns including semantic requirements
	engine.RecordEpisode(&memory.EpisodicTurn{
		Role:       "user",
		Content:    "Always use PostgreSQL for data storage.",
		Importance: 0.95,
		Timestamp:  now,
	})
	for i := 1; i <= 5; i++ {
		engine.RecordEpisode(&memory.EpisodicTurn{
			Role:        "assistant",
			Content:     "Running auxiliary diagnostic and status checks on cluster",
			Importance:  0.2,
			Timestamp:   now,
		})
	}

	engine.Consolidate(now)

	// Recall with small token budget
	recalled, tokensUsed := engine.Recall(memory.RecallQuery{
		QueryText:        "PostgreSQL data storage",
		TokenBudget:      60,
		MinRetention:     0.10,
		CurrentTimestamp: now,
	})

	if len(recalled) == 0 {
		t.Fatalf("Expected at least 1 memory recalled")
	}
	if tokensUsed > 60 {
		t.Fatalf("Tokens used (%d) exceeded strict token budget (60)", tokensUsed)
	}
	if !strings.Contains(strings.ToLower(recalled[0].Content), "postgresql") {
		t.Fatalf("Top recalled memory should match query relevance: %s", recalled[0].Content)
	}
}

func TestMemory_PruningDecayedEphemeral(t *testing.T) {
	cfg := memory.DefaultMemoryConfig()
	cfg.DecayRate = 0.50
	cfg.MinRetentionScore = 0.30
	cfg.ConsolidationBatchSize = 1
	engine := memory.NewMemoryEngine("test-pruning", cfg)

	past := time.Now().Add(-100 * time.Hour)

	// Trivial command from long ago
	engine.RecordEpisode(&memory.EpisodicTurn{
		Role:        "assistant",
		Action:      "ls /tmp",
		Observation: "empty",
		Importance:  0.05,
		Timestamp:   past,
	})

	// Consolidate in the present
	now := time.Now()
	report := engine.Consolidate(now)

	if report.PrunedDecayedNodes == 0 && report.ActiveMemoryNodes > 0 {
		t.Fatalf("Expected trivial decayed episodic reflection to be pruned after 100 hours of inactivity")
	}
}

func TestMemory_ScorecardAndMermaid(t *testing.T) {
	cfg := memory.DefaultMemoryConfig()
	engine := memory.NewMemoryEngine("test-reporting", cfg)

	now := time.Now()
	engine.RecordEpisode(&memory.EpisodicTurn{
		Role:       "user",
		Content:    "Requirement: standardizes_on gRPC for inter-service communication.",
		Importance: 0.90,
		Timestamp:  now,
	})
	engine.RecordEpisode(&memory.EpisodicTurn{
		Role:        "assistant",
		Action:      "protoc --go_out=.",
		Observation: "compiled protobuf",
		Importance:  0.4,
		Timestamp:   now,
	})

	report := engine.Consolidate(now)
	decayEng := memory.NewDecayEngine(cfg)

	scorecard := memory.RenderMemoryScorecard(report, decayEng, now)
	if !strings.Contains(scorecard, "AGENTVCR // AUTONOMOUS HIERARCHICAL MEMORY") {
		t.Fatalf("Scorecard missing header: %s", scorecard)
	}

	mermaid := memory.RenderMemoryMermaid(engine)
	if !strings.Contains(mermaid, "graph TD") || !strings.Contains(mermaid, "Reflections Layer") {
		t.Fatalf("Mermaid diagram missing expected structure: %s", mermaid)
	}
}
