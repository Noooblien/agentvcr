package attest

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// Certificate contains cryptographic proof of an agent trajectory's integrity.
type Certificate struct {
	SessionID  string    `json:"session_id"`
	AgentName  string    `json:"agent_name"`
	EventCount int       `json:"event_count"`
	RootHash   string    `json:"root_hash"`
	SignedAt   time.Time `json:"signed_at"`
	PublicKey  string    `json:"public_key"`
	Signature  string    `json:"signature"`
}

// VerificationResult details the outcome of an attestation verification.
type VerificationResult struct {
	Valid        bool   `json:"valid"`
	SignatureOK  bool   `json:"signature_ok"`
	HashChainOK  bool   `json:"hash_chain_ok"`
	TamperedStep int    `json:"tampered_step,omitempty"` // -1 if not tampered
	Details      string `json:"details"`
}

// KeyPair holds an Ed25519 signing keypair.
type KeyPair struct {
	PublicKey  ed25519.PublicKey
	PrivateKey ed25519.PrivateKey
}

// GenerateKeyPair generates a new cryptographic Ed25519 keypair for signing trajectories.
func GenerateKeyPair() (*KeyPair, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ed25519 keys: %w", err)
	}
	return &KeyPair{
		PublicKey:  pub,
		PrivateKey: priv,
	}, nil
}

// ComputeSessionRootHash builds a deterministic Merkle-style root hash of all chained events.
func ComputeSessionRootHash(session *trace.Session) string {
	h := sha256.New()
	h.Write([]byte(session.ID))
	h.Write([]byte(session.AgentName))

	prevChecksum := ""
	for _, ev := range session.Events {
		payloadBytes, _ := json.Marshal(ev.Payload)
		stepHash := trace.ComputeChecksum(prevChecksum, ev.Index, ev.Type, payloadBytes)
		h.Write([]byte(stepHash))
		prevChecksum = stepHash
	}

	return hex.EncodeToString(h.Sum(nil))
}

// SignTrajectory signs an agent trajectory session using the provided private key.
func SignTrajectory(session *trace.Session, privKey ed25519.PrivateKey) (*Certificate, error) {
	if session == nil || len(session.Events) == 0 {
		return nil, fmt.Errorf("cannot sign empty or nil session")
	}

	rootHash := ComputeSessionRootHash(session)
	signedAt := time.Now().UTC()

	digest := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d:%s:%s",
		session.ID, session.AgentName, len(session.Events), rootHash, signedAt.Format(time.RFC3339Nano))))

	sig := ed25519.Sign(privKey, digest[:])

	pubKey := privKey.Public().(ed25519.PublicKey)

	return &Certificate{
		SessionID:  session.ID,
		AgentName:  session.AgentName,
		EventCount: len(session.Events),
		RootHash:   rootHash,
		SignedAt:   signedAt,
		PublicKey:  hex.EncodeToString(pubKey),
		Signature:  hex.EncodeToString(sig),
	}, nil
}

// VerifyTrajectory verifies that the session events have not been tampered with and match the certificate.
func VerifyTrajectory(session *trace.Session, cert *Certificate) *VerificationResult {
	res := &VerificationResult{
		TamperedStep: -1,
	}

	if session == nil || cert == nil {
		res.Details = "session or certificate is nil"
		return res
	}

	if session.ID != cert.SessionID {
		res.Details = fmt.Sprintf("session ID mismatch: cert=%s, session=%s", cert.SessionID, session.ID)
		return res
	}

	if len(session.Events) != cert.EventCount {
		res.Details = fmt.Sprintf("event count mismatch: cert=%d, session=%d (events added or deleted)",
			cert.EventCount, len(session.Events))
		return res
	}

	// 1. Verify Ed25519 Signature
	pubKeyBytes, err := hex.DecodeString(cert.PublicKey)
	if err != nil || len(pubKeyBytes) != ed25519.PublicKeySize {
		res.Details = "invalid certificate public key"
		return res
	}

	sigBytes, err := hex.DecodeString(cert.Signature)
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		res.Details = "invalid certificate signature format"
		return res
	}

	digest := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d:%s:%s",
		cert.SessionID, cert.AgentName, cert.EventCount, cert.RootHash, cert.SignedAt.Format(time.RFC3339Nano))))

	if !ed25519.Verify(pubKeyBytes, digest[:], sigBytes) {
		res.Details = "cryptographic signature verification failed: signature is invalid or forged"
		return res
	}
	res.SignatureOK = true

	// 2. Verify Merkle / Event Chain Integrity
	currentRoot := ComputeSessionRootHash(session)
	if currentRoot != cert.RootHash {
		res.Details = "trajectory hash chain mismatch: payload or sequence was altered after signing"
		// Identify the tampered step
		res.TamperedStep = detectTamperedStep(session, cert)
		return res
	}

	res.HashChainOK = true
	res.Valid = true
	res.Details = "Trajectory integrity cryptographically verified. No tampering detected."
	return res
}

func detectTamperedStep(session *trace.Session, cert *Certificate) int {
	prevChecksum := ""
	for _, ev := range session.Events {
		payloadBytes, _ := json.Marshal(ev.Payload)
		expectedChecksum := trace.ComputeChecksum(prevChecksum, ev.Index, ev.Type, payloadBytes)
		if ev.Checksum != "" && ev.Checksum != expectedChecksum {
			return ev.Index
		}
		prevChecksum = expectedChecksum
	}
	return 1
}
