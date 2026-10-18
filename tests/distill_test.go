package tests

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/distill"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func createDistillSampleSessions() (*trace.Session, *trace.Session) {
	now := time.Now().UTC()

	// 1. Chosen Session (Clean, successful 3-step execution)
	chosen := &trace.Session{
		ID:        "session-chosen-01",
		AgentName: "code-specialist",
		Status:    "completed",
		Events: []trace.TraceEvent{
			{
				ID:        "c-0",
				Index:     0,
				Timestamp: now.Add(-10 * time.Minute),
				Type:      trace.EventLLMRequest,
				Payload:   map[string]interface{}{"prompt": "Optimize database query on users table to resolve high latency"},
			},
			{
				ID:        "c-1",
				Index:     1,
				Timestamp: now.Add(-9 * time.Minute),
				Type:      trace.EventToolCall,
				Payload: map[string]interface{}{
					"tool": "pg_analyze",
					"args": map[string]interface{}{"query": "SELECT * FROM users WHERE email = 'test@example.com'"},
				},
			},
			{
				ID:        "c-2",
				Index:     2,
				Timestamp: now.Add(-8 * time.Minute),
				Type:      trace.EventToolResult,
				Payload: map[string]interface{}{
					"tool":   "pg_analyze",
					"output": "Seq Scan detected. Cost 1400. Recommending CREATE INDEX idx_users_email ON users(email);",
				},
			},
			{
				ID:        "c-3",
				Index:     3,
				Timestamp: now.Add(-7 * time.Minute),
				Type:      trace.EventLLMResponse,
				Payload: map[string]interface{}{
					"content": "Sequential scan resolved. Created index idx_users_email. Query latency dropped from 250ms to 0.4ms.",
				},
			},
		},
	}

	// 2. Rejected Session (Failed, hallucinated tool call with error)
	rejected := &trace.Session{
		ID:        "session-rejected-02",
		AgentName: "code-specialist",
		Status:    "failed",
		Events: []trace.TraceEvent{
			{
				ID:        "r-0",
				Index:     0,
				Timestamp: now.Add(-10 * time.Minute),
				Type:      trace.EventLLMRequest,
				Payload:   map[string]interface{}{"prompt": "Optimize database query on users table to resolve high latency"},
			},
			{
				ID:        "r-1",
				Index:     1,
				Timestamp: now.Add(-9 * time.Minute),
				Type:      trace.EventToolCall,
				Payload: map[string]interface{}{
					"tool": "drop_database_all",
					"args": map[string]interface{}{"force": true},
				},
			},
			{
				ID:        "r-2",
				Index:     2,
				Timestamp: now.Add(-8 * time.Minute),
				Type:      trace.EventError,
				Payload: map[string]interface{}{
					"error":    "PermissionDenied: Dropping database is strictly prohibited by security policy",
					"is_error": true,
				},
			},
			{
				ID:        "r-3",
				Index:     3,
				Timestamp: now.Add(-7 * time.Minute),
				Type:      trace.EventLLMResponse,
				Payload: map[string]interface{}{
					"content": "Execution aborted due to security violation.",
				},
			},
		},
	}

	return chosen, rejected
}

func TestDistill_ExtractSFT(t *testing.T) {
	chosen, _ := createDistillSampleSessions()

	cfg := distill.DefaultDistillConfig()
	cfg.SystemPromptOverride = "You are a senior systems engineer."
	engine := distill.NewEngine(cfg)

	sft, err := engine.ExtractSFT(chosen)
	if err != nil {
		t.Fatalf("Failed to extract SFT: %v", err)
	}

	if len(sft.Messages) == 0 {
		t.Fatalf("Expected non-empty messages, got 0")
	}

	// First message should be system prompt override
	if sft.Messages[0].Role != "system" || sft.Messages[0].Content != "You are a senior systems engineer." {
		t.Errorf("Unexpected system prompt: %+v", sft.Messages[0])
	}

	// Second message should be user prompt
	if sft.Messages[1].Role != "user" || !strings.Contains(sft.Messages[1].Content, "Optimize database") {
		t.Errorf("Unexpected user prompt: %+v", sft.Messages[1])
	}

	// Tool call and result should be properly mapped
	hasTool := false
	hasAssistant := false
	for _, m := range sft.Messages {
		if m.Role == "tool" {
			hasTool = true
		}
		if m.Role == "assistant" && strings.Contains(m.Content, "Sequential scan resolved") {
			hasAssistant = true
		}
	}

	if !hasTool {
		t.Errorf("Expected tool result message in SFT")
	}
	if !hasAssistant {
		t.Errorf("Expected assistant response message in SFT")
	}
}

