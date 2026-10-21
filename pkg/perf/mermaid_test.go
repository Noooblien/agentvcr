package perf

import (
	"strings"
	"testing"
)

func TestPerformance_MermaidAndMarkdown(t *testing.T) {
	cfg := DefaultBenchmarkConfig()
	cfg.Target = TargetStorage
	cfg.Iterations = 50
	cfg.WarmupIterations = 5

	report, err := RunBenchmark(cfg)
	if err != nil {
		t.Fatalf("benchmark failed: %v", err)
	}

	mermaid := RenderPerformanceMermaid(report)
	if !strings.Contains(mermaid, "flowchart TD") {
		t.Errorf("mermaid missing flowchart TD header")
	}

	md := RenderPerformanceMarkdown(report)
	if !strings.Contains(md, "## AgentVCR Sub-Millisecond Latency") {
		t.Errorf("markdown report missing header")
	}
}
