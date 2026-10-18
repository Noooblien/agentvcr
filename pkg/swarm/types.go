package swarm

import (
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// SwarmTopology defines the structural coordination pattern of the multi-agent swarm.
type SwarmTopology string

const (
	TopologyHandoff      SwarmTopology = "dynamic_handoff"
	TopologyDebateJury   SwarmTopology = "debate_jury"
	TopologyHierarchical SwarmTopology = "hierarchical_router"
	TopologyRoundRobin   SwarmTopology = "round_robin"
)

// AgentRole defines specialized functional roles within a swarm.
type AgentRole string

const (
	RoleTriage     AgentRole = "triage"
	RolePlanner    AgentRole = "planner"
	RoleCoder      AgentRole = "coder"
	RoleAuditor    AgentRole = "security_auditor"
	RoleReviewer   AgentRole = "reviewer"
	RoleDebater    AgentRole = "debater"
	RoleJudge      AgentRole = "jury_judge"
)

// HandoffContract defines the parameters and context payload transferred during an agent handoff.
type HandoffContract struct {
	SourceAgent      string                 `json:"source_agent"`
	TargetAgent      string                 `json:"target_agent"`
	Reason           string                 `json:"reason"`
	ContextVariables map[string]interface{} `json:"context_variables"`
	TurnIndex        int                    `json:"turn_index"`
	Timestamp        time.Time              `json:"timestamp"`
}

// SwarmAgent represents a distinct autonomous participant in the swarm.
type SwarmAgent struct {
	Name             string                                                                                 `json:"name"`
	Role             AgentRole                                                                              `json:"role"`
	Instructions     string                                                                                 `json:"instructions"`
	AllowedHandoffs  []string                                                                               `json:"allowed_handoffs"`
	Tools            []string                                                                               `json:"tools"`
	ExecutionHandler func(ctx *SwarmContext, input string) (*AgentOutput, error)                            `json:"-"`
}

// AgentOutput represents the result produced by an agent execution step.
type AgentOutput struct {
	Content          string                 `json:"content"`
	ToolCalls        []string               `json:"tool_calls,omitempty"`
	ContextUpdates   map[string]interface{} `json:"context_updates,omitempty"`
	HandoffTo        string                 `json:"handoff_to,omitempty"`
	HandoffReason    string                 `json:"handoff_reason,omitempty"`
	TokensConsumed   int                    `json:"tokens_consumed"`
}

// DebateBallot records a formal position and rationale cast by an agent in a debate jury.
type DebateBallot struct {
	AgentName  string    `json:"agent_name"`
	Position   string    `json:"position"`
	Rationale  string    `json:"rationale"`
	Confidence float64   `json:"confidence"` // 0.0 - 1.0
	Weight     float64   `json:"weight"`     // Voting weight
	Timestamp  time.Time `json:"timestamp"`
}

// JuryVerdict summarizes the consensus determination of a swarm debate.
type JuryVerdict struct {
	ConsensusReached bool            `json:"consensus_reached"`
	WinningPosition  string          `json:"winning_position"`
	TotalBallots     int             `json:"total_ballots"`
	ConfidenceScore  float64         `json:"confidence_score"`
	PositionTally    map[string]int  `json:"position_tally"`
	DissentingAgents []string        `json:"dissenting_agents,omitempty"`
	JudgementSummary string          `json:"judgement_summary"`
}

// SwarmContext encapsulates the shared working state across all agents in an active swarm run.
type SwarmContext struct {
	SwarmID          string                 `json:"swarm_id"`
	Topology         SwarmTopology          `json:"topology"`
	ActiveAgent      string                 `json:"active_agent"`
	ContextVariables map[string]interface{} `json:"context_variables"`
	HandoffHistory   []HandoffContract      `json:"handoff_history"`
	TurnCount        int                    `json:"turn_count"`
	MaxHandoffHops   int                    `json:"max_handoff_hops"`
	StartTime        time.Time              `json:"start_time"`
}

// SwarmTurnRecord captures a single atomic execution turn within the swarm.
type SwarmTurnRecord struct {
	TurnIndex      int                    `json:"turn_index"`
	AgentName      string                 `json:"agent_name"`
	Role           AgentRole              `json:"role"`
	InputContent   string                 `json:"input_content"`
	OutputContent  string                 `json:"output_content"`
	HandoffTarget  string                 `json:"handoff_target,omitempty"`
	TokensConsumed int                    `json:"tokens_consumed"`
	DurationMs     int64                  `json:"duration_ms"`
	Timestamp      time.Time              `json:"timestamp"`
}

// SwarmReport provides an end-to-end audit summary of a swarm trajectory.
type SwarmReport struct {
	SwarmID              string            `json:"swarm_id"`
	Topology             SwarmTopology     `json:"topology"`
	InitialAgent         string            `json:"initial_agent"`
	FinalAgent           string            `json:"final_agent"`
	TotalTurns           int               `json:"total_turns"`
	TotalHandoffs        int               `json:"total_handoffs"`
	TotalTokens          int               `json:"total_tokens"`
	CycleDetected        bool              `json:"cycle_detected"`
	CycleDetails         string            `json:"cycle_details,omitempty"`
	CircuitBreakerFired  bool              `json:"circuit_breaker_fired"`
	Verdict              *JuryVerdict      `json:"verdict,omitempty"`
	TurnHistory          []SwarmTurnRecord `json:"turn_history"`
	HandoffChain         []string          `json:"handoff_chain"`
	UniqueAgentsInvolved []string          `json:"unique_agents_involved"`
	ExecutionDurationMs  int64             `json:"execution_duration_ms"`
	TraceSession         *trace.Session    `json:"trace_session,omitempty"`
	Passed               bool              `json:"passed"`
}
