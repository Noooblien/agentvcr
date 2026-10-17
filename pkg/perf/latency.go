package perf

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"

	"github.com/Noooblien/agentvcr/pkg/compact"
	"github.com/Noooblien/agentvcr/pkg/mcp"
	"github.com/Noooblien/agentvcr/pkg/snapshot"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

// CalculatePercentiles computes statistical percentile distributions and throughput from duration samples.
func CalculatePercentiles(durations []time.Duration, totalDuration time.Duration, allocs uint64, bytes uint64) LatencyPercentiles {
	n := len(durations)
	if n == 0 {
		return LatencyPercentiles{}
	}

	usValues := make([]float64, n)
	var sum float64
	for i, d := range durations {
		us := float64(d.Nanoseconds()) / 1000.0
		usValues[i] = us
		sum += us
	}

	sort.Float64s(usValues)

	mean := sum / float64(n)
	var varianceSum float64
	for _, v := range usValues {
		diff := v - mean
		varianceSum += diff * diff
	}
	stdDev := math.Sqrt(varianceSum / float64(n))

	getPercentile := func(p float64) float64 {
		idx := int(math.Ceil((p/100.0)*float64(n))) - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= n {
			idx = n - 1
		}
		return usValues[idx]
	}

	totalMs := float64(totalDuration.Milliseconds())
	if totalMs <= 0 {
		totalMs = float64(totalDuration.Nanoseconds()) / 1_000_000.0
	}
	var opsPerSec float64
	if totalDuration > 0 {
		opsPerSec = float64(n) / totalDuration.Seconds()
	}

	var allocsPerOp, bytesPerOp uint64
	if n > 0 {
		allocsPerOp = allocs / uint64(n)
		bytesPerOp = bytes / uint64(n)
	}

	return LatencyPercentiles{
		MinUs:       usValues[0],
		MaxUs:       usValues[n-1],
		MeanUs:      mean,
		P50Us:       getPercentile(50.0),
		P90Us:       getPercentile(90.0),
		P95Us:       getPercentile(95.0),
		P99Us:       getPercentile(99.0),
		P999Us:      getPercentile(99.9),
		StdDevUs:    stdDev,
		TotalOps:    n,
		DurationMs:  totalMs,
		OpsPerSec:   opsPerSec,
		AllocsPerOp: allocsPerOp,
		BytesPerOp:  bytesPerOp,
	}
}

// BenchmarkProxyIngestion profiles telemetry trace event creation, SHA-256 payload checksumming, and chaining.
func BenchmarkProxyIngestion(iterations, warmup int) (LatencyPercentiles, error) {
	samplePayload := map[string]interface{}{
		"model": "gpt-4o",
		"messages": []interface{}{
			map[string]interface{}{"role": "system", "content": "You are an autonomous engineering agent."},
			map[string]interface{}{"role": "user", "content": "Refactor the token compactor and run unit tests."},
		},
		"temperature": 0.2,
	}

	sampleBytes, _ := json.Marshal(samplePayload)
	for i := 0; i < warmup; i++ {
		_ = trace.ComputeChecksum("genesis", i, trace.EventLLMRequest, sampleBytes)
	}

	var m1, m2 runtime.MemStats
	runtime.ReadMemStats(&m1)

	durations := make([]time.Duration, iterations)
	start := time.Now()

	for i := 0; i < iterations; i++ {
		t0 := time.Now()
		_ = trace.ComputeChecksum("genesis", i, trace.EventLLMRequest, sampleBytes)
		durations[i] = time.Since(t0)
	}

	totalDur := time.Since(start)
	runtime.ReadMemStats(&m2)

	allocs := m2.Mallocs - m1.Mallocs
	bytes := m2.TotalAlloc - m1.TotalAlloc

	return CalculatePercentiles(durations, totalDur, allocs, bytes), nil
}

// BenchmarkMCPInterception profiles JSON-RPC 2.0 frame parsing, argument hashing, and validation.
func BenchmarkMCPInterception(iterations, warmup int) (LatencyPercentiles, error) {
	rawArgs := map[string]interface{}{
		"command": "git status",
		"cwd":     "/home/user/workspace",
		"timeout": float64(30),
		"nested": map[string]interface{}{
			"env":    "production",
			"strict": true,
		},
	}

	validator := mcp.NewValidator()
	schema := mcp.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"command": map[string]interface{}{"type": "string"},
			"cwd":     map[string]interface{}{"type": "string"},
		},
		Required: []string{"command"},
	}

	for i := 0; i < warmup; i++ {
		_ = mcp.HashArguments(rawArgs)
		_ = validator.ValidateArguments(schema, rawArgs)
	}

	var m1, m2 runtime.MemStats
	runtime.ReadMemStats(&m1)

	durations := make([]time.Duration, iterations)
	start := time.Now()

	for i := 0; i < iterations; i++ {
		t0 := time.Now()
		_ = mcp.HashArguments(rawArgs)
		_ = validator.ValidateArguments(schema, rawArgs)
		durations[i] = time.Since(t0)
	}

	totalDur := time.Since(start)
	runtime.ReadMemStats(&m2)

	allocs := m2.Mallocs - m1.Mallocs
	bytes := m2.TotalAlloc - m1.TotalAlloc

	return CalculatePercentiles(durations, totalDur, allocs, bytes), nil
}

