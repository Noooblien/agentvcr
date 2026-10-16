package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/leaderboard"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func TestLeaderboard_EvaluateScorecard(t *testing.T) {
	now := time.Now()
	session := &trace.Session{
		ID:        "agent-eval-1",
		AgentName: "code-specialist",
		CreatedAt: now,
		Metadata: map[string]string{
			"model":   "claude-3-7-sonnet",
			"version": "v2.4.0",
		},
		Events: []trace.TraceEvent{
			{
				Index:      1,
				Timestamp:  now,
				Type:       trace.EventLLMRequest,
				DurationMs: 120,
				Payload:    map[string]interface{}{"prompt": "Refactor router"},
			},
			{
				Index:      2,
				Timestamp:  now.Add(200 * time.Millisecond),
				Type:       trace.EventLLMResponse,
				DurationMs: 350,
				Payload: map[string]interface{}{
					"body": map[string]interface{}{
						"usage": map[string]interface{}{
							"prompt_tokens":     float64(500),
							"completion_tokens": float64(150),
							"total_tokens":      float64(650),
						},
					},
				},
			},
			{
				Index:      3,
				Timestamp:  now.Add(600 * time.Millisecond),
				Type:       trace.EventToolCall,
				DurationMs: 80,
				Payload: map[string]interface{}{
					"tool_name": "replace_code",
				},
			},
			{
				Index:      4,
				Timestamp:  now.Add(700 * time.Millisecond),
				Type:       trace.EventToolResult,
				DurationMs: 50,
				Payload: map[string]interface{}{
					"status": "success",
				},
			},
		},
	}

	card := leaderboard.EvaluateSessionScorecard(session)

	if card.AgentName != "code-specialist" {
		t.Errorf("Expected agent name 'code-specialist', got '%s'", card.AgentName)
	}
	if card.Model != "claude-3-7-sonnet" {
		t.Errorf("Expected model 'claude-3-7-sonnet', got '%s'", card.Model)
	}
	if card.TotalTokens != 650 {
		t.Errorf("Expected 650 tokens, got %d", card.TotalTokens)
	}
	if card.SuccessRate != 100.0 {
		t.Errorf("Expected 100%% success rate, got %.1f%%", card.SuccessRate)
	}
	if card.ToolAccuracyRate != 100.0 {
		t.Errorf("Expected 100%% tool accuracy, got %.1f%%", card.ToolAccuracyRate)
	}
	if card.CompositeScore <= 0 {
		t.Errorf("Expected positive composite score, got %.2f", card.CompositeScore)
	}
}

func TestLeaderboard_BuildRankings(t *testing.T) {

	// High-performing agent
	sessGood := &trace.Session{
		ID:        "sess-champion",
		AgentName: "champion-bot",
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventToolCall, DurationMs: 100},
			{Index: 2, Type: trace.EventToolResult, DurationMs: 50, Payload: map[string]interface{}{"status": "success"}},
		},
	}

	// Lower-performing agent with errors
	sessFlaky := &trace.Session{
		ID:        "sess-flaky",
		AgentName: "flaky-bot",
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventToolCall, DurationMs: 500},
			{Index: 2, Type: trace.EventToolResult, DurationMs: 50, Payload: map[string]interface{}{"error": "command timed out"}},
			{Index: 3, Type: trace.EventError, DurationMs: 50},
		},
	}

	lb := leaderboard.BuildLeaderboard([]*trace.Session{sessFlaky, sessGood}, "Agent Reliability Benchmark")

	if lb.TotalEntries != 2 {
		t.Fatalf("Expected 2 entries, got %d", lb.TotalEntries)
	}
	if lb.TopAgent != "champion-bot" {
		t.Errorf("Expected top agent 'champion-bot', got '%s'", lb.TopAgent)
	}
	if lb.Entries[0].Rank != 1 || lb.Entries[0].AgentName != "champion-bot" {
		t.Errorf("Expected champion-bot at Rank 1, got %v", lb.Entries[0])
	}
	if lb.Entries[1].Rank != 2 || lb.Entries[1].AgentName != "flaky-bot" {
		t.Errorf("Expected flaky-bot at Rank 2, got %v", lb.Entries[1])
	}

	rendered := leaderboard.RenderLeaderboardTable(lb)
	if !strings.Contains(rendered, "champion-bot") || !strings.Contains(rendered, "🥇 #1") {
		t.Errorf("Expected leaderboard table to contain champion-bot with rank 1 badge, got:\n%s", rendered)
	}
}

func TestLeaderboard_RegressionDiff_Passing(t *testing.T) {
	baseline := &trace.Session{
		ID:        "baseline-v1",
		AgentName: "agent-v1",
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventToolCall, DurationMs: 200},
			{Index: 2, Type: trace.EventToolResult, DurationMs: 100, Payload: map[string]interface{}{"status": "success"}},
		},
	}

	candidate := &trace.Session{
		ID:        "candidate-v2",
		AgentName: "agent-v2",
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventToolCall, DurationMs: 150},
			{Index: 2, Type: trace.EventToolResult, DurationMs: 80, Payload: map[string]interface{}{"status": "success"}},
		},
	}

	diff := leaderboard.DiffSessions(baseline, candidate, nil)
	if !diff.PassGate {
		t.Errorf("Expected candidate to pass CI/CD gate, reasons: %v", diff.RegressionReasons)
	}
	if diff.RegressionDetected {
		t.Errorf("Expected zero regressions detected")
	}

	rendered := leaderboard.RenderRegressionDiffCard(diff)
	if !strings.Contains(rendered, "[PASS] APPROVED FOR PRODUCTION") {
		t.Errorf("Expected passing CI/CD status in card, got: %s", rendered)
	}
}

func TestLeaderboard_RegressionDiff_RegressionDetected(t *testing.T) {
	baseline := &trace.Session{
		ID:        "baseline-v1",
		AgentName: "agent-v1",
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventToolCall, DurationMs: 100},
			{Index: 2, Type: trace.EventToolResult, DurationMs: 50, Payload: map[string]interface{}{"status": "success"}},
		},
	}

	candidate := &trace.Session{
		ID:        "candidate-broken",
		AgentName: "agent-v2-broken",
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventToolCall, DurationMs: 900},
			{Index: 2, Type: trace.EventToolResult, DurationMs: 100, Payload: map[string]interface{}{"error": "permission denied"}},
			{Index: 3, Type: trace.EventError, DurationMs: 50},
		},
	}

	diff := leaderboard.DiffSessions(baseline, candidate, nil)
	if diff.PassGate {
		t.Errorf("Expected candidate to fail CI/CD gate due to regressions")
	}
	if !diff.RegressionDetected {
		t.Errorf("Expected RegressionDetected to be true")
	}
	if len(diff.RegressionReasons) == 0 {
		t.Errorf("Expected regression reasons to be populated")
	}

	rendered := leaderboard.RenderRegressionDiffCard(diff)
	if !strings.Contains(rendered, "[FAIL] REGRESSION DETECTED") {
		t.Errorf("Expected failing status in scorecard, got: %s", rendered)
	}
}
