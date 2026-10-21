package triage

import (
	"sync"
	"testing"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

func TestTriageEngine_CleanSession(t *testing.T) {
	engine := NewTriageEngine(DefaultTriageConfig())
	s := &trace.Session{
		ID:        "clean_test",
		AgentName: "clean_agent",
		Events: []trace.TraceEvent{
			{ID: "e0", Index: 0, Type: trace.EventToolCall, Payload: map[string]interface{}{"tool": "read_file"}},
			{ID: "e1", Index: 1, Type: trace.EventToolResult, Payload: map[string]interface{}{"content": "clean content"}},
		},
	}

	report := engine.Diagnose(s)
	if !report.IsCleanRun {
		t.Errorf("expected clean run, got false")
	}
	if len(report.Findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(report.Findings))
	}
	if report.FailedEvents != 0 {
		t.Errorf("expected 0 failed events, got %d", report.FailedEvents)
	}
}

func TestTriageEngine_RateLimitDiagnosis(t *testing.T) {
	engine := NewTriageEngine(DefaultTriageConfig())
	s := &trace.Session{
		ID:        "ratelimit_test",
		AgentName: "ratelimit_agent",
		Events: []trace.TraceEvent{
			{ID: "e0", Index: 0, Type: trace.EventToolCall, Payload: map[string]interface{}{"tool": "fetch_api", "url": "https://api.openai.com"}},
			{ID: "e1", Index: 1, Type: trace.EventError, Payload: map[string]interface{}{"error": "Error 429: Too many requests, rate limit exceeded"}},
		},
	}

	report := engine.Diagnose(s)
	if report.IsCleanRun {
		t.Fatalf("expected failed run")
	}
	if len(report.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(report.Findings))
	}

	finding := report.Findings[0]
	if finding.Category != FaultResourceExhaustion {
		t.Errorf("expected category %s, got %s", FaultResourceExhaustion, finding.Category)
	}
	if finding.Remediation.Strategy != StrategyExponentialBackoff {
		t.Errorf("expected strategy %s, got %s", StrategyExponentialBackoff, finding.Remediation.Strategy)
	}
	if finding.IncitingStepIndex != 0 {
		t.Errorf("expected inciting step 0, got %d", finding.IncitingStepIndex)
	}
}

func TestTriageEngine_RunawayLoop(t *testing.T) {
	cfg := DefaultTriageConfig()
	cfg.LoopDetectionThreshold = 3
	engine := NewTriageEngine(cfg)

	s := &trace.Session{
		ID:        "loop_test",
		AgentName: "loop_agent",
		Events: []trace.TraceEvent{
			{ID: "e0", Index: 0, Type: trace.EventToolCall, Payload: map[string]interface{}{"tool": "bash", "cmd": "cat file.txt"}},
			{ID: "e1", Index: 1, Type: trace.EventToolCall, Payload: map[string]interface{}{"tool": "bash", "cmd": "cat file.txt"}},
			{ID: "e2", Index: 2, Type: trace.EventToolCall, Payload: map[string]interface{}{"tool": "bash", "cmd": "cat file.txt"}},
		},
	}

	report := engine.Diagnose(s)
	if report.IsCleanRun {
		t.Fatalf("expected loop detection failure")
	}
	if len(report.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(report.Findings))
	}

	finding := report.Findings[0]
	if finding.Category != FaultRunawayLoop {
		t.Errorf("expected category %s, got %s", FaultRunawayLoop, finding.Category)
	}
	if finding.Severity != SeverityCritical {
		t.Errorf("expected severity CRITICAL, got %s", finding.Severity)
	}
	if finding.Remediation.Strategy != StrategyLoopBreakIntervention {
		t.Errorf("expected strategy %s, got %s", StrategyLoopBreakIntervention, finding.Remediation.Strategy)
	}
}

func TestTriageEngine_PermissionDenied(t *testing.T) {
	engine := NewTriageEngine(DefaultTriageConfig())
	s := &trace.Session{
		ID:        "perm_test",
		AgentName: "perm_agent",
		Events: []trace.TraceEvent{
			{ID: "e0", Index: 0, Type: trace.EventToolCall, Payload: map[string]interface{}{"tool": "write_file", "path": "/etc/shadow"}},
			{ID: "e1", Index: 1, Type: trace.EventError, Payload: map[string]interface{}{"error": "open /etc/shadow: permission denied"}},
		},
	}

	report := engine.Diagnose(s)
	if len(report.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(report.Findings))
	}

	finding := report.Findings[0]
	if finding.Category != FaultPermissionDenied {
		t.Errorf("expected category %s, got %s", FaultPermissionDenied, finding.Category)
	}
	if finding.Remediation.Strategy != StrategyPathRedirection {
		t.Errorf("expected strategy %s, got %s", StrategyPathRedirection, finding.Remediation.Strategy)
	}
}

func TestTriageEngine_Concurrency(t *testing.T) {
	engine := NewTriageEngine(DefaultTriageConfig())
	sessions := GetDemoSessions()

	var wg sync.WaitGroup
	workers := 20
	iterations := 50

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				s := sessions[i%len(sessions)]
				rep := engine.Diagnose(s)
				if rep == nil || rep.SessionID == "" {
					t.Errorf("worker %d got invalid report", id)
					return
				}
			}
		}(w)
	}

	wg.Wait()
}
