package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/cluster"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func TestCluster_GenerateFleetReport(t *testing.T) {
	now := time.Now()

	s1 := &trace.Session{
		ID:        "session-1",
		AgentName: "code-agent",
		Events: []trace.TraceEvent{
			{Index: 1, Timestamp: now, Type: trace.EventToolCall, Payload: map[string]interface{}{"tool_name": "search_code"}},
			{Index: 2, Timestamp: now, Type: trace.EventToolCall, Payload: map[string]interface{}{"tool_name": "replace_file"}},
		},
	}

	s2 := &trace.Session{
		ID:        "session-2",
		AgentName: "code-agent",
		Events: []trace.TraceEvent{
			{Index: 1, Timestamp: now, Type: trace.EventToolCall, Payload: map[string]interface{}{"tool_name": "search_code"}},
			{Index: 2, Timestamp: now, Type: trace.EventToolCall, Payload: map[string]interface{}{"tool_name": "replace_file"}},
		},
	}

	s3 := &trace.Session{
		ID:        "session-3",
		AgentName: "research-agent",
		Events: []trace.TraceEvent{
			{Index: 1, Timestamp: now, Type: trace.EventToolCall, Payload: map[string]interface{}{"tool_name": "search_web"}},
		},
	}

	report := cluster.GenerateFleetReport([]*trace.Session{s1, s2, s3})

	if report.TotalFleetSessions != 3 {
		t.Fatalf("expected 3 sessions, got %d", report.TotalFleetSessions)
	}

	if len(report.AgentHeatmaps) != 2 {
		t.Fatalf("expected 2 agents in heatmaps, got %d", len(report.AgentHeatmaps))
	}

	codeAgentHeatmap, ok := report.AgentHeatmaps["code-agent"]
	if !ok || codeAgentHeatmap.TotalSessions != 2 {
		t.Fatalf("expected 2 sessions for code-agent")
	}

	if codeAgentHeatmap.ToolFrequency["search_code"] != 2 {
		t.Errorf("expected search_code frequency 2, got %d", codeAgentHeatmap.ToolFrequency["search_code"])
	}

	// Signature verification
	if len(report.Clusters) != 2 {
		t.Fatalf("expected 2 distinct clusters, got %d", len(report.Clusters))
	}

	if report.Clusters[0].Count != 2 {
		t.Errorf("expected dominant cluster count 2, got %d", report.Clusters[0].Count)
	}

	rendered := cluster.RenderASCIIHeatmap(report)
	if !strings.Contains(rendered, "Multi-Agent Fleet Analytics") {
		t.Errorf("expected ASCII render to include header, got: %s", rendered)
	}
}
