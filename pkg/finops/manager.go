package finops

import (
	"context"
	"sync"
	"time"
)

// InterceptorFunc represents downstream model execution.
type InterceptorFunc func(ctx context.Context, model string, promptTokens, completionTokens int) (int, int, error)

// FinOpsManager acts as the root coordinator managing multi-agent budgets and circuit breakers.
type FinOpsManager struct {
	mu         sync.RWMutex
	systemID   string
	calculator *PricingCalculator
	trackers   map[string]*AgentFinOpsTracker
}

// NewFinOpsManager creates a centralized FinOps manager.
func NewFinOpsManager(systemID string) *FinOpsManager {
	if systemID == "" {
		systemID = "agentvcr-finops"
	}
	return &FinOpsManager{
		systemID:   systemID,
		calculator: NewPricingCalculator(),
		trackers:   make(map[string]*AgentFinOpsTracker),
	}
}

// Calculator returns the shared pricing calculator.
func (fm *FinOpsManager) Calculator() *PricingCalculator {
	return fm.calculator
}

// RegisterAgent sets up a budget policy for an agent.
func (fm *FinOpsManager) RegisterAgent(policy BudgetPolicy) *AgentFinOpsTracker {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	tracker := NewAgentFinOpsTracker(policy, fm.calculator)
	fm.trackers[policy.AgentID] = tracker
	return tracker
}

// GetTracker retrieves an existing tracker or creates one with default defensive limits.
func (fm *FinOpsManager) GetTracker(agentID string) *AgentFinOpsTracker {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	if t, ok := fm.trackers[agentID]; ok {
		return t
	}

	defaultPolicy := DefaultBudgetPolicy(agentID, 1.00)
	tracker := NewAgentFinOpsTracker(defaultPolicy, fm.calculator)
	fm.trackers[agentID] = tracker
	return tracker
}

// ExecuteTurn intercepts an LLM invocation, checks pre-flight limits, enforces downgrades/throttling,
// invokes the underlying turn function, and logs resulting financial consumption.
func (fm *FinOpsManager) ExecuteTurn(
	ctx context.Context,
	agentID string,
	requestedModel string,
	estimatedPromptTokens int,
	now time.Time,
	invoker InterceptorFunc,
) (EnforcementAction, string, float64, error) {
	tracker := fm.GetTracker(agentID)

	// 1. Pre-flight check
	allowed, action, targetModel, err := tracker.PreFlightCheck(requestedModel, now)
	if !allowed {
		return action, targetModel, 0, err
	}

	// 2. If action is throttle, introduce a small synthetic pause to slow down burn rate
	if action == ActionThrottle {
		time.Sleep(20 * time.Millisecond)
	}

	// 3. Execute downstream invoker
	actualPrompt, actualCompletion, execErr := invoker(ctx, targetModel, estimatedPromptTokens, 0)
	if execErr != nil {
		return action, targetModel, 0, execErr
	}

	// 4. Record consumption
	cost, recordErr := tracker.RecordConsumption(requestedModel, targetModel, actualPrompt, actualCompletion, 0, now)
	if recordErr != nil {
		return action, targetModel, cost, recordErr
	}

	return action, targetModel, cost, nil
}

// Audit creates a complete system-wide financial compliance report across all registered agents.
func (fm *FinOpsManager) Audit(now time.Time) *FinOpsAuditReport {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	var telemetries []FinOpsTelemetry
	allCompliant := true
	totalCost := 0.0
	var totalTokens int64
	totalSaved := 0.0

	for _, t := range fm.trackers {
		tel := t.Telemetry(now)
		telemetries = append(telemetries, tel)
		totalCost += tel.TotalCostUSD
		totalTokens += (tel.TotalPromptTokens + tel.TotalCompletionTokens)
		totalSaved += t.totalDollarsSaved

		if tel.CircuitTripped {
			allCompliant = false
		}
	}

	return &FinOpsAuditReport{
		SystemID:          fm.systemID,
		Timestamp:         now,
		TotalAgents:       len(fm.trackers),
		AllCompliant:      allCompliant,
		TotalCostUSD:      totalCost,
		TotalTokens:       totalTokens,
		TotalDollarsSaved: totalSaved,
		AgentsTelemetry:   telemetries,
	}
}
