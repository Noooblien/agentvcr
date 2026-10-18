package tests

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/flamegraph"
	"github.com/Noooblien/agentvcr/pkg/storage"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func TestFlamegraph_EndToEndStorageAndReconstruction(t *testing.T) {
	tempDir := t.TempDir()
	store, err := storage.NewStore(tempDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	now := time.Now().UTC()
	session := &trace.Session{
		ID:        "trace_integration_e2e_01",
		AgentName: "Integration Test Agent",
		CreatedAt: now,
		UpdatedAt: now.Add(5 * time.Second),
		Status:    "completed",
		Events: []trace.TraceEvent{
			{
				ID:         "ev_1",
				Index:      0,
				Timestamp:  now,
				Type:       trace.EventLLMRequest,
				DurationMs: 25,
				Payload: map[string]interface{}{
					"model":         "claude-3-5-sonnet",
					"prompt_tokens": 1500,
				},
				Checksum: "chk1",
			},
			{
				ID:         "ev_2",
				Index:      1,
				Timestamp:  now.Add(25 * time.Millisecond),
				Type:       trace.EventLLMResponse,
				DurationMs: 1200,
				Payload: map[string]interface{}{
					"model":             "claude-3-5-sonnet",
					"completion_tokens": 300,
				},
				Checksum: "chk2",
			},
			{
				ID:         "ev_3",
				Index:      2,
				Timestamp:  now.Add(1225 * time.Millisecond),
				Type:       trace.EventToolCall,
				DurationMs: 10,
				Payload: map[string]interface{}{
					"tool":    "curl_fetch",
					"call_id": "call_c1",
				},
				Checksum: "chk3",
			},
			{
				ID:         "ev_4",
				Index:      3,
				Timestamp:  now.Add(1235 * time.Millisecond),
				Type:       trace.EventToolResult,
				DurationMs: 850,
				Payload: map[string]interface{}{
					"call_id": "call_c1",
					"status":  200,
				},
				Checksum: "chk4",
			},
		},
	}

	if err := store.SaveSession(session); err != nil {
		t.Fatalf("failed to save session: %v", err)
	}

	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}

	profile, err := flamegraph.BuildProfile(loaded)
	if err != nil {
		t.Fatalf("failed to build flame profile from loaded session: %v", err)
	}

	if profile.SessionID != session.ID {
		t.Errorf("expected session ID %s, got %s", session.ID, profile.SessionID)
	}

	if profile.TotalDurationMs < 2000 {
		t.Errorf("expected duration >= 2000ms, got %dms", profile.TotalDurationMs)
	}

	// Verify categories
	statInf := profile.CategoryBreakdown[flamegraph.CategoryInference]
	if statInf.Count != 1 || statInf.DurationMs != 1200 {
		t.Errorf("expected 1 inference op with 1200ms, got count %d, dur %dms", statInf.Count, statInf.DurationMs)
	}

	statTool := profile.CategoryBreakdown[flamegraph.CategoryTool]
	if statTool.Count != 1 || statTool.DurationMs != 850 {
		t.Errorf("expected 1 tool op with 850ms, got count %d, dur %dms", statTool.Count, statTool.DurationMs)
	}

	// Verify Chrome tracing export
	chromeBytes, err := flamegraph.ExportChromeTrace(profile)
	if err != nil {
		t.Fatalf("chrome trace export failed: %v", err)
	}
	var chromeEvents []map[string]interface{}
	if err := json.Unmarshal(chromeBytes, &chromeEvents); err != nil {
		t.Fatalf("invalid chrome trace JSON: %v", err)
	}
	if len(chromeEvents) < 3 {
		t.Errorf("expected at least 3 chrome events, got %d", len(chromeEvents))
	}

	// Verify Speedscope export
	speedscopeBytes, err := flamegraph.ExportSpeedscope(profile)
	if err != nil {
		t.Fatalf("speedscope export failed: %v", err)
	}
	var speedDoc map[string]interface{}
	if err := json.Unmarshal(speedscopeBytes, &speedDoc); err != nil {
		t.Fatalf("invalid speedscope JSON: %v", err)
	}
	if speedDoc["exporter"] != "AgentVCR Execution Flamegraph v1.0" {
		t.Errorf("unexpected exporter: %v", speedDoc["exporter"])
	}
}

func TestFlamegraph_MultiScenarioScoring(t *testing.T) {
	scenarios := []string{"swe", "research", "runaway"}

	for _, sc := range scenarios {
		session := flamegraph.GenerateDemoSession(sc)
		profile, err := flamegraph.BuildProfile(session)
		if err != nil {
			t.Fatalf("failed to build profile for scenario %s: %v", sc, err)
		}

		if profile.TotalTurns == 0 {
			t.Errorf("scenario %s has 0 turns", sc)
		}
		if profile.TotalDurationMs <= 0 {
			t.Errorf("scenario %s has invalid duration: %d", sc, profile.TotalDurationMs)
		}

		term := flamegraph.RenderTerminalFlame(profile, "latency")
		if !strings.Contains(term, "AGENTVCR EXECUTION FLAMEGRAPH") {
			t.Errorf("scenario %s terminal output missing header", sc)
		}
	}
}
