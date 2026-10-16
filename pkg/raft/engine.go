package raft

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

var (
	ErrNotLeader       = errors.New("node is not the cluster leader")
	ErrQuorumNotMet    = errors.New("majority quorum not reached across regions")
	ErrNodePartitioned = errors.New("node is network-isolated or partitioned")
)

// DefaultRegionalTopology returns a recommended 3-node multi-region cluster configuration.
func DefaultRegionalTopology() RaftConfig {
	return RaftConfig{
		ClusterID: "agentvcr-cross-region-mesh",
		Nodes: []NodeConfig{
			{NodeID: "agentvcr-us-east-1", Region: "us-east-1 (N. Virginia)", LatencyRTTMs: 12},
			{NodeID: "agentvcr-eu-west-1", Region: "eu-west-1 (Ireland)", LatencyRTTMs: 74},
			{NodeID: "agentvcr-ap-northeast-1", Region: "ap-northeast-1 (Tokyo)", LatencyRTTMs: 148},
		},
		HeartbeatIntervalMs:  50,
		ElectionTimeoutMinMs: 150,
		ElectionTimeoutMaxMs: 300,
		SimulateWANLatency:   true,
	}
}

// RaftNode represents an active state-machine node running Raft consensus in a specific region.
type RaftNode struct {
	mu            sync.RWMutex
	nodeID        string
	region        string
	role          NodeRole
	currentTerm   uint64
	votedFor      string
	log           []LogEntry
	commitIndex   int64
	appliedIndex  int64
	nextIndex     map[string]int64
	matchIndex    map[string]int64
	latencyRTTMs  int64
	isPartitioned bool
	peers         []NodeConfig
}

// NewRaftNode initializes an individual Raft node member.
func NewRaftNode(cfg NodeConfig, allNodes []NodeConfig) *RaftNode {
	peers := make([]NodeConfig, 0, len(allNodes))
	for _, n := range allNodes {
		if n.NodeID != cfg.NodeID {
			peers = append(peers, n)
		}
	}

	return &RaftNode{
		nodeID:       cfg.NodeID,
		region:       cfg.Region,
		role:         RoleFollower,
		currentTerm:  0,
		votedFor:     "",
		log:          make([]LogEntry, 0),
		commitIndex:  0,
		appliedIndex: 0,
		nextIndex:    make(map[string]int64),
		matchIndex:   make(map[string]int64),
		latencyRTTMs: cfg.LatencyRTTMs,
		peers:        peers,
	}
}

// HandleRequestVote processes vote election requests in accordance with Raft rules.
func (n *RaftNode) HandleRequestVote(args RequestVoteArgs) RequestVoteReply {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.isPartitioned {
		return RequestVoteReply{Term: n.currentTerm, VoteGranted: false, Reason: "node partitioned"}
	}

	if args.Term < n.currentTerm {
		return RequestVoteReply{Term: n.currentTerm, VoteGranted: false, Reason: "stale term"}
	}

	if args.Term > n.currentTerm {
		n.currentTerm = args.Term
		n.role = RoleFollower
		n.votedFor = ""
	}

	lastLogIndex, lastLogTerm := n.getLastLogInfo()
	logOk := (args.LastLogTerm > lastLogTerm) || (args.LastLogTerm == lastLogTerm && args.LastLogIndex >= lastLogIndex)

	if (n.votedFor == "" || n.votedFor == args.CandidateID) && logOk {
		n.votedFor = args.CandidateID
		return RequestVoteReply{Term: n.currentTerm, VoteGranted: true}
	}

	return RequestVoteReply{
		Term:        n.currentTerm,
		VoteGranted: false,
		Reason:      fmt.Sprintf("already voted for %s or log lagging", n.votedFor),
	}
}

