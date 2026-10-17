package perf

import (
	"runtime"
	"time"
)

// BenchmarkTarget identifies which subsystem of AgentVCR to profile.
type BenchmarkTarget string

const (
	TargetAll      BenchmarkTarget = "all"
	TargetProxy    BenchmarkTarget = "proxy"
	TargetMCP      BenchmarkTarget = "mcp"
	TargetCoW      BenchmarkTarget = "cow"
	TargetCompact  BenchmarkTarget = "compact"
	TargetStorage  BenchmarkTarget = "storage"
	TargetStress   BenchmarkTarget = "stress"
)

// LatencyPercentiles captures statistical distribution metrics of execution timings.
type LatencyPercentiles struct {
	MinUs        float64 `json:"min_us"`
	MaxUs        float64 `json:"max_us"`
	MeanUs       float64 `json:"mean_us"`
	P50Us        float64 `json:"p50_us"`
	P90Us        float64 `json:"p90_us"`
	P95Us        float64 `json:"p95_us"`
	P99Us        float64 `json:"p99_us"`
	P999Us       float64 `json:"p999_us"`
	StdDevUs     float64 `json:"stddev_us"`
	TotalOps     int     `json:"total_ops"`
	DurationMs   float64 `json:"duration_ms"`
	OpsPerSec    float64 `json:"ops_per_sec"`
	AllocsPerOp  uint64  `json:"allocs_per_op"`
	BytesPerOp   uint64  `json:"bytes_per_op"`
}

// BenchmarkResult records the performance outcome for a single targeted benchmark.
type BenchmarkResult struct {
	Target          BenchmarkTarget    `json:"target"`
	Name            string             `json:"name"`
	Concurrency     int                `json:"concurrency"`
	Iterations      int                `json:"iterations"`
	Percentiles     LatencyPercentiles `json:"percentiles"`
	SLAThresholdUs  float64            `json:"sla_threshold_us"`
	PassesSLA       bool               `json:"passes_sla"`
	ErrorCount      int                `json:"error_count"`
	Description     string             `json:"description"`
}

// SuiteReport contains aggregated performance benchmark results across all tested targets.
type SuiteReport struct {
	Timestamp       time.Time         `json:"timestamp"`
	GoVersion       string            `json:"go_version"`
	NumCPU          int               `json:"num_cpu"`
	Results         []BenchmarkResult `json:"results"`
	OverallPassed   bool              `json:"overall_passed"`
	TotalDurationMs float64           `json:"total_duration_ms"`
}

// BenchmarkConfig defines tuning knobs and thresholds for running benchmarks.
type BenchmarkConfig struct {
	Target            BenchmarkTarget `json:"target"`
	Concurrency       int             `json:"concurrency"`
	Iterations        int             `json:"iterations"`
	WarmupIterations  int             `json:"warmup_iterations"`
	MaxP99Us          float64         `json:"max_p99_us"`
	Strict            bool            `json:"strict"`
}

// DefaultBenchmarkConfig returns production defaults for microsecond profiling.
func DefaultBenchmarkConfig() BenchmarkConfig {
	return BenchmarkConfig{
		Target:           TargetAll,
		Concurrency:      4,
		Iterations:       500,
		WarmupIterations: 20,
		MaxP99Us:         1000.0, // Default SLA: P99 < 1000 µs (1 ms)
		Strict:           false,
	}
}

// NewSuiteReport initializes a performance suite report with hardware and runtime environment info.
func NewSuiteReport() *SuiteReport {
	return &SuiteReport{
		Timestamp:     time.Now().UTC(),
		GoVersion:     runtime.Version(),
		NumCPU:        runtime.NumCPU(),
		Results:       make([]BenchmarkResult, 0),
		OverallPassed: true,
	}
}
