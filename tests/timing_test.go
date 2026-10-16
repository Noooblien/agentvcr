package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/timing"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func TestTiming_ConstantTimeTrajectoryPasses(t *testing.T) {
	session := &trace.Session{
		ID:        "session_timing_clean",
		AgentName: "safe-agent",
		CreatedAt: time.Now().UTC(),
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventLLMRequest, DurationMs: 100, Payload: map[string]interface{}{"prompt": "hello world"}},
			{Index: 2, Type: trace.EventLLMResponse, DurationMs: 100, Payload: map[string]interface{}{"content": "ok"}},
		},
	}

	analyzer := timing.NewAnalyzer(4.0, false)
	report, _ := analyzer.AnalyzeSession(session)

	if !report.TimingLeakagePassed {
		t.Errorf("expected clean trajectory to pass timing analysis, got leak: %f", report.TotalInformationLeakBits)
	}
	if len(report.Vulnerabilities) != 0 {
		t.Errorf("expected 0 vulnerabilities, got %d", len(report.Vulnerabilities))
	}
}

func TestTiming_KVCacheFingerprintDetected(t *testing.T) {
	session := &trace.Session{
		ID:        "session_timing_leak",
		AgentName: "leaky-agent",
		CreatedAt: time.Now().UTC(),
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventLLMRequest, DurationMs: 200, Payload: map[string]interface{}{"prompt": "heavy computation"}},
			{Index: 2, Type: trace.EventLLMRequest, DurationMs: 200, Payload: map[string]interface{}{"prompt": "heavy computation 2"}},
			{
				Index:      3,
				Type:       trace.EventLLMRequest,
				DurationMs: 5, // Abnormally fast execution indicative of warm cross-tenant KV cache
				Payload: map[string]interface{}{
					"prompt":    "secret patient health record query",
					"cache_hit": true,
				},
			},
		},
	}

	analyzer := timing.NewAnalyzer(2.0, true)
	report, mitigated := analyzer.AnalyzeSession(session)

	if report.TimingLeakagePassed {
		t.Errorf("expected KV cache timing vulnerability to fail threshold")
	}
	if len(report.Vulnerabilities) == 0 {
		t.Fatalf("expected at least 1 vulnerability detected")
	}

	foundKVCache := false
	for _, v := range report.Vulnerabilities {
		if v.VulnerabilityType == "kv_cache_fingerprint" {
			foundKVCache = true
			break
		}
	}
	if !foundKVCache {
		t.Errorf("expected kv_cache_fingerprint vulnerability")
	}

	// Verify constant-time padding
	if mitigated == nil {
		t.Fatalf("expected mitigated session clone")
	}
	if mitigated.Events[2].DurationMs < 50 {
		t.Errorf("expected quantized duration >= 50ms, got: %d", mitigated.Events[2].DurationMs)
	}
}

func TestTiming_ScorecardRendering(t *testing.T) {
	session := &trace.Session{
		ID:        "session_timing_card",
		AgentName: "card-agent",
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventLLMRequest, DurationMs: 80},
		},
	}

	analyzer := timing.NewAnalyzer(4.0, false)
	report, _ := analyzer.AnalyzeSession(session)

	card := timing.RenderTimingScorecard(report)
	if !strings.Contains(card, "AGENTVCR PROMPT-LEAKAGE SIDE-CHANNEL") {
		t.Errorf("expected title in scorecard")
	}
	if !strings.Contains(card, "session_timing_card") {
		t.Errorf("expected session ID in scorecard")
	}
}
