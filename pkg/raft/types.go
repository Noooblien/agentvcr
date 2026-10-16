package raft

import (
	"time"
)

// NodeRole represents the Raft consensus role of a node in the multi-region cluster.
type NodeRole string

const (
	RoleFollower  NodeRole = "Follower"
	RoleCandidate NodeRole = "Candidate"
	RoleLeader    NodeRole = "Leader"
)

// LogEntry represents an immutable trajectory event or state transition synced across the multi-region ledger.
type LogEntry struct {
	Index        int64     `json:"index"`
	Term         uint64    `json:"term"`
	SessionID    string    `json:"session_id"`
	StepIndex    int       `json:"step_index"`
	EventDigest  string    `json:"event_digest"` // SHA-256 fingerprint of trajectory event
	PayloadType  string    `json:"payload_type"` // e.g. "tool_call", "llm_request", "checkpoint"
	Data         []byte    `json:"data"`
	Timestamp    time.Time `json:"timestamp"`
	OriginRegion string    `json:"origin_region"`
}

// RaftNodeInfo describes the observable operational state of a Raft node in a specific cloud region.
type RaftNodeInfo struct {
	NodeID           string    `json:"node_id"`
	Region           string    `json:"region"` // e.g. "us-east-1", "eu-west-1", "ap-northeast-1"
	Role             NodeRole  `json:"role"`
	CurrentTerm      uint64    `json:"current_term"`
	VotedFor         string    `json:"voted_for,omitempty"`
	LastLogIndex     int64     `json:"last_log_index"`
	LastLogTerm      uint64    `json:"last_log_term"`
	CommitIndex      int64     `json:"commit_index"`
	AppliedIndex     int64     `json:"applied_index"`
	ReplicationLagMs int64     `json:"replication_lag_ms"`
	NetworkRTTMs     int64     `json:"network_rtt_ms"`
	IsHealthy        bool      `json:"is_healthy"`
	IsPartitioned    bool      `json:"is_partitioned"`
}

// RequestVoteArgs represents the RPC payload sent by Candidates during a Raft election term.
type RequestVoteArgs struct {
	Term         uint64 `json:"term"`
	CandidateID  string `json:"candidate_id"`
	LastLogIndex int64  `json:"last_log_index"`
	LastLogTerm  uint64 `json:"last_log_term"`
}

// RequestVoteReply represents the RPC response granted or rejected by a peer node.
type RequestVoteReply struct {
	Term        uint64 `json:"term"`
	VoteGranted bool   `json:"vote_granted"`
	Reason      string `json:"reason,omitempty"`
}

// AppendEntriesArgs represents the RPC payload sent by the Leader to replicate trajectory logs and heartbeats.
type AppendEntriesArgs struct {
	Term         uint64     `json:"term"`
	LeaderID     string     `json:"leader_id"`
	PrevLogIndex int64      `json:"prev_log_index"`
	PrevLogTerm  uint64     `json:"prev_log_term"`
	Entries      []LogEntry `json:"entries"`
	LeaderCommit int64      `json:"leader_commit"`
}

// AppendEntriesReply represents the follower's response to an AppendEntries RPC.
type AppendEntriesReply struct {
	Term       uint64 `json:"term"`
	Success    bool   `json:"success"`
	MatchIndex int64  `json:"match_index"`
	Reason     string `json:"reason,omitempty"`
}

// NodeConfig specifies static network topology and regional characteristics for a cluster member.
type NodeConfig struct {
	NodeID       string `json:"node_id"`
	Region       string `json:"region"`
	LatencyRTTMs int64  `json:"latency_rtt_ms"`
}

// RaftConfig defines cluster-wide parameters for multi-region replication.
type RaftConfig struct {
	ClusterID             string        `json:"cluster_id"`
	Nodes                 []NodeConfig  `json:"nodes"`
	HeartbeatIntervalMs   int           `json:"heartbeat_interval_ms"`
	ElectionTimeoutMinMs  int           `json:"election_timeout_min_ms"`
	ElectionTimeoutMaxMs  int           `json:"election_timeout_max_ms"`
	SimulateWANLatency    bool          `json:"simulate_wan_latency"`
}

// ReplicationStepResult records the quorum commitment outcome for a specific trajectory event.
type ReplicationStepResult struct {
	EntryIndex           int64    `json:"entry_index"`
	Term                 uint64   `json:"term"`
	StepIndex            int      `json:"step_index"`
	QuorumReached        bool     `json:"quorum_reached"`
	AcksCount            int      `json:"acks_count"`
	TotalNodes           int      `json:"total_nodes"`
	CommitLatencyMs      int64    `json:"commit_latency_ms"`
	ParticipatingRegions []string `json:"participating_regions"`
}

// RaftReport contains the comprehensive attestation and health scorecard for multi-region trajectory sync.
type RaftReport struct {
	SessionID                string                  `json:"session_id"`
	ClusterID                string                  `json:"cluster_id"`
	LeaderID                 string                  `json:"leader_id"`
	LeaderRegion             string                  `json:"leader_region"`
	Term                     uint64                  `json:"term"`
	TotalNodes               int                     `json:"total_nodes"`
	QuorumThreshold          int                     `json:"quorum_threshold"`
	Nodes                    []RaftNodeInfo          `json:"nodes"`
	TotalEntriesCommitted    int                     `json:"total_entries_committed"`
	AverageReplicationLagMs  float64                 `json:"average_replication_lag_ms"`
	MaxReplicationLagMs      int64                   `json:"max_replication_lag_ms"`
	CrossRegionSyncPassed    bool                    `json:"cross_region_sync_passed"`
	ReplicationSteps         []ReplicationStepResult `json:"replication_steps"`
	PartitionsDetected       int                     `json:"partitions_detected"`
	GeneratedAt              time.Time               `json:"generated_at"`
}