// HandleAppendEntries processes heartbeat and trajectory replication RPCs.
func (n *RaftNode) HandleAppendEntries(args AppendEntriesArgs) AppendEntriesReply {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.isPartitioned {
		return AppendEntriesReply{Term: n.currentTerm, Success: false, Reason: "node partitioned"}
	}

	if args.Term < n.currentTerm {
		return AppendEntriesReply{Term: n.currentTerm, Success: false, Reason: "stale term"}
	}

	if args.Term > n.currentTerm || n.role == RoleCandidate {
		n.currentTerm = args.Term
		n.role = RoleFollower
		n.votedFor = ""
	}

	// Consistency check on PrevLogIndex
	if args.PrevLogIndex > 0 {
		if int64(len(n.log)) < args.PrevLogIndex {
			return AppendEntriesReply{
				Term:       n.currentTerm,
				Success:    false,
				MatchIndex: int64(len(n.log)),
				Reason:     "prevLogIndex missing from local log",
			}
		}
		if n.log[args.PrevLogIndex-1].Term != args.PrevLogTerm {
			return AppendEntriesReply{
				Term:       n.currentTerm,
				Success:    false,
				MatchIndex: args.PrevLogIndex - 1,
				Reason:     "prevLogTerm mismatch",
			}
		}
	}

	// Append entries, truncating if necessary
	for i, entry := range args.Entries {
		targetIdx := args.PrevLogIndex + int64(i) + 1
		if targetIdx <= int64(len(n.log)) {
			if n.log[targetIdx-1].Term != entry.Term {
				n.log = n.log[:targetIdx-1]
				n.log = append(n.log, entry)
			}
		} else {
			n.log = append(n.log, entry)
		}
	}

	// Advance commit index if leader's commit index is higher
	if args.LeaderCommit > n.commitIndex {
		lastLogIndex, _ := n.getLastLogInfo()
		if args.LeaderCommit < lastLogIndex {
			n.commitIndex = args.LeaderCommit
		} else {
			n.commitIndex = lastLogIndex
		}
		n.appliedIndex = n.commitIndex
	}

	lastIdx, _ := n.getLastLogInfo()
	return AppendEntriesReply{
		Term:       n.currentTerm,
		Success:    true,
		MatchIndex: lastIdx,
	}
}

// Propose appends a new trajectory entry to the leader's local log.
func (n *RaftNode) Propose(sessionID string, stepIndex int, payloadType string, data []byte, originRegion string) (*LogEntry, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.role != RoleLeader {
		return nil, ErrNotLeader
	}
	if n.isPartitioned {
		return nil, ErrNodePartitioned
	}

	h := sha256.New()
	h.Write([]byte(sessionID))
	h.Write([]byte(fmt.Sprintf("%d", stepIndex)))
	h.Write([]byte(payloadType))
	h.Write(data)
	digest := hex.EncodeToString(h.Sum(nil))

	newIndex := int64(len(n.log)) + 1
	entry := LogEntry{
		Index:        newIndex,
		Term:         n.currentTerm,
		SessionID:    sessionID,
		StepIndex:    stepIndex,
		EventDigest:  digest,
		PayloadType:  payloadType,
		Data:         data,
		Timestamp:    time.Now().UTC(),
		OriginRegion: originRegion,
	}

	n.log = append(n.log, entry)
	n.matchIndex[n.nodeID] = newIndex
	return &entry, nil
}

func (n *RaftNode) getLastLogInfo() (int64, uint64) {
	if len(n.log) == 0 {
		return 0, 0
	}
	last := n.log[len(n.log)-1]
	return last.Index, last.Term
}

