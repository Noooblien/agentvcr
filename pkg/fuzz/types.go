package fuzz

import (
	"time"
)

// FuzzTargetType defines the attack surface under adversarial evaluation.
type FuzzTargetType string

const (
	TargetMCPFrame         FuzzTargetType = "MCP_JSONRPC_FRAME"
	TargetLLMPromptContext FuzzTargetType = "LLM_PROMPT_CONTEXT"
	TargetToolOutputBuffer FuzzTargetType = "TOOL_OUTPUT_BUFFER"
	TargetFilesystemCoW    FuzzTargetType = "FILESYSTEM_COW_STATE"
	TargetStateTransition  FuzzTargetType = "AGENT_STATE_TRANSITION"
)

// MutationCategory identifies the domain of adversarial perturbation.
type MutationCategory string

const (
	MutationPromptJailbreak   MutationCategory = "PROMPT_JAILBREAK"
	MutationMalformedJSONRPC  MutationCategory = "MALFORMED_JSONRPC"
	MutationPayloadTruncation MutationCategory = "PAYLOAD_TRUNCATION"
	MutationTypeConfusion     MutationCategory = "TYPE_CONFUSION"
	MutationExtremeLatency    MutationCategory = "EXTREME_LATENCY"
	MutationResourceExhaustion MutationCategory = "RESOURCE_EXHAUSTION"
	MutationNullByteInjection MutationCategory = "NULL_BYTE_INJECTION"
	MutationUnicodeExplosion  MutationCategory = "UNICODE_EXPLOSION"
)

// FailureClassification identifies how the agent or runtime failed under fuzzing.
type FailureClassification string

const (
	FailureNone              FailureClassification = "NONE_RESILIENT"
	FailureCrashPanic        FailureClassification = "CRASH_PANIC"
	FailureUnhandled500      FailureClassification = "UNHANDLED_ERROR"
	FailurePromptJailbreak   FailureClassification = "PROMPT_CONSTRAINTS_VIOLATED"
	FailureDeadlockHang      FailureClassification = "DEADLOCK_TIMEOUT_HANG"
	FailureStateCorruption   FailureClassification = "CORRUPTED_SESSION_STATE"
	FailureMemorySpikeOOM    FailureClassification = "MEMORY_EXHAUSTION_OOM"
)

// FuzzTestCase captures an individualized adversarial trial.
type FuzzTestCase struct {
	CaseID           string                `json:"case_id"`
	Target           FuzzTargetType        `json:"target"`
	Category         MutationCategory      `json:"category"`
	StepIndex        int                   `json:"step_index"`
	InputPayload     string                `json:"input_payload"`
	Perturbation     string                `json:"perturbation"`
	ExpectedBehavior string                `json:"expected_behavior"`
	ObservedBehavior string                `json:"observed_behavior"`
	FailureClass     FailureClassification `json:"failure_class"`
	Duration         time.Duration         `json:"duration"`
	Surviving        bool                  `json:"surviving"`
	Severity         string                `json:"severity"` // "CRITICAL", "HIGH", "MEDIUM", "LOW"
}

// FuzzCampaignReport summarizes an entire adversarial fuzzing execution.
type FuzzCampaignReport struct {
	CampaignID          string          `json:"campaign_id"`
	SessionID           string          `json:"session_id"`
	TotalMutations      int             `json:"total_mutations"`
	CrashesDetected     int             `json:"crashes_detected"`
	JailbreaksDetected  int             `json:"jailbreaks_detected"`
	HangsDetected       int             `json:"hangs_detected"`
	StateCorruptions    int             `json:"state_corruptions"`
	ResilientCount      int             `json:"resilient_count"`
	AgentRobustnessScore float64         `json:"agent_robustness_score"` // 0.0 - 100.0
	Status              string          `json:"status"`                 // "HARDENED", "VULNERABLE", "CRITICAL"
	ExecutedAt          time.Time       `json:"executed_at"`
	Cases               []FuzzTestCase  `json:"cases"`
}

// FuzzEngineConfig configures coverage-guided and grammar mutation settings.
type FuzzEngineConfig struct {
	RandomSeed        int64         `json:"random_seed"`
	MaxMutations      int           `json:"max_mutations"`
	TimeoutPerCase    time.Duration `json:"timeout_per_case"`
	EnableJailbreak   bool          `json:"enable_jailbreak"`
	EnableJSONRPCFuzz bool          `json:"enable_jsonrpc_fuzz"`
	StrictCI          bool          `json:"strict_ci"`
}
