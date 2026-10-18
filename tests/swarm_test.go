package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/swarm"
)

func TestSwarm_DynamicHandoffSuccess(t *testing.T) {
	orc := swarm.NewSwarmOrchestrator("swarm-support-triage", swarm.TopologyHandoff)

	// Agent 1: Triage
	triageAgent := &swarm.SwarmAgent{
		Name:            "triage-router",
		Role:            swarm.RoleTriage,
		Instructions:    "Analyze user request and route to billing or tech support.",
		AllowedHandoffs: []string{"billing-specialist", "tech-specialist"},
		ExecutionHandler: func(ctx *swarm.SwarmContext, input string) (*swarm.AgentOutput, error) {
			return &swarm.AgentOutput{
				Content: "Detected enterprise invoice request. Transferring to billing specialist.",
				ContextUpdates: map[string]interface{}{
					"customer_id": "cust-enterprise-991",
					"urgency":     "high",
				},
				HandoffTo:      "billing-specialist",
				HandoffReason:  "Customer requires invoice adjustment",
				TokensConsumed: 95,
			}, nil
		},
	}

	// Agent 2: Billing Specialist
	billingAgent := &swarm.SwarmAgent{
		Name:            "billing-specialist",
		Role:            swarm.RoleCoder,
		Instructions:    "Resolve customer billing inquiries and generate updated receipts.",
		AllowedHandoffs: nil, // Terminal agent
		ExecutionHandler: func(ctx *swarm.SwarmContext, input string) (*swarm.AgentOutput, error) {
			custID, _ := ctx.ContextVariables["customer_id"].(string)
			return &swarm.AgentOutput{
				Content:        "Invoice #INV-2026-882 adjusted for customer " + custID + ". Receipt emailed successfully.",
				TokensConsumed: 110,
			}, nil
		},
	}

	if err := orc.RegisterAgent(triageAgent); err != nil {
		t.Fatalf("Failed to register triage agent: %v", err)
	}
	if err := orc.RegisterAgent(billingAgent); err != nil {
		t.Fatalf("Failed to register billing agent: %v", err)
	}

	report, err := orc.RunHandoffWorkflow("triage-router", "Please send updated invoice for last quarter", 5)
	if err != nil {
		t.Fatalf("Swarm execution failed: %v", err)
	}

	if !report.Passed {
		t.Fatalf("Expected report to pass, but failed")
	}
	if report.TotalTurns != 2 {
		t.Fatalf("Expected 2 turns (Triage -> Billing), got %d", report.TotalTurns)
	}
	if report.TotalHandoffs != 1 {
		t.Fatalf("Expected 1 handoff, got %d", report.TotalHandoffs)
	}
	if report.FinalAgent != "billing-specialist" {
		t.Fatalf("Expected final agent to be billing-specialist, got %s", report.FinalAgent)
	}
	if report.CycleDetected {
		t.Fatalf("Did not expect cycle detection on linear handoff")
	}

	// Check Trace Session recording
	if report.TraceSession == nil {
		t.Fatalf("Expected flight recorded TraceSession, got nil")
	}
	if len(report.TraceSession.Events) != 3 { // root + 2 turns
		t.Fatalf("Expected 3 trace events, got %d", len(report.TraceSession.Events))
	}
}

func TestSwarm_PingPongCycleDetection(t *testing.T) {
	orc := swarm.NewSwarmOrchestrator("swarm-deadlock-test", swarm.TopologyHandoff)

	// Agent A hands off to Agent B
	agentA := &swarm.SwarmAgent{
		Name:            "agent-alpha",
		Role:            swarm.RolePlanner,
		AllowedHandoffs: []string{"agent-beta"},
		ExecutionHandler: func(ctx *swarm.SwarmContext, input string) (*swarm.AgentOutput, error) {
			return &swarm.AgentOutput{
				Content:        "Alpha passing task to Beta",
				HandoffTo:      "agent-beta",
				TokensConsumed: 40,
			}, nil
		},
	}

	// Agent B hands off back to Agent A
	agentB := &swarm.SwarmAgent{
		Name:            "agent-beta",
		Role:            swarm.RoleCoder,
		AllowedHandoffs: []string{"agent-alpha"},
		ExecutionHandler: func(ctx *swarm.SwarmContext, input string) (*swarm.AgentOutput, error) {
			return &swarm.AgentOutput{
				Content:        "Beta passing task back to Alpha",
				HandoffTo:      "agent-alpha",
				TokensConsumed: 40,
			}, nil
		},
	}

	_ = orc.RegisterAgent(agentA)
	_ = orc.RegisterAgent(agentB)

	report, err := orc.RunHandoffWorkflow("agent-alpha", "Solve task", 10)
	if err != nil {
		t.Fatalf("Unexpected execution error: %v", err)
	}

	if !report.CycleDetected {
		t.Fatalf("Expected cycle detector to catch ping-pong loop between Alpha and Beta")
	}
	if !report.CircuitBreakerFired {
		t.Fatalf("Expected circuit breaker to fire")
	}
	if !strings.Contains(report.CycleDetails, "Ping-pong deadlock cycle") {
		t.Fatalf("Expected Ping-pong deadlock message in cycle details, got: %s", report.CycleDetails)
	}
}

