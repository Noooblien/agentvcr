package tests

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Noooblien/agentvcr/pkg/chaos"
)

func TestChaos_DemoSuite(t *testing.T) {
	suite, err := chaos.RunDemoSuite(false)
	if err != nil {
		t.Fatalf("failed running chaos demo suite: %v", err)
	}

	if suite.TotalScenarios != 5 {
		t.Fatalf("expected 5 scenarios, got %d", suite.TotalScenarios)
	}
	if suite.PassedScenarios < 4 {
		t.Fatalf("expected at least 4 passed scenarios, got %d", suite.PassedScenarios)
	}
	if suite.OverallResilience <= 0.0 {
		t.Fatalf("expected positive overall resilience, got %.1f", suite.OverallResilience)
	}
	if suite.TotalFaultsInjected == 0 {
		t.Fatalf("expected injected faults > 0")
	}

	// Verify Terminal Scorecard
	term := chaos.RenderTerminalScorecard(suite)
	if !strings.Contains(term, "AGENTVCR AUTONOMOUS AGENT CHAOS") {
		t.Fatalf("scorecard missing header")
	}
	if !strings.Contains(term, "EXPERIMENT SCENARIO BREAKDOWN") {
		t.Fatalf("scorecard missing breakdown table")
	}

	// Verify Markdown Report
	md := chaos.RenderMarkdownReport(suite)
	if !strings.Contains(md, "### AgentVCR Chaos & Fault Injection Benchmark Report") {
		t.Fatalf("markdown report missing header")
	}
	if !strings.Contains(md, "```mermaid") {
		t.Fatalf("markdown report missing mermaid diagram")
	}

	// Verify JSON Serialization
	jsonStr, err := chaos.RenderJSON(suite)
	if err != nil {
		t.Fatalf("failed to render JSON: %v", err)
	}

	var parsed chaos.ChaosBenchmarkSuite
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if parsed.TotalScenarios != suite.TotalScenarios {
		t.Fatalf("json unmarshal scenario count mismatch: %d vs %d", parsed.TotalScenarios, suite.TotalScenarios)
	}
}

func TestChaos_StrictMode(t *testing.T) {
	// In strict mode, degraded scenarios should not pass
	suite, err := chaos.RunDemoSuite(true)
	if err != nil {
		t.Fatalf("failed running strict chaos demo suite: %v", err)
	}

	// Cascading Outage should fail in strict mode
	for _, rep := range suite.Reports {
		if rep.ExperimentName == "Cascading Outage" {
			if rep.Passed {
				t.Fatalf("expected Cascading Outage to fail under strict mode")
			}
		}
	}
}
