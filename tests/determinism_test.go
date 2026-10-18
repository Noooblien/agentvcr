package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/determinism"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func TestDeterminism_NominalExactReplay(t *testing.T) {
	cfg := determinism.DefaultConfig()
	engine := determinism.NewEngine(cfg)

	now := time.Now()
	baseline := &trace.Session{
		ID:        "base-101",
		AgentName: "code-agent",
		CreatedAt: now,
		Events: []trace.TraceEvent{
			{ID: "e1", Index: 1, Type: trace.EventLLMRequest, DurationMs: 100, Checksum: "c1", Payload: map[string]interface{}{"prompt": "solve task", "tokens": 100}},
			{ID: "e2", Index: 2, Type: trace.EventToolCall, DurationMs: 50, Checksum: "c2", Payload: map[string]interface{}{"tool_name": "git_status", "arguments": map[string]interface{}{"short": true}}},
			{ID: "e3", Index: 3, Type: trace.EventToolResult, DurationMs: 20, Checksum: "c3", Payload: map[string]interface{}{"tool_name": "git_status", "content": "clean"}},
			{ID: "e4", Index: 4, Type: trace.EventLLMResponse, DurationMs: 150, Checksum: "c4", Payload: map[string]interface{}{"content": "done", "tokens": 50}},
		},
	}

	target := &trace.Session{
		ID:        "target-101",
		AgentName: "code-agent",
		CreatedAt: now.Add(5 * time.Minute),
		Events: []trace.TraceEvent{
			{ID: "t1", Index: 1, Type: trace.EventLLMRequest, DurationMs: 95, Checksum: "c1", Payload: map[string]interface{}{"prompt": "solve task", "tokens": 100}},
			{ID: "t2", Index: 2, Type: trace.EventToolCall, DurationMs: 48, Checksum: "c2", Payload: map[string]interface{}{"tool_name": "git_status", "arguments": map[string]interface{}{"short": true}}},
			{ID: "t3", Index: 3, Type: trace.EventToolResult, DurationMs: 22, Checksum: "c3", Payload: map[string]interface{}{"tool_name": "git_status", "content": "clean"}},
			{ID: "t4", Index: 4, Type: trace.EventLLMResponse, DurationMs: 145, Checksum: "c4", Payload: map[string]interface{}{"content": "done", "tokens": 50}},
		},
	}

	report, err := engine.Evaluate(baseline, target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !report.Passed {
		t.Fatalf("expected report to pass, got failed status: %s", report.Status)
	}
	if report.DeterminismScore < 99.9 {
		t.Fatalf("expected 100%% score, got %.2f", report.DeterminismScore)
	}
	if report.Metrics.ToolParityPercent != 100.0 {
		t.Fatalf("expected 100%% tool parity, got %.1f%%", report.Metrics.ToolParityPercent)
	}
	if report.Metrics.ArgumentParityPercent != 100.0 {
		t.Fatalf("expected 100%% argument parity, got %.1f%%", report.Metrics.ArgumentParityPercent)
	}
	if report.Metrics.Tokens.DeltaTokens != 0 {
		t.Fatalf("expected 0 token delta, got %d", report.Metrics.Tokens.DeltaTokens)
	}
	if len(report.Divergences) != 0 {
		t.Fatalf("expected 0 divergences, got %d", len(report.Divergences))
	}
}

