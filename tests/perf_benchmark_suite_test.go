package tests

import (
	"strings"
	"testing"

	"github.com/Noooblien/agentvcr/pkg/perf"
)

func TestIntegration_PerfBenchmark_SuiteExecution(t *testing.T) {
	cfg := perf.DefaultBenchmarkConfig()
	cfg.Target = perf.TargetStorage
	cfg.Iterations = 30
	cfg.WarmupIterations = 5

	report, err := perf.RunBenchmark(cfg)
	if err != nil {
		t.Fatalf("failed to run performance benchmark: %v", err)
	}

	scorecard := perf.RenderPerformanceScorecard(report)
	if !strings.Contains(scorecard, "LATENCY & PERFORMANCE SCORECARD") {
		t.Errorf("scorecard title not found")
	}

	mermaid := perf.RenderPerformanceMermaid(report)
	if !strings.Contains(mermaid, "flowchart TD") {
		t.Errorf("mermaid flowchart not found")
	}

	md := perf.RenderPerformanceMarkdown(report)
	if !strings.Contains(md, "## AgentVCR Sub-Millisecond Latency & Micro-Benchmark Audit Report") {
		t.Errorf("markdown report not found")
	}
}
