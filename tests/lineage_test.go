package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/lineage"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func TestLineage_Extraction(t *testing.T) {
	now := time.Now()
	session := &trace.Session{
		ID:        "lineage-session-1",
		AgentName: "lineage-agent",
		CreatedAt: now,
		Events: []trace.TraceEvent{
			{
				ID:        "ev-1",
				Index:     1,
				Timestamp: now.Add(150 * time.Microsecond),
				Type:      trace.EventStateChange,
				Payload: map[string]interface{}{
					"key":    "config.env",
					"value":  "production",
					"action": "set",
				},
			},
			{
				ID:        "ev-2",
				Index:     2,
				Timestamp: now.Add(400 * time.Microsecond),
				Type:      trace.EventToolCall,
				Payload: map[string]interface{}{
					"tool_name": "save_variable",
					"args": map[string]interface{}{
						"key":   "auth.token",
						"value": "token_abc123",
					},
				},
			},
			{
				ID:        "ev-3",
				Index:     3,
				Timestamp: now.Add(900 * time.Microsecond),
				Type:      trace.EventToolCall,
				Payload: map[string]interface{}{
					"tool_name": "read_state",
					"args": map[string]interface{}{
						"key": "auth.token",
					},
				},
			},
			{
				ID:        "ev-4",
				Index:     4,
				Timestamp: now.Add(1500 * time.Microsecond),
				Type:      trace.EventLLMResponse,
				Payload: map[string]interface{}{
					"context_updates": map[string]interface{}{
						"user.intent": "deploy_cluster",
					},
				},
			},
		},
	}

	graph, err := lineage.ExtractLineage(session)
	if err != nil {
		t.Fatalf("ExtractLineage failed: %v", err)
	}

	if graph.TotalSteps != 4 {
		t.Errorf("Expected 4 steps, got %d", graph.TotalSteps)
	}
	if graph.UniqueKeys != 3 {
		t.Errorf("Expected 3 unique keys, got %d", graph.UniqueKeys)
	}
	if len(graph.Timeline) != 4 {
		t.Errorf("Expected 4 mutations in timeline, got %d", len(graph.Timeline))
	}

	v, err := graph.GetVariableHistory("auth.token")
	if err != nil {
		t.Fatalf("Failed to get variable history for 'auth.token': %v", err)
	}
	if v.WriteCount != 1 || v.ReadCount != 1 {
		t.Errorf("Expected 1 write and 1 read for 'auth.token', got %d writes, %d reads", v.WriteCount, v.ReadCount)
	}
	if v.CurrentValue != "token_abc123" {
		t.Errorf("Expected 'token_abc123', got %v", v.CurrentValue)
	}
}

func TestLineage_TimeTravelReconstruction(t *testing.T) {
	now := time.Now()
	session := &trace.Session{
		ID:        "lineage-session-timetravel",
		AgentName: "state-agent",
		CreatedAt: now,
		Events: []trace.TraceEvent{
			{
				Index:     1,
				Timestamp: now.Add(100 * time.Microsecond),
				Type:      trace.EventStateChange,
				Payload: map[string]interface{}{
					"key":    "counter",
					"value":  10,
					"action": "set",
				},
			},
			{
				Index:     2,
				Timestamp: now.Add(300 * time.Microsecond),
				Type:      trace.EventStateChange,
				Payload: map[string]interface{}{
					"key":    "counter",
					"value":  20,
					"action": "update",
				},
			},
			{
				Index:     3,
				Timestamp: now.Add(600 * time.Microsecond),
				Type:      trace.EventStateChange,
				Payload: map[string]interface{}{
					"key":    "temp_key",
					"value":  "ephemeral",
					"action": "set",
				},
			},
			{
				Index:     4,
				Timestamp: now.Add(900 * time.Microsecond),
				Type:      trace.EventStateChange,
				Payload: map[string]interface{}{
					"key":    "temp_key",
					"action": "delete",
				},
			},
		},
	}

	graph, err := lineage.ExtractLineage(session)
	if err != nil {
		t.Fatalf("ExtractLineage failed: %v", err)
	}

	// At step 1: counter = 10, temp_key does not exist
	stateStep1 := graph.StateAt(1)
	if stateStep1["counter"] != 10 {
		t.Errorf("Expected counter=10 at step 1, got %v", stateStep1["counter"])
	}
	if _, exists := stateStep1["temp_key"]; exists {
		t.Errorf("temp_key should not exist at step 1")
	}

	// At step 2: counter = 20
	stateStep2 := graph.StateAt(2)
	if stateStep2["counter"] != 20 {
		t.Errorf("Expected counter=20 at step 2, got %v", stateStep2["counter"])
	}

	// At step 3: counter = 20, temp_key = ephemeral
	stateStep3 := graph.StateAt(3)
	if stateStep3["temp_key"] != "ephemeral" {
		t.Errorf("Expected temp_key='ephemeral' at step 3, got %v", stateStep3["temp_key"])
	}

	// At step 4: temp_key deleted, counter = 20
	stateStep4 := graph.StateAt(4)
	if _, exists := stateStep4["temp_key"]; exists {
		t.Errorf("temp_key should be deleted at step 4")
	}
	if stateStep4["counter"] != 20 {
		t.Errorf("Expected counter=20 at step 4, got %v", stateStep4["counter"])
	}
}

func TestLineage_DivergenceRisks(t *testing.T) {
	now := time.Now()
	session := &trace.Session{
		ID:        "lineage-risks-session",
		AgentName: "flaky-agent",
		CreatedAt: now,
		Events: []trace.TraceEvent{
			{
				Index:     1,
				Timestamp: now.Add(50 * time.Microsecond),
				Type:      trace.EventStateChange,
				Payload: map[string]interface{}{
					"key":    "undefined_var",
					"action": "read",
				},
			},
			{
				Index:     2,
				Timestamp: now.Add(100 * time.Microsecond),
				Type:      trace.EventStateChange,
				Payload: map[string]interface{}{
					"key":    "ghost_var",
					"action": "delete",
				},
			},
		},
	}

	report, err := lineage.GenerateLineageReport(session, "", -1)
	if err != nil {
		t.Fatalf("GenerateLineageReport failed: %v", err)
	}

	if report.EphemeralSandboxValid {
		t.Errorf("Expected sandbox to be invalid due to uninitialized read and premature delete")
	}

	if len(report.DivergenceRisks) < 2 {
		t.Errorf("Expected at least 2 divergence risks, got %d", len(report.DivergenceRisks))
	}

	rendered := lineage.RenderLineageScorecard(report)
	if !strings.Contains(rendered, "Uninitialized state read") {
		t.Errorf("Expected scorecard to mention uninitialized read, got: %s", rendered)
	}
	if !strings.Contains(rendered, "Premature state deletion") {
		t.Errorf("Expected scorecard to mention premature state deletion, got: %s", rendered)
	}
}
