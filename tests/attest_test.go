package tests

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/attest"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func createAttestSession() *trace.Session {
	now := time.Now()
	s := &trace.Session{
		ID:        "session-attest-1",
		AgentName: "compliance-bot",
		Status:    "completed",
		CreatedAt: now,
		UpdatedAt: now.Add(2 * time.Second),
		Events: []trace.TraceEvent{
			{
				Index:     1,
				Timestamp: now,
				Type:      trace.EventToolCall,
				Payload:   map[string]interface{}{"action": "transfer_funds", "amount": 100},
			},
			{
				Index:     2,
				Timestamp: now.Add(1 * time.Second),
				Type:      trace.EventToolResult,
				Payload:   map[string]interface{}{"status": "approved", "tx_id": "0xabc"},
			},
		},
	}

	// Populate checksums
	prev := ""
	for i := range s.Events {
		s.Events[i].Checksum = trace.ComputeChecksum(prev, s.Events[i].Index, s.Events[i].Type, []byte(`{"action":"transfer_funds"}`))
		prev = s.Events[i].Checksum
	}
	return s
}

func TestAttest_SignAndVerifySuccess(t *testing.T) {
	session := createAttestSession()
	keypair, err := attest.GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate keypair: %v", err)
	}

	cert, err := attest.SignTrajectory(session, keypair.PrivateKey)
	if err != nil {
		t.Fatalf("failed to sign trajectory: %v", err)
	}

	if cert.SessionID != session.ID {
		t.Errorf("cert session ID mismatch: %s vs %s", cert.SessionID, session.ID)
	}
	if cert.Signature == "" || cert.PublicKey == "" {
		t.Errorf("cert signature or pubkey is empty")
	}

	result := attest.VerifyTrajectory(session, cert)
	if !result.Valid || !result.SignatureOK || !result.HashChainOK {
		t.Fatalf("expected valid verification, got: %+v", result)
	}
}

func TestAttest_TamperedPayloadDetected(t *testing.T) {
	session := createAttestSession()
	keypair, err := attest.GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate keypair: %v", err)
	}

	cert, err := attest.SignTrajectory(session, keypair.PrivateKey)
	if err != nil {
		t.Fatalf("failed to sign trajectory: %v", err)
	}

	// Maliciously tamper with event payload (e.g. modify amount to 1000000)
	session.Events[0].Payload["amount"] = 1000000

	result := attest.VerifyTrajectory(session, cert)
	if result.Valid {
		t.Fatalf("expected verification to FAIL on tampered session")
	}
	if !result.SignatureOK {
		t.Errorf("signature should still be mathematically valid against the cert")
	}
	if result.HashChainOK {
		t.Errorf("hash chain check should fail due to tampered payload")
	}
}

func TestAttest_ForgedSignatureDetected(t *testing.T) {
	session := createAttestSession()
	keypair1, _ := attest.GenerateKeyPair()
	keypair2, _ := attest.GenerateKeyPair()

	cert, err := attest.SignTrajectory(session, keypair1.PrivateKey)
	if err != nil {
		t.Fatalf("failed to sign: %v", err)
	}

	// Substitute public key with another key without re-signing
	cert.PublicKey = hex.EncodeToString(keypair2.PublicKey)

	result := attest.VerifyTrajectory(session, cert)
	if result.Valid {
		t.Fatalf("expected verification to fail for forged key")
	}
	if result.SignatureOK {
		t.Errorf("expected signature check to fail")
	}
}
