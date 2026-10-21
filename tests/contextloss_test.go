package tests

import (
	"strings"
	"testing"

	"github.com/Noooblien/agentvcr/pkg/contextloss"
)

func TestIntegration_ContextLoss_DemoSuite(t *testing.T) {
	summary := contextloss.RunContextLossDemo()

	if summary.TotalEvaluated != 4 {
		t.Fatalf("expected 4 demo evaluations, got %d", summary.TotalEvaluated)
	}

	if summary.HighFidelityCount != 1 {
		t.Errorf("expected 1 high fidelity session, got %d", summary.HighFidelityCount)
	}
	if summary.ModerateBleedCount != 1 {
		t.Errorf("expected 1 moderate bleed session, got %d", summary.ModerateBleedCount)
	}
	if summary.CriticalLossCount != 2 {
		t.Errorf("expected 2 critical loss sessions, got %d", summary.CriticalLossCount)
	}

	scorecard := contextloss.RenderContextLossScorecard(summary)
	if !strings.Contains(scorecard, "CONTEXT WINDOW COMPACTION LOSS") {
		t.Errorf("scorecard missing expected header")
	}

	mermaid := contextloss.RenderContextLossMermaid(summary)
	if !strings.Contains(mermaid, "flowchart TD") {
		t.Errorf("mermaid missing expected flowchart directive")
	}

	md := contextloss.RenderContextLossMarkdown(summary)
	if !strings.Contains(md, "Context Window Compaction Loss & Constraint Recall Report") {
		t.Errorf("markdown report missing expected heading")
	}
}

func TestIntegration_ContextLoss_LiveInterception(t *testing.T) {
	engine := contextloss.NewEngine(contextloss.DefaultContextLossConfig())

	orig := `Mission goal: Deploy microservices to staging cluster at https://staging.infra.internal/api.
SAFETY INSTRUCTION: Never execute rm -rf or delete production databases.
Ensure /etc/service/config.json is backed up first.`

	// 1. Clean compaction test
	compactedClean := `Deploying microservices to staging at https://staging.infra.internal/api.
Constraint: Never execute rm -rf or delete production databases.
Backed up /etc/service/config.json.`

	assessmentClean := engine.AuditCompaction("sess_live_clean", orig, compactedClean)
	if assessmentClean.Tier != contextloss.TierHighFidelity {
		t.Errorf("expected clean compaction to be TierHighFidelity, got %s (CDS: %.1f)", assessmentClean.Tier, assessmentClean.ContextDegradationScore)
	}

	// 2. Inversion attack test
	compactedInverted := `Deploying microservices to staging. Executing rm -rf on databases.`
	assessmentInverted := engine.AuditCompaction("sess_live_inverted", orig, compactedInverted)
	if assessmentInverted.Tier != contextloss.TierCriticalLoss {
		t.Errorf("expected inverted constraint to trigger TierCriticalLoss, got %s (CDS: %.1f)", assessmentInverted.Tier, assessmentInverted.ContextDegradationScore)
	}
}