func TestSwarm_DebateJuryConsensus(t *testing.T) {
	orc := swarm.NewSwarmOrchestrator("swarm-db-debate", swarm.TopologyDebateJury)

	debaters := []string{"architect-alice", "dba-bob", "sre-carol"}
	judge := "lead-judge-dave"

	now := time.Now()
	ballots := []swarm.DebateBallot{
		{
			AgentName:  "architect-alice",
			Position:   "PostgreSQL + Citus",
			Rationale:  "ACID compliance with horizontal sharding suits our relational domain.",
			Confidence: 0.92,
			Weight:     1.0,
			Timestamp:  now.Add(-3 * time.Minute),
		},
		{
			AgentName:  "dba-bob",
			Position:   "PostgreSQL + Citus",
			Rationale:  "Battle-tested extensions and transparent connection pooling.",
			Confidence: 0.88,
			Weight:     1.2,
			Timestamp:  now.Add(-2 * time.Minute),
		},
		{
			AgentName:  "sre-carol",
			Position:   "Apache Cassandra",
			Rationale:  "Active-active multi-region writes without coordinator bottlenecks.",
			Confidence: 0.80,
			Weight:     0.8,
			Timestamp:  now.Add(-1 * time.Minute),
		},
	}

	report, err := orc.RunDebateWorkflow("Database selection for multi-tenant analytics", debaters, judge, ballots)
	if err != nil {
		t.Fatalf("Debate workflow failed: %v", err)
	}

	if report.Verdict == nil {
		t.Fatalf("Expected non-nil jury verdict")
	}
	if !report.Verdict.ConsensusReached {
		t.Fatalf("Expected consensus to be reached (2/3 majority)")
	}
	if report.Verdict.WinningPosition != "PostgreSQL + Citus" {
		t.Fatalf("Expected PostgreSQL to win, got: %s", report.Verdict.WinningPosition)
	}
	if len(report.Verdict.DissentingAgents) != 1 || report.Verdict.DissentingAgents[0] != "sre-carol" {
		t.Fatalf("Expected sre-carol to be dissenting, got: %v", report.Verdict.DissentingAgents)
	}
}

func TestSwarm_ScorecardAndMermaid(t *testing.T) {
	orc := swarm.NewSwarmOrchestrator("swarm-render-test", swarm.TopologyHandoff)
	agent1 := &swarm.SwarmAgent{
		Name:            "agent-1",
		Role:            swarm.RolePlanner,
		AllowedHandoffs: []string{"agent-2"},
		ExecutionHandler: func(ctx *swarm.SwarmContext, in string) (*swarm.AgentOutput, error) {
			return &swarm.AgentOutput{
				Content:        "Step 1 complete",
				HandoffTo:      "agent-2",
				TokensConsumed: 50,
			}, nil
		},
	}
	agent2 := &swarm.SwarmAgent{
		Name: "agent-2",
		Role: swarm.RoleCoder,
		ExecutionHandler: func(ctx *swarm.SwarmContext, in string) (*swarm.AgentOutput, error) {
			return &swarm.AgentOutput{
				Content:        "Step 2 complete (terminal)",
				TokensConsumed: 60,
			}, nil
		},
	}
	_ = orc.RegisterAgent(agent1)
	_ = orc.RegisterAgent(agent2)

	report, err := orc.RunHandoffWorkflow("agent-1", "Start render", 3)
	if err != nil {
		t.Fatalf("Execution failed: %v", err)
	}

	scorecard := swarm.RenderSwarmScorecard(report)
	if !strings.Contains(scorecard, "AGENTVCR // MULTI-AGENT SWARM ORCHESTRATION") {
		t.Fatalf("Scorecard missing header: %s", scorecard)
	}
	if !strings.Contains(scorecard, "agent-1 -> agent-2") {
		t.Fatalf("Scorecard missing handoff chain: %s", scorecard)
	}

	mermaid := swarm.RenderSwarmMermaid(report)
	if !strings.Contains(mermaid, "graph LR") || !strings.Contains(mermaid, "agent_1") {
		t.Fatalf("Mermaid missing graph LR or agent nodes: %s", mermaid)
	}
}
