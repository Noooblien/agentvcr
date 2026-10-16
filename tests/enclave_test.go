package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/enclave"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func TestEnclave_GenerateAndVerifyQuote_Success(t *testing.T) {
	session := &trace.Session{
		ID:        "session_enclave_valid_01",
		AgentName: "confidential-agent",
		CreatedAt: time.Now().UTC(),
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventLLMRequest, Payload: map[string]interface{}{"prompt": "secret data query"}},
			{Index: 2, Type: trace.EventToolCall, Payload: map[string]interface{}{"tool": "decrypt_vault"}},
		},
	}

	rootKey, err := enclave.GenerateHardwareRootKey()
	if err != nil {
		t.Fatalf("failed to generate hardware root key: %v", err)
	}

	nonce := "noncesecure1234567890abcdef"
	quote, err := enclave.GenerateAttestationQuote(session, nonce, rootKey)
	if err != nil {
		t.Fatalf("failed to generate quote: %v", err)
	}

	if quote.ReportBody.Technology != enclave.TechAMDSEVSNP {
		t.Errorf("expected AMD SEV-SNP technology, got: %s", quote.ReportBody.Technology)
	}
	if quote.ReportBody.IsDebuggable {
		t.Errorf("production enclave should not be debuggable")
	}

	// Verify quote
	ver := enclave.VerifyAttestationQuote(session, quote, rootKey, nonce)
	if !ver.Valid {
		t.Fatalf("expected valid verification, got failure: %s", ver.FailureReason)
	}
	if !ver.HardwareSignatureOK || !ver.ReportDataBindingOK || !ver.NonDebugCompliant {
		t.Errorf("expected all invariant flags to pass, got: %+v", ver)
	}
}

func TestEnclave_TamperedTrajectoryDetected(t *testing.T) {
	session := &trace.Session{
		ID:        "session_enclave_tamper_test",
		AgentName: "tamper-agent",
		CreatedAt: time.Now().UTC(),
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventLLMRequest, Payload: map[string]interface{}{"action": "view_balance"}},
		},
	}

	rootKey, _ := enclave.GenerateHardwareRootKey()
	nonce := "nonce_test_998877"
	quote, _ := enclave.GenerateAttestationQuote(session, nonce, rootKey)

	// Tamper with session events after quote was signed
	tamperedSession := &trace.Session{
		ID:        session.ID,
		AgentName: session.AgentName,
		CreatedAt: session.CreatedAt,
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventLLMRequest, Payload: map[string]interface{}{"action": "transfer_funds_fraud"}},
		},
	}

	ver := enclave.VerifyAttestationQuote(tamperedSession, quote, rootKey, nonce)
	if ver.Valid {
		t.Errorf("expected verification to fail for tampered trajectory")
	}
	if ver.ReportDataBindingOK {
		t.Errorf("expected report data binding to fail")
	}
}

func TestEnclave_DebuggableEnclaveRejected(t *testing.T) {
	session := &trace.Session{
		ID:        "session_debug_rejection",
		AgentName: "test-agent",
		Events:    []trace.TraceEvent{{Index: 1, Type: trace.EventStateChange}},
	}

	rootKey, _ := enclave.GenerateHardwareRootKey()
	nonce := "nonce_debug_1"
	quote, _ := enclave.GenerateAttestationQuote(session, nonce, rootKey)

	// Simulate debug flag turned on (insecure development environment)
	quote.ReportBody.IsDebuggable = true

	ver := enclave.VerifyAttestationQuote(session, quote, rootKey, nonce)
	if ver.Valid {
		t.Errorf("expected debuggable enclave to be rejected")
	}
	if ver.NonDebugCompliant {
		t.Errorf("expected NonDebugCompliant=false")
	}
}

func TestEnclave_ScorecardRendering(t *testing.T) {
	session := &trace.Session{
		ID:        "session_enclave_card",
		AgentName: "card-agent",
		Events:    []trace.TraceEvent{{Index: 1, Type: trace.EventLLMRequest}},
	}

	rootKey, _ := enclave.GenerateHardwareRootKey()
	nonce := "nonce_card_99"
	quote, _ := enclave.GenerateAttestationQuote(session, nonce, rootKey)
	ver := enclave.VerifyAttestationQuote(session, quote, rootKey, nonce)

	report := &enclave.EnclaveScorecardReport{
		SessionID:          session.ID,
		Quote:              *quote,
		Verification:       ver,
		EventCountAttested: 1,
		EnclavePassed:      ver.Valid,
		HardwareTrustTier:  "Silicon-Bound FIPS 140-3 Hardware Enclave",
		GeneratedAt:        time.Now().UTC(),
	}

	card := enclave.RenderEnclaveScorecard(report)
	if !strings.Contains(card, "AGENTVCR CONFIDENTIAL COMPUTING HARDWARE ENCLAVE") {
		t.Errorf("expected header in scorecard")
	}
	if !strings.Contains(card, "AMD SEV-SNP") {
		t.Errorf("expected AMD SEV-SNP in scorecard")
	}
	if !strings.Contains(card, "session_enclave_card") {
		t.Errorf("expected session ID in scorecard")
	}
}