func TestDistill_PairDPO(t *testing.T) {
	chosen, rejected := createDistillSampleSessions()

	cfg := distill.DefaultDistillConfig()
	engine := distill.NewEngine(cfg)

	dpo, err := engine.PairDPO(chosen, rejected)
	if err != nil {
		t.Fatalf("Failed to generate DPO pair: %v", err)
	}

	if !strings.Contains(dpo.Prompt, "Optimize database") {
		t.Errorf("Unexpected DPO prompt: %s", dpo.Prompt)
	}

	if len(dpo.Chosen) == 0 {
		t.Errorf("Expected non-empty chosen trajectory")
	}

	if len(dpo.Rejected) == 0 {
		t.Errorf("Expected non-empty rejected trajectory")
	}

	// Verify reward margin is positive and exceeds minimum threshold
	if dpo.RewardMargin < cfg.MinRewardMargin {
		t.Errorf("Expected reward margin >= %.2f, got %.2f", cfg.MinRewardMargin, dpo.RewardMargin)
	}

	// Trajectory should diverge at step 1 (where chosen ran pg_analyze and rejected ran drop_database_all)
	if dpo.DivergenceAt < 1 {
		t.Errorf("Expected divergence step >= 1, got %d", dpo.DivergenceAt)
	}
}

func TestDistill_ShareGPTFormat(t *testing.T) {
	chosen, _ := createDistillSampleSessions()
	engine := distill.NewEngine(distill.DefaultDistillConfig())

	sft, err := engine.ExtractSFT(chosen)
	if err != nil {
		t.Fatalf("ExtractSFT error: %v", err)
	}

	sharegpt := distill.ConvertToShareGPT(sft, "sgpt-01")
	if sharegpt.ID != "sgpt-01" {
		t.Errorf("Unexpected ID: %s", sharegpt.ID)
	}

	if len(sharegpt.Conversations) != len(sft.Messages) {
		t.Errorf("Length mismatch: %d vs %d", len(sharegpt.Conversations), len(sft.Messages))
	}

	firstConv := sharegpt.Conversations[0]
	if firstConv.From != "human" {
		t.Errorf("Expected 'human', got %s", firstConv.From)
	}
}

func TestDistill_ExportJSONL(t *testing.T) {
	chosen, rejected := createDistillSampleSessions()
	engine := distill.NewEngine(distill.DefaultDistillConfig())

	dpo, err := engine.PairDPO(chosen, rejected)
	if err != nil {
		t.Fatalf("PairDPO error: %v", err)
	}

	jsonlBytes, err := distill.ExportJSONL([]distill.DPORow{*dpo})
	if err != nil {
		t.Fatalf("ExportJSONL error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(jsonlBytes)), "\n")
	if len(lines) != 1 {
		t.Fatalf("Expected 1 line in JSONL, got %d", len(lines))
	}

	var parsed distill.DPORow
	if err := json.Unmarshal([]byte(lines[0]), &parsed); err != nil {
		t.Fatalf("Failed to unmarshal JSONL line: %v", err)
	}

	if parsed.Prompt != dpo.Prompt {
		t.Errorf("Prompt mismatch in JSONL: %s vs %s", parsed.Prompt, dpo.Prompt)
	}
}

func TestDistill_ScorecardAndMermaid(t *testing.T) {
	chosen, rejected := createDistillSampleSessions()
	engine := distill.NewEngine(distill.DefaultDistillConfig())

	report := engine.DistillSessionPair(chosen, rejected)
	if !report.Passed {
		t.Errorf("Expected distillation report to pass")
	}

	scorecard := distill.RenderDistillScorecard(report)
	if !strings.Contains(scorecard, "AGENTVCR // TRAJECTORY DISTILLATION") {
		t.Errorf("Expected header in scorecard: %s", scorecard)
	}

	mermaid := distill.RenderDistillMermaid(report)
	if !strings.Contains(mermaid, "graph TD") {
		t.Errorf("Expected mermaid graph: %s", mermaid)
	}
}
