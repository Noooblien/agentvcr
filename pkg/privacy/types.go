package privacy

import (
	"time"
)

// TokenCategory represents the classified type of sensitive data identified in a trajectory.
type TokenCategory string

const (
	CategoryCredential TokenCategory = "credential" // API keys, bearer tokens, JWTs, AWS keys
	CategoryEmail      TokenCategory = "email"      // User emails
	CategoryIPAddress  TokenCategory = "ip_address" // IPv4/IPv6 addresses
	CategoryFinancial  TokenCategory = "financial"  // Credit card patterns, IBANs
	CategoryPII        TokenCategory = "pii"        // SSNs, phone numbers, personal identifiers
)

// DPConfig holds the differential privacy parameters and budgeting thresholds.
type DPConfig struct {
	Epsilon             float64 `json:"epsilon"`               // Privacy loss parameter (smaller = stronger privacy)
	Delta               float64 `json:"delta"`                 // Probability of failure (typically <= 1e-5)
	Sensitivity         float64 `json:"sensitivity"`           // Global sensitivity (delta f) of numerical queries
	TotalBudgetEpsilon  float64 `json:"total_budget_epsilon"`  // Max cumulative epsilon allowed
	KAnonymityThreshold int     `json:"k_anonymity_threshold"` // Minimum group size for quasi-identifier equivalence
	LDiversityThreshold int     `json:"l_diversity_threshold"` // Minimum distinct sensitive values in equivalence class
	DeterministicSeed   int64   `json:"deterministic_seed"`    // Optional random seed for reproducible testing
}

// DefaultDPConfig provides production-grade differential privacy defaults.
func DefaultDPConfig() DPConfig {
	return DPConfig{
		Epsilon:             1.0,
		Delta:               1e-5,
		Sensitivity:         1.0,
		TotalBudgetEpsilon:  10.0,
		KAnonymityThreshold: 3,
		LDiversityThreshold: 2,
		DeterministicSeed:   0,
	}
}

// DPBudgetTracker records cumulative privacy loss across queries and trajectory exports.
type DPBudgetTracker struct {
	TotalEpsilon     float64   `json:"total_epsilon"`
	SpentEpsilon     float64   `json:"spent_epsilon"`
	RemainingEpsilon float64   `json:"remaining_epsilon"`
	QueryCount       int       `json:"query_count"`
	BudgetExhausted  bool      `json:"budget_exhausted"`
	LastUpdated      time.Time `json:"last_updated"`
}

// MaskingReport summarizes token masking and redaction across a trajectory.
type MaskingReport struct {
	TotalTokensMasked int            `json:"total_tokens_masked"`
	CategoryCounts    map[string]int `json:"category_counts"`
	MaskedKeysCount   int            `json:"masked_keys_count"`
	MaskedEmailsCount int            `json:"masked_emails_count"`
	MaskedIPsCount    int            `json:"masked_ips_count"`
	MaskedPIICount    int            `json:"masked_pii_count"`
	RedactedHashes    []string       `json:"redacted_hashes"`
}

// EquivalenceClass represents a partition of agent sessions sharing identical quasi-identifiers.
type EquivalenceClass struct {
	QIDSignature               string   `json:"qid_signature"`
	AgentName                  string   `json:"agent_name"`
	ModelBucket                string   `json:"model_bucket"`
	ToolSet                    []string `json:"tool_set"`
	StepCountBucket            string   `json:"step_count_bucket"`
	SessionIDs                 []string `json:"session_ids"`
	MemberCount                int      `json:"member_count"`
	IsKCompliant               bool     `json:"is_k_compliant"`
	UniqueSensitiveValuesCount int      `json:"unique_sensitive_values_count"`
	IsLCompliant               bool     `json:"is_l_compliant"`
}

// KAnonymityReport details fleet-wide re-identification auditing results.
type KAnonymityReport struct {
	KThreshold              int      `json:"k_threshold"`
	LThreshold              int      `json:"l_threshold"`
	TotalSessionsAudited    int      `json:"total_sessions_audited"`
	TotalEquivalenceClasses int      `json:"total_equivalence_classes"`
	CompliantClasses        int      `json:"compliant_classes"`
	ViolatingClasses        int      `json:"violating_classes"`
	VulnerableSessionIDs    []string `json:"vulnerable_session_ids"`
	ReidentificationRisk    float64  `json:"reidentification_risk"` // 0.0 to 1.0
	Passed                  bool     `json:"passed"`
}

// PrivacyAuditReport consolidates differential privacy noise, token masking, and k-anonymity checks.
type PrivacyAuditReport struct {
	SessionID         string           `json:"session_id"`
	Epsilon           float64          `json:"epsilon"`
	Delta             float64          `json:"delta"`
	LaplaceNoiseScale float64          `json:"laplace_noise_scale"`
	Budget            DPBudgetTracker  `json:"budget"`
	Masking           MaskingReport    `json:"masking"`
	KAnonymity        KAnonymityReport `json:"k_anonymity"`
	OverallCompliance bool             `json:"overall_compliance"`
	Recommendations   []string         `json:"recommendations"`
}
