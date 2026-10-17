package tests

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/finops"
)

func TestFinOps_PricingCalculator(t *testing.T) {
	calc := finops.NewPricingCalculator()

	// 1,000 uncached input tokens on gpt-4o ($0.005) + 1,000 output tokens ($0.015) = $0.0200
	cost, err := calc.CalculateCost("gpt-4o", 1000, 1000, 0)
	if err != nil {
		t.Fatalf("Cost calculation failed: %v", err)
	}
	expected := 0.0200
	if cost < expected-0.0001 || cost > expected+0.0001 {
		t.Fatalf("Expected $%.4f, got $%.4f", expected, cost)
	}

	// Cached token discount: 1,000 cached tokens on gpt-4o ($0.0025)
	costCached, _ := calc.CalculateCost("gpt-4o", 1000, 0, 1000)
	if costCached < 0.0024 || costCached > 0.0026 {
		t.Fatalf("Expected cached input cost $0.0025, got $%.4f", costCached)
	}

	// Model downgrade savings: gpt-4o ($0.020) vs gpt-4o-mini ($0.00075)
	savings := calc.CalculateSavings("gpt-4o", "gpt-4o-mini", 1000, 1000)
	if savings < 0.015 {
		t.Fatalf("Expected significant downgrade savings (> $0.015), got $%.4f", savings)
	}
}

func TestFinOps_PreFlight_AllowanceAndWarnings(t *testing.T) {
	policy := finops.BudgetPolicy{
		PolicyID:             "policy-test-warn",
		AgentID:              "agent-test",
		MaxCostUSD:           1.00,
		WarnThresholdPct:     0.70,
		ThrottleThresholdPct: 0.90,
		DowngradeModel:       "gpt-4o-mini",
		EnforceHardLimit:     true,
	}
	tracker := finops.NewAgentFinOpsTracker(policy, finops.NewPricingCalculator())
	now := time.Now()

	// Phase 1: Under 70% threshold -> ALLOW
	allowed, action, model, err := tracker.PreFlightCheck("gpt-4o", now)
	if !allowed || action != finops.ActionAllow || err != nil {
		t.Fatalf("Expected ActionAllow, got %s (err: %v)", action, err)
	}
	if model != "gpt-4o" {
		t.Fatalf("Expected model gpt-4o, got %s", model)
	}

	// Burn $0.75 of budget
	_, _ = tracker.RecordConsumption("gpt-4o", "gpt-4o-mini", 5000000, 0, 0, now) // Large burn to cross 70%

	// Phase 2: Crossed 75% -> WARN or THROTTLE/DOWNGRADE
	allowed2, action2, _, _ := tracker.PreFlightCheck("gpt-4o", now)
	if !allowed2 {
		t.Fatalf("Request should still be permitted under warning/throttle threshold")
	}
	if action2 != finops.ActionWarn && action2 != finops.ActionDowngrade && action2 != finops.ActionThrottle {
		t.Fatalf("Expected warning or downgrade action, got %s", action2)
	}
}

func TestFinOps_AutomaticDowngrade_Enforcement(t *testing.T) {
	policy := finops.BudgetPolicy{
		PolicyID:             "policy-downgrade",
		AgentID:              "agent-downgrade",
		MaxCostUSD:           0.10, // $0.10 budget
		WarnThresholdPct:     0.50,
		ThrottleThresholdPct: 0.70, // 70% threshold triggers downgrade
		DowngradeModel:       "gpt-4o-mini",
		EnforceHardLimit:     true,
	}
	tracker := finops.NewAgentFinOpsTracker(policy, finops.NewPricingCalculator())
	now := time.Now()

	// Ingest $0.08 of spend (80% utilization)
	_, _ = tracker.RecordConsumption("gpt-4o", "gpt-4o", 4000, 4000, 0, now)

	// Pre-flight check should trigger model downgrade to gpt-4o-mini
	allowed, action, targetModel, err := tracker.PreFlightCheck("gpt-4o", now)
	if !allowed || err != nil {
		t.Fatalf("Expected execution allowed with downgrade, err: %v", err)
	}
	if action != finops.ActionDowngrade {
		t.Fatalf("Expected ActionDowngrade, got: %s", action)
	}
	if targetModel != "gpt-4o-mini" {
		t.Fatalf("Expected targetModel 'gpt-4o-mini', got: %s", targetModel)
	}
}