// BenchmarkContextCompaction profiles lossless context window deduplication and token waste pruning.
func BenchmarkContextCompaction(iterations, warmup int) (LatencyPercentiles, error) {
	cfg := compact.DefaultCompactorConfig()
	comp := compact.NewCompactor(cfg)

	now := time.Now()
	makeSession := func() *trace.Session {
		return &trace.Session{
			ID:        "bench-compact-session",
			AgentName: "benchmark-agent",
			Events: []trace.TraceEvent{
				{Index: 1, Timestamp: now, Type: trace.EventToolCall, Payload: map[string]interface{}{"tool_name": "list_files", "dir": "pkg"}},
				{Index: 2, Timestamp: now.Add(time.Millisecond), Type: trace.EventToolResult, Payload: map[string]interface{}{"output": "types.go\nengine.go\n"}},
				{Index: 3, Timestamp: now.Add(2 * time.Millisecond), Type: trace.EventToolCall, Payload: map[string]interface{}{"tool_name": "list_files", "dir": "pkg"}},
				{Index: 4, Timestamp: now.Add(3 * time.Millisecond), Type: trace.EventToolResult, Payload: map[string]interface{}{"output": "types.go\nengine.go\n"}},
				{Index: 5, Timestamp: now.Add(4 * time.Millisecond), Type: trace.EventLLMResponse, Payload: map[string]interface{}{"content": "All files inspected."}},
			},
		}
	}

	for i := 0; i < warmup; i++ {
		s := makeSession()
		_, _, _ = comp.CompactSession(s, "compacted-warmup")
	}

	var m1, m2 runtime.MemStats
	runtime.ReadMemStats(&m1)

	durations := make([]time.Duration, iterations)
	start := time.Now()

	for i := 0; i < iterations; i++ {
		s := makeSession()
		t0 := time.Now()
		_, _, _ = comp.CompactSession(s, fmt.Sprintf("compacted-%d", i))
		durations[i] = time.Since(t0)
	}

	totalDur := time.Since(start)
	runtime.ReadMemStats(&m2)

	allocs := m2.Mallocs - m1.Mallocs
	bytes := m2.TotalAlloc - m1.TotalAlloc

	return CalculatePercentiles(durations, totalDur, allocs, bytes), nil
}

// BenchmarkCoWBranching profiles ephemeral copy-on-write workspace branching and microsecond rollbacks.
func BenchmarkCoWBranching(iterations, warmup int) (LatencyPercentiles, error) {
	tempBase, err := os.MkdirTemp("", "agentvcr-cow-bench-base-*")
	if err != nil {
		return LatencyPercentiles{}, err
	}
	defer os.RemoveAll(tempBase)

	tempWork, err := os.MkdirTemp("", "agentvcr-cow-bench-work-*")
	if err != nil {
		return LatencyPercentiles{}, err
	}
	defer os.RemoveAll(tempWork)

	_ = os.WriteFile(filepath.Join(tempBase, "config.json"), []byte(`{"version": "1.0", "active": true}`), 0644)
	_ = os.WriteFile(filepath.Join(tempBase, "main.go"), []byte(`package main; func main(){}`), 0644)

	cow, err := snapshot.NewCoWFilesystem("bench-session", tempBase, tempWork)
	if err != nil {
		return LatencyPercentiles{}, err
	}

	for i := 0; i < warmup; i++ {
		branch, err := cow.Branch(fmt.Sprintf("warmup-branch-%d", i))
		if err == nil {
			_ = branch.RollbackTo("layer_upper_0")
		}
	}

	var m1, m2 runtime.MemStats
	runtime.ReadMemStats(&m1)

	durations := make([]time.Duration, iterations)
	start := time.Now()

	for i := 0; i < iterations; i++ {
		t0 := time.Now()
		branch, err := cow.Branch(fmt.Sprintf("bench-branch-%d", i))
		if err == nil {
			_ = branch.RollbackTo("layer_upper_0")
		}
		durations[i] = time.Since(t0)
	}

	totalDur := time.Since(start)
	runtime.ReadMemStats(&m2)

	allocs := m2.Mallocs - m1.Mallocs
	bytes := m2.TotalAlloc - m1.TotalAlloc

	return CalculatePercentiles(durations, totalDur, allocs, bytes), nil
}

