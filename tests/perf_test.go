package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/perf"
)

func TestCalculatePercentiles(t *testing.T) {
	durations := []time.Duration{
		10 * time.Microsecond,
		20 * time.Microsecond,
		30 * time.Microsecond,
		40 * time.Microsecond,
		50 * time.Microsecond,
		60 * time.Microsecond,
		70 * time.Microsecond,
		80 * time.Microsecond,
		90 * time.Microsecond,
		100 * time.Microsecond,
	}

	totalDur := 550 * time.Microsecond
	pct := perf.CalculatePercentiles(durations, totalDur, 20, 1024)

	if pct.TotalOps != 10 {
		t.Fatalf("expected 10 ops, got %d", pct.TotalOps)
	}
	if pct.MinUs != 10.0 {
		t.Errorf("expected MinUs=10.0, got %f", pct.MinUs)
	}
	if pct.MaxUs != 100.0 {
		t.Errorf("expected MaxUs=100.0, got %f", pct.MaxUs)
	}
	if pct.P50Us < 45.0 || pct.P50Us > 55.0 {
		t.Errorf("expected P50Us ~50.0, got %f", pct.P50Us)
	}
	if pct.P90Us < 85.0 || pct.P90Us > 95.0 {
		t.Errorf("expected P90Us ~90.0, got %f", pct.P90Us)
	}
	if pct.AllocsPerOp != 2 {
		t.Errorf("expected AllocsPerOp=2, got %d", pct.AllocsPerOp)
	}
}

func TestSubsystemBenchmarks(t *testing.T) {
	// 1. Proxy Ingestion
	proxyPct, err := perf.BenchmarkProxyIngestion(50, 5)
	if err != nil {
		t.Fatalf("BenchmarkProxyIngestion failed: %v", err)
	}
	if proxyPct.TotalOps != 50 || proxyPct.P50Us <= 0 {
		t.Errorf("unexpected proxy benchmark result: %+v", proxyPct)
	}

	// 2. MCP Interception
	mcpPct, err := perf.BenchmarkMCPInterception(50, 5)
	if err != nil {
		t.Fatalf("BenchmarkMCPInterception failed: %v", err)
	}
	if mcpPct.TotalOps != 50 || mcpPct.P50Us <= 0 {
		t.Errorf("unexpected mcp benchmark result: %+v", mcpPct)
	}

	// 3. Context Compaction
	compactPct, err := perf.BenchmarkContextCompaction(20, 2)
	if err != nil {
		t.Fatalf("BenchmarkContextCompaction failed: %v", err)
	}
	if compactPct.TotalOps != 20 || compactPct.P50Us <= 0 {
		t.Errorf("unexpected compact benchmark result: %+v", compactPct)
	}

	// 4. CoW Branching
	cowPct, err := perf.BenchmarkCoWBranching(20, 2)
	if err != nil {
		t.Fatalf("BenchmarkCoWBranching failed: %v", err)
	}
	if cowPct.TotalOps != 20 || cowPct.P50Us <= 0 {
		t.Errorf("unexpected cow benchmark result: %+v", cowPct)
	}

	// 5. Storage Serialization
	storagePct, err := perf.BenchmarkStorageSerialization(50, 5)
	if err != nil {
		t.Fatalf("BenchmarkStorageSerialization failed: %v", err)
	}
	if storagePct.TotalOps != 50 || storagePct.P50Us <= 0 {
		t.Errorf("unexpected storage benchmark result: %+v", storagePct)
	}
}

func TestStressRunner(t *testing.T) {
	runner := perf.NewStressRunner(4, 40)
	res, err := runner.RunConcurrentFlightRecorder()
	if err != nil {
		t.Fatalf("RunConcurrentFlightRecorder failed: %v", err)
	}

	if res.TotalOperations != 40 {
		t.Errorf("expected 40 total operations, got %d", res.TotalOperations)
	}
	if res.ErrorCount != 0 {
		t.Errorf("expected 0 errors, got %d", res.ErrorCount)
	}
	if res.ThroughputOpsSec <= 0 {
		t.Errorf("expected positive throughput, got %f", res.ThroughputOpsSec)
	}
}

func TestRunBenchmarkSuite(t *testing.T) {
	cfg := perf.BenchmarkConfig{
		Target:           perf.TargetAll,
		Concurrency:      2,
		Iterations:       30,
		WarmupIterations: 5,
		MaxP99Us:         50_000.0, // High SLA for testing environment
		Strict:           false,
	}

	report, err := perf.RunBenchmark(cfg)
	if err != nil {
		t.Fatalf("RunBenchmark failed: %v", err)
	}

	if len(report.Results) < 5 {
		t.Fatalf("expected at least 5 benchmark results, got %d", len(report.Results))
	}

	card := perf.RenderPerformanceScorecard(report)
	if !strings.Contains(card, "AGENTVCR SUB-MILLISECOND LATENCY & PERFORMANCE SCORECARD") {
		t.Errorf("scorecard missing header: %s", card)
	}
	if !strings.Contains(card, "PASS") {
		t.Errorf("scorecard missing PASS status: %s", card)
	}
}
