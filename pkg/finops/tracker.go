package finops

import (
	"fmt"
	"sync"
	"time"
)

// AgentFinOpsTracker monitors spend velocity and enforces circuit breaker budget limits per agent.
type AgentFinOpsTracker struct {
	mu                  sync.RWMutex
	policy              BudgetPolicy
	calculator          *PricingCalculator
	totalCostUSD        float64
	totalPromptTokens   int64
	totalCompletionTokens int64
	totalCachedTokens   int64
	totalInvocations    int64
	totalDollarsSaved   float64
	deltas              []TokenUsageDelta
	circuitTripped      bool
	hardKillReason      string
	activeModelOverride string
}

// NewAgentFinOpsTracker initializes a budget and burn-rate tracker for an agent.
func NewAgentFinOpsTracker(policy BudgetPolicy, calculator *PricingCalculator) *AgentFinOpsTracker {
	if calculator == nil {
		calculator = NewPricingCalculator()
	}
	return &AgentFinOpsTracker{
		policy:     policy,
		calculator: calculator,
		deltas:     make([]TokenUsageDelta, 0),
	}
}

// PreFlightCheck evaluates if the agent is allowed to execute the next prompt turn.
// Returns: (allowed, action, targetModel, error)
func (t *AgentFinOpsTracker) PreFlightCheck(requestedModel string, now time.Time) (bool, EnforcementAction, string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	targetModel := requestedModel
	if t.activeModelOverride != "" {
		targetModel = t.activeModelOverride
	}

	// 1. If circuit is already hard-tripped
	if t.circuitTripped {
		return false, ActionHardKill, targetModel, fmt.Errorf("finops circuit tripped: %s", t.hardKillReason)
	}

	utilization := 0.0
	if t.policy.MaxCostUSD > 0 {
		utilization = t.totalCostUSD / t.policy.MaxCostUSD
	}

	// 2. Evaluate Hard Limit Breaches
	if t.policy.EnforceHardLimit && t.policy.MaxCostUSD > 0 && t.totalCostUSD >= t.policy.MaxCostUSD {
		t.circuitTripped = true
		t.hardKillReason = fmt.Sprintf("budget ceiling reached ($%.4f / $%.4f)", t.totalCostUSD, t.policy.MaxCostUSD)
		return false, ActionHardKill, targetModel, fmt.Errorf("finops circuit tripped: %s", t.hardKillReason)
	}

	totalTokens := t.totalPromptTokens + t.totalCompletionTokens
	if t.policy.EnforceHardLimit && t.policy.MaxTotalTokens > 0 && totalTokens >= t.policy.MaxTotalTokens {
		t.circuitTripped = true
		t.hardKillReason = fmt.Sprintf("token ceiling reached (%d / %d tokens)", totalTokens, t.policy.MaxTotalTokens)
		return false, ActionHardKill, targetModel, fmt.Errorf("finops circuit tripped: %s", t.hardKillReason)
	}

	// 3. Evaluate Burn-Rate Spikes within Sliding Window
	burnRate := t.calculateBurnRateLocked(now)
	if t.policy.MaxCostPerMinute > 0 && burnRate > t.policy.MaxCostPerMinute {
		// Spend velocity anomaly detected: downgrade model or throttle
		if t.policy.DowngradeModel != "" && targetModel != t.policy.DowngradeModel {
			t.activeModelOverride = t.policy.DowngradeModel
			targetModel = t.policy.DowngradeModel
			return true, ActionDowngrade, targetModel, nil
		}
		return true, ActionThrottle, targetModel, nil
	}

	// 4. Evaluate Threshold Enforcements
	if t.policy.ThrottleThresholdPct > 0 && utilization >= t.policy.ThrottleThresholdPct {
		if t.policy.DowngradeModel != "" && targetModel != t.policy.DowngradeModel {
			t.activeModelOverride = t.policy.DowngradeModel
			targetModel = t.policy.DowngradeModel
			return true, ActionDowngrade, targetModel, nil
		}
		return true, ActionThrottle, targetModel, nil
	}

	if t.policy.WarnThresholdPct > 0 && utilization >= t.policy.WarnThresholdPct {
		return true, ActionWarn, targetModel, nil
	}

	return true, ActionAllow, targetModel, nil
}