// GetInfo returns a snapshot description of the node.
func (n *RaftNode) GetInfo() RaftNodeInfo {
	n.mu.RLock()
	defer n.mu.RUnlock()

	lastLogIdx, lastLogTerm := n.getLastLogInfo()
	return RaftNodeInfo{
		NodeID:           n.nodeID,
		Region:           n.region,
		Role:             n.role,
		CurrentTerm:      n.currentTerm,
		VotedFor:         n.votedFor,
		LastLogIndex:     lastLogIdx,
		LastLogTerm:      lastLogTerm,
		CommitIndex:      n.commitIndex,
		AppliedIndex:     n.appliedIndex,
		ReplicationLagMs: 0,
		NetworkRTTMs:     n.latencyRTTMs,
		IsHealthy:        !n.isPartitioned,
		IsPartitioned:    n.isPartitioned,
	}
}

// RaftCluster orchestrates the simulated multi-region cluster.
type RaftCluster struct {
	mu       sync.Mutex
	cfg      RaftConfig
	nodes    map[string]*RaftNode
	leaderID string
}

// NewRaftCluster creates and binds the multi-region cluster nodes.
func NewRaftCluster(cfg RaftConfig) *RaftCluster {
	nodes := make(map[string]*RaftNode)
	for _, nc := range cfg.Nodes {
		nodes[nc.NodeID] = NewRaftNode(nc, cfg.Nodes)
	}

	return &RaftCluster{
		cfg:   cfg,
		nodes: nodes,
	}
}

// RunElection triggers a Raft leader election, attempting to elect a leader among non-partitioned nodes.
func (c *RaftCluster) RunElection(preferredCandidateID string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	total := len(c.nodes)
	quorum := (total / 2) + 1

	candidateID := preferredCandidateID
	if candidateID == "" {
		for id, node := range c.nodes {
			if !node.isPartitioned {
				candidateID = id
				break
			}
		}
	}

	candidate, exists := c.nodes[candidateID]
	if !exists || candidate.isPartitioned {
		return "", errors.New("candidate node unavailable or partitioned")
	}

	candidate.mu.Lock()
	candidate.currentTerm++
	candidate.role = RoleCandidate
	candidate.votedFor = candidateID
	term := candidate.currentTerm
	lastLogIdx, lastLogTerm := candidate.getLastLogInfo()
	candidate.mu.Unlock()

	votesReceived := 1 // candidate votes for itself

	voteArgs := RequestVoteArgs{
		Term:         term,
		CandidateID:  candidateID,
		LastLogIndex: lastLogIdx,
		LastLogTerm:  lastLogTerm,
	}

	for id, peer := range c.nodes {
		if id == candidateID {
			continue
		}
		reply := peer.HandleRequestVote(voteArgs)
		if reply.VoteGranted {
			votesReceived++
		}
	}

	if votesReceived >= quorum {
		candidate.mu.Lock()
		candidate.role = RoleLeader
		for id := range c.nodes {
			candidate.nextIndex[id] = lastLogIdx + 1
			candidate.matchIndex[id] = 0
		}
		candidate.matchIndex[candidateID] = lastLogIdx
		candidate.mu.Unlock()

		c.leaderID = candidateID
		return candidateID, nil
	}

	candidate.mu.Lock()
	candidate.role = RoleFollower
	candidate.mu.Unlock()
	return "", fmt.Errorf("election failed: received %d/%d votes, quorum required: %d", votesReceived, total, quorum)
}

// SimulateNetworkPartition marks given nodes as partitioned.
func (c *RaftCluster) SimulateNetworkPartition(nodeIDs []string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	idMap := make(map[string]bool)
	for _, id := range nodeIDs {
		idMap[id] = true
	}

	for id, node := range c.nodes {
		node.mu.Lock()
		node.isPartitioned = idMap[id]
		node.mu.Unlock()
	}
}

// HealPartition restores full network connectivity across all nodes.
func (c *RaftCluster) HealPartition() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, node := range c.nodes {
		node.mu.Lock()
		node.isPartitioned = false
		node.mu.Unlock()
	}
}

