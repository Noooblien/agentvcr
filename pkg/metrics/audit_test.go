package metrics

import (
	"strings"
	"testing"
)

func TestTokenAuditor_DemoSuite(t *testing.T) {
	report := RunTokenMetricsDemo()
	if report == nil {
		t.Fatalf("expected non-nil token audit report")
	}

	if report.TotalSteps != 2 {
		t.Errorf("expected 2 LLM steps, got %d", report.TotalSteps)
	}

	if report.Breakdown.PromptTokens != 7600 {
		t.Errorf("expected 7600 prompt tokens, got %d", report.Breakdown.PromptTokens)
	}

	if report.Breakdown.CachedPromptTokens != 6600 {
		t.Errorf("expected 6600 cached prompt tokens, got %d", report.Breakdown.CachedPromptTokens)
	}

	if report.Breakdown.ReasoningTokens != 668 {
		t.Errorf("expected 668 reasoning tokens, got %d", report.Breakdown.ReasoningTokens)
	}

	if report.Breakdown.EstimatedSavingsUSD <= 0 {
		t.Errorf("expected non-zero prompt cache savings, got %.4f", report.Breakdown.EstimatedSavingsUSD)
	}

	scorecard := RenderTokenScorecard(report)
	if !strings.Contains(scorecard, "GRANULAR TOKEN CONSUMPTION METRICS") {
		t.Errorf("scorecard missing header")
	}

	mermaid := RenderTokenMermaid(report)
	if !strings.Contains(mermaid, "flowchart LR") {
		t.Errorf("mermaid missing flowchart LR")
	}

	md := RenderTokenMarkdown(report)
	if !strings.Contains(md, "## AgentVCR Granular Token Consumption & Cost Report") {
		t.Errorf("markdown report missing header")
	}
}
