package consensus

// ByzantineFaultType classifies the kind of malicious or malfunctioning behavior detected in multi-agent executions.
type ByzantineFaultType string

const (
	FaultEquivocation       ByzantineFaultType = "equivocation"         // Agent emitted conflicting decisions in the same round
	FaultHallucinatedTally  ByzantineFaultType = "hallucinated_tally"   // Coordinator claimed consensus when quorum was not reached
	FaultRogueAction        ByzantineFaultType = "rogue_action"         // Agent executed action contradicting reached consensus
	FaultOutlierDivergence  ByzantineFaultType = "outlier_divergence"   // Agent vote or output diverged abnormally from swarm
	FaultQuorumBreach       ByzantineFaultType = "quorum_breach"        // Insufficient healthy votes to form Byzantine Fault Tolerant quorum
)

// AgentProposal represents a vote, claim, or proposed action by an agent in a consensus round.
type AgentProposal struct {
	AgentID    string      `json:"agent_id"`
	StepIndex  int         `json:"step_index"`
	Decision   string      `json:"decision"`             // e.g. "approve", "reject", "retry", "escalate"
	Payload    interface{} `json:"payload,omitempty"`
	Confidence float64     `json:"confidence,omitempty"` // 0.0 - 1.0
	Rationale  string      `json:"rationale,omitempty"`
}

// ConsensusRound encapsulates a deliberation or voting cycle among multiple agents.
type ConsensusRound struct {
	RoundID            string          `json:"round_id"`
	QuorumThreshold    float64         `json:"quorum_threshold"`    // e.g. 0.67 for Byzantine 2/3 + 1 supermajority
	Proposals          []AgentProposal `json:"proposals"`
	AgreedDecision     string          `json:"agreed_decision"`     // Decision with supermajority
	VoteDistribution   map[string]int  `json:"vote_distribution"`   // decision -> vote count
	TotalVotes         int             `json:"total_votes"`
	ConsensusReached   bool            `json:"consensus_reached"`
	TallyAccuracyValid bool            `json:"tally_accuracy_valid"`
	ExecutedAction     string          `json:"executed_action,omitempty"`
}

// ByzantineFault records an observed malicious or erratic behavior by an agent.
type ByzantineFault struct {
	AgentID     string             `json:"agent_id"`
	FaultType   ByzantineFaultType `json:"fault_type"`
	StepIndex   int                `json:"step_index"`
	Severity    string             `json:"severity"` // "critical", "high", "medium"
	Description string             `json:"description"`
	Evidence    string             `json:"evidence"`
}

// ConsensusReport encapsulates multi-agent consensus analysis and Byzantine fault detection.
type ConsensusReport struct {
	SessionIDs           []string         `json:"session_ids"`
	TotalRounds          int              `json:"total_rounds"`
	RoundsWithConsensus  int              `json:"rounds_with_consensus"`
	ConsensusRate        float64          `json:"consensus_rate"`
	TotalAgents          int              `json:"total_agents"`
	FaultyAgentsCount    int              `json:"faulty_agents_count"`
	Faults               []ByzantineFault `json:"faults"`
	IsByzantineResilient bool             `json:"is_byzantine_resilient"`
	Rounds               []ConsensusRound `json:"rounds"`
}