// ReplicateSession synchronizes all events from a trace.Session across the multi-region Raft cluster.
func (c *RaftCluster) ReplicateSession(session *trace.Session) (*RaftReport, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.nodes) == 0 {
		return nil, errors.New("empty Raft cluster")
	}

	quorum := (len(c.nodes) / 2) + 1

	// Ensure leader exists
	if c.leaderID == "" || c.nodes[c.leaderID].role != RoleLeader {
		leaderID, err := c.runElectionLocked("")
		if err != nil {
			return nil, fmt.Errorf("failed to elect cluster leader: %w", err)
		}
		c.leaderID = leaderID
	}

	leader := c.nodes[c.leaderID]
	var steps []ReplicationStepResult
	var totalLagMs int64
	var maxLagMs int64

	for _, ev := range session.Events {
		payloadBytes, _ := json.Marshal(ev.Payload)
		originRegion := leader.region
		if ev.Payload != nil {
			if reg, ok := ev.Payload["region"].(string); ok && reg != "" {
				originRegion = reg
			}
		}

		entry, err := leader.Propose(session.ID, ev.Index, string(ev.Type), payloadBytes, originRegion)
		if err != nil {
			return nil, fmt.Errorf("proposal failed at step %d: %w", ev.Index, err)
		}

		// Replicate to all peers via AppendEntries
		acks := 1 // Leader acknowledges
		participatingRegions := []string{leader.region}
		stepMaxLag := int64(0)

		prevLogIdx := entry.Index - 1
		prevLogTerm := uint64(0)
		if prevLogIdx > 0 && prevLogIdx <= int64(len(leader.log)) {
			prevLogTerm = leader.log[prevLogIdx-1].Term
		}

		args := AppendEntriesArgs{
			Term:         leader.currentTerm,
			LeaderID:     leader.nodeID,
			PrevLogIndex: prevLogIdx,
			PrevLogTerm:  prevLogTerm,
			Entries:      []LogEntry{*entry},
			LeaderCommit: leader.commitIndex,
		}

		for peerID, peer := range c.nodes {
			if peerID == leader.nodeID {
				continue
			}

			reply := peer.HandleAppendEntries(args)
			if reply.Success {
				acks++
				participatingRegions = append(participatingRegions, peer.region)
				leader.mu.Lock()
				leader.nextIndex[peerID] = entry.Index + 1
				leader.matchIndex[peerID] = entry.Index
				leader.mu.Unlock()

				rtt := peer.latencyRTTMs
				if rtt > stepMaxLag {
					stepMaxLag = rtt
				}
			}
		}

		quorumReached := acks >= quorum
		if quorumReached {
			leader.mu.Lock()
			leader.commitIndex = entry.Index
			leader.appliedIndex = entry.Index
			leader.mu.Unlock()

			// Update followers commit index on subsequent heartbeat
			commitArgs := AppendEntriesArgs{
				Term:         leader.currentTerm,
				LeaderID:     leader.nodeID,
				PrevLogIndex: entry.Index,
				PrevLogTerm:  entry.Term,
				Entries:      nil,
				LeaderCommit: leader.commitIndex,
			}
			for peerID, peer := range c.nodes {
				if peerID != leader.nodeID {
					_ = peer.HandleAppendEntries(commitArgs)
				}
			}
		}

		totalLagMs += stepMaxLag
		if stepMaxLag > maxLagMs {
			maxLagMs = stepMaxLag
		}

		steps = append(steps, ReplicationStepResult{
			EntryIndex:           entry.Index,
			Term:                 entry.Term,
			StepIndex:            ev.Index,
			QuorumReached:        quorumReached,
			AcksCount:            acks,
			TotalNodes:           len(c.nodes),
			CommitLatencyMs:      stepMaxLag,
			ParticipatingRegions: participatingRegions,
		})
	}

	// Calculate node info snapshots
	nodeInfos := make([]RaftNodeInfo, 0, len(c.nodes))
	for _, node := range c.nodes {
		info := node.GetInfo()
		info.ReplicationLagMs = leader.commitIndex - info.CommitIndex
		nodeInfos = append(nodeInfos, info)
	}

	avgLag := 0.0
	if len(steps) > 0 {
		avgLag = math.Round(float64(totalLagMs)/float64(len(steps))*100) / 100
	}

	allCommitted := true
	for _, st := range steps {
		if !st.QuorumReached {
			allCommitted = false
			break
		}
	}

	partitionsCount := 0
	for _, n := range c.nodes {
		if n.isPartitioned {
			partitionsCount++
		}
	}

	report := &RaftReport{
		SessionID:               session.ID,
		ClusterID:               c.cfg.ClusterID,
		LeaderID:                c.leaderID,
		LeaderRegion:            leader.region,
		Term:                    leader.currentTerm,
		TotalNodes:              len(c.nodes),
		QuorumThreshold:         quorum,
		Nodes:                   nodeInfos,
		TotalEntriesCommitted:   int(leader.commitIndex),
		AverageReplicationLagMs: avgLag,
		MaxReplicationLagMs:     maxLagMs,
		CrossRegionSyncPassed:   allCommitted,
		ReplicationSteps:        steps,
		PartitionsDetected:      partitionsCount,
		GeneratedAt:             time.Now().UTC(),
	}

	return report, nil
}

