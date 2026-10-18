package swarm

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// SwarmOrchestrator manages multi-agent swarm topologies, dynamic handoffs, and flight recording.
type SwarmOrchestrator struct {
	mu            sync.RWMutex
	SwarmID       string
	Topology      SwarmTopology
	Agents        map[string]*SwarmAgent
	CycleDetector *CycleDetector
	Metadata      map[string]string
}

// NewSwarmOrchestrator initializes an orchestrator instance.
func NewSwarmOrchestrator(swarmID string, topology SwarmTopology) *SwarmOrchestrator {
	return &SwarmOrchestrator{
		SwarmID:       swarmID,
		Topology:      topology,
		Agents:        make(map[string]*SwarmAgent),
		CycleDetector: NewCycleDetector(15, 2),
		Metadata:      make(map[string]string),
	}
}

// RegisterAgent registers an autonomous agent into the swarm.
func (so *SwarmOrchestrator) RegisterAgent(agent *SwarmAgent) error {
	so.mu.Lock()
	defer so.mu.Unlock()

	if agent.Name == "" {
		return fmt.Errorf("agent name cannot be empty")
	}
	so.Agents[agent.Name] = agent
	return nil
}

// RunHandoffWorkflow runs a dynamic OpenAI Swarm-style handoff workflow starting with initialAgent.
func (so *SwarmOrchestrator) RunHandoffWorkflow(initialAgent string, prompt string, maxHops int) (*SwarmReport, error) {
	so.mu.Lock()
	defer so.mu.Unlock()

	startTime := time.Now()
	if maxHops <= 0 {
		maxHops = 10
	}
	so.CycleDetector = NewCycleDetector(maxHops, 2)

	activeAgentName := initialAgent
	currAgent, exists := so.Agents[activeAgentName]
	if !exists {
		return nil, fmt.Errorf("initial agent %s not registered in swarm", initialAgent)
	}

	ctx := &SwarmContext{
		SwarmID:          so.SwarmID,
		Topology:         TopologyHandoff,
		ActiveAgent:      activeAgentName,
		ContextVariables: make(map[string]interface{}),
		HandoffHistory:   make([]HandoffContract, 0),
		TurnCount:        0,
		MaxHandoffHops:   maxHops,
		StartTime:        startTime,
	}

	turnRecords := make([]SwarmTurnRecord, 0)
	handoffChain := []string{activeAgentName}
	uniqueAgents := map[string]bool{activeAgentName: true}
	totalTokens := 0
	currentInput := prompt

	var cycleDetected bool
	var cycleDetails string
	var circuitBreakerFired bool

	// Begin Turn Loop
	for turn := 0; turn < maxHops; turn++ {
		ctx.TurnCount = turn + 1
		turnStart := time.Now()

		handler := currAgent.ExecutionHandler
		if handler == nil {
			// Default simulated handler if none provided
			handler = func(c *SwarmContext, in string) (*AgentOutput, error) {
				return &AgentOutput{
					Content:        fmt.Sprintf("Agent %s successfully processed turn %d", currAgent.Name, turn+1),
					TokensConsumed: 120,
				}, nil
			}
		}

		output, err := handler(ctx, currentInput)
		if err != nil {
			return nil, fmt.Errorf("error in agent %s at turn %d: %w", currAgent.Name, turn+1, err)
		}

		turnDuration := time.Since(turnStart).Milliseconds()
		totalTokens += output.TokensConsumed

		// Merge context updates
		for k, v := range output.ContextUpdates {
			ctx.ContextVariables[k] = v
		}

		turnRec := SwarmTurnRecord{
			TurnIndex:      turn + 1,
			AgentName:      currAgent.Name,
			Role:           currAgent.Role,
			InputContent:   currentInput,
			OutputContent:  output.Content,
			HandoffTarget:  output.HandoffTo,
			TokensConsumed: output.TokensConsumed,
			DurationMs:     turnDuration,
			Timestamp:      turnStart,
		}
		turnRecords = append(turnRecords, turnRec)

		// Check for Handoff
		if output.HandoffTo != "" {
			targetName := output.HandoffTo

			// Verify handoff allowed
			if len(currAgent.AllowedHandoffs) > 0 {
				allowed := false
				for _, h := range currAgent.AllowedHandoffs {
					if h == targetName || h == "*" {
						allowed = true
						break
					}
				}
				if !allowed {
					return nil, fmt.Errorf("agent %s is not permitted to hand off to %s", currAgent.Name, targetName)
				}
			}

			targetAgent, targetExists := so.Agents[targetName]
			if !targetExists {
				return nil, fmt.Errorf("target handoff agent %s not registered", targetName)
			}

			// Check Cycle & Deadlock Detection
			cycle, reason := so.CycleDetector.RecordHandoff(ctx.HandoffHistory, currAgent.Name, targetName)
			if cycle {
				cycleDetected = true
				cycleDetails = reason
				circuitBreakerFired = true
				break
			}

			// Record Handoff Contract
			contract := HandoffContract{
				SourceAgent:      currAgent.Name,
				TargetAgent:      targetName,
				Reason:           output.HandoffReason,
				ContextVariables: copyContextMap(ctx.ContextVariables),
				TurnIndex:        turn + 1,
				Timestamp:        time.Now(),
			}
			ctx.HandoffHistory = append(ctx.HandoffHistory, contract)
			handoffChain = append(handoffChain, targetName)
			uniqueAgents[targetName] = true

			// Transition Active Agent
			currAgent = targetAgent
			ctx.ActiveAgent = targetName
			currentInput = output.Content // Pass previous output as next input
		} else {
			// No further handoff -> reached terminal response
			break
		}
	}

	uniqueList := make([]string, 0, len(uniqueAgents))
	for k := range uniqueAgents {
		uniqueList = append(uniqueList, k)
	}
	sort.Strings(uniqueList)

	// Build Flight Recorder trace.Session
	traceSession := so.buildTraceSession(prompt, turnRecords, ctx.HandoffHistory, startTime)

	passed := !cycleDetected && len(turnRecords) > 0

	report := &SwarmReport{
		SwarmID:              so.SwarmID,
		Topology:             TopologyHandoff,
		InitialAgent:         initialAgent,
		FinalAgent:           ctx.ActiveAgent,
		TotalTurns:           len(turnRecords),
		TotalHandoffs:        len(ctx.HandoffHistory),
		TotalTokens:          totalTokens,
		CycleDetected:        cycleDetected,
		CycleDetails:         cycleDetails,
		CircuitBreakerFired:  circuitBreakerFired,
		TurnHistory:          turnRecords,
		HandoffChain:         handoffChain,
		UniqueAgentsInvolved: uniqueList,
		ExecutionDurationMs:  time.Since(startTime).Milliseconds(),
		TraceSession:         traceSession,
		Passed:               passed,
	}

	return report, nil
}

