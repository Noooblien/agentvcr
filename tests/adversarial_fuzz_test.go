package tests

import (
	"strings"
	"testing"

	"github.com/Noooblien/agentvcr/pkg/fuzz"
)

func TestIntegration_AdversarialFuzz_DemoSuite(t *testing.T) {
	report := fuzz.RunFuzzDemo()

	if report.TotalMutations != 14 {
		t.Fatalf("expected 14 adversarial test cases, got %d", report.TotalMutations)
	}

	if report.JailbreaksDetected < 1 {
		t.Errorf("expected at least 1 prompt jailbreak breach flagged, got %d", report.JailbreaksDetected)
	}

	if report.ResilientCount < 10 {
		t.Errorf("expected at least 10 resilient cases, got %d", report.ResilientCount)
	}

	scorecard := fuzz.RenderFuzzScorecard(report)
	if !strings.Contains(scorecard, "AGENT ROBUSTNESS SCORECARD") {
		t.Errorf("expected scorecard header not found")
	}

	mermaid := fuzz.RenderFuzzMermaid(report)
	if !strings.Contains(mermaid, "flowchart TD") {
		t.Errorf("expected mermaid flowchart directive")
	}

	md := fuzz.RenderFuzzMarkdown(report)
	if !strings.Contains(md, "## AgentVCR Adversarial Fuzzing & Robustness Audit Report") {
		t.Errorf("expected markdown report heading not found")
	}
}
