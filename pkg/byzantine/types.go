package byzantine

import (
	"crypto/ed25519"
	"time"
)

// NodeType represents the operational role of an agent node in the Byzantine consensus network.
type NodeType string

const (
	NodePrimary  NodeType = "primary"  // Round proposer / coordinator
	NodeReplica  NodeType = "replica"  // Active validator peer
	NodeObserver NodeType = "observer" // Passive audit node
)

// NodeStatus indicates whether an agent node is active, slashed, or recovering.
type NodeStatus string

const (
	StatusActive      NodeStatus = "active"
	StatusSlashed     NodeStatus = "slashed"     // Caught equivocating, poisoning, or Byzantine behavior
	StatusQuarantined NodeStatus = "quarantined" // Temporarily isolated pending state synchronization
	StatusRecovering  NodeStatus = "recovering"  // Fast-forwarding state from quorum commit certificates
)

// ConsensusPhase denotes the stages in the PBFT-style pipelined semantic protocol.
type ConsensusPhase string

const (
	PhaseIdle       ConsensusPhase = "IDLE"
	PhasePrePrepare ConsensusPhase = "PRE_PREPARE"
	PhasePrepare    ConsensusPhase = "PREPARE"
	PhaseCommit     ConsensusPhase = "COMMIT"
	PhaseFinalized  ConsensusPhase = "FINALIZED"
	PhaseViewChange ConsensusPhase = "VIEW_CHANGE"
	PhaseRecovering ConsensusPhase = "RECOVERING"
)

// FaultCategory categorizes malicious or abnormal agent behaviors.
type FaultCategory string

const (
	FaultEquivocation       FaultCategory = "equivocation"         // Double-proposing or double-voting in same view/round
	FaultSemanticPoisoning  FaultCategory = "semantic_poisoning"   // Radical adversarial drift or injection in proposal
	FaultStateFork          FaultCategory = "state_fork"           // Creating an unauthorized parallel branch
	FaultTimeoutCensorship  FaultCategory = "timeout_censorship"   // Primary failed to broadcast proposal in time
	FaultInvalidSignature   FaultCategory = "invalid_signature"    // Cryptographically forged or corrupt signature
	FaultCorruptedStateRoot FaultCategory = "corrupted_state_root" // Attempting to commit an unverified state transition
)

// AgentActionProposal represents an agent's proposed action, tool invocation, or conclusion.
type AgentActionProposal struct {
	ProposalID       string            `json:"proposal_id"`
	AgentID          string            `json:"agent_id"`
	Round            int               `json:"round"`
	View             int               `json:"view"`
	ActionIntent     string            `json:"action_intent"`     // e.g. "deploy_microservice"
	NormalizedTokens []string          `json:"normalized_tokens"`
	SemanticHash     string            `json:"semantic_hash"`
	RawPayload       string            `json:"raw_payload"`
	StateRoot        string            `json:"state_root"`
	Metadata         map[string]string `json:"metadata,omitempty"`
	Timestamp        time.Time         `json:"timestamp"`
	Signature        string            `json:"signature"`
}

// SemanticCluster groups syntactically diverse agent proposals that share equivalent semantics.
type SemanticCluster struct {
	CanonicalIntent   string                `json:"canonical_intent"`
	ClusterHash       string                `json:"cluster_hash"`
	Proposals         []AgentActionProposal `json:"proposals"`
	Weight            int                   `json:"weight"`             // Number of proposals in cluster
	AverageSimilarity float64               `json:"average_similarity"` // 0.0 to 1.0
	SupermajorityMet  bool                  `json:"supermajority_met"`
}

// CommitCertificate aggregates 2f + 1 cryptographic validator signatures for a block.
type CommitCertificate struct {
	BlockIndex   int               `json:"block_index"`
	Round        int               `json:"round"`
	View         int               `json:"view"`
	SemanticHash string            `json:"semantic_hash"`
	StateRoot    string            `json:"state_root"`
	Signatures   map[string]string `json:"signatures"` // agentID -> hex signature
	QuorumSize   int               `json:"quorum_size"`
	TotalNodes   int               `json:"total_nodes"`
	CreatedAt    time.Time         `json:"created_at"`
}

