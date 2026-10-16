package timing

import (
	"time"
)

// TimingMetric captures the high-resolution microsecond latency profile of a single trajectory event.
type TimingMetric struct {
	StepIndex        int           `json:"step_index"`
	EventType        string        `json:"event_type"`
	DurationMs       int64         `json:"duration_ms"`
	PromptByteLen    int           `json:"prompt_byte_len"`
	ResponseByteLen  int           `json:"response_byte_len"`
	TTFTMs           int64         `json:"ttft_ms"` // Time-to-first-token in ms
	TokensPerSecond  float64       `json:"tokens_per_second"`
	CacheHitRatio    float64       `json:"cache_hit_ratio"` // 0.0 to 1.0 (KV cache affinity)
	MemoryBandwidthMB float64      `json:"memory_bandwidth_mb"`
}

// SideChannelVulnerability describes a detected timing leakage risk.
type SideChannelVulnerability struct {
	StepIndex           int     `json:"step_index"`
	VulnerabilityType   string  `json:"vulnerability_type"`   // e.g. "kv_cache_fingerprint", "differential_latency_leak", "token_length_bias"
	Severity            string  `json:"severity"`             // "low", "medium", "critical"
	MutualInformationBits float64 `json:"mutual_information_bits"` // Estimated information leakage in bits
	ConfidenceScore     float64 `json:"confidence_score"`
	Description         string  `json:"description"`
	MitigationAdvice    string  `json:"mitigation_advice"`
}

// TimingAnalysisReport contains the complete statistical side-channel analysis of an agent trajectory.
type TimingAnalysisReport struct {
	SessionID                 string                     `json:"session_id"`
	TotalStepsAnalyzed        int                        `json:"total_steps_analyzed"`
	MeanLatencyMs             float64                    `json:"mean_latency_ms"`
	MedianLatencyMs           float64                    `json:"median_latency_ms"`
	P99LatencyMs              float64                    `json:"p99_latency_ms"`
	StandardDeviationMs       float64                    `json:"standard_deviation_ms"`
	CacheDifferentialMs       float64                    `json:"cache_differential_ms"`
	TotalInformationLeakBits  float64                    `json:"total_information_leak_bits"`
	Vulnerabilities           []SideChannelVulnerability `json:"vulnerabilities"`
	TimingLeakagePassed       bool                       `json:"timing_leakage_passed"`
	MitigatedTrajectoryCloneID string                    `json:"mitigated_trajectory_clone_id,omitempty"`
	GeneratedAt               time.Time                  `json:"generated_at"`
}
