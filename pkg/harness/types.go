package harness

import (
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// BenchmarkStandard defines standardized AI agent evaluation suites.
type BenchmarkStandard string

const (
	BenchmarkSWEBench    BenchmarkStandard = "swe_bench"    // SWE-bench Software Engineering Benchmark
	BenchmarkWebArena    BenchmarkStandard = "webarena"     // WebArena Multi-Turn Web Agent Tasks
	BenchmarkHumanEval   BenchmarkStandard = "humaneval"    // OpenAI HumanEval Coding Suite
	BenchmarkAgentBench  BenchmarkStandard = "agentbench"   // Multi-environment AgentBench
	BenchmarkCustomSuite BenchmarkStandard = "custom_suite" // Custom Trajectory Evaluation Suite
)

// TaskSpec represents a single standardized benchmark challenge or issue.
type TaskSpec struct {
	TaskID          string            `json:"task_id"`
	Standard        BenchmarkStandard `json:"standard"`
	Repository      string            `json:"repository,omitempty"` // e.g. "psf/requests"
	ProblemStatement string            `json:"problem_statement"`
	EnvironmentSetup string            `json:"environment_setup,omitempty"`
	GoldPatch       string            `json:"gold_patch,omitempty"`
	TestPatch       string            `json:"test_patch,omitempty"`
	FailToPassTests []string          `json:"fail_to_pass_tests"`
	PassToPassTests []string          `json:"pass_to_pass_tests"`
	MaxStepsAllowed int               `json:"max_steps_allowed"`
	TimeoutSeconds  int               `json:"timeout_seconds"`
}

// TrialResult captures the execution and verification of one agent attempt on a task.
type TrialResult struct {
	TaskID          string         `json:"task_id"`
	TrialIndex      int            `json:"trial_index"`
	SessionID       string         `json:"session_id"`
	ModelID         string         `json:"model_id"`
	Resolved        bool           `json:"resolved"`
	PassedTests     []string       `json:"passed_tests"`
	FailedTests     []string       `json:"failed_tests"`
	SyntaxValid     bool           `json:"syntax_valid"`
	ExecutionTimeMs int64          `json:"execution_time_ms"`
	TokensConsumed  int            `json:"tokens_consumed"`
	CostUSD         float64        `json:"cost_usd"`
	StepCount       int            `json:"step_count"`
	ErrorReason     string         `json:"error_reason,omitempty"`
	TraceSession    *trace.Session `json:"trace_session,omitempty"`
}

// TaskAggregate aggregates multiple sampled attempts (trials) for a task to calculate pass@k.
type TaskAggregate struct {
	TaskID       string        `json:"task_id"`
	Standard     BenchmarkStandard `json:"standard"`
	TotalTrials  int           `json:"total_trials"` // N
	PassedTrials int           `json:"passed_trials"` // C
	PassAt1      float64       `json:"pass_at_1"`
	PassAt5      float64       `json:"pass_at_5"`
	PassAt10     float64       `json:"pass_at_10"`
	AvgTokens    int           `json:"avg_tokens"`
	AvgLatencyMs int64         `json:"avg_latency_ms"`
	Trials       []TrialResult `json:"trials"`
}

// HarnessConfig configures the evaluation harness execution parameters.
type HarnessConfig struct {
	Standard           BenchmarkStandard `json:"standard"`
	NumSamples         int               `json:"num_samples"`          // N sampled trajectories per task
	MaxConcurrency     int               `json:"max_concurrency"`      // Parallel workers
	StrictVerification bool              `json:"strict_verification"`  // Run actual test harness commands
	TimeoutPerTask     time.Duration     `json:"timeout_per_task"`
	MinPassRateAt1     float64           `json:"min_pass_rate_at_1"`   // Quality gate threshold (e.g. 0.40)
	SuiteName          string            `json:"suite_name"`
}

// DefaultHarnessConfig returns production defaults for SWE-bench & HumanEval evaluation.
func DefaultHarnessConfig() HarnessConfig {
	return HarnessConfig{
		Standard:           BenchmarkSWEBench,
		NumSamples:         5,
		MaxConcurrency:     4,
		StrictVerification: true,
		TimeoutPerTask:     5 * time.Minute,
		MinPassRateAt1:     0.50,
		SuiteName:          "SWE-bench Lite Autonomous Evaluation Suite",
	}
}

// BenchmarkSummaryReport summarizes multi-task pass@k benchmark results.
type BenchmarkSummaryReport struct {
	ReportID             string                   `json:"report_id"`
	SuiteName            string                   `json:"suite_name"`
	Standard             BenchmarkStandard        `json:"standard"`
	GeneratedAt          time.Time                `json:"generated_at"`
	TotalTasks           int                      `json:"total_tasks"`
	TotalTrialsRun       int                      `json:"total_trials_run"`
	ResolvedTasksCount   int                      `json:"resolved_tasks_count"`
	MeanPassAt1          float64                  `json:"mean_pass_at_1"`
	MeanPassAt5          float64                  `json:"mean_pass_at_5"`
	MeanPassAt10         float64                  `json:"mean_pass_at_10"`
	TotalTokensConsumed  int                      `json:"total_tokens_consumed"`
	TotalCostUSD         float64                  `json:"total_cost_usd"`
	AvgTaskDurationMs    int64                    `json:"avg_task_duration_ms"`
	GatePassed           bool                     `json:"gate_passed"`
	TaskAggregates       map[string]*TaskAggregate `json:"task_aggregates"`
}