// BenchmarkStorageSerialization profiles JSON trajectory serialization and deserialization speeds.
func BenchmarkStorageSerialization(iterations, warmup int) (LatencyPercentiles, error) {
	now := time.Now()
	sess := &trace.Session{
		ID:        "storage-bench-session",
		AgentName: "storage-agent",
		Events: []trace.TraceEvent{
			{Index: 1, Timestamp: now, Type: trace.EventLLMRequest, Payload: map[string]interface{}{"prompt": "Inspect repo"}},
			{Index: 2, Timestamp: now.Add(time.Millisecond), Type: trace.EventLLMResponse, Payload: map[string]interface{}{"content": "Executing tests"}},
			{Index: 3, Timestamp: now.Add(2 * time.Millisecond), Type: trace.EventToolCall, Payload: map[string]interface{}{"tool": "go test"}},
		},
	}

	data, err := json.Marshal(sess)
	if err != nil {
		return LatencyPercentiles{}, err
	}

	for i := 0; i < warmup; i++ {
		var decoded trace.Session
		_ = json.Unmarshal(data, &decoded)
		_, _ = json.Marshal(&decoded)
	}

	var m1, m2 runtime.MemStats
	runtime.ReadMemStats(&m1)

	durations := make([]time.Duration, iterations)
	start := time.Now()

	for i := 0; i < iterations; i++ {
		t0 := time.Now()
		var decoded trace.Session
		_ = json.Unmarshal(data, &decoded)
		_, _ = json.Marshal(&decoded)
		durations[i] = time.Since(t0)
	}

	totalDur := time.Since(start)
	runtime.ReadMemStats(&m2)

	allocs := m2.Mallocs - m1.Mallocs
	bytes := m2.TotalAlloc - m1.TotalAlloc

	return CalculatePercentiles(durations, totalDur, allocs, bytes), nil
}

// RunBenchmark executes targeted benchmarks based on config and compiles a comprehensive SuiteReport.
func RunBenchmark(cfg BenchmarkConfig) (*SuiteReport, error) {
	report := NewSuiteReport()
	t0 := time.Now()

	benchmarks := []struct {
		target   BenchmarkTarget
		name     string
		desc     string
		slaUs    float64
		run      func(int, int) (LatencyPercentiles, error)
	}{
		{
			target: TargetProxy,
			name:   "Proxy Event Ingestion & Checksumming",
			desc:   "In-flight telemetry event creation and SHA-256 integrity hashing",
			slaUs:  cfg.MaxP99Us,
			run:    BenchmarkProxyIngestion,
		},
		{
			target: TargetMCP,
			name:   "MCP Stdio/SSE Frame Interception",
			desc:   "JSON-RPC 2.0 argument digestion and schema validation",
			slaUs:  cfg.MaxP99Us,
			run:    BenchmarkMCPInterception,
		},
		{
			target: TargetCompact,
			name:   "Context Window Deduplication & Compaction",
			desc:   "Lossless token payload deduplication and prompt cache alignment",
			slaUs:  cfg.MaxP99Us * 1.5,
			run:    BenchmarkContextCompaction,
		},
		{
			target: TargetCoW,
			name:   "CoW Filesystem Instant Branch & Rollback",
			desc:   "O(1) isolated branch allocation and delta layer reset",
			slaUs:  cfg.MaxP99Us * 2.0,
			run:    BenchmarkCoWBranching,
		},
		{
			target: TargetStorage,
			name:   "Storage JSON Serialization & Merkle Integrity",
			desc:   "Fast trajectory persistence round-trip and decoding",
			slaUs:  cfg.MaxP99Us,
			run:    BenchmarkStorageSerialization,
		},
	}

	for _, b := range benchmarks {
		if cfg.Target != TargetAll && cfg.Target != b.target {
			continue
		}

		pct, err := b.run(cfg.Iterations, cfg.WarmupIterations)
		if err != nil {
			return nil, fmt.Errorf("benchmark %s failed: %w", b.name, err)
		}

		passesSLA := pct.P99Us <= b.slaUs
		if !passesSLA {
			report.OverallPassed = false
		}

		res := BenchmarkResult{
			Target:         b.target,
			Name:           b.name,
			Concurrency:    cfg.Concurrency,
			Iterations:     cfg.Iterations,
			Percentiles:    pct,
			SLAThresholdUs: b.slaUs,
			PassesSLA:      passesSLA,
			Description:    b.desc,
		}
		report.Results = append(report.Results, res)
	}

	report.TotalDurationMs = float64(time.Since(t0).Milliseconds())
	return report, nil
}
