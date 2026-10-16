package tests

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/gossip"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func TestGossip_QuorumCommitmentSuccess(t *testing.T) {
	cfg := gossip.DefaultGossipConfig("validator-01")
	node := gossip.NewMeshNode(cfg)

	node.AddPeer("validator-02", "10.0.0.2:9090")
	node.AddPeer("validator-03", "10.0.0.3:9090")

	data := []byte("checkpoint-state-step-5")
	msg := node.BroadcastProposal(1, "session-gossip-1", 5, data)

	// Peer 2 votes for proposal
	vote2 := &gossip.GossipMessage{
		MessageID:   "vote-peer-02-1",
		OriginPeer:  "validator-02",
		Round:       1,
		PayloadHash: msg.PayloadHash,
		Data:        data,
		Signature:   "sig:val-02",
	}
	_, err := node.ReceiveGossip(vote2)
	if err != nil {
		t.Fatalf("ReceiveGossip failed: %v", err)
	}

	// 2 of 3 votes (self + peer2) = 66.7% >= 67% quorum
	entry, committed := node.TryCommitQuorum("session-gossip-1", 5, 1, msg.PayloadHash)
	if !committed {
		t.Fatalf("Expected ledger entry to be committed with supermajority")
	}

	if entry.Index != 1 {
		t.Errorf("Expected ledger index 1, got %d", entry.Index)
	}
	if entry.ValidatorCount != 2 {
		t.Errorf("Expected 2 validator signatures, got %d", entry.ValidatorCount)
	}

	report := node.GenerateGossipReport()
	if report.LedgerEntriesCount != 1 {
		t.Errorf("Expected 1 committed ledger entry, got %d", report.LedgerEntriesCount)
	}
	if !report.QuorumIntegrityOK {
		t.Errorf("Expected QuorumIntegrityOK to be true")
	}

	card := gossip.RenderGossipScorecard(report)
	if !strings.Contains(card, "BYZANTINE FAULT TOLERANCE MAINTAINED") {
		t.Errorf("Expected PASS status in scorecard, got:\n%s", card)
	}
}

func TestGossip_ByzantineTamperingAndEquivocation(t *testing.T) {
	cfg := gossip.DefaultGossipConfig("validator-root")
	node := gossip.NewMeshNode(cfg)

	node.AddPeer("rogue-agent", "10.0.0.66:9090")

	// 1. Test payload tampering
	data := []byte("valid-ledger-entry")
	h := sha256.Sum256(data)
	tamperedMsg := &gossip.GossipMessage{
		MessageID:   "msg-rogue-01",
		OriginPeer:  "rogue-agent",
		Round:       1,
		PayloadHash: hex.EncodeToString(h[:]),
		Data:        []byte("tampered-corrupted-data"),
	}

	_, err := node.ReceiveGossip(tamperedMsg)
	if err == nil {
		t.Errorf("Expected error on tampered payload hash")
	}

	// 2. Test equivocation (voting for conflicting hashes in same round)
	legitHash1 := "1111111111111111111111111111111111111111111111111111111111111111"
	legitHash2 := "2222222222222222222222222222222222222222222222222222222222222222"

	v1 := &gossip.GossipMessage{
		MessageID:   "vote-equiv-1",
		OriginPeer:  "rogue-agent",
		Round:       2,
		PayloadHash: legitHash1,
		Data:        []byte("data-1"),
	}
	h1 := sha256.Sum256([]byte("data-1"))
	v1.PayloadHash = hex.EncodeToString(h1[:])

	v2 := &gossip.GossipMessage{
		MessageID:   "vote-equiv-2",
		OriginPeer:  "rogue-agent",
		Round:       2,
		PayloadHash: legitHash2,
		Data:        []byte("data-2"),
	}
	h2 := sha256.Sum256([]byte("data-2"))
	v2.PayloadHash = hex.EncodeToString(h2[:])

	_, _ = node.ReceiveGossip(v1)
	_, _ = node.ReceiveGossip(v2)

	report := node.GenerateGossipReport()
	if report.ByzantinePeers == 0 {
		t.Errorf("Expected rogue agent to be marked Byzantine")
	}
	if len(report.Violations) == 0 {
		t.Errorf("Expected recorded violations for Byzantine equivocation")
	}

	card := gossip.RenderGossipScorecard(report)
	if !strings.Contains(card, "[FAIL]") {
		t.Errorf("Expected FAIL status in scorecard, got:\n%s", card)
	}
}

func TestGossip_SimulateSessionGossip(t *testing.T) {
	now := time.Now()
	session := &trace.Session{
		ID:        "session-p2p-test",
		AgentName: "orchestration-agent",
		Events: []trace.TraceEvent{
			{Index: 1, Timestamp: now, Type: trace.EventStateChange, Payload: map[string]interface{}{"key": "leader", "value": "node-1"}},
			{Index: 2, Timestamp: now.Add(50 * time.Millisecond), Type: trace.EventToolResult, Payload: map[string]interface{}{"status": "success"}},
		},
	}

	report := gossip.SimulateSessionGossip(session, 4)
	if report.LedgerEntriesCount != 2 {
		t.Errorf("Expected 2 committed ledger entries, got %d", report.LedgerEntriesCount)
	}
	if !report.QuorumIntegrityOK {
		t.Errorf("Expected QuorumIntegrityOK to be true in simulation")
	}
}
