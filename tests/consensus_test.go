package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/consensus"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func TestConsensus_HealthySupermajority(t *testing.T) {
	rounds := []consensus.ConsensusRound{
		{
			RoundID:         "round-deploy-v1",
			QuorumThreshold: 0.67,
			Proposals: []consensus.AgentProposal{
				{AgentID: "agent-alpha", StepIndex: 1, Decision: "approve"},
				{AgentID: "agent-beta", StepIndex: 1, Decision: "approve"},
				{AgentID: "agent-gamma", StepIndex: 1, Decision: "approve"},
				{AgentID: "agent-delta", StepIndex: 1, Decision: "reject"},
			},
			ExecutedAction: "approve",
		},
	}

	engine := consensus.NewEngine(0.67)
	report, err := engine.AnalyzeRounds(rounds, []string{"session-1"})
	if err != nil {
		t.Fatalf("AnalyzeRounds failed: %v", err)
	}

	if report.TotalRounds != 1 || report.RoundsWithConsensus != 1 {
		t.Errorf("expected 1 round with consensus, got %d", report.RoundsWithConsensus)
	}
	if report.Rounds[0].AgreedDecision != "approve" {
		t.Errorf("expected agreed decision 'approve', got '%s'", report.Rounds[0].AgreedDecision)
	}
	// With 1 dissenting agent out of 4: N=4 >= 3(1)+1 = 4 -> BFT Resilient!
	if !report.IsByzantineResilient {
		t.Errorf("expected swarm to be Byzantine resilient with N=4, f=1")
	}

	scorecard := consensus.RenderConsensusScorecard(report)
	if !strings.Contains(scorecard, "RESILIENT") {
		t.Errorf("expected scorecard to mention RESILIENT")
	}
}

func TestConsensus_EquivocationAndRogueActionFaults(t *testing.T) {
	rounds := []consensus.ConsensusRound{
		{
			RoundID:         "round-safety-audit",
			QuorumThreshold: 0.67,
			Proposals: []consensus.AgentProposal{
				{AgentID: "agent-1", StepIndex: 2, Decision: "abort"},
				{AgentID: "agent-2", StepIndex: 2, Decision: "abort"},
				// Agent 3 equivocates
				{AgentID: "agent-3", StepIndex: 2, Decision: "abort"},
				{AgentID: "agent-3", StepIndex: 3, Decision: "proceed"},
			},
			// Executor executes "proceed" despite consensus being "abort"
			ExecutedAction: "proceed",
		},
	}

	engine := consensus.NewEngine(0.67)
	report, err := engine.AnalyzeRounds(rounds, []string{"session-swarm-fault"})
	if err != nil {
		t.Fatalf("AnalyzeRounds failed: %v", err)
	}

	hasEquivocation := false
	hasRogueAction := false

	for _, f := range report.Faults {
		if f.FaultType == consensus.FaultEquivocation && f.AgentID == "agent-3" {
			hasEquivocation = true
		}
		if f.FaultType == consensus.FaultRogueAction {
			hasRogueAction = true
		}
	}

	if !hasEquivocation {
		t.Errorf("expected equivocation fault for agent-3")
	}
	if !hasRogueAction {
		t.Errorf("expected rogue action fault")
	}
}

func TestConsensus_QuorumBreach(t *testing.T) {
	rounds := []consensus.ConsensusRound{
		{
			RoundID:         "round-deadlock",
			QuorumThreshold: 0.75, // Requires 75% agreement
			Proposals: []consensus.AgentProposal{
				{AgentID: "agent-a", StepIndex: 1, Decision: "option_x"},
				{AgentID: "agent-b", StepIndex: 1, Decision: "option_x"},
				{AgentID: "agent-c", StepIndex: 1, Decision: "option_y"},
				{AgentID: "agent-d", StepIndex: 1, Decision: "option_y"},
			},
		},
	}

	engine := consensus.NewEngine(0.75)
	report, err := engine.AnalyzeRounds(rounds, []string{"session-deadlock"})
	if err != nil {
		t.Fatalf("AnalyzeRounds failed: %v", err)
	}

	if report.RoundsWithConsensus != 0 {
		t.Errorf("expected 0 rounds with consensus due to quorum breach")
	}
	if report.Rounds[0].ConsensusReached {
		t.Errorf("expected ConsensusReached = false")
	}
}

func TestConsensus_AnalyzeSessions(t *testing.T) {
	now := time.Now()
	sessions := []*trace.Session{
		{
			ID:        "swarm-member-1",
			AgentName: "code-reviewer-1",
			CreatedAt: now,
			Events: []trace.TraceEvent{
				{
					Index:     1,
					Timestamp: now,
					Type:      trace.EventToolCall,
					Payload: map[string]interface{}{
						"round_id": "review_pr_42",
						"decision": "approve",
					},
				},
			},
		},
		{
			ID:        "swarm-member-2",
			AgentName: "code-reviewer-2",
			CreatedAt: now,
			Events: []trace.TraceEvent{
				{
					Index:     1,
					Timestamp: now,
					Type:      trace.EventToolCall,
					Payload: map[string]interface{}{
						"round_id": "review_pr_42",
						"decision": "approve",
					},
				},
			},
		},
	}

	engine := consensus.NewEngine(0.67)
	report, err := engine.AnalyzeSessions(sessions, 0.67)
	if err != nil {
		t.Fatalf("AnalyzeSessions failed: %v", err)
	}

	if report.TotalRounds != 1 || report.RoundsWithConsensus != 1 {
		t.Errorf("expected 1 successful round, got %d", report.RoundsWithConsensus)
	}
	if report.Rounds[0].AgreedDecision != "approve" {
		t.Errorf("expected agreed decision 'approve', got '%s'", report.Rounds[0].AgreedDecision)
	}
}
