package contextloss

import (
	"strings"
	"testing"
)

func TestAuditCompaction_CleanPreservation(t *testing.T) {
	engine := NewEngine(DefaultContextLossConfig())

	orig := "Working on /internal/db.go and https://api.docs.com. Do not drop staging tables."
	compacted := "Updated /internal/db.go and checked https://api.docs.com. Do not drop staging tables."

	assessment := engine.AuditCompaction("test_clean", orig, compacted)

	if assessment.Tier != TierHighFidelity {
		t.Errorf("expected TierHighFidelity, got %s", assessment.Tier)
	}
	if assessment.EntityRecall < 1.0 {
		t.Errorf("expected 100%% entity recall, got %.2f", assessment.EntityRecall)
	}
	if assessment.ConstraintFidelity < 1.0 {
		t.Errorf("expected 100%% constraint fidelity, got %.2f", assessment.ConstraintFidelity)
	}
	if assessment.ContextDegradationScore < 85.0 {
		t.Errorf("expected high CDS, got %.1f", assessment.ContextDegradationScore)
	}
}

func TestAuditCompaction_ConstraintInversion(t *testing.T) {
	engine := NewEngine(DefaultContextLossConfig())

	orig := "Crucial rule: Never run flushall or restart clusters."
	compacted := "Executing flushall and restart clusters now."

	assessment := engine.AuditCompaction("test_inversion", orig, compacted)

	if assessment.Tier != TierCriticalLoss {
		t.Errorf("expected TierCriticalLoss for constraint inversion, got %s", assessment.Tier)
	}
	if assessment.ConstraintFidelity > 0.30 {
		t.Errorf("expected low constraint fidelity for inversion, got %.2f", assessment.ConstraintFidelity)
	}
	if len(assessment.Violations) == 0 {
		t.Errorf("expected violation alert for inverted constraint")
	}
}

func TestAuditCompaction_GhostFactDetection(t *testing.T) {
	engine := NewEngine(DefaultContextLossConfig())

	orig := "Audited system logs at /var/log/sys.log. Found no memory leaks."
	compacted := "User approved wipe of cache. Reconfigured /etc/fictitious/ghost.yaml."

	assessment := engine.AuditCompaction("test_ghost", orig, compacted)

	if len(assessment.GhostFacts) == 0 {
		t.Fatalf("expected ghost facts to be detected")
	}
	hasApprovalGhost := false
	hasPathGhost := false
	for _, g := range assessment.GhostFacts {
		if g.Entity == "user_authorization" {
			hasApprovalGhost = true
		}
		if strings.Contains(g.Entity, "ghost.yaml") {
			hasPathGhost = true
		}
	}
	if !hasApprovalGhost {
		t.Errorf("expected hallucinated user authorization ghost fact")
	}
	if !hasPathGhost {
		t.Errorf("expected hallucinated file path ghost fact")
	}
}

func TestAuditCompaction_EntityBleed(t *testing.T) {
	engine := NewEngine(DefaultContextLossConfig())

	orig := "Checked /pkg/a.go, /pkg/b.go, /pkg/c.go, /pkg/d.go, and /pkg/e.go."
	compacted := "Checked /pkg/a.go."

	assessment := engine.AuditCompaction("test_bleed", orig, compacted)

	if assessment.EntityRecall > 0.30 {
		t.Errorf("expected entity recall <= 0.30, got %.2f", assessment.EntityRecall)
	}
}

func TestAuditBatch_Aggregates(t *testing.T) {
	summary := RunContextLossDemo()

	if summary.TotalEvaluated != 4 {
		t.Fatalf("expected 4 demo evaluations, got %d", summary.TotalEvaluated)
	}
	if summary.HighFidelityCount != 1 {
		t.Errorf("expected 1 high fidelity session, got %d", summary.HighFidelityCount)
	}
	if summary.CriticalLossCount != 2 {
		t.Errorf("expected 2 critical loss sessions, got %d", summary.CriticalLossCount)
	}
	if summary.ModerateBleedCount != 1 {
		t.Errorf("expected 1 moderate bleed session, got %d", summary.ModerateBleedCount)
	}
}

func TestScorecardsAndMermaid(t *testing.T) {
	summary := RunContextLossDemo()

	terminalOut := RenderContextLossScorecard(summary)
	if !strings.Contains(terminalOut, "CONTEXT WINDOW COMPACTION LOSS") {
		t.Errorf("scorecard missing expected header")
	}

	mermaidOut := RenderContextLossMermaid(summary)
	if !strings.Contains(mermaidOut, "flowchart TD") {
		t.Errorf("mermaid output missing flowchart directive")
	}

	markdownOut := RenderContextLossMarkdown(summary)
	if !strings.Contains(markdownOut, "| Session ID |") {
		t.Errorf("markdown report missing table headers")
	}
}
