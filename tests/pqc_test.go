package tests

import (
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/pqc"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func TestPQC_KeyGenerationAndKyberKEM(t *testing.T) {
	keyPair, err := pqc.GenerateDilithiumKeyPair(pqc.AlgorithmDilithium3)
	if err != nil {
		t.Fatalf("failed to generate Dilithium keypair: %v", err)
	}

	if len(keyPair.PublicKey) == 0 || len(keyPair.PrivateKey) == 0 {
		t.Errorf("empty public or private key in generated Dilithium keypair")
	}

	// Test ML-KEM key encapsulation
	kem, err := pqc.EncapsulateKyberKEM(keyPair.PublicKey, pqc.AlgorithmKyber768)
	if err != nil {
		t.Fatalf("failed to encapsulate Kyber KEM: %v", err)
	}

	if len(kem.Ciphertext) != 64 { // 32 bytes hex-encoded = 64
		t.Errorf("expected 64-char hex ciphertext, got: %s", kem.Ciphertext)
	}
	if len(kem.SharedSecret) != 64 {
		t.Errorf("expected 64-char hex shared secret, got: %s", kem.SharedSecret)
	}
}

func TestPQC_SignAndVerify_Success(t *testing.T) {
	keyPair, err := pqc.GenerateDilithiumKeyPair(pqc.AlgorithmDilithium3)
	if err != nil {
		t.Fatalf("failed to generate keypair: %v", err)
	}

	session := &trace.Session{
		ID:        "session_pqc_valid",
		AgentName: "quantum-secure-agent",
		CreatedAt: time.Now(),
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventLLMRequest, Payload: map[string]interface{}{"query": "deploy contract"}},
			{Index: 2, Type: trace.EventToolCall, Payload: map[string]interface{}{"tool": "web3_send"}},
		},
	}

	cert, err := pqc.SignTrajectoryPQC(session, keyPair, nil)
	if err != nil {
		t.Fatalf("failed to sign trajectory with PQC: %v", err)
	}

	if cert.Algorithm != pqc.AlgorithmDilithium3 {
		t.Errorf("expected algorithm %s, got %s", pqc.AlgorithmDilithium3, cert.Algorithm)
	}
	if cert.EventCount != 2 {
		t.Errorf("expected event count 2, got %d", cert.EventCount)
	}

	res := pqc.VerifyTrajectoryPQC(session, cert, nil)
	if !res.Valid {
		t.Errorf("expected valid verification, got: %+v", res)
	}
	if !res.PostQuantumSignatureOK || !res.HashChainOK {
		t.Errorf("signature and hash chain must be OK: %+v", res)
	}
}

func TestPQC_HybridMode_SuccessAndFailure(t *testing.T) {
	dilithiumKey, _ := pqc.GenerateDilithiumKeyPair(pqc.AlgorithmDilithium3)
	edPub, edPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate ed25519 key: %v", err)
	}

	session := &trace.Session{
		ID:        "session_hybrid_test",
		AgentName: "hybrid-agent",
		CreatedAt: time.Now(),
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventLLMRequest, Payload: map[string]interface{}{"task": "authenticate"}},
		},
	}

	cert, err := pqc.SignTrajectoryPQC(session, dilithiumKey, edPriv)
	if err != nil {
		t.Fatalf("failed to sign in hybrid mode: %v", err)
	}

	if cert.Algorithm != pqc.AlgorithmHybridEd25519Dilithium {
		t.Errorf("expected hybrid algorithm, got: %s", cert.Algorithm)
	}
	if cert.ClassicalSignature == "" {
		t.Errorf("classical signature must not be empty in hybrid mode")
	}

	// Verify valid hybrid
	res := pqc.VerifyTrajectoryPQC(session, cert, edPub)
	if !res.Valid || !res.ClassicalSignatureOK {
		t.Errorf("expected valid hybrid verification: %+v", res)
	}

	// Tampering detection: modify payload in session
	session.Events[0].Payload["task"] = "ATTACK_MUTATION"
	tamperedRes := pqc.VerifyTrajectoryPQC(session, cert, edPub)
	if tamperedRes.Valid {
		t.Errorf("tampered session should fail PQC verification")
	}
	if tamperedRes.QuantumSecurityStatus != "REJECTED_MERKLE_ROOT_TAMPERING" {
		t.Errorf("unexpected status: %s", tamperedRes.QuantumSecurityStatus)
	}
}

func TestPQC_ScorecardRendering(t *testing.T) {
	keyPair, _ := pqc.GenerateDilithiumKeyPair(pqc.AlgorithmDilithium3)
	session := &trace.Session{
		ID:        "session_scorecard_test",
		AgentName: "agent-scorecard",
		CreatedAt: time.Now(),
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventStateChange, Payload: map[string]interface{}{"status": "ready"}},
		},
	}

	cert, _ := pqc.SignTrajectoryPQC(session, keyPair, nil)
	ver := pqc.VerifyTrajectoryPQC(session, cert, nil)
	kem, _ := pqc.EncapsulateKyberKEM(keyPair.PublicKey, pqc.AlgorithmKyber768)

	report := &pqc.PQCReport{
		SessionID:    session.ID,
		Certificate:  *cert,
		Verification: ver,
		KEMResult:    kem,
		Passed:       ver.Valid,
	}

	card := pqc.RenderPQCScorecard(report)
	if !strings.Contains(card, "POST-QUANTUM CRYPTOGRAPHIC TRAJECTORY ATTESTATION") {
		t.Errorf("missing scorecard header: %s", card)
	}
	if !strings.Contains(card, "[PASS]") {
		t.Errorf("expected PASS status in scorecard: %s", card)
	}
	if !strings.Contains(card, "ML-KEM") {
		t.Errorf("expected ML-KEM section in scorecard: %s", card)
	}
}