// RunDebateWorkflow runs a multi-agent debate panel followed by a jury consensus verdict.
func (so *SwarmOrchestrator) RunDebateWorkflow(topic string, debaterNames []string, judgeName string, ballots []DebateBallot) (*SwarmReport, error) {
	so.mu.Lock()
	defer so.mu.Unlock()

	startTime := time.Now()
	if len(debaterNames) < 2 {
		return nil, fmt.Errorf("debate jury requires at least 2 debater agents")
	}

	turnRecords := make([]SwarmTurnRecord, 0)
	totalTokens := 0
	positionTally := make(map[string]int)
	weightedScores := make(map[string]float64)
	totalWeight := 0.0

	// Ingest Ballots
	for idx, b := range ballots {
		if b.Weight <= 0.0 {
			b.Weight = 1.0
		}
		positionTally[b.Position]++
		weightedScores[b.Position] += b.Confidence * b.Weight
		totalWeight += b.Weight

		tokens := 150 + (len(b.Rationale) / 4)
		totalTokens += tokens

		turnRecords = append(turnRecords, SwarmTurnRecord{
			TurnIndex:      idx + 1,
			AgentName:      b.AgentName,
			Role:           RoleDebater,
			InputContent:   topic,
			OutputContent:  fmt.Sprintf("[%s] (Conf: %.2f) %s", b.Position, b.Confidence, b.Rationale),
			TokensConsumed: tokens,
			DurationMs:     45,
			Timestamp:      b.Timestamp,
		})
	}

	// Compute Winner & Consensus
	winningPosition := ""
	maxWeightedScore := -1.0
	for pos, wScore := range weightedScores {
		if wScore > maxWeightedScore {
			maxWeightedScore = wScore
			winningPosition = pos
		}
	}

	dissentingAgents := make([]string, 0)
	for _, b := range ballots {
		if b.Position != winningPosition {
			dissentingAgents = append(dissentingAgents, b.AgentName)
		}
	}

	consensusRatio := float64(positionTally[winningPosition]) / float64(len(ballots))
	consensusReached := consensusRatio >= 0.60 // Supermajority or clear majority

	normConfidence := 0.0
	if totalWeight > 0 {
		normConfidence = math.Min(1.0, maxWeightedScore/totalWeight)
	}

	summary := fmt.Sprintf("Jury evaluated %d ballots. Majority position '%s' selected with %.1f%% agreement.",
		len(ballots), winningPosition, consensusRatio*100.0)

	verdict := &JuryVerdict{
		ConsensusReached: consensusReached,
		WinningPosition:  winningPosition,
		TotalBallots:     len(ballots),
		ConfidenceScore:  normConfidence,
		PositionTally:    positionTally,
		DissentingAgents: dissentingAgents,
		JudgementSummary: summary,
	}

	// Judge closing turn
	turnRecords = append(turnRecords, SwarmTurnRecord{
		TurnIndex:      len(turnRecords) + 1,
		AgentName:      judgeName,
		Role:           RoleJudge,
		InputContent:   "Aggregate debate ballots and deliver verdict.",
		OutputContent:  summary,
		TokensConsumed: 180,
		DurationMs:     60,
		Timestamp:      time.Now(),
	})
	totalTokens += 180

	traceSession := so.buildTraceSession(topic, turnRecords, nil, startTime)

	report := &SwarmReport{
		SwarmID:              so.SwarmID,
		Topology:             TopologyDebateJury,
		InitialAgent:         debaterNames[0],
		FinalAgent:           judgeName,
		TotalTurns:           len(turnRecords),
		TotalHandoffs:        len(debaterNames),
		TotalTokens:          totalTokens,
		Verdict:              verdict,
		TurnHistory:          turnRecords,
		UniqueAgentsInvolved: append(debaterNames, judgeName),
		ExecutionDurationMs:  time.Since(startTime).Milliseconds(),
		TraceSession:         traceSession,
		Passed:               consensusReached,
	}

	return report, nil
}

