package stego

import (
	"time"
)

// CovertChannelType specifies the steganographic or covert communication mechanism detected.
type CovertChannelType string

const (
	ChannelZeroWidth          CovertChannelType = "zero_width_unicode"   // Zero-width characters encoding binary bits
	ChannelHomoglyph          CovertChannelType = "homoglyph_substitution" // Mixed Cyrillic/Greek lookalikes in Latin text
	ChannelWhitespaceBinary   CovertChannelType = "whitespace_encoding"   // Space/tab trailing binary steganography
	ChannelAcrosticPattern    CovertChannelType = "acrostic_pattern"      // First-letter/first-word line covert message
	ChannelEntropyAnomaly     CovertChannelType = "entropy_anomaly"       // Unusually high Shannon entropy indicating ciphertext
)

// StegoFinding records a single detected covert signal in agent messages or tool inputs.
type StegoFinding struct {
	FindingID      string            `json:"finding_id"`
	ChannelType    CovertChannelType `json:"channel_type"`
	Severity       string            `json:"severity"` // "CRITICAL", "HIGH", "MEDIUM"
	Confidence     float64           `json:"confidence"` // 0.0 - 1.0
	Description    string            `json:"description"`
	DecodedSecret  string            `json:"decoded_secret,omitempty"`
	RawEvidence    string            `json:"raw_evidence"`
	CharOffset     int               `json:"char_offset"`
	EventIndex     int               `json:"event_index"`
	EventType      string            `json:"event_type"`
	Timestamp      time.Time         `json:"timestamp"`
}

// StegoReport compiles steganographic analysis and covert communication forensics across a trace.
type StegoReport struct {
	SessionID             string                  `json:"session_id"`
	AgentName             string                  `json:"agent_name"`
	TotalEventsAnalyzed   int                     `json:"total_events_analyzed"`
	CovertSignalsDetected int                     `json:"covert_signals_detected"`
	CriticalBreaches      int                     `json:"critical_breaches"`
	DecodedSecrets        []string                `json:"decoded_secrets,omitempty"`
	ChannelDistribution   map[CovertChannelType]int `json:"channel_distribution"`
	Findings              []StegoFinding          `json:"findings"`
	AverageEntropy        float64                 `json:"average_entropy"`
	CovertChannelPresent  bool                    `json:"covert_channel_present"`
	ExecutionDuration     time.Duration           `json:"execution_duration"`
}

// StegoConfig configures detection thresholds for the steganography analyzer.
type StegoConfig struct {
	MinEntropyAnomalyThreshold float64 // Entropy threshold above which text is flagged (e.g. 4.8 bits/char)
	CheckHomoglyphs            bool
	CheckZeroWidth             bool
	CheckWhitespace            bool
}

// DefaultStegoConfig returns production-grade defaults for covert channel analysis.
func DefaultStegoConfig() StegoConfig {
	return StegoConfig{
		MinEntropyAnomalyThreshold: 4.85,
		CheckHomoglyphs:            true,
		CheckZeroWidth:             true,
		CheckWhitespace:            true,
	}
}
