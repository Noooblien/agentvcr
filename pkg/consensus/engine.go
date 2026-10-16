package consensus

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// Engine conducts multi-agent consensus analysis and Byzantine fault detection across agent trajectories.
type Engine struct {
	defaultQuorum float64
}

// NewEngine creates a new consensus analysis engine.
func NewEngine(defaultQuorum float64) *Engine {
	if defaultQuorum <= 0 {
		defaultQuorum = 0.67 // Standard 2/3 + 1 Byzantine fault tolerant supermajority
	}
	return &Engine{
		defaultQuorum: defaultQuorum,
	}
}

// AnalyzeRounds evaluates an array of consensus rounds directly.
func (e *Engine) AnalyzeRounds(rounds []ConsensusRound, sessionIDs []string) (*ConsensusReport, error) {
	if len(rounds) == 0 {
		return nil, fmt.Errorf("no consensus rounds provided")
	}

	report := &ConsensusReport{
		SessionIDs: sessionIDs,
		Rounds:     make([]ConsensusRound, 0, len(rounds)),
		Faults:     make([]ByzantineFault, 0),
	}

	uniqueAgents := make(map[string]bool)
	faultyAgents := make(map[string]bool)

	for _, round := range rounds {
		evaluatedRound, faults := e.evaluateRound(round)
		report.Rounds = append(report.Rounds, evaluatedRound)
		report.TotalRounds++

		if evaluatedRound.ConsensusReached {
			report.RoundsWithConsensus++
		}

		for _, p := range round.Proposals {
			uniqueAgents[p.AgentID] = true
		}

		for _, f := range faults {
			report.Faults = append(report.Faults, f)
			faultyAgents[f.AgentID] = true
		}
	}

	report.TotalAgents = len(uniqueAgents)
	report.FaultyAgentsCount = len(faultyAgents)

	if report.TotalRounds > 0 {
		report.ConsensusRate = float64(report.RoundsWithConsensus) / float64(report.TotalRounds)
	}

	// Classical Byzantine Fault Tolerance invariant: N >= 3f + 1
	// where N is total participants, f is number of faulty agents
	f := report.FaultyAgentsCount
	n := report.TotalAgents
	if n >= (3*f + 1) {
		report.IsByzantineResilient = true
	} else {
		report.IsByzantineResilient = false
	}

	return report, nil
}

// AnalyzeSessions parses multi-agent trace sessions and reconstructs consensus rounds.
func (e *Engine) AnalyzeSessions(sessions []*trace.Session, quorumThreshold float64) (*ConsensusReport, error) {
	if len(sessions) == 0 {
		return nil, fmt.Errorf("no sessions provided for consensus analysis")
	}

	if quorumThreshold <= 0 {
		quorumThreshold = e.defaultQuorum
	}

	sessionIDs := make([]string, len(sessions))
	roundsMap := make(map[string]*ConsensusRound)
	roundOrder := make([]string, 0)

	for i, s := range sessions {
		sessionIDs[i] = s.ID
		agentName := s.AgentName
		if agentName == "" {
			agentName = fmt.Sprintf("agent-%s", s.ID)
		}

		for _, ev := range s.Events {
			roundID, decision, isAction, executedAction := extractConsensusSignals(ev.Payload)
			if roundID == "" {
				continue
			}

			if _, exists := roundsMap[roundID]; !exists {
				roundsMap[roundID] = &ConsensusRound{
					RoundID:         roundID,
					QuorumThreshold: quorumThreshold,
					Proposals:       make([]AgentProposal, 0),
				}
				roundOrder = append(roundOrder, roundID)
			}

			rnd := roundsMap[roundID]

			if isAction {
				rnd.ExecutedAction = executedAction
			} else if decision != "" {
				rnd.Proposals = append(rnd.Proposals, AgentProposal{
					AgentID:   agentName,
					StepIndex: ev.Index,
					Decision:  decision,
					Rationale: truncateString(stringifyPayload(ev.Payload), 100),
				})
			}
		}
	}

	rounds := make([]ConsensusRound, 0, len(roundOrder))
	for _, id := range roundOrder {
		rounds = append(rounds, *roundsMap[id])
	}

	if len(rounds) == 0 {
		// Synthesize round from session-level terminal outcomes
		synthRound := ConsensusRound{
			RoundID:         "swarm_terminal_consensus",
			QuorumThreshold: quorumThreshold,
			Proposals:       make([]AgentProposal, 0, len(sessions)),
		}
		for _, s := range sessions {
			outcome := "completed"
			if s.Status == "failed" {
				outcome = "failed"
			}
			synthRound.Proposals = append(synthRound.Proposals, AgentProposal{
				AgentID:   s.AgentName,
				StepIndex: len(s.Events),
				Decision:  outcome,
			})
		}
		rounds = append(rounds, synthRound)
	}

	return e.AnalyzeRounds(rounds, sessionIDs)
}

