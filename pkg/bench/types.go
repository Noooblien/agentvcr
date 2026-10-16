package bench

// ProbeType defines the type of memory regression or long-horizon benchmark probe.
type ProbeType string

const (
	ProbeNeedleRecall           ProbeType = "needle_recall"
	ProbeConstraintPreservation ProbeType = "constraint_preservation"
	ProbeStateConsistency       ProbeType = "state_consistency"
	ProbeContextEfficiency      ProbeType = "context_efficiency"
)

// MemoryProbe represents a deterministic test probe against agent memory and context retention.
type MemoryProbe struct {
	ID               string      `json:"id"`
	Type             ProbeType   `json:"type"`
	Description      string      `json:"description"`
	Needle           string      `json:"needle,omitempty"`            // Value/token expected to be recalled
	ForbiddenPattern string      `json:"forbidden_pattern,omitempty"` // Regex or substring that must NEVER appear
	IntroduceStep    int         `json:"introduce_step,omitempty"`    // Step index where fact/constraint was given
	ProbeStep        int         `json:"probe_step,omitempty"`        // Step index where recall is verified
	StateKey         string      `json:"state_key,omitempty"`         // State variable key
	ExpectedValue    interface{} `json:"expected_value,omitempty"`    // Expected state value
}

// BenchmarkConfig defines the benchmark suite and passing thresholds.
type BenchmarkConfig struct {
	SuiteName                string        `json:"suite_name"`
	MinRecallScore           float64       `json:"min_recall_score"`           // Minimum required recall accuracy (0.0 to 1.0)
	MaxConstraintViolations  int           `json:"max_constraint_violations"`  // Maximum allowed constraint breaks (usually 0)
	MaxContextBloatRatio     float64       `json:"max_context_bloat_ratio"`    // Threshold for token context inflation
	Probes                   []MemoryProbe `json:"probes"`
}

// ProbeResult records the outcome of an individual benchmark probe.
type ProbeResult struct {
	ProbeID     string    `json:"probe_id"`
	Type        ProbeType `json:"type"`
	Passed      bool      `json:"passed"`
	StepIndex   int       `json:"step_index"`
	Expected    string    `json:"expected"`
	Observed    string    `json:"observed"`
	RetentionGap int      `json:"retention_gap,omitempty"` // Steps between introduction and probe
	Message     string    `json:"message"`
}

// BenchmarkReport encapsulates aggregate metrics and per-probe outcomes.
type BenchmarkReport struct {
	SessionID            string        `json:"session_id"`
	SuiteName            string        `json:"suite_name"`
	TotalProbes          int           `json:"total_probes"`
	PassedProbes         int           `json:"passed_probes"`
	FailedProbes         int           `json:"failed_probes"`
	RecallScore          float64       `json:"recall_score"`           // Passed needle recall probes / total needle probes
	ConstraintViolations int           `json:"constraint_violations"` // Number of negative constraint violations
	MaxRetentionHorizon  int           `json:"max_retention_horizon"`  // Max step gap successfully preserved
	AverageStepGap       float64       `json:"average_step_gap"`       // Average step gap of evaluated probes
	ContextBloatRatio    float64       `json:"context_bloat_ratio"`    // Ratio of token expansion across trajectory
	PassedSLA            bool          `json:"passed_sla"`
	Results              []ProbeResult `json:"results"`
}
