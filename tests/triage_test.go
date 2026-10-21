package tests

import (
	"strings"
	"testing"

	"github.com/Noooblien/agentvcr/pkg/triage"
)

func TestIntegration_Triage_EndToEnd(t *testing.T) {
	reports, err := triage.RunDemoSuite()
	if err != nil {
		t.Fatalf("failed to run demo suite: %v", err)
	}

	if len(reports) != 4 {
		t.Fatalf("expected 4 triage reports, got %d", len(reports))
	}

	// Report 1: Clean nominal run
	r1 := reports[0]
	if !r1.IsCleanRun || len(r1.Findings) != 0 {
		t.Errorf("scenario 1 should be clean run, got %v with %d findings", r1.IsCleanRun, len(r1.Findings))
	}

	// Report 2: Rate limit
	r2 := reports[1]
	if r2.IsCleanRun || r2.PrimaryCategory != triage.FaultResourceExhaustion {
		t.Errorf("scenario 2 should diagnose %s, got %s", triage.FaultResourceExhaustion, r2.PrimaryCategory)
	}

	// Report 3: Runaway tool loop
	r3 := reports[2]
	if r3.IsCleanRun || r3.PrimaryCategory != triage.FaultRunawayLoop {
		t.Errorf("scenario 3 should diagnose %s, got %s", triage.FaultRunawayLoop, r3.PrimaryCategory)
	}
	if r3.OverallSeverity != triage.SeverityCritical {
		t.Errorf("scenario 3 should have critical severity, got %s", r3.OverallSeverity)
	}

	// Report 4: Permission denied
	r4 := reports[3]
	if r4.IsCleanRun || r4.PrimaryCategory != triage.FaultPermissionDenied {
		t.Errorf("scenario 4 should diagnose %s, got %s", triage.FaultPermissionDenied, r4.PrimaryCategory)
	}

	// Render tests
	for _, r := range reports {
		scorecard := triage.RenderTriageScorecard(r)
		if len(scorecard) == 0 || !strings.Contains(scorecard, "SCORECARD") {
			t.Errorf("scenario %s produced invalid scorecard", r.SessionID)
		}

		mermaid := triage.RenderTriageMermaid(r)
		if len(mermaid) == 0 || !strings.Contains(mermaid, "flowchart TD") {
			t.Errorf("scenario %s produced invalid mermaid diagram", r.SessionID)
		}
	}

	markdown := triage.RenderMarkdownReport(reports)
	if !strings.Contains(markdown, "AgentVCR Automated Root-Cause Triage & Remediation Report") {
		t.Errorf("markdown report missing expected title")
	}
}
