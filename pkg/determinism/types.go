package determinism

import (
	"time"
)

// DeterminismStatus categorizes the outcome of the determinism evaluation.
type DeterminismStatus string

const (
	StatusPass     DeterminismStatus = "PASS"
	StatusDegraded DeterminismStatus = "DEGRADED"
	StatusBreach   DeterminismStatus = "BREACH"
)

// DivergenceKind indicates what caused a trajectory divergence.
type DivergenceKind string

const (
	DivergenceToolMismatch   DivergenceKind = "TOOL_MISMATCH"
	DivergenceArgumentDrift  DivergenceKind = "ARGUMENT_DRIFT"
	DivergenceTokenSurge     DivergenceKind = "TOKEN_SURGE"
	DivergenceMissingStep    DivergenceKind = "MISSING_STEP"
	DivergenceUnexpectedStep DivergenceKind = "UNEXPECTED_STEP"
	DivergenceChecksumDrift  DivergenceKind = "CHECKSUM_DRIFT"
)

// Severity indicates impact of a divergence.
type Severity string

const (
	SeverityInfo     Severity = "INFO"
	SeverityWarn     Severity = "WARN"
	SeverityCritical Severity = "CRITICAL"
)

// DivergenceEvent details a point of divergence between baseline and replayed target.
type DivergenceEvent struct {
	StepIndex   int            `json:"step_index"`
	Kind        DivergenceKind `json:"kind"`
	Severity    Severity       `json:"severity"`
	Description string         `json:"description"`
	BaselineVal string         `json:"baseline_val,omitempty"`
	TargetVal   string         `json:"target_val,omitempty"`
}

// LatencyPercentiles contains step execution latency distributions in milliseconds.
type LatencyPercentiles struct {
	P50Ms           float64 `json:"p50_ms"`
	P90Ms           float64 `json:"p90_ms"`
	P95Ms           float64 `json:"p95_ms"`
	P99Ms           float64 `json:"p99_ms"`
	MeanMs          float64 `json:"mean_ms"`
	JitterMs        float64 `json:"jitter_ms"`         // Standard deviation of step latency
	ProxyOverheadUs int64   `json:"proxy_overhead_us"` // Microseconds interception overhead per step
}

// TokenDivergence tracks token count variations between baseline and target runs.
type TokenDivergence struct {
	BaselineTokens int     `json:"baseline_tokens"`
	TargetTokens   int     `json:"target_tokens"`
	DeltaTokens    int     `json:"delta_tokens"`
	BloatRatio     float64 `json:"bloat_ratio"` // TargetTokens / BaselineTokens
	StabilityScore float64 `json:"stability_score"` // 0.0 to 100.0%
}

// MetricSample aggregates empirical metrics from replay comparison.
type MetricSample struct {
	TotalBaselineSteps    int                `json:"total_baseline_steps"`
	TotalTargetSteps      int                `json:"total_target_steps"`
	MatchingToolSteps     int                `json:"matching_tool_steps"`
	TotalToolCalls        int                `json:"total_tool_calls"`
	ToolParityPercent     float64            `json:"tool_parity_percent"`     // 0.0 - 100.0%
	ArgumentParityPercent float64            `json:"argument_parity_percent"` // 0.0 - 100.0%
	ChecksumParityPercent float64            `json:"checksum_parity_percent"` // 0.0 - 100.0%
	Tokens                TokenDivergence    `json:"tokens"`
	Latency               LatencyPercentiles `json:"latency"`
	DeterminismScore      float64            `json:"determinism_score"` // Weighted 0.0 - 100.0%
}

// DeterminismConfig defines assertion gates and thresholds for CI/CD replay validation.
type DeterminismConfig struct {
	MinScore           float64 `json:"min_score"`             // e.g. 85.0%
	MinToolParity      float64 `json:"min_tool_parity"`       // e.g. 90.0%
	MaxTokenBloatRatio float64 `json:"max_token_bloat_ratio"` // e.g. 1.20 (max 20% bloat)
	MaxJitterMs        float64 `json:"max_jitter_ms"`         // e.g. 150.0 ms
	EnforceStrict      bool    `json:"enforce_strict"`        // Fail on any critical divergence
}

// DefaultConfig provides recommended production enterprise thresholds.
func DefaultConfig() DeterminismConfig {
	return DeterminismConfig{
		MinScore:           85.0,
		MinToolParity:      90.0,
		MaxTokenBloatRatio: 1.20,
		MaxJitterMs:        150.0,
		EnforceStrict:      false,
	}
}

// DeterminismReport represents the full benchmark result.
type DeterminismReport struct {
	SuiteName      string            `json:"suite_name"`
	BaselineID     string            `json:"baseline_id"`
	TargetID       string            `json:"target_id"`
	Status         DeterminismStatus `json:"status"`
	Passed         bool              `json:"passed"`
	DeterminismScore float64         `json:"determinism_score"`
	Metrics        MetricSample      `json:"metrics"`
	Divergences    []DivergenceEvent `json:"divergences"`
	Summary        string            `json:"summary"`
	EvaluatedAt    time.Time         `json:"evaluated_at"`
	ExecutionDurUs int64             `json:"execution_dur_us"`
}
