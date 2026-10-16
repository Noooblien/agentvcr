package zkp

import (
	"time"
)

// ProofClaim defines the high-level policy claim proven without exposing raw trajectory text or secrets.
type ProofClaim struct {
	ClaimID                string   `json:"claim_id"`
	Description            string   `json:"description"`
	RequireZeroPII         bool     `json:"require_zero_pii"`
	RequireZeroToolErrors  bool     `json:"require_zero_tool_errors"`
	MaxCostUSDLimit        float64  `json:"max_cost_usd_limit,omitempty"`
	MaxTokensLimit         int      `json:"max_tokens_limit,omitempty"`
	ForbiddenHostPrefixes  []string `json:"forbidden_host_prefixes,omitempty"`
	PermittedToolWhitelist []string `json:"permitted_tool_whitelist,omitempty"`
}

// MerkleAuditNode represents an individual node in the cryptographic trajectory Merkle tree.
type MerkleAuditNode struct {
	Index        int    `json:"index"`
	EventType    string `json:"event_type"`
	LeafHash     string `json:"leaf_hash"`
	PedersenCommitment string `json:"pedersen_commitment"` // Blinding commitment for payload privacy
}

// MerkleProofPath provides the sparse inclusion proof for an audited step.
type MerkleProofPath struct {
	StepIndex int      `json:"step_index"`
	LeafHash  string   `json:"leaf_hash"`
	Siblings  []string `json:"siblings"`
}

// ZKAttestationProof represents the tamper-proof zero-knowledge proof package.
type ZKAttestationProof struct {
	ProofID               string            `json:"proof_id"`
	SessionID             string            `json:"session_id"`
	AgentName             string            `json:"agent_name"`
	CreatedAt             time.Time         `json:"created_at"`
	TotalSteps            int               `json:"total_steps"`
	MerkleRoot            string            `json:"merkle_root"`
	Claim                 ProofClaim        `json:"claim"`
	ClaimSatisfied        bool              `json:"claim_satisfied"`
	Commitments           []string          `json:"commitments"`
	ChallengeHash         string            `json:"challenge_hash"` // Fiat-Shamir heuristic non-interactive challenge
	ResponseSignature     string            `json:"response_signature"`
	PublicVerificationKey string            `json:"public_verification_key"`
	AuditPaths            []MerkleProofPath `json:"audit_paths,omitempty"`
}

// ZKVerifyResult details the zero-knowledge verification outcome.
type ZKVerifyResult struct {
	Valid              bool     `json:"valid"`
	MerkleIntegrityOK  bool     `json:"merkle_integrity_ok"`
	ClaimConformant    bool     `json:"claim_conformant"`
	ZeroKnowledgeValid bool     `json:"zero_knowledge_valid"`
	Details            string   `json:"details"`
	Violations         []string `json:"violations,omitempty"`
}
