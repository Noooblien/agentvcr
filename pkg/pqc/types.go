package pqc

import (
	"time"
)

// PQCAlgorithm defines the post-quantum cryptographic scheme used for attestation or encapsulation.
type PQCAlgorithm string

const (
	// ML-DSA (CRYSTALS-Dilithium) - NIST FIPS 204 Digital Signature Standard
	AlgorithmDilithium3 PQCAlgorithm = "ML-DSA-65 (Dilithium3)"
	AlgorithmDilithium5 PQCAlgorithm = "ML-DSA-87 (Dilithium5)"

	// ML-KEM (CRYSTALS-Kyber) - NIST FIPS 203 Key Encapsulation Mechanism
	AlgorithmKyber768  PQCAlgorithm = "ML-KEM-768 (Kyber768)"
	AlgorithmKyber1024 PQCAlgorithm = "ML-KEM-1024 (Kyber1024)"

	// Hybrid Post-Quantum + Classical mode (Dilithium + Ed25519)
	AlgorithmHybridEd25519Dilithium PQCAlgorithm = "Hybrid-Ed25519-Dilithium3"
)

// DilithiumKeyPair contains post-quantum public and private keys for digital signatures.
type DilithiumKeyPair struct {
	Algorithm  PQCAlgorithm `json:"algorithm"`
	PublicKey  string       `json:"public_key"`  // Hex-encoded post-quantum lattice public key
	PrivateKey string       `json:"private_key"` // Hex-encoded private seed/key
	CreatedAt  time.Time    `json:"created_at"`
}

// KyberKEMResult holds encapsulated key material and shared secret.
type KyberKEMResult struct {
	Algorithm     PQCAlgorithm `json:"algorithm"`
	Ciphertext    string       `json:"ciphertext"`     // Hex-encoded KEM ciphertext
	SharedSecret  string       `json:"shared_secret"`  // Hex-encoded 256-bit symmetric key
	EncapsulatedAt time.Time   `json:"encapsulated_at"`
}

// PQCCertificate represents a tamper-proof post-quantum cryptographic attestation certificate for a trajectory.
type PQCCertificate struct {
	SessionID         string       `json:"session_id"`
	AgentName         string       `json:"agent_name"`
	Algorithm         PQCAlgorithm `json:"algorithm"`
	EventCount        int          `json:"event_count"`
	RootMerkleHash    string       `json:"root_merkle_hash"`
	TrajectoryDigest  string       `json:"trajectory_digest"`
	PQCPublicKey      string       `json:"pqc_public_key"`
	PQCSignature      string       `json:"pqc_signature"`
	ClassicalSignature string      `json:"classical_signature,omitempty"` // For hybrid Ed25519 mode
	SignedAt          time.Time    `json:"signed_at"`
	SecurityLevelBits int          `json:"security_level_bits"` // e.g. 192 bits (NIST Level 3)
}

// PQCVerificationResult details the cryptographic verification verdict.
type PQCVerificationResult struct {
	Valid                 bool         `json:"valid"`
	PostQuantumSignatureOK bool        `json:"post_quantum_signature_ok"`
	ClassicalSignatureOK   bool        `json:"classical_signature_ok"`
	HashChainOK           bool         `json:"hash_chain_ok"`
	Algorithm             PQCAlgorithm `json:"algorithm"`
	TamperedStep          int          `json:"tampered_step"` // -1 if no tampering
	QuantumSecurityStatus string       `json:"quantum_security_status"`
	Details               string       `json:"details"`
}

// PQCReport consolidates post-quantum attestation evaluation for display.
type PQCReport struct {
	SessionID          string                `json:"session_id"`
	Certificate        PQCCertificate        `json:"certificate"`
	Verification       PQCVerificationResult `json:"verification"`
	KEMResult          *KyberKEMResult       `json:"kem_result,omitempty"`
	Passed             bool                  `json:"passed"`
}
