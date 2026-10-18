package regress

import (
	"time"
)

// RegressionSeverity denotes the criticality of an assertion failure.
type RegressionSeverity string

const (
	SeverityCritical RegressionSeverity = "CRITICAL" // Security or data integrity failure (e.g. forbidden tool, auth skipped)
	SeverityMajor    RegressionSeverity = "MAJOR"    // Functional degradation (e.g. required step omitted, schema mismatch)
	SeverityMinor    RegressionSeverity = "MINOR"    // Performance or efficiency regression (e.g. token bloat, latency drift)
)

// RegressionVerdict represents the overall evaluation outcome.
type RegressionVerdict string

const (
	VerdictPass RegressionVerdict = "PASS"
	VerdictFail RegressionVerdict = "REGRESSION_DETECTED"
)

// AssertionFailure records a specific broken contract.
type AssertionFailure struct {
	RuleID      string             `json:"rule_id"`
	Severity    RegressionSeverity `json:"severity"`
	Message     string             `json:"message"`
	Observed    interface{}        `json:"observed"`
	Expected    interface{}        `json:"expected"`
	Remediation string             `json:"remediation"`
}

// GoldenSpec defines the declarative expectations and constraints for a task.
type GoldenSpec struct {
	TaskID                     string   `json:"task_id"`
	TaskName                   string   `json:"task_name"`
	GoldenSessionID            string   `json:"golden_session_id"`
	ExpectedToolSequence       []string `json:"expected_tool_sequence"`       // Reference golden tool order
	MaxTokens                  int      `json:"max_tokens"`                  // Hard token ceiling
	MaxTokenBloatPercent       float64  `json:"max_token_bloat_percent"`     // Max percentage above golden tokens (e.g. 20.0%)
	MaxDurationMs              int64    `json:"max_duration_ms"`             // Latency budget
	MaxSteps                   int      `json:"max_steps"`                   // Maximum allowed execution steps
	RequiredTools              []string `json:"required_tools"`              // Must be invoked at least once
	ForbiddenTools             []string `json:"forbidden_tools"`             // Must NEVER be invoked
	MaxConsecutiveToolRetries  int      `json:"max_consecutive_tool_retries"`// Loop prevention threshold
	RequiredOutputKeys         []string `json:"required_output_keys"`        // Final state validation
	MinSequenceSimilarity      float64  `json:"min_sequence_similarity"`     // Minimum LCS alignment (0.0 - 1.0)
	MaxCostUSD                 float64  `json:"max_cost_usd"`
}

// CandidateExecution encapsulates the observed metrics and trace of the candidate run.
type CandidateExecution struct {
	SessionID        string                 `json:"session_id"`
	AgentModel       string                 `json:"agent_model"`
	PromptVersion    string                 `json:"prompt_version"`
	ExecutedTools    []string               `json:"executed_tools"`
	TotalTokens      int                    `json:"total_tokens"`
	DurationMs       int64                  `json:"duration_ms"`
	StepCount        int                    `json:"step_count"`
	FinalOutput      map[string]interface{} `json:"final_output"`
	EstimatedCostUSD float64                `json:"estimated_cost_usd"`
	ErrorsEncountered []string              `json:"errors_encountered"`
}

// SequenceAlignmentResult details the LCS and distance metrics between golden and candidate tool paths.
type SequenceAlignmentResult struct {
	GoldenLength       int     `json:"golden_length"`
	CandidateLength    int     `json:"candidate_length"`
	LCSLength          int     `json:"lcs_length"`
	SimilarityScore    float64 `json:"similarity_score"` // 0.0 to 1.0
	MissingTools       []string`json:"missing_tools"`
	ExtraneousTools    []string`json:"extraneous_tools"`
	OrderPreserved     bool    `json:"order_preserved"`
}

// RegressionReport encapsulates the complete evaluation against golden baselines.
type RegressionReport struct {
	TaskID             string                   `json:"task_id"`
	TaskName           string                   `json:"task_name"`
	CandidateSessionID string                   `json:"candidate_session_id"`
	GoldenSessionID    string                   `json:"golden_session_id"`
	Verdict            RegressionVerdict        `json:"verdict"`
	PassRate           float64                  `json:"pass_rate"` // e.g. 100% or 75%
	Failures           []AssertionFailure       `json:"failures"`
	Alignment          SequenceAlignmentResult  `json:"alignment"`
	TokenDelta         int                      `json:"token_delta"`
	TokenDeltaPercent  float64                  `json:"token_delta_percent"`
	DurationDeltaMs    int64                    `json:"duration_delta_ms"`
	CostDeltaUSD       float64                  `json:"cost_delta_usd"`
	EvaluatedAt        time.Time                `json:"evaluated_at"`
}