func TestFinOps_RunawayCircuitBreaker_HardKill(t *testing.T) {
	policy := finops.BudgetPolicy{
		PolicyID:         "policy-kill",
		AgentID:          "agent-loop",
		MaxCostUSD:       0.05, // Tight $0.05 limit
		EnforceHardLimit: true,
	}
	tracker := finops.NewAgentFinOpsTracker(policy, finops.NewPricingCalculator())
	now := time.Now()

	// Burn $0.06 (exceeding $0.05 hard ceiling)
	_, _ = tracker.RecordConsumption("o1", "o1", 1000, 1000, 0, now)

	// Next preflight must HARD_KILL and reject execution
	allowed, action, _, err := tracker.PreFlightCheck("o1", now)
	if allowed {
		t.Fatalf("Expected execution to be BLOCKED after hard ceiling exceeded")
	}
	if action != finops.ActionHardKill {
		t.Fatalf("Expected ActionHardKill, got: %s", action)
	}
	if err == nil || !strings.Contains(err.Error(), "finops circuit tripped") {
		t.Fatalf("Expected circuit tripped error, got: %v", err)
	}

	// Verify reset
	tracker.ResetCircuit(0.50) // Expand budget to $0.50
	allowedAfterReset, _, _, errReset := tracker.PreFlightCheck("o1", now)
	if !allowedAfterReset || errReset != nil {
		t.Fatalf("Expected execution permitted after circuit reset, err: %v", errReset)
	}
}

func TestFinOps_Manager_ExecuteTurn_Interceptor(t *testing.T) {
	manager := finops.NewFinOpsManager("test-finops-mgr")
	now := time.Now()

	manager.RegisterAgent(finops.BudgetPolicy{
		PolicyID:         "policy-mgr-test",
		AgentID:          "agent-worker",
		MaxCostUSD:       1.00,
		EnforceHardLimit: true,
	})

	ctx := context.Background()
	invokedModel := ""
	mockInvoker := func(ctx context.Context, model string, promptTokens, completionTokens int) (int, int, error) {
		invokedModel = model
		return 500, 250, nil
	}

	action, model, cost, err := manager.ExecuteTurn(ctx, "agent-worker", "gpt-4o", 500, now, mockInvoker)
	if err != nil {
		t.Fatalf("ExecuteTurn failed: %v", err)
	}
	if action != finops.ActionAllow {
		t.Fatalf("Expected ActionAllow, got: %s", action)
	}
	if invokedModel != "gpt-4o" || model != "gpt-4o" {
		t.Fatalf("Expected gpt-4o invoked, got %s", invokedModel)
	}
	if cost <= 0 {
		t.Fatalf("Expected positive cost recorded, got: $%.6f", cost)
	}

	report := manager.Audit(now)
	if report.TotalCostUSD <= 0 {
		t.Fatalf("Expected non-zero portfolio spend in audit report")
	}
	if len(report.AgentsTelemetry) != 1 {
		t.Fatalf("Expected 1 agent telemetry entry, got %d", len(report.AgentsTelemetry))
	}
}

func TestFinOps_ScorecardAndMermaid(t *testing.T) {
	manager := finops.NewFinOpsManager("test-scorecard")
	now := time.Now()

	manager.RegisterAgent(finops.BudgetPolicy{
		PolicyID:         "policy-card",
		AgentID:          "agent-healthy",
		MaxCostUSD:       1.00,
		EnforceHardLimit: true,
	})
	manager.RegisterAgent(finops.BudgetPolicy{
		PolicyID:         "policy-card-killed",
		AgentID:          "agent-killed",
		MaxCostUSD:       0.01,
		EnforceHardLimit: true,
	})

	trackerKilled := manager.GetTracker("agent-killed")
	_, _ = trackerKilled.RecordConsumption("o1", "o1", 1000, 500, 0, now) // Exceeds $0.01

	report := manager.Audit(now)

	scorecard := finops.RenderFinOpsScorecard(report)
	if !strings.Contains(scorecard, "AGENTVCR // FINOPS TOKEN BUDGET") {
		t.Fatalf("Scorecard missing header: %s", scorecard)
	}
	if !strings.Contains(scorecard, "RUNAWAY SPEND DETECTED") {
		t.Fatalf("Scorecard should flag runaway spend on killed agent")
	}

	mermaid := finops.RenderFinOpsMermaid(report)
	if !strings.Contains(mermaid, "graph TD") || !strings.Contains(mermaid, "CIRCUIT TRIPPED") {
		t.Fatalf("Mermaid diagram missing circuit trip node: %s", mermaid)
	}
}