// LedgerBlock represents an immutable, cryptographically chained block in the consensus ledger.
type LedgerBlock struct {
	Index           int                   `json:"index"`
	PrevHash        string                `json:"prev_hash"`
	Round           int                   `json:"round"`
	View            int                   `json:"view"`
	LeaderID        string                `json:"leader_id"`
	CanonicalIntent string                `json:"canonical_intent"`
	SemanticHash    string                `json:"semantic_hash"`
	StateRoot       string                `json:"state_root"`
	Proposals       []AgentActionProposal `json:"proposals"`
	CommitCert      *CommitCertificate    `json:"commit_cert"`
	Timestamp       time.Time             `json:"timestamp"`
	BlockHash       string                `json:"block_hash"`
}

// ByzantineFault records evidence of malicious or corrupt agent behavior.
type ByzantineFault struct {
	FaultID     string        `json:"fault_id"`
	AgentID     string        `json:"agent_id"`
	Category    FaultCategory `json:"category"`
	Round       int           `json:"round"`
	View        int           `json:"view"`
	Severity    string        `json:"severity"` // "CRITICAL", "HIGH", "MEDIUM"
	Description string        `json:"description"`
	Evidence    string        `json:"evidence"`
	ActionTaken string        `json:"action_taken"` // e.g. "Slashed and Quarantined"
	Timestamp   time.Time     `json:"timestamp"`
}

// RecoveryAction captures automated self-healing and state reconciliation operations.
type RecoveryAction struct {
	ActionID     string    `json:"action_id"`
	TargetNodeID string    `json:"target_node_id"`
	ActionType   string    `json:"action_type"` // "ViewChange", "StateRollback", "QuorumCatchup"
	FromView     int       `json:"from_view"`
	ToView       int       `json:"to_view"`
	FromBlock    int       `json:"from_block"`
	ToBlock      int       `json:"to_block"`
	Status       string    `json:"status"` // "RECONCILED", "SUCCESS"
	Details      string    `json:"details"`
	Timestamp    time.Time `json:"timestamp"`
}

// NodeIdentity encapsulates cryptographic keys and state for an agent node.
type NodeIdentity struct {
	AgentID    string             `json:"agent_id"`
	Role       NodeType           `json:"role"`
	Status     NodeStatus         `json:"status"`
	PublicKey  ed25519.PublicKey  `json:"-"`
	PrivateKey ed25519.PrivateKey `json:"-"`
	PubKeyHex  string             `json:"pub_key_hex"`
	Reputation float64            `json:"reputation"` // 0.0 - 1.0
}

// ByzantineReport encapsulates multi-agent consensus metrics, fault investigations, and ledger health.
type ByzantineReport struct {
	SessionIDs               []string         `json:"session_ids"`
	TotalNodes               int              `json:"total_nodes"`
	HonestNodes              int              `json:"honest_nodes"`
	ByzantineNodes           int              `json:"byzantine_nodes"`
	MaxFaultTolerance        int              `json:"max_fault_tolerance"` // f = floor((N-1)/3)
	TotalRounds              int              `json:"total_rounds"`
	RoundsWithConsensus      int              `json:"rounds_with_consensus"`
	ConsensusRate            float64          `json:"consensus_rate"`
	SemanticEquivalenceScore float64          `json:"semantic_equivalence_score"`
	LedgerBlockCount         int              `json:"ledger_block_count"`
	LedgerValid              bool             `json:"ledger_valid"`
	FaultsDetected           []ByzantineFault `json:"faults_detected"`
	RecoveriesExecuted       []RecoveryAction `json:"recoveries_executed"`
	IsByzantineResilient     bool             `json:"is_byzantine_resilient"`
	ExecutionDuration        time.Duration    `json:"execution_duration"`
}