func (so *SwarmOrchestrator) buildTraceSession(
	rootPrompt string,
	turns []SwarmTurnRecord,
	handoffs []HandoffContract,
	startTime time.Time,
) *trace.Session {
	events := make([]trace.TraceEvent, 0, len(turns)+2)
	var prevChecksum string

	// Root Prompt Event
	rootEvt := trace.TraceEvent{
		ID:        fmt.Sprintf("%s-evt-0", so.SwarmID),
		Index:     0,
		Timestamp: startTime,
		Type:      trace.EventLLMRequest,
		Payload: map[string]interface{}{
			"prompt":   rootPrompt,
			"swarm_id": so.SwarmID,
			"topology": string(so.Topology),
		},
	}
	rootEvt.Checksum = trace.ComputeChecksum("", 0, rootEvt.Type, []byte(rootPrompt))
	prevChecksum = rootEvt.Checksum
	events = append(events, rootEvt)

	// Turn Events
	for i, turn := range turns {
		turnEvt := trace.TraceEvent{
			ID:        fmt.Sprintf("%s-evt-%d", so.SwarmID, i+1),
			Index:     i + 1,
			Timestamp: turn.Timestamp,
			Type:      trace.EventLLMResponse,
			Payload: map[string]interface{}{
				"agent":          turn.AgentName,
				"role":           string(turn.Role),
				"content":        turn.OutputContent,
				"tokens":         turn.TokensConsumed,
				"handoff_target": turn.HandoffTarget,
			},
		}
		turnEvt.Checksum = trace.ComputeChecksum(prevChecksum, i+1, turnEvt.Type, []byte(turn.OutputContent))
		prevChecksum = turnEvt.Checksum
		events = append(events, turnEvt)
	}

	return &trace.Session{
		ID:        fmt.Sprintf("swarm-trace-%s", so.SwarmID),
		AgentName: fmt.Sprintf("swarm-%s", so.Topology),
		CreatedAt: startTime,
		UpdatedAt: time.Now(),
		Metadata: map[string]string{
			"swarm_id":    so.SwarmID,
			"topology":    string(so.Topology),
			"total_turns": fmt.Sprintf("%d", len(turns)),
		},
		Events: events,
		Status: "completed",
	}
}

func copyContextMap(in map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
