package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/raft"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func TestRaft_LeaderElectionAndQuorum(t *testing.T) {
	cfg := raft.DefaultRegionalTopology()
	cluster := raft.NewRaftCluster(cfg)

	// Elect leader
	leaderID, err := cluster.RunElection("agentvcr-us-east-1")
	if err != nil {
		t.Fatalf("expected election to succeed, got error: %v", err)
	}

	if leaderID != "agentvcr-us-east-1" {
		t.Errorf("expected leader to be agentvcr-us-east-1, got: %s", leaderID)
	}

	// Verify candidate request vote handling
	node := raft.NewRaftNode(cfg.Nodes[0], cfg.Nodes)
	reply := node.HandleRequestVote(raft.RequestVoteArgs{
		Term:         2,
		CandidateID:  "agentvcr-eu-west-1",
		LastLogIndex: 0,
		LastLogTerm:  0,
	})

	if !reply.VoteGranted {
		t.Errorf("expected vote to be granted for higher term, got: %+v", reply)
	}
	if reply.Term != 2 {
		t.Errorf("expected term 2, got %d", reply.Term)
	}

	// Stale term rejection
	staleReply := node.HandleRequestVote(raft.RequestVoteArgs{
		Term:         1,
		CandidateID:  "stale-node",
		LastLogIndex: 0,
		LastLogTerm:  0,
	})
	if staleReply.VoteGranted {
		t.Errorf("expected vote to be rejected for stale term")
	}
}

func TestRaft_LogReplicationAndQuorumCommit(t *testing.T) {
	cfg := raft.DefaultRegionalTopology()
	cluster := raft.NewRaftCluster(cfg)

	session := &trace.Session{
		ID:        "session_raft_sync_001",
		AgentName: "cross-region-agent",
		CreatedAt: time.Now().UTC(),
		Events: []trace.TraceEvent{
			{
				Index: 1,
				Type:  trace.EventLLMRequest,
				Payload: map[string]interface{}{
					"prompt": "Synthesize market analysis",
					"region": "us-east-1",
				},
			},
			{
				Index: 2,
				Type:  trace.EventToolCall,
				Payload: map[string]interface{}{
					"tool":   "fetch_quotes",
					"symbol": "NVDA",
				},
			},
			{
				Index: 3,
				Type:  trace.EventToolResult,
				Payload: map[string]interface{}{
					"result": "price=135.50",
				},
			},
		},
	}

	report, err := cluster.ReplicateSession(session)
	if err != nil {
		t.Fatalf("expected successful replication, got: %v", err)
	}

	if !report.CrossRegionSyncPassed {
		t.Errorf("expected CrossRegionSyncPassed=true, got false")
	}
	if report.TotalEntriesCommitted != 3 {
		t.Errorf("expected 3 entries committed, got: %d", report.TotalEntriesCommitted)
	}
	if report.QuorumThreshold != 2 { // 3 nodes -> quorum is 2
		t.Errorf("expected quorum threshold 2, got: %d", report.QuorumThreshold)
	}
	if len(report.ReplicationSteps) != 3 {
		t.Errorf("expected 3 replication steps, got: %d", len(report.ReplicationSteps))
	}
	if report.AverageReplicationLagMs <= 0 {
		t.Errorf("expected positive average replication lag, got: %f", report.AverageReplicationLagMs)
	}
}

func TestRaft_NetworkPartitionAndSplitBrainProtection(t *testing.T) {
	cfg := raft.DefaultRegionalTopology()
	cluster := raft.NewRaftCluster(cfg)

	// Isolate 2 out of 3 nodes (majority partitioned away)
	cluster.SimulateNetworkPartition([]string{"agentvcr-eu-west-1", "agentvcr-ap-northeast-1"})

	// Attempting election with only 1 isolated node should fail quorum
	_, err := cluster.RunElection("agentvcr-us-east-1")
	if err == nil {
		t.Errorf("expected election to fail due to lack of quorum")
	}

	// Heal partition
	cluster.HealPartition()

	// Election should now succeed
	leaderID, err := cluster.RunElection("agentvcr-us-east-1")
	if err != nil {
		t.Fatalf("expected election to succeed after healing, got: %v", err)
	}
	if leaderID != "agentvcr-us-east-1" {
		t.Errorf("expected leader agentvcr-us-east-1, got: %s", leaderID)
	}
}

func TestRaft_ScorecardRendering(t *testing.T) {
	cfg := raft.DefaultRegionalTopology()
	cluster := raft.NewRaftCluster(cfg)

	session := &trace.Session{
		ID:        "session_raft_scorecard_test",
		AgentName: "raft-scorecard-agent",
		CreatedAt: time.Now().UTC(),
		Events: []trace.TraceEvent{
			{
				Index: 1,
				Type:  trace.EventStateChange,
				Payload: map[string]interface{}{
					"status": "active",
				},
			},
		},
	}

	report, err := cluster.ReplicateSession(session)
	if err != nil {
		t.Fatalf("ReplicateSession failed: %v", err)
	}

	card := raft.RenderRaftScorecard(report)
	if !strings.Contains(card, "AGENTVCR MULTI-REGION ACTIVE-ACTIVE RAFT CONSENSUS") {
		t.Errorf("expected title in scorecard, got: %s", card)
	}
	if !strings.Contains(card, "session_raft_scorecard_test") {
		t.Errorf("expected session ID in scorecard")
	}
	if !strings.Contains(card, "QUORUM_OK") {
		t.Errorf("expected QUORUM_OK in scorecard")
	}
}
