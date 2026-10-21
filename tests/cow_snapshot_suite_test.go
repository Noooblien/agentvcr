package tests

import (
	"strings"
	"testing"

	"github.com/Noooblien/agentvcr/pkg/snapshot"
)

func TestIntegration_CoWSnapshot_DemoSuite(t *testing.T) {
	report, err := snapshot.RunCoWDemo()
	if err != nil {
		t.Fatalf("failed to run CoW demo: %v", err)
	}

	if report.SessionID == "" {
		t.Errorf("expected non-empty session ID")
	}

	scorecard := snapshot.RenderCoWScorecard(report)
	if !strings.Contains(scorecard, "COW LAYERED FILESYSTEM SNAPSHOT") {
		t.Errorf("scorecard title not rendered properly")
	}

	mermaid := snapshot.RenderCoWMermaid(report)
	if !strings.Contains(mermaid, "flowchart TD") {
		t.Errorf("mermaid flowchart not rendered properly")
	}

	md := snapshot.RenderCoWMarkdown(report)
	if !strings.Contains(md, "## AgentVCR Layered CoW Filesystem Snapshot Report") {
		t.Errorf("markdown report heading missing")
	}
}
