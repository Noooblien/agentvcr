package gossip

import (
	"time"
)

// PeerStatus defines the state of a P2P agent node in the gossip network.
type PeerStatus string

const (
	PeerOnline      PeerStatus = "online"
	PeerSuspect     PeerStatus = "suspect"     // Heartbeat delayed; suspected network partition
	PeerByzantine   PeerStatus = "byzantine"   // Confirmed equivocation or invalid ledger state
	PeerOffline     PeerStatus = "offline"
)

// GossipMessage represents an ephemeral peer-to-peer message disseminating trajectory attestations or state updates.
type GossipMessage struct {
	MessageID   string    `json:"message_id"`
	OriginPeer  string    `json:"origin_peer"`
	SequenceNum uint64    `json:"sequence_num"`
	Round       int       `json:"round"`
	PayloadHash string    `json:"payload_hash"`
	PayloadType string    `json:"payload_type"` // e.g. "checkpoint", "vote", "attestation"
	Data        []byte    `json:"data"`
	Timestamp   time.Time `json:"timestamp"`
	Signature   string    `json:"signature"`
}

// LedgerEntry represents an immutable entry committed to the distributed Byzantine quorum ledger.
type LedgerEntry struct {
	Index        int       `json:"index"`
	Round        int       `json:"round"`
	SessionID    string    `json:"session_id"`
	StepIndex    int       `json:"step_index"`
	StateHash    string    `json:"state_hash"`
	Signatures   []string  `json:"signatures"`   // Quorum validator signatures
	CommittedAt  time.Time `json:"committed_at"`
	ValidatorCount int     `json:"validator_count"`
	QuorumPassed bool      `json:"quorum_passed"`
}

// PeerInfo records metadata and reputation of an agent peer in the gossip mesh.
type PeerInfo struct {
	PeerID          string     `json:"peer_id"`
	Address         string     `json:"address"`
	Status          PeerStatus `json:"status"`
	LastHeartbeat   time.Time  `json:"last_heartbeat"`
	ReputationScore float64    `json:"reputation_score"` // 0.0 to 1.0
	MessagesRelayed int        `json:"messages_relayed"`
	FaultsDetected  int        `json:"faults_detected"`
}

// GossipConfig specifies consensus quorum parameters and gossip propagation intervals.
type GossipConfig struct {
	PeerID            string        `json:"peer_id"`
	QuorumThreshold   float64       `json:"quorum_threshold"`    // e.g. 0.67 (2/3+ supermajority)
	Fanout            int           `json:"fanout"`              // number of random peers to gossip per round
	HeartbeatInterval time.Duration `json:"heartbeat_interval"`
	MaxRounds         int           `json:"max_rounds"`
}

// DefaultGossipConfig returns production defaults for multi-agent gossip consensus.
func DefaultGossipConfig(peerID string) GossipConfig {
	return GossipConfig{
		PeerID:            peerID,
		QuorumThreshold:   0.67,
		Fanout:            3,
		HeartbeatInterval: 100 * time.Millisecond,
		MaxRounds:         10,
	}
}

// GossipReport provides a summary of distributed consensus, quorum health, and ledger commits.
type GossipReport struct {
	NodeID             string        `json:"node_id"`
	TotalPeers         int           `json:"total_peers"`
	ActivePeers        int           `json:"active_peers"`
	ByzantinePeers     int           `json:"byzantine_peers"`
	LedgerEntriesCount int           `json:"ledger_entries_count"`
	QuorumIntegrityOK  bool          `json:"quorum_integrity_ok"`
	ConsensusLatencyMs int64         `json:"consensus_latency_ms"`
	Entries            []LedgerEntry `json:"entries"`
	Violations         []string      `json:"violations,omitempty"`
}