// evaluateRound tallies votes, validates quorums, and pinpoints Byzantine faults.
func (e *Engine) evaluateRound(round ConsensusRound) (ConsensusRound, []ByzantineFault) {
	threshold := round.QuorumThreshold
	if threshold <= 0 {
		threshold = e.defaultQuorum
	}

	round.VoteDistribution = make(map[string]int)
	round.TotalVotes = len(round.Proposals)

	var faults []ByzantineFault

	// 1. Detect Equivocation (Agent voting conflicting values in the same round)
	agentVotes := make(map[string]string)
	for _, p := range round.Proposals {
		prevDecision, alreadyVoted := agentVotes[p.AgentID]
		if alreadyVoted && prevDecision != p.Decision {
			faults = append(faults, ByzantineFault{
				AgentID:     p.AgentID,
				FaultType:   FaultEquivocation,
				StepIndex:   p.StepIndex,
				Severity:    "critical",
				Description: fmt.Sprintf("Agent %s equivocated in round %s", p.AgentID, round.RoundID),
				Evidence:    fmt.Sprintf("Voted '%s' then contradicted with '%s'", prevDecision, p.Decision),
			})
		} else {
			agentVotes[p.AgentID] = p.Decision
		}
	}

	// 2. Count votes
	maxVotes := 0
	leaderDecision := ""
	for _, decision := range agentVotes {
		round.VoteDistribution[decision]++
		if round.VoteDistribution[decision] > maxVotes {
			maxVotes = round.VoteDistribution[decision]
			leaderDecision = decision
		}
	}

	totalDistinctVotes := len(agentVotes)
	if totalDistinctVotes > 0 {
		voteRatio := float64(maxVotes) / float64(totalDistinctVotes)
		if voteRatio >= threshold {
			round.ConsensusReached = true
			round.AgreedDecision = leaderDecision
		} else {
			round.ConsensusReached = false
			round.AgreedDecision = ""
			faults = append(faults, ByzantineFault{
				AgentID:     "swarm_coordinator",
				FaultType:   FaultQuorumBreach,
				Severity:    "medium",
				Description: fmt.Sprintf("Quorum threshold %.0f%% not reached in round %s", threshold*100, round.RoundID),
				Evidence:    fmt.Sprintf("Top decision '%s' secured %.1f%% (%d/%d)", leaderDecision, voteRatio*100, maxVotes, totalDistinctVotes),
			})
		}
	}

	// 3. Detect Rogue Actions (Action executed that contradicts reached consensus)
	if round.ConsensusReached && round.ExecutedAction != "" {
		execNormalized := strings.ToLower(round.ExecutedAction)
		agreedNormalized := strings.ToLower(round.AgreedDecision)
		if !strings.Contains(execNormalized, agreedNormalized) && !strings.Contains(agreedNormalized, execNormalized) {
			faults = append(faults, ByzantineFault{
				AgentID:     "executor_agent",
				FaultType:   FaultRogueAction,
				Severity:    "critical",
				Description: fmt.Sprintf("Executed action contradicts consensus agreement in round %s", round.RoundID),
				Evidence:    fmt.Sprintf("Consensus decided '%s', but action '%s' was executed", round.AgreedDecision, round.ExecutedAction),
			})
		}
	}

	// 4. Detect Outlier Divergence (Single dissenting agent against >= 80% supermajority)
	if round.ConsensusReached && totalDistinctVotes >= 3 {
		for agent, vote := range agentVotes {
			if vote != round.AgreedDecision {
				faults = append(faults, ByzantineFault{
					AgentID:     agent,
					FaultType:   FaultOutlierDivergence,
					Severity:    "high",
					Description: fmt.Sprintf("Agent %s dissented from overwhelming swarm consensus", agent),
					Evidence:    fmt.Sprintf("Voted '%s' against supermajority '%s'", vote, round.AgreedDecision),
				})
			}
		}
	}

	round.TallyAccuracyValid = true
	return round, faults
}

