package tests_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/compact"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func createTestBloatedSession() *trace.Session {
	now := time.Now()
	events := []trace.TraceEvent{
		{
			ID:        "evt_0",
			Index:     0,
			Timestamp: now,
			Type:      trace.EventLLMRequest,
			Payload: map[string]interface{}{
				"prompt": "Current local time: 2026-10-17T05:30:00Z\nYou are an autonomous software agent. Please check workspace status.",
			},
		},
		{
			ID:        "evt_1",
			Index:     1,
			Timestamp: now.Add(100 * time.Millisecond),
			Type:      trace.EventToolCall,
			Payload: map[string]interface{}{
				"tool_name": "bash",
				"arguments": map[string]interface{}{"command": "git status"},
			},
		},
		{
			ID:        "evt_2",
			Index:     2,
			Timestamp: now.Add(200 * time.Millisecond),
			Type:      trace.EventToolResult,
			Payload: map[string]interface{}{
				"tool_name": "bash",
				"content":   "On branch main\nYour branch is up to date with 'origin/main'.\n\nChanges not staged for commit:\n  (use \"git add <file>...\" to update what will be committed)\n  (use \"git restore <file>...\" to discard changes in working directory)\n\tmodified:   pkg/snapshot/cow.go\n\tmodified:   pkg/mcp/interceptor.go\n\tmodified:   pkg/mcp/types.go\n\tmodified:   cmd/agentvcr/main.go\n\tmodified:   tests/mcp_subsystem_test.go\n\nUntracked files:\n  (use \"git add <file>...\" to include in what will be committed)\n\tpkg/compact/\n\ttests/compact_test.go\n\nno changes added to commit (use \"git add\" and/or \"git commit -a\")\n[Detailed log summary: 45 files inspected, 1226 insertions, 48 deletions across workspace]",
				"isError":   false,
			},
		},
		{
			ID:        "evt_3",
			Index:     3,
			Timestamp: now.Add(300 * time.Millisecond),
			Type:      trace.EventToolCall,
			Payload: map[string]interface{}{
				"tool_name": "read_file",
				"arguments": map[string]interface{}{"path": "pkg/config.go"},
			},
		},
		{
			ID:        "evt_4",
			Index:     4,
			Timestamp: now.Add(400 * time.Millisecond),
			Type:      trace.EventToolResult,
			Payload: map[string]interface{}{
				"tool_name": "read_file",
				"content":   "package config\n\nconst Version = \"1.0.0\"\n",
				"isError":   false,
			},
		},
		{
			ID:        "evt_5",
			Index:     5,
			Timestamp: now.Add(500 * time.Millisecond),
			Type:      trace.EventToolCall,
			Payload: map[string]interface{}{
				"tool_name": "write_file",
				"arguments": map[string]interface{}{"path": "pkg/config.go", "content": "package config\n\nconst Version = \"1.1.0\"\n"},
			},
		},
		{
			ID:        "evt_6",
			Index:     6,
			Timestamp: now.Add(600 * time.Millisecond),
			Type:      trace.EventToolResult,
			Payload: map[string]interface{}{
				"tool_name": "write_file",
				"content":   "File pkg/config.go successfully updated.",
				"isError":   false,
			},
		},
		// DUPLICATE CALL: repeated identical git status output
		{
			ID:        "evt_7",
			Index:     7,
			Timestamp: now.Add(700 * time.Millisecond),
			Type:      trace.EventToolCall,
			Payload: map[string]interface{}{
				"tool_name": "bash",
				"arguments": map[string]interface{}{"command": "git status"},
			},
		},
		{
			ID:        "evt_8",
			Index:     8,
			Timestamp: now.Add(800 * time.Millisecond),
			Type:      trace.EventToolResult,
			Payload: map[string]interface{}{
				"tool_name": "bash",
				"content":   "On branch main\nYour branch is up to date with 'origin/main'.\n\nChanges not staged for commit:\n  (use \"git add <file>...\" to update what will be committed)\n  (use \"git restore <file>...\" to discard changes in working directory)\n\tmodified:   pkg/snapshot/cow.go\n\tmodified:   pkg/mcp/interceptor.go\n\tmodified:   pkg/mcp/types.go\n\tmodified:   cmd/agentvcr/main.go\n\tmodified:   tests/mcp_subsystem_test.go\n\nUntracked files:\n  (use \"git add <file>...\" to include in what will be committed)\n\tpkg/compact/\n\ttests/compact_test.go\n\nno changes added to commit (use \"git add\" and/or \"git commit -a\")\n[Detailed log summary: 45 files inspected, 1226 insertions, 48 deletions across workspace]",
				"isError":   false,
			},
		},
	}

	return &trace.Session{
		ID:        "session_bloated_1",
		AgentName: "TestAgent",
		CreatedAt: now,
		Status:    "recorded",
		Events:    events,
	}
}

