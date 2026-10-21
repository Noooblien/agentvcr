package deadlock

import (
	"time"
)

// DeadlockType classifies the causal topology of a multi-agent stall.
type DeadlockType string

const (
	TypeCircularWait       DeadlockType = "CIRCULAR_WAIT"        // Circular dependency cycle in message RPC / handoff graph
	TypeResourceContention DeadlockType = "RESOURCE_CONTENTION"  // Exclusive lock collision across files, workspaces, or ports
	TypeOrphanedHandoff    DeadlockType = "ORPHANED_HANDOFF"     // Wait on terminated, crashed, or unreachable subagent
	TypeLivelockSpin       DeadlockType = "LIVELOCK_SPIN"        // Active thrashing/ping-pong message loop without state progression
)

// ResolutionStrategy dictates the autonomous mitigation policy.
type ResolutionStrategy string

const (
	StrategyPreemptOldest      ResolutionStrategy = "PREEMPT_OLDEST"       // Terminate or cancel the oldest blocking request
	StrategyBreakCycleTimeout  ResolutionStrategy = "BREAK_CYCLE_TIMEOUT"  // Force timeout error injection into the cycle edge
	StrategySynthesizeUnblock  ResolutionStrategy = "SYNTHESIZE_UNBLOCK"   // Provide synthetic fallback response to release lock
	StrategyEscalateSupervisor ResolutionStrategy = "ESCALATE_SUPERVISOR"  // Trigger human/supervisor intervention
)

// AgentNode captures the live execution and wait state of an agent.
type AgentNode struct {
	AgentID            string        `json:"agent_id"`
	AgentName          string        `json:"agent_name"`
	State              string        `json:"state"` // "RUNNING", "WAITING", "BLOCKED", "IDLE"
	WaitingForAgentID  string        `json:"waiting_for_agent_id,omitempty"`
	WaitingForResource string        `json:"waiting_for_resource,omitempty"`
	WaitDuration       time.Duration `json:"wait_duration"`
	TurnIndex          int           `json:"turn_index"`
	LastActivity       time.Time     `json:"last_activity"`
}

// DependencyEdge represents a directed wait relationship: FromAgent is waiting on ToAgent or Resource.
type DependencyEdge struct {
	FromAgent string        `json:"from_agent"`
	ToAgent   string        `json:"to_agent"`
	Resource  string        `json:"resource,omitempty"`
	Reason    string        `json:"reason"`
	Since     time.Time     `json:"since"`
	Duration  time.Duration `json:"duration"`
}

// DeadlockCycle represents an invariant violation where a directed cycle blocks execution.
type DeadlockCycle struct {
	CycleID            string             `json:"cycle_id"`
	Type               DeadlockType       `json:"type"`
	InvolvedAgents     []string           `json:"involved_agents"`
	InvolvedResources  []string           `json:"involved_resources,omitempty"`
	CyclePath          []string           `json:"cycle_path"` // e.g. ["agent-A", "agent-B", "agent-C", "agent-A"]
	StallDuration      time.Duration      `json:"stall_duration"`
	ProposedResolution ResolutionStrategy `json:"proposed_resolution"`
	ResolutionAction   string             `json:"resolution_action"`
	Severity           string             `json:"severity"` // "CRITICAL", "HIGH", "MEDIUM"
}

// DeadlockAssessment captures the complete graph audit for a multi-agent swarm.
type DeadlockAssessment struct {
	SwarmID              string          `json:"swarm_id"`
	TotalAgents          int             `json:"total_agents"`
	TotalEdges           int             `json:"total_edges"`
	HasDeadlock          bool            `json:"has_deadlock"`
	HasLivelock          bool            `json:"has_livelock"`
	Cycles               []DeadlockCycle `json:"cycles"`
	DeadlockFreedomScore float64         `json:"deadlock_freedom_score"` // 0.0 to 100.0 (100 = Acyclic/Healthy)
	Status               string          `json:"status"`                 // "HEALTHY", "DEGRADED", "DEADLOCKED"
	EvaluatedAt          time.Time       `json:"evaluated_at"`
}

// DeadlockConfig configures thresholds for graph analysis.
type DeadlockConfig struct {
	StallThreshold     time.Duration `json:"stall_threshold"`     // Min wait before flagging a stall (default: 5s)
	LivelockTurnRepeat int           `json:"livelock_turn_repeat"` // Ping-pong exchange repeat count (default: 3)
	MaxSwarmAgents     int           `json:"max_swarm_agents"`
}

// DeadlockSummary aggregates multiple swarm assessments.
type DeadlockSummary struct {
	TotalSwarmsAudited   int                   `json:"total_swarms_audited"`
	DeadlocksDetected    int                   `json:"deadlocks_detected"`
	LivelocksDetected    int                   `json:"livelocks_detected"`
	CyclesResolved       int                   `json:"cycles_resolved"`
	AverageDFS           float64               `json:"average_dfs"`
	Assessments          []*DeadlockAssessment `json:"assessments"`
}