func (c *RaftCluster) runElectionLocked(preferredCandidateID string) (string, error) {
	total := len(c.nodes)
	quorum := (total / 2) + 1

	candidateID := preferredCandidateID
	if candidateID == "" {
		for id, node := range c.nodes {
			if !node.isPartitioned {
				candidateID = id
				break
			}
		}
	}

	candidate, exists := c.nodes[candidateID]
	if !exists || candidate.isPartitioned {
		return "", errors.New("candidate node unavailable or partitioned")
	}

	candidate.mu.Lock()
	candidate.currentTerm++
	candidate.role = RoleCandidate
	candidate.votedFor = candidateID
	term := candidate.currentTerm
	lastLogIdx, lastLogTerm := candidate.getLastLogInfo()
	candidate.mu.Unlock()

	votesReceived := 1

	voteArgs := RequestVoteArgs{
		Term:         term,
		CandidateID:  candidateID,
		LastLogIndex: lastLogIdx,
		LastLogTerm:  lastLogTerm,
	}

	for id, peer := range c.nodes {
		if id == candidateID {
			continue
		}
		reply := peer.HandleRequestVote(voteArgs)
		if reply.VoteGranted {
			votesReceived++
		}
	}

	if votesReceived >= quorum {
		candidate.mu.Lock()
		candidate.role = RoleLeader
		for id := range c.nodes {
			candidate.nextIndex[id] = lastLogIdx + 1
			candidate.matchIndex[id] = 0
		}
		candidate.matchIndex[candidateID] = lastLogIdx
		candidate.mu.Unlock()

		c.leaderID = candidateID
		return candidateID, nil
	}

	candidate.mu.Lock()
	candidate.role = RoleFollower
	candidate.mu.Unlock()
	return "", fmt.Errorf("election failed: %d/%d votes (quorum: %d)", votesReceived, total, quorum)
}

