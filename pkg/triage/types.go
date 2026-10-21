package triage

import (
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// FaultCategory classifies the fundamental origin of an agent execution failure.
type FaultCategory string

const (
	FaultModelHallucination       FaultCategory = "MODEL_HALLUCINATION"        // Invalid arguments, non-existent tool, hallucinated flags
	FaultEnvironmentOutage        FaultCategory = "ENVIRONMENT_OUTAGE"         // 5xx upstream failure, socket refused, network drop
	FaultResourceExhaustion       FaultCategory = "RESOURCE_EXHAUSTION"        // HTTP 429 rate limit, OOM, context window overflow
	FaultPermissionDenied         FaultCategory = "PERMISSION_DENIED"          // 401/403, EACCES, sandbox violation
	FaultRunawayLoop              FaultCategory = "RUNAWAY_LOOP"               // Repetitive identical tool calls (OWASP-LLM08)
	FaultSemanticAssertionFailure FaultCategory = "SEMANTIC_ASSERTION_FAILURE" // Output contract or goal invariant unsatisfied
)

// FaultSeverity indicates the impact level on the agent task.
type FaultSeverity string

const (
	SeverityLow      FaultSeverity = "LOW"
	SeverityMedium   FaultSeverity = "MEDIUM"
	SeverityHigh     FaultSeverity = "HIGH"
	SeverityCritical FaultSeverity = "CRITICAL"
)

// RemediationStrategy specifies the automated compensatory action.
type RemediationStrategy string

const (
	StrategySchemaCorrection    RemediationStrategy = "SCHEMA_CORRECTION"      // Adjust JSON argument types or field names
	StrategyExponentialBackoff  RemediationStrategy = "EXPONENTIAL_BACKOFF"    // Add jittered exponential backoff retry
	StrategyFallbackToolRouting RemediationStrategy = "FALLBACK_TOOL_ROUTING"  // Reroute to secondary tool (e.g. curl -> wget)
	StrategyPromptGuardrailPatch RemediationStrategy = "PROMPT_GUARDRAIL_PATCH" // Inject corrective prompt steering instruction
	StrategyPathRedirection     RemediationStrategy = "PATH_REDIRECTION"       // Redirect privileged path to user workspace
	StrategyLoopBreakIntervention RemediationStrategy = "LOOP_BREAK_INTERVENTION" // Inject circuit-breaker break instruction
)

// RemediationAction describes an actionable synthesized fix for a diagnosed fault.
type RemediationAction struct {
	ActionID            string              `json:"action_id"`
	Strategy            RemediationStrategy `json:"strategy"`
	TargetStepIndex     int                 `json:"target_step_index"`
	TargetTool          string              `json:"target_tool,omitempty"`
	PatchDescription    string              `json:"patch_description"`
	SuggestedPayload    map[string]interface{}`json:"suggested_payload,omitempty"`
	CorrectivePrompt    string              `json:"corrective_prompt,omitempty"`
	ConfidenceScore     float64             `json:"confidence_score"` // 0.0 - 1.0
	AutomatedApplicable bool                `json:"automated_applicable"`
}

// TriageFinding encapsulates a localized failure root cause and its causal chain.
type TriageFinding struct {
	FindingID         string            `json:"finding_id"`
	Category          FaultCategory     `json:"category"`
	Severity          FaultSeverity     `json:"severity"`
	IncitingStepIndex int               `json:"inciting_step_index"`
	IncitingEventID   string            `json:"inciting_event_id"`
	IncitingEventType trace.EventType   `json:"inciting_event_type"`
	FailedToolName    string            `json:"failed_tool_name,omitempty"`
	ErrorMessage      string            `json:"error_message"`
	RootCauseSummary  string            `json:"root_cause_summary"`
	CausalChainSteps  []int             `json:"causal_chain_steps"`
	Confidence        float64           `json:"confidence"` // 0.0 - 1.0
	Remediation       RemediationAction `json:"remediation"`
}

// TriageReport aggregates the complete diagnostic evaluation of an execution trace.
type TriageReport struct {
	SessionID        string              `json:"session_id"`
	AgentName        string              `json:"agent_name"`
	AnalyzedAt       time.Time           `json:"analyzed_at"`
	TotalEvents      int                 `json:"total_events"`
	FailedEvents     int                 `json:"failed_events"`
	PrimaryCategory  FaultCategory       `json:"primary_category"`
	OverallSeverity  FaultSeverity       `json:"overall_severity"`
	Findings         []TriageFinding     `json:"findings"`
	Remediations     []RemediationAction `json:"remediations"`
	ExecutionTimeUs  int64               `json:"execution_time_us"`
	IsCleanRun       bool                `json:"is_clean_run"`
}

// TriageConfig configures the diagnostic engine heuristics.
type TriageConfig struct {
	LoopDetectionThreshold int     `json:"loop_detection_threshold"` // Default 3 identical calls
	MinConfidenceThreshold float64 `json:"min_confidence_threshold"` // Default 0.70
	EnableAutoPatch        bool    `json:"enable_auto_patch"`        // Generate ready-to-apply patches
}

// DefaultTriageConfig returns production defaults for autonomous triage.
func DefaultTriageConfig() TriageConfig {
	return TriageConfig{
		LoopDetectionThreshold: 3,
		MinConfidenceThreshold: 0.70,
		EnableAutoPatch:        true,
	}
}
