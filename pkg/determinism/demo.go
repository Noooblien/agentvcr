package determinism

import (
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// DemoScenario represents a paired baseline and target run for benchmark demonstration.
type DemoScenario struct {
	Name        string
	Description string
	Baseline    *trace.Session
	Target      *trace.Session
}

// GenerateDemoScenarios builds 4 real-world agent evaluation benchmarks.
func GenerateDemoScenarios() []DemoScenario {
	baseTime := time.Now().Add(-1 * time.Hour)

	// Scenario 1: Nominal 100% Deterministic Replay
	sc1Baseline := &trace.Session{
		ID:        "baseline_repo_search_v1",
		AgentName: "code-search-agent",
		CreatedAt: baseTime,
		Events: []trace.TraceEvent{
			{ID: "b1", Index: 1, Type: trace.EventLLMRequest, Timestamp: baseTime, DurationMs: 140, Checksum: "chk_101", Payload: map[string]interface{}{"prompt": "Find all HTTP handlers", "tokens": 120}},
			{ID: "b2", Index: 2, Type: trace.EventToolCall, Timestamp: baseTime.Add(200 * time.Millisecond), DurationMs: 35, Checksum: "chk_102", Payload: map[string]interface{}{"tool_name": "ast_grep", "arguments": map[string]interface{}{"pattern": "http.HandleFunc"}}},
			{ID: "b3", Index: 3, Type: trace.EventToolResult, Timestamp: baseTime.Add(250 * time.Millisecond), DurationMs: 20, Checksum: "chk_103", Payload: map[string]interface{}{"tool_name": "ast_grep", "result": "3 matches found"}},
			{ID: "b4", Index: 4, Type: trace.EventToolCall, Timestamp: baseTime.Add(300 * time.Millisecond), DurationMs: 50, Checksum: "chk_104", Payload: map[string]interface{}{"tool_name": "read_source", "arguments": map[string]interface{}{"file": "server.go", "line": 42}}},
			{ID: "b5", Index: 5, Type: trace.EventLLMResponse, Timestamp: baseTime.Add(500 * time.Millisecond), DurationMs: 190, Checksum: "chk_105", Payload: map[string]interface{}{"content": "Handler registered at server.go:42", "tokens": 180}},
		},
	}
	sc1Target := &trace.Session{
		ID:        "replay_repo_search_exact",
		AgentName: "code-search-agent",
		CreatedAt: baseTime.Add(10 * time.Minute),
		Events: []trace.TraceEvent{
			{ID: "t1", Index: 1, Type: trace.EventLLMRequest, Timestamp: baseTime, DurationMs: 138, Checksum: "chk_101", Payload: map[string]interface{}{"prompt": "Find all HTTP handlers", "tokens": 120}},
			{ID: "t2", Index: 2, Type: trace.EventToolCall, Timestamp: baseTime.Add(200 * time.Millisecond), DurationMs: 34, Checksum: "chk_102", Payload: map[string]interface{}{"tool_name": "ast_grep", "arguments": map[string]interface{}{"pattern": "http.HandleFunc"}}},
			{ID: "t3", Index: 3, Type: trace.EventToolResult, Timestamp: baseTime.Add(250 * time.Millisecond), DurationMs: 21, Checksum: "chk_103", Payload: map[string]interface{}{"tool_name": "ast_grep", "result": "3 matches found"}},
			{ID: "t4", Index: 4, Type: trace.EventToolCall, Timestamp: baseTime.Add(300 * time.Millisecond), DurationMs: 48, Checksum: "chk_104", Payload: map[string]interface{}{"tool_name": "read_source", "arguments": map[string]interface{}{"file": "server.go", "line": 42}}},
			{ID: "t5", Index: 5, Type: trace.EventLLMResponse, Timestamp: baseTime.Add(500 * time.Millisecond), DurationMs: 185, Checksum: "chk_105", Payload: map[string]interface{}{"content": "Handler registered at server.go:42", "tokens": 180}},
		},
	}

	// Scenario 2: Minor Argument Drift & Token Variation
	sc2Baseline := &trace.Session{
		ID:        "baseline_db_migration",
		AgentName: "db-agent",
		CreatedAt: baseTime,
		Events: []trace.TraceEvent{
			{ID: "b2_1", Index: 1, Type: trace.EventToolCall, Timestamp: baseTime, DurationMs: 40, Checksum: "chk_201", Payload: map[string]interface{}{"tool_name": "query_schema", "arguments": map[string]interface{}{"table": "users", "depth": 1}}},
			{ID: "b2_2", Index: 2, Type: trace.EventLLMResponse, Timestamp: baseTime.Add(100 * time.Millisecond), DurationMs: 160, Checksum: "chk_202", Payload: map[string]interface{}{"tokens": 200}},
		},
	}
	sc2Target := &trace.Session{
		ID:        "target_db_migration_drift",
		AgentName: "db-agent",
		CreatedAt: baseTime.Add(10 * time.Minute),
		Events: []trace.TraceEvent{
			{ID: "t2_1", Index: 1, Type: trace.EventToolCall, Timestamp: baseTime, DurationMs: 45, Checksum: "chk_201_diff", Payload: map[string]interface{}{"tool_name": "query_schema", "arguments": map[string]interface{}{"table": "users", "depth": 2}}},
			{ID: "t2_2", Index: 2, Type: trace.EventLLMResponse, Timestamp: baseTime.Add(100 * time.Millisecond), DurationMs: 175, Checksum: "chk_202_diff", Payload: map[string]interface{}{"tokens": 215}},
		},
	}

	// Scenario 3: Tool Hallucination & Path Divergence
	sc3Baseline := &trace.Session{
		ID:        "baseline_auth_audit",
		AgentName: "sec-auditor",
		CreatedAt: baseTime,
		Events: []trace.TraceEvent{
			{ID: "b3_1", Index: 1, Type: trace.EventToolCall, Timestamp: baseTime, DurationMs: 50, Checksum: "chk_301", Payload: map[string]interface{}{"tool_name": "verify_token", "arguments": map[string]interface{}{"mode": "strict"}}},
			{ID: "b3_2", Index: 2, Type: trace.EventToolCall, Timestamp: baseTime.Add(100 * time.Millisecond), DurationMs: 60, Checksum: "chk_302", Payload: map[string]interface{}{"tool_name": "check_rbac", "arguments": map[string]interface{}{"role": "admin"}}},
		},
	}
	sc3Target := &trace.Session{
		ID:        "target_auth_audit_hallucinate",
		AgentName: "sec-auditor",
		CreatedAt: baseTime.Add(10 * time.Minute),
		Events: []trace.TraceEvent{
			{ID: "t3_1", Index: 1, Type: trace.EventToolCall, Timestamp: baseTime, DurationMs: 52, Checksum: "chk_301", Payload: map[string]interface{}{"tool_name": "verify_token", "arguments": map[string]interface{}{"mode": "strict"}}},
			{ID: "t3_2", Index: 2, Type: trace.EventToolCall, Timestamp: baseTime.Add(100 * time.Millisecond), DurationMs: 85, Checksum: "chk_303", Payload: map[string]interface{}{"tool_name": "bypass_auth_check", "arguments": map[string]interface{}{"allow_all": true}}},
		},
	}

	// Scenario 4: Token Explosion & Context Runaway
	sc4Baseline := &trace.Session{
		ID:        "baseline_summary_task",
		AgentName: "summary-agent",
		CreatedAt: baseTime,
		Events: []trace.TraceEvent{
			{ID: "b4_1", Index: 1, Type: trace.EventLLMRequest, Timestamp: baseTime, DurationMs: 80, Checksum: "chk_401", Payload: map[string]interface{}{"tokens": 300}},
			{ID: "b4_2", Index: 2, Type: trace.EventLLMResponse, Timestamp: baseTime.Add(100 * time.Millisecond), DurationMs: 150, Checksum: "chk_402", Payload: map[string]interface{}{"tokens": 150}},
		},
	}
	sc4Target := &trace.Session{
		ID:        "target_summary_token_explosion",
		AgentName: "summary-agent",
		CreatedAt: baseTime.Add(10 * time.Minute),
		Events: []trace.TraceEvent{
			{ID: "t4_1", Index: 1, Type: trace.EventLLMRequest, Timestamp: baseTime, DurationMs: 90, Checksum: "chk_401", Payload: map[string]interface{}{"tokens": 300}},
			{ID: "t4_2", Index: 2, Type: trace.EventLLMResponse, Timestamp: baseTime.Add(100 * time.Millisecond), DurationMs: 420, Checksum: "chk_404", Payload: map[string]interface{}{"tokens": 1250}},
		},
	}

	return []DemoScenario{
		{
			Name:        "Nominal 100% Deterministic Replay",
			Description: "Exact trajectory match with zero tool drift, 0% token delta, and microsecond-level proxy overhead.",
			Baseline:    sc1Baseline,
			Target:      sc1Target,
		},
		{
			Name:        "Minor Argument Drift & Parameter Variation",
			Description: "Tool invoked correctly but parameter drifted (depth 1 -> 2) causing 7.5% token inflation.",
			Baseline:    sc2Baseline,
			Target:      sc2Target,
		},
		{
			Name:        "Tool Hallucination & Path Divergence",
			Description: "Model hallucinates and replaces check_rbac with unauthorized bypass_auth_check tool.",
			Baseline:    sc3Baseline,
			Target:      sc3Target,
		},
		{
			Name:        "Context Window Explosion & Token Surge",
			Description: "Candidate model produces runaway 2.8x token output explosion compared to recorded baseline.",
			Baseline:    sc4Baseline,
			Target:      sc4Target,
		},
	}
}