// RecordConsumption logs actual completed tokens and updates running burn metrics.
func (t *AgentFinOpsTracker) RecordConsumption(originalModel, usedModel string, promptTokens, completionTokens, cachedTokens int, now time.Time) (float64, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	cost, err := t.calculator.CalculateCost(usedModel, promptTokens, completionTokens, cachedTokens)
	if err != nil {
		return 0, err
	}

	t.totalCostUSD += cost
	t.totalPromptTokens += int64(promptTokens)
	t.totalCompletionTokens += int64(completionTokens)
	t.totalCachedTokens += int64(cachedTokens)
	t.totalInvocations++

	// Track savings if model was downgraded
	if originalModel != "" && originalModel != usedModel {
		savings := t.calculator.CalculateSavings(originalModel, usedModel, promptTokens, completionTokens)
		t.totalDollarsSaved += savings
	}

	delta := TokenUsageDelta{
		ModelName:        usedModel,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		CachedTokens:     cachedTokens,
		CostUSD:          cost,
		Timestamp:        now,
	}

	t.deltas = append(t.deltas, delta)
	t.pruneDeltasLocked(now)

	// Post-check hard budget breach
	if t.policy.EnforceHardLimit && t.policy.MaxCostUSD > 0 && t.totalCostUSD >= t.policy.MaxCostUSD {
		t.circuitTripped = true
		t.hardKillReason = fmt.Sprintf("cost ceiling exhausted post-invocation ($%.4f / $%.4f)", t.totalCostUSD, t.policy.MaxCostUSD)
	}

	return cost, nil
}

// ResetCircuit manually un-trips a circuit breaker (e.g. after human-in-the-loop approval or budget expansion).
func (t *AgentFinOpsTracker) ResetCircuit(newMaxCostUSD float64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if newMaxCostUSD > t.policy.MaxCostUSD {
		t.policy.MaxCostUSD = newMaxCostUSD
	}
	t.circuitTripped = false
	t.hardKillReason = ""
	t.activeModelOverride = ""
}

func (t *AgentFinOpsTracker) pruneDeltasLocked(now time.Time) {
	window := t.policy.SlidingWindow
	if window <= 0 {
		window = time.Minute
	}
	cutoff := now.Add(-window)

	validIdx := 0
	for i, d := range t.deltas {
		if d.Timestamp.After(cutoff) {
			validIdx = i
			break
		}
	}
	if validIdx > 0 {
		t.deltas = t.deltas[validIdx:]
	}
}

func (t *AgentFinOpsTracker) calculateBurnRateLocked(now time.Time) float64 {
	window := t.policy.SlidingWindow
	if window <= 0 {
		window = time.Minute
	}
	cutoff := now.Add(-window)

	windowCost := 0.0
	for _, d := range t.deltas {
		if d.Timestamp.After(cutoff) {
			windowCost += d.CostUSD
		}
	}

	minutes := window.Minutes()
	if minutes <= 0 {
		minutes = 1.0
	}
	return windowCost / minutes
}

// Telemetry returns a live snapshot of the agent's FinOps metrics.
func (t *AgentFinOpsTracker) Telemetry(now time.Time) FinOpsTelemetry {
	t.mu.RLock()
	defer t.mu.RUnlock()

	util := 0.0
	remaining := 0.0
	if t.policy.MaxCostUSD > 0 {
		util = t.totalCostUSD / t.policy.MaxCostUSD
		remaining = t.policy.MaxCostUSD - t.totalCostUSD
		if remaining < 0 {
			remaining = 0
		}
	}

	currentAction := ActionAllow
	if t.circuitTripped {
		currentAction = ActionHardKill
	} else if util >= t.policy.ThrottleThresholdPct {
		if t.activeModelOverride != "" {
			currentAction = ActionDowngrade
		} else {
			currentAction = ActionThrottle
		}
	} else if util >= t.policy.WarnThresholdPct {
		currentAction = ActionWarn
	}

	return FinOpsTelemetry{
		AgentID:               t.policy.AgentID,
		CurrentAction:         currentAction,
		TotalCostUSD:          t.totalCostUSD,
		RemainingBudgetUSD:    remaining,
		BudgetUtilization:     util,
		TotalPromptTokens:     t.totalPromptTokens,
		TotalCompletionTokens: t.totalCompletionTokens,
		TotalCachedTokens:     t.totalCachedTokens,
		BurnRateUSDPerMin:     t.calculateBurnRateLocked(now),
		ActiveModelOverride:   t.activeModelOverride,
		CircuitTripped:        t.circuitTripped,
		HardKillReason:        t.hardKillReason,
		TotalInvocations:      t.totalInvocations,
	}
}
