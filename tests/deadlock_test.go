package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/deadlock"
)

func TestIntegration_Deadlock_DemoSuite(t *testing.T) {
	summary := deadlock.RunDeadlockDemo()

	if summary.TotalSwarmsAudited != 4 {
		t.Fatalf("expected 4 demo swarms audited, got %d", summary.TotalSwarmsAudited)
	}

	if summary.DeadlocksDetected != 3 {
		t.Errorf("expected 3 deadlocked swarms, got %d", summary.DeadlocksDetected)
	}

	if summary.CyclesResolved < 3 {
		t.Errorf("expected at least 3 resolved cycles, got %d", summary.CyclesResolved)
	}

	scorecard := deadlock.RenderDeadlockScorecard(summary.Assessments[1])
	if !strings.Contains(scorecard, "MULTI-AGENT DISTRIBUTED DEADLOCK") {
		t.Errorf("scorecard missing expected header")
	}

	mermaid := deadlock.RenderDeadlockMermaid(summary.Assessments[1])
	if !strings.Contains(mermaid, "flowchart LR") {
		t.Errorf("mermaid missing expected flowchart LR directive")
	}

	md := deadlock.RenderDeadlockMarkdown(summary.Assessments[1])
	if !strings.Contains(md, "AgentVCR Distributed Deadlock & Wait-For Graph Report") {
		t.Errorf("markdown report missing expected heading")
	}
}

func TestIntegration_Deadlock_LiveSwarmAnalysis(t *testing.T) {
	cfg := deadlock.DefaultDeadlockConfig()
	engine := deadlock.NewEngine(cfg)

	// Live 2-agent circular dependency
	nodes := []deadlock.AgentNode{
		{AgentID: "agent-alpha", AgentName: "Alpha", State: "WAITING"},
		{AgentID: "agent-beta", AgentName: "Beta", State: "WAITING"},
	}
	edges := []deadlock.DependencyEdge{
		{FromAgent: "agent-alpha", ToAgent: "agent-beta", Duration: 20 * time.Second},
		{FromAgent: "agent-beta", ToAgent: "agent-alpha", Duration: 20 * time.Second},
	}

	assessment := engine.AssessSwarm("swarm-live-test", nodes, edges)
	if !assessment.HasDeadlock {
		t.Fatalf("expected live swarm to detect deadlock")
	}
	if assessment.Status != "DEADLOCKED" {
		t.Fatalf("expected status DEADLOCKED, got %s", assessment.Status)
	}
	if assessment.DeadlockFreedomScore >= 70.0 {
		t.Fatalf("expected degraded score < 70.0, got %.1f", assessment.DeadlockFreedomScore)
	}
}
