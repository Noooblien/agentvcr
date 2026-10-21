package snapshot

import (
	"strings"
	"testing"
)

func TestCoW_DemoSuite(t *testing.T) {
	report, err := RunCoWDemo()
	if err != nil {
		t.Fatalf("expected CoW demo to succeed: %v", err)
	}

	if report.TotalLayers < 2 {
		t.Errorf("expected at least 2 active layers, got %d", report.TotalLayers)
	}

	scorecard := RenderCoWScorecard(report)
	if !strings.Contains(scorecard, "COW LAYERED FILESYSTEM SNAPSHOT") {
		t.Errorf("scorecard missing header")
	}

	mermaid := RenderCoWMermaid(report)
	if !strings.Contains(mermaid, "flowchart TD") {
		t.Errorf("mermaid missing flowchart TD")
	}

	md := RenderCoWMarkdown(report)
	if !strings.Contains(md, "## AgentVCR Layered CoW Filesystem Snapshot Report") {
		t.Errorf("markdown missing title")
	}
}
