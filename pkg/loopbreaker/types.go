package loopbreaker

import "time"

// CyclePattern categorizes the structural periodicity of a detected agent loop.
type CyclePattern string

const (
	PatternNone                  CyclePattern = "NONE"
	PatternSelfLoop              CyclePattern = "PERIOD_1_SELF_LOOP"             // A -> A -> A
	PatternBipartiteOscillation  CyclePattern = "PERIOD_2_BIPARTITE_OSCILLATION" // A -> B -> A -> B
	PatternMultiStepCycle        CyclePattern = "PERIOD_K_CIRCULAR_DEADLOCK"     // A -> B -> C -> A -> B -> C
	PatternArgumentThrashing     CyclePattern = "ARGUMENT_THRASHING_STAGNATION"  // Same tool, identical failing args
)

// LoopAction defines the automated intervention mechanism enforced by the circuit breaker.
type LoopAction string

const (
	ActionAllow               LoopAction = "ALLOW"
	ActionWarn                LoopAction = "WARN"
	ActionInjectSteering      LoopAction = "INJECT_STEERING"
	ActionTripCircuitBreaker  LoopAction = "TRIP_CIRCUIT_BREAKER"
)

// LoopConfig specifies tunable cycle detection and circuit breaker intervention parameters.
type LoopConfig struct {
	MaxPeriod                  int     `json:"max_period"`                    // Max cycle period (e.g., 5)
	MinRecurrence              int     `json:"min_recurrence"`                // Min repetitions before flagging (e.g., 2)
	ArgumentSimilarityThreshold float64 `json:"argument_similarity_threshold"` // 0.0 - 1.0 (e.g., 0.80)
	MaxWarningsTolerated       int     `json:"max_warnings_tolerated"`        // Warnings before hard trip (e.g., 1)
	DefaultCostPerTokenUSD     float64 `json:"default_cost_per_token_usd"`    // Cost model (e.g., 0.000005)
}

// DefaultLoopConfig returns industry-standard production thresholds for agent loop containment.
func DefaultLoopConfig() LoopConfig {
	return LoopConfig{
		MaxPeriod:                   5,
		MinRecurrence:               2,
		ArgumentSimilarityThreshold: 0.80,
		MaxWarningsTolerated:        1,
		DefaultCostPerTokenUSD:      0.000005, // $5 / 1M tokens
	}
}

// StepRecord captures a single historical or live tool invocation event.
type StepRecord struct {
	StepIndex      int                    `json:"step_index"`
	Timestamp      time.Time              `json:"timestamp"`
	ToolName       string                 `json:"tool_name"`
	Arguments      map[string]interface{} `json:"arguments"`
	ArgumentsJSON  string                 `json:"arguments_json"`
	OutputJSON     string                 `json:"output_json"`
	IsError        bool                   `json:"is_error"`
	TokensConsumed int                    `json:"tokens_consumed"`
}

// CycleDetection encapsulates the evaluation result of a potential reasoning loop.
type CycleDetection struct {
	Detected             bool         `json:"detected"`
	Pattern              CyclePattern `json:"pattern"`
	Period               int          `json:"period"`
	RecurrenceCount      int          `json:"recurrence_count"`
	Confidence           float64      `json:"confidence"`
	RepeatingTools       []string     `json:"repeating_tools"`
	ArgumentSimilarity   float64      `json:"argument_similarity"`
	Action               LoopAction   `json:"action"`
	SteeringPrompt       string       `json:"steering_prompt,omitempty"`
	TokensWasted         int          `json:"tokens_wasted"`
	EstimatedCostUSD     float64      `json:"estimated_cost_usd"`
	TriggerStepIndex     int          `json:"trigger_step_index"`
	Description          string       `json:"description"`
}

// LoopBreakerSummary aggregates metrics across a full trajectory inspection.
type LoopBreakerSummary struct {
	TotalStepsEvaluated    int              `json:"total_steps_evaluated"`
	CyclesDetected         int              `json:"cycles_detected"`
	WarningsIssued         int              `json:"warnings_issued"`
	SteeringInterventions  int              `json:"steering_interventions"`
	CircuitBreakersTripped int              `json:"circuit_breakers_tripped"`
	TotalTokensSaved       int              `json:"total_tokens_saved"`
	EstimatedSavingsUSD    float64          `json:"estimated_savings_usd"`
	Detections             []CycleDetection `json:"detections"`
}
