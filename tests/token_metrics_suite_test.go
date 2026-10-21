package tests

import (
	"strings"
	"testing"

	"github.com/Noooblien/agentvcr/pkg/metrics"
)

func TestIntegration_TokenMetrics_DemoSuite(t *testing.T) {
	report := metrics.RunTokenMetricsDemo()
	if report == nil {
		t.Fatalf("expected non-nil demo report")
	}

	if report.SessionID != "trace_token_audit_1760800000" {
		t.Errorf("unexpected session ID: %s", report.SessionID)
	}

	if report.TokenEfficiencyScore < 80.0 {
		t.Errorf("expected high efficiency score due to cache hits, got %.1f", report.TokenEfficiencyScore)
	}

	scorecard := metrics.RenderTokenScorecard(report)
	if !strings.Contains(scorecard, "GRANULAR TOKEN CONSUMPTION METRICS") {
		t.Errorf("scorecard title not found")
	}

	mermaid := metrics.RenderTokenMermaid(report)
	if !strings.Contains(mermaid, "flowchart LR") {
		t.Errorf("mermaid flowchart not found")
	}

	md := metrics.RenderTokenMarkdown(report)
	if !strings.Contains(md, "## AgentVCR Granular Token Consumption & Cost Report") {
		t.Errorf("markdown report not found")
	}
}