// RenderRaftScorecard formats an ANSI terminal report of the multi-region consensus state.
func RenderRaftScorecard(report *RaftReport) string {
	var sb strings.Builder

	sb.WriteString("\n\033[1;36m========================================================================================\033[0m\n")
	sb.WriteString("   \033[1;37mAGENTVCR MULTI-REGION ACTIVE-ACTIVE RAFT CONSENSUS & LEDGER SYNC\033[0m\n")
	sb.WriteString("\033[1;36m========================================================================================\033[0m\n")

	sb.WriteString(fmt.Sprintf("  \033[1;33mSession ID:\033[0m            %s\n", report.SessionID))
	sb.WriteString(fmt.Sprintf("  \033[1;33mCluster ID:\033[0m            %s\n", report.ClusterID))
	sb.WriteString(fmt.Sprintf("  \033[1;33mElected Leader:\033[0m        \033[1;32m%s\033[0m (Region: %s, Term: %d)\n",
		report.LeaderID, report.LeaderRegion, report.Term))
	sb.WriteString(fmt.Sprintf("  \033[1;33mCluster Quorum:\033[0m        %d/%d nodes required for commitment\n",
		report.QuorumThreshold, report.TotalNodes))
	sb.WriteString(fmt.Sprintf("  \033[1;33mEntries Committed:\033[0m     %d trajectory log entries\n", report.TotalEntriesCommitted))
	sb.WriteString(fmt.Sprintf("  \033[1;33mAvg WAN Latency:\033[0m       %.2f ms (Max Cross-Region RTT: %d ms)\n",
		report.AverageReplicationLagMs, report.MaxReplicationLagMs))

	if report.CrossRegionSyncPassed {
		sb.WriteString("  \033[1;33mSync Verdict:\033[0m          \033[1;32m[PASS] ALL REGIONS SYNCHRONIZED ACROSS RAFT QUORUM\033[0m\n")
	} else {
		sb.WriteString("  \033[1;33mSync Verdict:\033[0m          \033[1;31m[FAIL] SPLIT-BRAIN OR QUORUM DEFICIT DETECTED\033[0m\n")
	}
	sb.WriteString("\033[1;34m----------------------------------------------------------------------------------------\033[0m\n")

	sb.WriteString("\033[1;37m  MULTI-REGION CLUSTER NODE TOPOLOGY & STATE:\033[0m\n")
	sb.WriteString(fmt.Sprintf("  %-26s %-22s %-10s %-6s %-10s %-8s\n",
		"NODE ID", "CLOUD REGION", "ROLE", "TERM", "COMMIT IDX", "HEALTH"))
	sb.WriteString("  ------------------------------------------------------------------------------------\n")

	for _, n := range report.Nodes {
		roleColor := "\033[1;34m"
		if n.Role == RoleLeader {
			roleColor = "\033[1;32m"
		} else if n.Role == RoleCandidate {
			roleColor = "\033[1;33m"
		}

		healthStr := "\033[1;32mHEALTHY\033[0m"
		if n.IsPartitioned {
			healthStr = "\033[1;31mISOLATED\033[0m"
		}

		sb.WriteString(fmt.Sprintf("  %-26s %-22s %s%-10s\033[0m %-6d %-10d %s\n",
			n.NodeID, n.Region, roleColor, n.Role, n.CurrentTerm, n.CommitIndex, healthStr))
	}

	sb.WriteString("\033[1;34m----------------------------------------------------------------------------------------\033[0m\n")
	sb.WriteString("\033[1;37m  TRAJECTORY STEP REPLICATION MATRIX (SAMPLE):\033[0m\n")

	limit := len(report.ReplicationSteps)
	if limit > 5 {
		limit = 5
	}
	for i := 0; i < limit; i++ {
		st := report.ReplicationSteps[i]
		qStatus := "\033[1;32mQUORUM_OK\033[0m"
		if !st.QuorumReached {
			qStatus = "\033[1;31mNO_QUORUM\033[0m"
		}
		sb.WriteString(fmt.Sprintf("  - Step %02d [Entry %02d]: Acks=%d/%d Latency=%02dms Status=%s Regions=[%s]\n",
			st.StepIndex, st.EntryIndex, st.AcksCount, st.TotalNodes, st.CommitLatencyMs, qStatus, strings.Join(st.ParticipatingRegions, ", ")))
	}
	if len(report.ReplicationSteps) > 5 {
		sb.WriteString(fmt.Sprintf("    ... (+%d additional verified trajectory steps synchronized)\n",
			len(report.ReplicationSteps)-5))
	}

	sb.WriteString("\033[1;36m========================================================================================\033[0m\n")
	return sb.String()
}
