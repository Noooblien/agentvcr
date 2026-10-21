package triage

import (
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// GetDemoSessions returns 4 realistic agent flight recording sessions representing distinct failure modes.
func GetDemoSessions() []*trace.Session {
	now := time.Now().UTC()

	// Scenario 1: Clean Nominal Execution
	s1 := &trace.Session{
		ID:        "sess_triage_clean",
		AgentName: "agent-lead-architect",
		Status:    "completed",
		CreatedAt: now.Add(-10 * time.Minute),
		Events: []trace.TraceEvent{
			{ID: "e1_0", Index: 0, Type: trace.EventLLMRequest, Timestamp: now.Add(-9 * time.Minute), Payload: map[string]interface{}{"prompt": "Inspect repo structure", "tokens": 120}},
			{ID: "e1_1", Index: 1, Type: trace.EventToolCall, Timestamp: now.Add(-8 * time.Minute), Payload: map[string]interface{}{"tool": "list_files", "dir": "."}},
			{ID: "e1_2", Index: 2, Type: trace.EventToolResult, Timestamp: now.Add(-7 * time.Minute), Payload: map[string]interface{}{"content": "main.go, go.mod, README.md"}},
			{ID: "e1_3", Index: 3, Type: trace.EventLLMResponse, Timestamp: now.Add(-6 * time.Minute), Payload: map[string]interface{}{"content": "Repository contains standard Go modules.", "tokens": 60}},
		},
	}

	// Scenario 2: Upstream Rate Limit 429 Storm
	s2 := &trace.Session{
		ID:        "sess_triage_ratelimit",
		AgentName: "agent-web-crawler",
		Status:    "failed",
		CreatedAt: now.Add(-15 * time.Minute),
		Events: []trace.TraceEvent{
			{ID: "e2_0", Index: 0, Type: trace.EventLLMRequest, Timestamp: now.Add(-14 * time.Minute), Payload: map[string]interface{}{"prompt": "Fetch latest CVE advisories", "tokens": 180}},
			{ID: "e2_1", Index: 1, Type: trace.EventToolCall, Timestamp: now.Add(-13 * time.Minute), Payload: map[string]interface{}{"tool": "fetch_url", "url": "https://api.cve.mitre.org/v1/recent"}},
			{ID: "e2_2", Index: 2, Type: trace.EventError, Timestamp: now.Add(-12 * time.Minute), Payload: map[string]interface{}{"error": "HTTP 429 Too Many Requests: Rate limit exceeded for API key"}},
		},
	}

	// Scenario 3: Runaway Tool Loop (OWASP-LLM08)
	s3 := &trace.Session{
		ID:        "sess_triage_loop",
		AgentName: "agent-refactor-bot",
		Status:    "failed",
		CreatedAt: now.Add(-20 * time.Minute),
		Events: []trace.TraceEvent{
			{ID: "e3_0", Index: 0, Type: trace.EventToolCall, Timestamp: now.Add(-19 * time.Minute), Payload: map[string]interface{}{"tool": "read_file", "path": "pkg/auth/token.go"}},
			{ID: "e3_1", Index: 1, Type: trace.EventToolCall, Timestamp: now.Add(-18 * time.Minute), Payload: map[string]interface{}{"tool": "read_file", "path": "pkg/auth/token.go"}},
			{ID: "e3_2", Index: 2, Type: trace.EventToolCall, Timestamp: now.Add(-17 * time.Minute), Payload: map[string]interface{}{"tool": "read_file", "path": "pkg/auth/token.go"}},
			{ID: "e3_3", Index: 3, Type: trace.EventToolCall, Timestamp: now.Add(-16 * time.Minute), Payload: map[string]interface{}{"tool": "read_file", "path": "pkg/auth/token.go"}},
		},
	}

	// Scenario 4: Permission Denied Root Path Violation
	s4 := &trace.Session{
		ID:        "sess_triage_eacces",
		AgentName: "agent-devops-deployer",
		Status:    "failed",
		CreatedAt: now.Add(-25 * time.Minute),
		Events: []trace.TraceEvent{
			{ID: "e4_0", Index: 0, Type: trace.EventLLMRequest, Timestamp: now.Add(-24 * time.Minute), Payload: map[string]interface{}{"prompt": "Update nginx ingress config", "tokens": 200}},
			{ID: "e4_1", Index: 1, Type: trace.EventToolCall, Timestamp: now.Add(-23 * time.Minute), Payload: map[string]interface{}{"tool": "write_file", "path": "/etc/nginx/nginx.conf", "content": "server { listen 80; }"}},
			{ID: "e4_2", Index: 2, Type: trace.EventError, Timestamp: now.Add(-22 * time.Minute), Payload: map[string]interface{}{"error": "open /etc/nginx/nginx.conf: permission denied (EACCES)"}},
		},
	}

	return []*trace.Session{s1, s2, s3, s4}
}

// RunDemoSuite executes root-cause triage across all demo sessions and returns their reports.
func RunDemoSuite() ([]*TriageReport, error) {
	engine := NewTriageEngine(DefaultTriageConfig())
	sessions := GetDemoSessions()

	reports := make([]*TriageReport, len(sessions))
	for i, s := range sessions {
		reports[i] = engine.Diagnose(s)
	}

	return reports, nil
}