func TestCompact_Profiler_DetectsAnomalies(t *testing.T) {
	session := createTestBloatedSession()
	cfg := compact.DefaultCompactorConfig()
	profiler := compact.NewProfiler(cfg)

	profile, err := profiler.ProfileSession(session)
	if err != nil {
		t.Fatalf("unexpected profiling error: %v", err)
	}

	if profile.TotalTokens <= 0 {
		t.Errorf("expected positive total tokens, got %d", profile.TotalTokens)
	}

	hasDuplicate := false
	hasKVJitter := false

	for _, anom := range profile.Anomalies {
		if anom.Category == "DUPLICATE_TOOL" {
			hasDuplicate = true
		}
		if anom.Category == "KV_CACHE_JITTER" {
			hasKVJitter = true
		}
	}

	if !hasDuplicate {
		t.Errorf("expected profiler to detect DUPLICATE_TOOL anomaly")
	}
	if !hasKVJitter {
		t.Errorf("expected profiler to detect KV_CACHE_JITTER anomaly")
	}
}

func TestCompact_LosslessCompaction_Deduplication(t *testing.T) {
	session := createTestBloatedSession()
	cfg := compact.DefaultCompactorConfig()
	compactor := compact.NewCompactor(cfg)

	compacted, report, err := compactor.CompactSession(session, "session_compacted_test")
	if err != nil {
		t.Fatalf("compaction failed: %v", err)
	}

	if report.DuplicateToolsPruned < 1 {
		t.Errorf("expected at least 1 duplicate tool pruned, got %d", report.DuplicateToolsPruned)
	}

	if report.TokensSaved <= 0 {
		t.Errorf("expected positive tokens saved, got %d", report.TokensSaved)
	}

	if report.ReductionPercent <= 0 {
		t.Errorf("expected positive reduction percentage, got %f", report.ReductionPercent)
	}

	// Verify duplicate event was converted to pointer
	dupEvt := compacted.Events[8]
	if isCompacted, ok := dupEvt.Payload["_agentvcr_compacted"].(bool); !ok || !isCompacted {
		t.Errorf("expected step 8 to be marked _agentvcr_compacted, got: %+v", dupEvt.Payload)
	}
}

func TestCompact_Aggressive_OversizedTruncation(t *testing.T) {
	now := time.Now()
	hugeContent := strings.Repeat("Lorem ipsum dolor sit amet, consectetur adipiscing elit. ", 300) // ~17,000 chars

	session := &trace.Session{
		ID:        "session_huge_payload",
		AgentName: "TestAgent",
		CreatedAt: now,
		Events: []trace.TraceEvent{
			{
				ID:        "evt_0",
				Index:     0,
				Timestamp: now,
				Type:      trace.EventToolResult,
				Payload: map[string]interface{}{
					"tool_name": "cat_log",
					"content":   hugeContent,
				},
			},
		},
	}

	cfg := compact.CompactorConfig{
		Strategy:            compact.StrategyAggressive,
		MaxToolResultTokens: 100, // Small threshold to trigger truncation
		DeduplicateTools:    true,
	}

	compactor := compact.NewCompactor(cfg)
	compacted, report, err := compactor.CompactSession(session, "session_truncated")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.TokensSaved <= 0 {
		t.Errorf("expected tokens saved via truncation, got %d", report.TokensSaved)
	}

	firstEvt := compacted.Events[0]
	if truncated, ok := firstEvt.Payload["_agentvcr_truncated"].(bool); !ok || !truncated {
		t.Errorf("expected _agentvcr_truncated flag set on payload")
	}
}

func TestCompact_ScorecardRendering(t *testing.T) {
	report := &compact.CompactionReport{
		OriginalSessionID:     "trace_raw_123",
		CompactedSessionID:    "trace_raw_123_compacted",
		OriginalTokens:        150000,
		CompactedTokens:       85000,
		TokensSaved:           65000,
		ReductionPercent:      43.33,
		OriginalCostUSD:       0.75,
		CompactedCostUSD:      0.42,
		SavingsUSD:            0.33,
		DuplicateToolsPruned:  4,
		RetriesCollapsed:      2,
		StaleReadsTrimmed:     1,
		KVCachePrefixAligned:  true,
		SemanticFidelityScore: 100.0,
		MeetsThreshold:        true,
		Timestamp:             time.Now(),
	}

	output := compact.RenderCompactorScorecard(report)
	if !strings.Contains(output, "43.33%") {
		t.Errorf("expected scorecard to display 43.33%%, got:\n%s", output)
	}
	if !strings.Contains(output, "OPTIMIZED & VERIFIED") {
		t.Errorf("expected scorecard status OPTIMIZED & VERIFIED, got:\n%s", output)
	}
}
