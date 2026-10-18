package chaos

import (
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// ChaosEngine executes controlled failure injection experiments against agent trajectories.
type ChaosEngine struct {
	mu  sync.Mutex
	rng *rand.Rand
}

// NewChaosEngine constructs a deterministic chaos engine.
func NewChaosEngine(seed int64) *ChaosEngine {
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	return &ChaosEngine{
		rng: rand.New(rand.NewSource(seed)),
	}
}

// EvaluateSession executes a chaos experiment over an agent execution session.
func (ce *ChaosEngine) EvaluateSession(session *trace.Session, cfg ChaosExperimentConfig) (*ChaosExperimentReport, error) {
	if session == nil {
		return nil, fmt.Errorf("session cannot be nil")
	}

	start := time.Now()
	ce.mu.Lock()
	if cfg.ChaosSeed != 0 {
		ce.rng = rand.New(rand.NewSource(cfg.ChaosSeed))
	}
	ce.mu.Unlock()

	report := &ChaosExperimentReport{
		ExperimentName:      cfg.ExperimentName,
		SessionID:           session.ID,
		AgentName:           session.AgentName,
		TotalStepsEvaluated: len(session.Events),
		Events:              make([]InjectedFaultEvent, 0),
	}

	ruleCounters := make(map[string]int)
	totalRecoveryTime := int64(0)

	for idx, event := range session.Events {
		toolName := extractToolName(event)

		// Check if any fault rule triggers
		rule, shouldTrigger := ce.shouldTriggerRule(idx, toolName, cfg.Rules, ruleCounters)
		if !shouldTrigger {
			continue
		}

		ruleCounters[rule.ID]++
		report.TotalFaultsInjected++

		// Inject fault and simulate agent response
		injectedEvent := ce.injectFault(idx, toolName, rule, event)
		report.Events = append(report.Events, injectedEvent)

		if injectedEvent.Recovered {
			report.TotalFaultsRecovered++
			totalRecoveryTime += injectedEvent.RecoveryTimeMs
		}
		report.TokenBlastRadiusTokens += injectedEvent.ExtraTokensSpent
	}

	// Calculate metrics
	if report.TotalFaultsInjected > 0 {
		report.RecoveryRatePct = float64(report.TotalFaultsRecovered) / float64(report.TotalFaultsInjected) * 100.0
	} else {
		report.RecoveryRatePct = 100.0
	}

	if report.TotalFaultsRecovered > 0 {
		report.MeanTimeToRecoveryMs = float64(totalRecoveryTime) / float64(report.TotalFaultsRecovered)
	}

	// Calculate overall Resilience Score (0.0 to 100.0)
	report.ResilienceScore = ce.calculateResilienceScore(report)

	// Determine verdict status
	if report.ResilienceScore >= 80.0 && report.RecoveryRatePct >= 75.0 {
		report.Status = StatusResilient
		report.Passed = true
	} else if report.ResilienceScore >= 50.0 {
		report.Status = StatusDegraded
		report.Passed = !cfg.StrictMode
	} else {
		report.Status = StatusCriticalFailure
		report.Passed = false
	}

	report.Duration = time.Since(start)
	return report, nil
}

func (ce *ChaosEngine) shouldTriggerRule(stepIdx int, toolName string, rules []FaultRule, counters map[string]int) (*FaultRule, bool) {
	for i := range rules {
		r := &rules[i]

		// Check step constraint
		if r.TargetStep > 0 && r.TargetStep != stepIdx {
			continue
		}

		// Check tool constraint
		if r.TargetTool != "" && !strings.EqualFold(r.TargetTool, toolName) {
			continue
		}

		// Check max occurrences
		if r.MaxOccurrences > 0 && counters[r.ID] >= r.MaxOccurrences {
			continue
		}

		// Check probability
		ce.mu.Lock()
		roll := ce.rng.Float64()
		ce.mu.Unlock()

		prob := r.Probability
		if prob <= 0 {
			prob = 1.0 // Default to guaranteed injection if 0
		}

		if roll <= prob {
			return r, true
		}
	}
	return nil, false
}

func (ce *ChaosEngine) injectFault(stepIdx int, toolName string, rule *FaultRule, event trace.TraceEvent) InjectedFaultEvent {
	injected := InjectedFaultEvent{
		StepIndex: stepIdx,
		ToolName:  toolName,
		FaultType: rule.Type,
	}

	switch rule.Type {
	case FaultRateLimit:
		injected.InjectedMessage = "HTTP 429 Too Many Requests: Rate limit exceeded, Retry-After: 2s"
		injected.InjectedDelayMs = 2000
		injected.Reaction = ReactionRetryBackoff
		injected.Recovered = true
		injected.RecoveryTimeMs = 2150
		injected.ExtraTokensSpent = 120

	case FaultToolTimeout:
		delayMs := rule.InjectedDelay.Milliseconds()
		if delayMs == 0 {
			delayMs = 3000
		}
		injected.InjectedMessage = fmt.Sprintf("ToolExecutionTimeout: tool '%s' timed out after %dms", toolName, delayMs)
		injected.InjectedDelayMs = delayMs
		injected.Reaction = ReactionFallbackTool
		injected.Recovered = true
		injected.RecoveryTimeMs = delayMs + 350
		injected.ExtraTokensSpent = 180

	case FaultPayloadCorrupt:
		injected.InjectedMessage = "JSONRPCError: unexpected end of JSON input at byte offset 482 (truncated frame)"
		injected.InjectedDelayMs = 15
		injected.Reaction = ReactionRetryBackoff
		injected.Recovered = true
		injected.RecoveryTimeMs = 320
		injected.ExtraTokensSpent = 95

	case FaultToolFlaky:
		injected.InjectedMessage = "ConnectionResetByPeer: upstream transport disconnected unexpectedly"
		injected.InjectedDelayMs = 120
		injected.Reaction = ReactionRetryBackoff
		injected.Recovered = true
		injected.RecoveryTimeMs = 450
		injected.ExtraTokensSpent = 110

	case FaultUpstreamOutage:
		injected.InjectedMessage = "HTTP 503 Service Unavailable: upstream MCP cluster unreachable"
		injected.InjectedDelayMs = 500
		injected.Reaction = ReactionGracefulDegrade
		injected.Recovered = false
		injected.RecoveryTimeMs = 0
		injected.ExtraTokensSpent = 240

	case FaultStateCorruption:
		injected.InjectedMessage = "StateConsistencyError: schema invariant violation on tool output"
		injected.InjectedDelayMs = 10
		injected.Reaction = ReactionUnhandledException
		injected.Recovered = false
		injected.RecoveryTimeMs = 0
		injected.ExtraTokensSpent = 350

	default:
		injected.InjectedMessage = "GenericFault: simulated fault condition"
		injected.Reaction = ReactionRetryBackoff
		injected.Recovered = true
		injected.RecoveryTimeMs = 100
		injected.ExtraTokensSpent = 50
	}

	if rule.CustomMessage != "" {
		injected.InjectedMessage = rule.CustomMessage
	}

	return injected
}

func (ce *ChaosEngine) calculateResilienceScore(report *ChaosExperimentReport) float64 {
	if report.TotalFaultsInjected == 0 {
		return 100.0
	}

	// Base score is recovery percentage
	score := report.RecoveryRatePct

	// Latency penalty: if MTTR > 2500ms, apply small deduction
	if report.MeanTimeToRecoveryMs > 2500.0 {
		excessSec := (report.MeanTimeToRecoveryMs - 2500.0) / 1000.0
		penalty := excessSec * 2.5
		if penalty > 15.0 {
			penalty = 15.0
		}
		score -= penalty
	}

	// Blast radius token penalty: if extra tokens per fault > 200, apply deduction
	avgExtraTokens := float64(report.TokenBlastRadiusTokens) / float64(report.TotalFaultsInjected)
	if avgExtraTokens > 200.0 {
		tokenPenalty := (avgExtraTokens - 200.0) / 50.0
		if tokenPenalty > 15.0 {
			tokenPenalty = 15.0
		}
		score -= tokenPenalty
	}

	if score < 0.0 {
		score = 0.0
	}
	if score > 100.0 {
		score = 100.0
	}
	return score
}

func extractToolName(event trace.TraceEvent) string {
	if event.Payload == nil {
		return ""
	}
	if name, ok := event.Payload["tool"].(string); ok {
		return name
	}
	if name, ok := event.Payload["tool_name"].(string); ok {
		return name
	}
	if name, ok := event.Payload["name"].(string); ok {
		return name
	}
	return string(event.Type)
}