func TestDeterminism_ArgumentDrift(t *testing.T) {
	cfg := determinism.DefaultConfig()
	engine := determinism.NewEngine(cfg)

	baseline := &trace.Session{
		ID: "base-drift",
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventToolCall, Payload: map[string]interface{}{"tool_name": "search", "arguments": map[string]interface{}{"query": "golang"}}},
		},
	}
	target := &trace.Session{
		ID: "target-drift",
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventToolCall, Payload: map[string]interface{}{"tool_name": "search", "arguments": map[string]interface{}{"query": "rust"}}},
		},
	}

	report, err := engine.Evaluate(baseline, target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.Metrics.ToolParityPercent != 100.0 {
		t.Fatalf("expected tool parity 100%%, got %.1f%%", report.Metrics.ToolParityPercent)
	}
	if report.Metrics.ArgumentParityPercent != 0.0 {
		t.Fatalf("expected argument parity 0%%, got %.1f%%", report.Metrics.ArgumentParityPercent)
	}
	if len(report.Divergences) != 1 {
		t.Fatalf("expected 1 divergence event, got %d", len(report.Divergences))
	}
	if report.Divergences[0].Kind != determinism.DivergenceArgumentDrift {
		t.Fatalf("expected DivergenceArgumentDrift, got %s", report.Divergences[0].Kind)
	}
}

func TestDeterminism_ToolMismatch(t *testing.T) {
	cfg := determinism.DefaultConfig()
	engine := determinism.NewEngine(cfg)

	baseline := &trace.Session{
		ID: "base-mismatch",
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventToolCall, Payload: map[string]interface{}{"tool_name": "cat_file"}},
		},
	}
	target := &trace.Session{
		ID: "target-mismatch",
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventToolCall, Payload: map[string]interface{}{"tool_name": "rm_rf"}},
		},
	}

	report, err := engine.Evaluate(baseline, target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.Metrics.ToolParityPercent != 0.0 {
		t.Fatalf("expected tool parity 0%%, got %.1f%%", report.Metrics.ToolParityPercent)
	}
	if len(report.Divergences) != 1 {
		t.Fatalf("expected 1 divergence, got %d", len(report.Divergences))
	}
	if report.Divergences[0].Kind != determinism.DivergenceToolMismatch {
		t.Fatalf("expected DivergenceToolMismatch, got %s", report.Divergences[0].Kind)
	}
	if report.Divergences[0].Severity != determinism.SeverityCritical {
		t.Fatalf("expected SeverityCritical, got %s", report.Divergences[0].Severity)
	}
}

func TestDeterminism_MissingAndExtraSteps(t *testing.T) {
	cfg := determinism.DefaultConfig()
	engine := determinism.NewEngine(cfg)

	// Target is missing second step
	base1 := &trace.Session{
		ID: "base-1",
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventToolCall, Payload: map[string]interface{}{"tool_name": "tool_a"}},
			{Index: 2, Type: trace.EventToolCall, Payload: map[string]interface{}{"tool_name": "tool_b"}},
		},
	}
	target1 := &trace.Session{
		ID: "target-1",
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventToolCall, Payload: map[string]interface{}{"tool_name": "tool_a"}},
		},
	}

	rep1, err := engine.Evaluate(base1, target1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rep1.Divergences) != 1 || rep1.Divergences[0].Kind != determinism.DivergenceMissingStep {
		t.Fatalf("expected DivergenceMissingStep, got %+v", rep1.Divergences)
	}

	// Target has unexpected extra step
	rep2, err := engine.Evaluate(target1, base1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rep2.Divergences) != 1 || rep2.Divergences[0].Kind != determinism.DivergenceUnexpectedStep {
		t.Fatalf("expected DivergenceUnexpectedStep, got %+v", rep2.Divergences)
	}
}

func TestDeterminism_TokenExplosion(t *testing.T) {
	cfg := determinism.DefaultConfig()
	cfg.MaxTokenBloatRatio = 1.25
	engine := determinism.NewEngine(cfg)

	baseline := &trace.Session{
		ID: "base-tokens",
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventLLMRequest, Payload: map[string]interface{}{"tokens": 200}},
		},
	}
	target := &trace.Session{
		ID: "target-tokens",
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventLLMRequest, Payload: map[string]interface{}{"tokens": 600}},
		},
	}

	report, err := engine.Evaluate(baseline, target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.Metrics.Tokens.BloatRatio != 3.0 {
		t.Fatalf("expected bloat ratio 3.0x, got %.2fx", report.Metrics.Tokens.BloatRatio)
	}
	if report.Passed {
		t.Fatalf("expected report to fail due to 3.0x token bloat exceeding 1.25x max threshold")
	}
	hasTokenSurge := false
	for _, d := range report.Divergences {
		if d.Kind == determinism.DivergenceTokenSurge {
			hasTokenSurge = true
			break
		}
	}
	if !hasTokenSurge {
		t.Fatalf("expected DivergenceTokenSurge in divergences list")
	}
}

