package flamegraph

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildProfile_Basic(t *testing.T) {
	session := GenerateDemoSession("swe")
	profile, err := BuildProfile(session)
	if err != nil {
		t.Fatalf("unexpected error building profile: %v", err)
	}

	if profile.SessionID != "trace_swe_agent_01" {
		t.Errorf("expected session ID trace_swe_agent_01, got %s", profile.SessionID)
	}

	if profile.TotalTurns != 4 {
		t.Errorf("expected 4 turns, got %d", profile.TotalTurns)
	}

	if profile.TotalToolsCalled != 4 {
		t.Errorf("expected 4 tools called, got %d", profile.TotalToolsCalled)
	}

	if profile.TotalTokens <= 0 {
		t.Errorf("expected positive total tokens, got %d", profile.TotalTokens)
	}

	if profile.TotalCostUSD <= 0 {
		t.Errorf("expected positive total cost, got %f", profile.TotalCostUSD)
	}

	if profile.Root == nil {
		t.Fatalf("expected non-nil root span")
	}

	if len(profile.Root.Children) != 4 {
		t.Errorf("expected 4 root children (turns), got %d", len(profile.Root.Children))
	}
}

func TestBuildProfile_CriticalPath(t *testing.T) {
	session := GenerateDemoSession("swe")
	profile, err := BuildProfile(session)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(profile.CriticalPath) == 0 {
		t.Fatalf("expected non-empty critical path")
	}

	hasToolOnCriticalPath := false
	for _, cp := range profile.CriticalPath {
		if cp.Category == CategoryTool {
			hasToolOnCriticalPath = true
			break
		}
	}

	if !hasToolOnCriticalPath {
		t.Errorf("expected tool execution to be flagged on critical path for Turn 2")
	}
}

func TestBuildProfile_Hotspots(t *testing.T) {
	session := GenerateDemoSession("swe")
	profile, err := BuildProfile(session)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(profile.Hotspots) == 0 {
		t.Fatalf("expected at least one hotspot detected")
	}

	foundBashHotspot := false
	for _, h := range profile.Hotspots {
		if strings.Contains(h.Name, "bash_exec") && h.Metric == "latency" {
			foundBashHotspot = true
			if h.Percentage < 20.0 {
				t.Errorf("expected bash_exec to consume >= 20%% of latency, got %.1f%%", h.Percentage)
			}
		}
	}

	if !foundBashHotspot {
		t.Errorf("expected bash_exec latency hotspot to be detected")
	}
}

func TestExport_Speedscope(t *testing.T) {
	session := GenerateDemoSession("research")
	profile, err := BuildProfile(session)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := ExportSpeedscope(profile)
	if err != nil {
		t.Fatalf("speedscope export failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("speedscope output is not valid JSON: %v", err)
	}

	if parsed["$schema"] != "https://www.speedscope.app/file-format-schema.json" {
		t.Errorf("unexpected schema: %v", parsed["$schema"])
	}

	profiles, ok := parsed["profiles"].([]interface{})
	if !ok || len(profiles) == 0 {
		t.Fatalf("missing profiles array in speedscope JSON")
	}
}

func TestExport_ChromeTrace(t *testing.T) {
	session := GenerateDemoSession("runaway")
	profile, err := BuildProfile(session)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := ExportChromeTrace(profile)
	if err != nil {
		t.Fatalf("chrome trace export failed: %v", err)
	}

	var events []map[string]interface{}
	if err := json.Unmarshal(data, &events); err != nil {
		t.Fatalf("chrome trace output is not valid JSON: %v", err)
	}

	if len(events) == 0 {
		t.Errorf("expected non-empty event list in chrome trace")
	}

	for _, ev := range events {
		if ev["ph"] != "X" {
			t.Errorf("expected complete event ph='X', got %v", ev["ph"])
		}
	}
}

func TestExport_TerminalAndMarkdown(t *testing.T) {
	session := GenerateDemoSession("swe")
	profile, err := BuildProfile(session)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	termOutput := RenderTerminalFlame(profile, "latency")
	if !strings.Contains(termOutput, "AGENTVCR EXECUTION FLAMEGRAPH") {
		t.Errorf("terminal flame output missing header")
	}
	if !strings.Contains(termOutput, "[CRITICAL PATH]") {
		t.Errorf("terminal flame output missing critical path badge")
	}

	mdOutput := RenderMarkdownReport(profile)
	if !strings.Contains(mdOutput, "# AgentVCR Execution Flamegraph Report") {
		t.Errorf("markdown report missing header")
	}
	if !strings.Contains(mdOutput, "| Category Resource Allocation |") && !strings.Contains(mdOutput, "## Category Resource Allocation") {
		t.Errorf("markdown report missing category table")
	}

	mermaidOutput := RenderMermaidGantt(profile)
	if !strings.Contains(mermaidOutput, "gantt") {
		t.Errorf("mermaid output missing gantt directive")
	}
}

func TestRunDemoSuite(t *testing.T) {
	formats := []ExportFormat{
		FormatTerminal,
		FormatSpeedscope,
		FormatChrome,
		FormatMermaid,
		FormatMarkdown,
	}

	for _, f := range formats {
		out, err := RunDemoSuite(f, "latency")
		if err != nil {
			t.Errorf("RunDemoSuite failed for format %s: %v", f, err)
		}
		if len(out) == 0 {
			t.Errorf("RunDemoSuite produced empty output for format %s", f)
		}
	}
}
