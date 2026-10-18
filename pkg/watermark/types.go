package watermark

import (
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// WatermarkStrategy defines the cryptographic & linguistic mechanism used to embed provenance.
type WatermarkStrategy string

const (
	StrategyZeroWidthUnicode WatermarkStrategy = "zero_width_unicode" // Invisible ZWSP/ZWNJ/ZWJ bit encoding
	StrategyHomoglyphSub     WatermarkStrategy = "homoglyph_sub"      // Visually identical UTF-8 codepoint substitution
	StrategySynonymPerturb   WatermarkStrategy = "synonym_perturb"    // Deterministic semantic synonym encoding
	StrategyHeaderSignature WatermarkStrategy = "header_signature"   // Cryptographic provenance header injection
)

// WatermarkPayload holds metadata embedded into the agent trajectory text.
type WatermarkPayload struct {
	SessionID   string    `json:"session_id"`
	AgentID     string    `json:"agent_id"`
	Model       string    `json:"model"`
	OrgID       string    `json:"org_id"`
	Timestamp   time.Time `json:"timestamp"`
	PayloadHash string    `json:"payload_hash"`
	Signature   string    `json:"signature"`
}

// WatermarkConfig holds configuration parameters for embedding and extraction.
type WatermarkConfig struct {
	Strategy      WatermarkStrategy `json:"strategy"`
	SecretKey     string            `json:"secret_key"`
	OrgID         string            `json:"org_id"`
	EmbedInterval int               `json:"embed_interval"`  // Embed every N words or turns
	Redundancy    int               `json:"redundancy"`      // Number of repetitions for resilience against cropping
	StripOnReplay bool              `json:"strip_on_replay"` // Strip watermark during deterministic replay
}

// DefaultWatermarkConfig returns production defaults for the watermark engine.
func DefaultWatermarkConfig() WatermarkConfig {
	return WatermarkConfig{
		Strategy:      StrategyZeroWidthUnicode,
		SecretKey:     "agentvcr-provenance-default-secret-key",
		OrgID:         "agentvcr-org-global",
		EmbedInterval: 1,
		Redundancy:    2,
		StripOnReplay: false,
	}
}

// ExtractionResult captures the outcome of scanning text for embedded watermarks.
type ExtractionResult struct {
	Detected         bool              `json:"detected"`
	Strategy         WatermarkStrategy `json:"strategy"`
	Payload          *WatermarkPayload `json:"payload,omitempty"`
	BitConfidence    float64           `json:"bit_confidence"` // 0.0 to 1.0 (accuracy/integrity of extracted bits)
	Tampered         bool              `json:"tampered"`
	ExtractedBits    int               `json:"extracted_bits"`
	RawMatchedString string            `json:"raw_matched_string,omitempty"`
}

// WatermarkAuditReport summarizes trajectory watermarking and extraction forensics.
type WatermarkAuditReport struct {
	ReportID              string             `json:"report_id"`
	SessionID             string             `json:"session_id"`
	Strategy              WatermarkStrategy  `json:"strategy"`
	TotalTurnsScanned     int                `json:"total_turns_scanned"`
	WatermarksEmbedded    int                `json:"watermarks_embedded"`
	WatermarksExtracted   int                `json:"watermarks_extracted"`
	AttestationValid      bool               `json:"attestation_valid"`
	OverallIntegrityScore float64            `json:"overall_integrity_score"` // 0.0 - 10.0
	Status                string             `json:"status"`                  // "VERIFIED", "TAMPERED", "NOT_DETECTED"
	Results               []ExtractionResult `json:"results"`
	TraceSession          *trace.Session     `json:"trace_session,omitempty"`
	Passed                bool               `json:"passed"`
}
