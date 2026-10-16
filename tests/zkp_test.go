package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/zkp"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func TestZKP_GenerateAndVerifyValidProof(t *testing.T) {
	now := time.Now()
	session := &trace.Session{
		ID:        "session-zkp-valid",
		AgentName: "compliant-finance-agent",
		Events: []trace.TraceEvent{
			{
				Index:      1,
				Timestamp:  now,
				Type:       trace.EventLLMRequest,
				Payload:    map[string]interface{}{"prompt": "Summarize quarterly balance sheet"},
			},
			{
				Index:      2,
				Timestamp:  now.Add(100 * time.Millisecond),
				Type:       trace.EventToolCall,
				Payload: map[string]interface{}{
					"tool_name": "fetch_balance_sheet",
					"args":      map[string]interface{}{"quarter": "Q3"},
				},
			},
			{
				Index:      3,
				Timestamp:  now.Add(200 * time.Millisecond),
				Type:       trace.EventToolResult,
				Payload: map[string]interface{}{
					"status": "success",
					"data":   map[string]interface{}{"revenue": 1000000},
				},
			},
		},
	}

	claim := zkp.ProofClaim{
		ClaimID:                "finance-compliance-v1",
		Description:            "Zero PII, zero tool errors, and strict tool whitelist",
		RequireZeroPII:         true,
		RequireZeroToolErrors:  true,
		PermittedToolWhitelist: []string{"fetch_balance_sheet"},
	}

	proof, err := zkp.GenerateProof(session, claim)
	if err != nil {
		t.Fatalf("GenerateProof failed: %v", err)
	}

	if proof.MerkleRoot == "" {
		t.Errorf("Expected non-empty Merkle root")
	}
	if !proof.ClaimSatisfied {
		t.Errorf("Expected claim to be satisfied")
	}
	if len(proof.Commitments) != 3 {
		t.Errorf("Expected 3 leaf commitments, got %d", len(proof.Commitments))
	}

	verifyResult, err := zkp.VerifyProof(proof)
	if err != nil {
		t.Fatalf("VerifyProof failed: %v", err)
	}

	if !verifyResult.Valid {
		t.Errorf("Expected proof verification to succeed, got: %s", verifyResult.Details)
	}
	if !verifyResult.ZeroKnowledgeValid {
		t.Errorf("Expected ZeroKnowledgeValid to be true")
	}

	scorecard := zkp.RenderZKProofScorecard(proof, verifyResult)
	if !strings.Contains(scorecard, "[PASS] CRYPTOGRAPHICALLY VALID") {
		t.Errorf("Expected PASS status in scorecard, got:\n%s", scorecard)
	}
}

func TestZKP_TamperedProofDetection(t *testing.T) {
	session := &trace.Session{
		ID:        "session-zkp-tamper",
		AgentName: "audited-agent",
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventToolCall, Payload: map[string]interface{}{"tool_name": "calculator"}},
		},
	}

	claim := zkp.ProofClaim{
		ClaimID:               "simple-claim",
		Description:           "Zero errors",
		RequireZeroToolErrors: true,
	}

	proof, err := zkp.GenerateProof(session, claim)
	if err != nil {
		t.Fatalf("GenerateProof failed: %v", err)
	}

	// Forge/tamper with challenge hash
	tamperedProof := *proof
	tamperedProof.ChallengeHash = "baddeadbeef1234567890abcdef"

	verifyResult, err := zkp.VerifyProof(&tamperedProof)
	if err != nil {
		t.Fatalf("VerifyProof failed: %v", err)
	}

	if verifyResult.Valid {
		t.Errorf("Expected verification to fail on tampered challenge hash")
	}
	if !strings.Contains(verifyResult.Details, "Challenge hash mismatch") {
		t.Errorf("Expected Challenge hash mismatch detail, got: %s", verifyResult.Details)
	}
}

func TestZKP_ClaimViolationRejection(t *testing.T) {
	now := time.Now()
	session := &trace.Session{
		ID:        "session-zkp-leak",
		AgentName: "leaky-agent",
		Events: []trace.TraceEvent{
			{
				Index:      1,
				Timestamp:  now,
				Type:       trace.EventLLMRequest,
				Payload: map[string]interface{}{
					"prompt": "Using API key sk-proj1234567890abcdef12345678 to query DB",
				},
			},
		},
	}

	claim := zkp.ProofClaim{
		ClaimID:        "privacy-check",
		Description:    "Zero PII / credentials",
		RequireZeroPII: true,
	}

	_, err := zkp.GenerateProof(session, claim)
	if err == nil {
		t.Fatalf("Expected GenerateProof to fail due to API key PII violation")
	}
	if !strings.Contains(err.Error(), "session fails claim verification") {
		t.Errorf("Expected claim verification error, got: %v", err)
	}
}