func TestDeterminism_LatencyPercentilesAndJitter(t *testing.T) {
	cfg := determinism.DefaultConfig()
	engine := determinism.NewEngine(cfg)

	baseline := &trace.Session{
		ID: "base-lat",
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventLLMRequest, DurationMs: 100},
			{Index: 2, Type: trace.EventLLMRequest, DurationMs: 200},
			{Index: 3, Type: trace.EventLLMRequest, DurationMs: 300},
			{Index: 4, Type: trace.EventLLMRequest, DurationMs: 400},
			{Index: 5, Type: trace.EventLLMRequest, DurationMs: 500},
		},
	}
	target := baseline

	report, err := engine.Evaluate(baseline, target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lat := report.Metrics.Latency
	if lat.P50Ms != 300.0 {
		t.Fatalf("expected P50 300.0 ms, got %.1f", lat.P50Ms)
	}
	if lat.MeanMs != 300.0 {
		t.Fatalf("expected Mean 300.0 ms, got %.1f", lat.MeanMs)
	}
	if lat.JitterMs <= 0.0 {
		t.Fatalf("expected positive JitterMs, got %.2f", lat.JitterMs)
	}
	if lat.ProxyOverheadUs <= 0 {
		t.Fatalf("expected positive ProxyOverheadUs, got %d", lat.ProxyOverheadUs)
	}
}

func TestDeterminism_StrictGateEnforcement(t *testing.T) {
	cfg := determinism.DefaultConfig()
	cfg.EnforceStrict = true
	cfg.MinScore = 50.0 // Set low min score
	engine := determinism.NewEngine(cfg)

	baseline := &trace.Session{
		ID: "base-strict",
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventToolCall, Payload: map[string]interface{}{"tool_name": "tool1"}},
		},
	}
	target := &trace.Session{
		ID: "target-strict",
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventToolCall, Payload: map[string]interface{}{"tool_name": "tool2"}},
		},
	}

	report, err := engine.Evaluate(baseline, target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.Passed {
		t.Fatalf("strict mode should have failed because of critical tool mismatch")
	}
}

func TestDeterminism_ScorecardAndMarkdownRendering(t *testing.T) {
	cfg := determinism.DefaultConfig()
	engine := determinism.NewEngine(cfg)

	scenarios := determinism.GenerateDemoScenarios()
	if len(scenarios) != 4 {
		t.Fatalf("expected 4 demo scenarios, got %d", len(scenarios))
	}

	rep, err := engine.Evaluate(scenarios[0].Baseline, scenarios[0].Target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ansiCard := determinism.RenderScorecard(rep)
	if !strings.Contains(ansiCard, "AgentVCR Trajectory Replay Determinism") {
		t.Fatalf("scorecard missing header: %s", ansiCard)
	}
	if !strings.Contains(ansiCard, "PASS / DETERMINISTIC") {
		t.Fatalf("scorecard missing pass badge: %s", ansiCard)
	}

	md := determinism.RenderMarkdown(rep)
	if !strings.Contains(md, "Tool Call Parity") {
		t.Fatalf("markdown missing Tool Call Parity: %s", md)
	}
	if !strings.Contains(md, "Proxy Interception Overhead") {
		t.Fatalf("markdown missing proxy overhead: %s", md)
	}

	jsonStr, err := determinism.RenderJSON(rep)
	if err != nil {
		t.Fatalf("failed to render JSON: %v", err)
	}
	if !strings.Contains(jsonStr, "\"determinism_score\"") {
		t.Fatalf("json missing determinism_score: %s", jsonStr)
	}
}
