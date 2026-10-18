package chaos

import (
	"testing"
)

func TestChaosEngine_NominalSession(t *testing.T) {
	session := generateBaseSession("agent-test", 5)
	engine := NewChaosEngine(42)

	cfg := ChaosExperimentConfig{
		ExperimentName: "Nominal Test",
		Rules:          []FaultRule{},
		StrictMode:     true,
	}

	report, err := engine.EvaluateSession(session, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.TotalFaultsInjected != 0 {
		t.Fatalf("expected 0 faults injected, got %d", report.TotalFaultsInjected)
	}
	if report.ResilienceScore != 100.0 {
		t.Fatalf("expected 100.0 resilience score, got %.1f", report.ResilienceScore)
	}
	if report.Status != StatusResilient {
		t.Fatalf("expected RESILIENT status, got %s", report.Status)
	}
}

func TestChaosEngine_FaultInjection_RateLimit(t *testing.T) {
	session := generateBaseSession("agent-test", 5)
	engine := NewChaosEngine(42)

	cfg := ChaosExperimentConfig{
		ExperimentName: "Rate Limit Test",
		Rules: []FaultRule{
			{
				ID:             "r-429",
				TargetTool:     "web_search",
				Type:           FaultRateLimit,
				Probability:    1.0,
				MaxOccurrences: 1,
			},
		},
		StrictMode: false,
	}

	report, err := engine.EvaluateSession(session, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.TotalFaultsInjected != 1 {
		t.Fatalf("expected 1 fault injected, got %d", report.TotalFaultsInjected)
	}
	if report.TotalFaultsRecovered != 1 {
		t.Fatalf("expected 1 fault recovered, got %d", report.TotalFaultsRecovered)
	}
	if report.RecoveryRatePct != 100.0 {
		t.Fatalf("expected 100%% recovery rate, got %.1f", report.RecoveryRatePct)
	}
	if report.TokenBlastRadiusTokens <= 0 {
		t.Fatalf("expected positive blast radius tokens, got %d", report.TokenBlastRadiusTokens)
	}
}

func TestChaosEngine_NilSession(t *testing.T) {
	engine := NewChaosEngine(42)
	_, err := engine.EvaluateSession(nil, ChaosExperimentConfig{})
	if err == nil {
		t.Fatalf("expected error on nil session")
	}
}

func TestChaosEngine_StepTargeting(t *testing.T) {
	session := generateBaseSession("agent-test", 6)
	engine := NewChaosEngine(42)

	cfg := ChaosExperimentConfig{
		ExperimentName: "Step Targeting Test",
		Rules: []FaultRule{
			{
				ID:          "step-3-hang",
				TargetStep:  3, // Step index 3
				Type:        FaultToolTimeout,
				Probability: 1.0,
			},
		},
	}

	report, err := engine.EvaluateSession(session, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.TotalFaultsInjected != 1 {
		t.Fatalf("expected 1 fault on targeted step, got %d", report.TotalFaultsInjected)
	}
	if report.Events[0].StepIndex != 3 {
		t.Fatalf("expected fault injected at step 3, got step %d", report.Events[0].StepIndex)
	}
}

func TestChaosEngine_OutageAndCriticalFailure(t *testing.T) {
	session := generateBaseSession("agent-test", 4)
	engine := NewChaosEngine(42)

	cfg := ChaosExperimentConfig{
		ExperimentName: "Severe Outage",
		Rules: []FaultRule{
			{
				ID:          "outage",
				Type:        FaultUpstreamOutage,
				Probability: 1.0,
			},
			{
				ID:          "state-err",
				Type:        FaultStateCorruption,
				Probability: 1.0,
			},
		},
		StrictMode: true,
	}

	report, err := engine.EvaluateSession(session, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.Passed {
		t.Fatalf("expected severe outage to fail in strict mode")
	}
	if report.Status != StatusCriticalFailure && report.Status != StatusDegraded {
		t.Fatalf("expected DEGRADED or CRITICAL_FAILURE status, got %s", report.Status)
	}
}