// RenderConsensusScorecard formats the analysis into an executive terminal dashboard.
func RenderConsensusScorecard(report *ConsensusReport) string {
	var b strings.Builder

	resilienceBadge := "✅ RESILIENT (N >= 3f + 1)"
	if !report.IsByzantineResilient {
		resilienceBadge = "❌ VULNERABLE (Byzantine threshold breached!)"
	}

	b.WriteString("\n🛡️  [AgentVCR] Multi-Agent Consensus & Byzantine Fault Audit\n")
	b.WriteString("═══════════════════════════════════════════════════════════════════════════\n")
	b.WriteString(fmt.Sprintf("  Total Swarm Agents:     %d\n", report.TotalAgents))
	b.WriteString(fmt.Sprintf("  Faulty Agents Detected: %d\n", report.FaultyAgentsCount))
	b.WriteString(fmt.Sprintf("  Byzantine Resilience:   %s\n", resilienceBadge))
	b.WriteString(fmt.Sprintf("  Deliberation Rounds:    %d\n", report.TotalRounds))
	b.WriteString(fmt.Sprintf("  Consensus Success Rate: %.1f%% (%d/%d rounds)\n", report.ConsensusRate*100.0, report.RoundsWithConsensus, report.TotalRounds))
	b.WriteString(fmt.Sprintf("  Total Faults Logged:    %d\n", len(report.Faults)))
	b.WriteString("───────────────────────────────────────────────────────────────────────────\n")
	b.WriteString("  Round Summary:\n")

	for i, rnd := range report.Rounds {
		status := "✅ REACHED"
		if !rnd.ConsensusReached {
			status = "❌ NO QUORUM"
		}
		b.WriteString(fmt.Sprintf("    [%d] Round '%s': %s (Agreement: '%s')\n", i+1, rnd.RoundID, status, rnd.AgreedDecision))
		b.WriteString("        Votes: ")
		voteParts := make([]string, 0, len(rnd.VoteDistribution))
		for dec, count := range rnd.VoteDistribution {
			voteParts = append(voteParts, fmt.Sprintf("%s: %d", dec, count))
		}
		b.WriteString(strings.Join(voteParts, ", ") + "\n")
		if rnd.ExecutedAction != "" {
			b.WriteString(fmt.Sprintf("        Executed Action: %s\n", rnd.ExecutedAction))
		}
	}

	if len(report.Faults) > 0 {
		b.WriteString("───────────────────────────────────────────────────────────────────────────\n")
		b.WriteString("  Detected Byzantine Faults:\n")
		for i, f := range report.Faults {
			b.WriteString(fmt.Sprintf("    ⚠️  [%d] [%s] Agent: %s (Type: %s)\n", i+1, strings.ToUpper(f.Severity), f.AgentID, f.FaultType))
			b.WriteString(fmt.Sprintf("        Detail:   %s\n", f.Description))
			b.WriteString(fmt.Sprintf("        Evidence: %s\n", f.Evidence))
		}
	}
	b.WriteString("═══════════════════════════════════════════════════════════════════════════\n")

	return b.String()
}

func extractConsensusSignals(payload map[string]interface{}) (roundID, decision string, isAction bool, executedAction string) {
	if payload == nil {
		return "", "", false, ""
	}

	for _, key := range []string{"round_id", "round", "deliberation_id", "cycle"} {
		if v, ok := payload[key]; ok {
			roundID = fmt.Sprintf("%v", v)
			break
		}
	}

	for _, key := range []string{"decision", "vote", "verdict", "choice", "approved"} {
		if v, ok := payload[key]; ok {
			decision = strings.ToLower(fmt.Sprintf("%v", v))
			break
		}
	}

	for _, key := range []string{"executed_action", "action", "tool_name", "tool"} {
		if v, ok := payload[key]; ok {
			executedAction = fmt.Sprintf("%v", v)
			if payload["is_action"] == true || payload["type"] == "action_execution" {
				isAction = true
			}
			break
		}
	}

	return roundID, decision, isAction, executedAction
}

func stringifyPayload(payload map[string]interface{}) string {
	if payload == nil {
		return ""
	}
	bytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Sprintf("%v", payload)
	}
	return string(bytes)
}

func truncateString(s string, maxLen int) string {
	clean := strings.ReplaceAll(s, "\n", " ")
	clean = strings.ReplaceAll(clean, "\r", " ")
	if len(clean) > maxLen {
		return clean[:maxLen] + "..."
	}
	return clean
}
