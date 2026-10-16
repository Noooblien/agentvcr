package throttle

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// Limiter manages real-time rate limiting, token quotas, and dynamic latency budget distribution.
type Limiter struct {
	mu             sync.Mutex
	policy         ThrottlePolicy
	reqBucket      *TokenBucket
	tokenBucket    *TokenBucket
	spentLatencyMs int64
	stepCount      int
}

// NewLimiter creates an initialized real-time Limiter instance.
func NewLimiter(policy ThrottlePolicy) *Limiter {
	now := time.Now()

	rpmRefill := float64(policy.MaxRPM) / 60.0
	if rpmRefill <= 0 {
		rpmRefill = 1.0
	}
	burst := float64(policy.BurstCapacity)
	if burst <= 0 {
		burst = 10.0
	}

	tpmRefill := float64(policy.MaxTPM) / 60.0
	if tpmRefill <= 0 {
		tpmRefill = 1000.0
	}

	return &Limiter{
		policy: policy,
		reqBucket: &TokenBucket{
			Capacity:     burst,
			Tokens:       burst,
			RefillRate:   rpmRefill,
			LastRefillAt: now,
		},
		tokenBucket: &TokenBucket{
			Capacity:     float64(policy.MaxTPM),
			Tokens:       float64(policy.MaxTPM),
			RefillRate:   tpmRefill,
			LastRefillAt: now,
		},
	}
}

// CheckAndConsume verifies whether an incoming request of tokenCost can proceed or must be throttled.
func (l *Limiter) CheckAndConsume(tokensRequested int, now time.Time) ThrottleDecision {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.refillBuckets(now)

	decision := ThrottleDecision{
		Allowed:           true,
		RemainingRPM:      int(l.reqBucket.Tokens),
		RemainingTPM:      int(l.tokenBucket.Tokens),
		BudgetBurnedMs:    l.spentLatencyMs,
		BudgetRemainingMs: l.policy.TotalLatencyBudgetMs - l.spentLatencyMs,
		ExhaustedBudget:   l.spentLatencyMs >= l.policy.TotalLatencyBudgetMs,
	}

	// 1. Check Latency Budget
	if l.policy.TotalLatencyBudgetMs > 0 && l.spentLatencyMs >= l.policy.TotalLatencyBudgetMs {
		decision.Allowed = false
		decision.ThrottleReason = fmt.Sprintf("Total latency budget exhausted (%dms / %dms)", l.spentLatencyMs, l.policy.TotalLatencyBudgetMs)
		return decision
	}

	// 2. Check RPM Token Bucket
	if l.reqBucket.Tokens < 1.0 {
		missing := 1.0 - l.reqBucket.Tokens
		delaySec := missing / l.reqBucket.RefillRate
		decision.Allowed = false
		decision.DelayRequired = time.Duration(delaySec * float64(time.Second))
		decision.ThrottleReason = fmt.Sprintf("RPM rate limit reached; throttle delay %v required", decision.DelayRequired)
		return decision
	}

	// 3. Check TPM Token Bucket
	reqTokens := float64(tokensRequested)
	if reqTokens > 0 && l.tokenBucket.Tokens < reqTokens {
		missing := reqTokens - l.tokenBucket.Tokens
		delaySec := missing / l.tokenBucket.RefillRate
		decision.Allowed = false
		decision.DelayRequired = time.Duration(delaySec * float64(time.Second))
		decision.ThrottleReason = fmt.Sprintf("TPM token quota exceeded (requested %d tokens); delay %v required", tokensRequested, decision.DelayRequired)
		return decision
	}

	// Consume tokens
	l.reqBucket.Tokens -= 1.0
	if reqTokens > 0 {
		l.tokenBucket.Tokens -= reqTokens
	}
	l.stepCount++

	decision.RemainingRPM = int(l.reqBucket.Tokens)
	decision.RemainingTPM = int(l.tokenBucket.Tokens)

	return decision
}

// RecordStepLatency updates the dynamic latency budget burned by a completed step.
func (l *Limiter) RecordStepLatency(durationMs int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.spentLatencyMs += durationMs
}

// DynamicStepAllocation calculates the adaptive target latency for an upcoming step based on remaining runway.
func (l *Limiter) DynamicStepAllocation(estimatedRemainingSteps int) int64 {
	l.mu.Lock()
	defer l.mu.Unlock()

	remainingBudget := l.policy.TotalLatencyBudgetMs - l.spentLatencyMs
	if remainingBudget <= 0 {
		return 0
	}

	if estimatedRemainingSteps <= 1 {
		return remainingBudget
	}

	allocated := remainingBudget / int64(estimatedRemainingSteps)
	if l.policy.PerStepBudgetMs > 0 && allocated > l.policy.PerStepBudgetMs {
		allocated = l.policy.PerStepBudgetMs
	}
	return allocated
}

func (l *Limiter) refillBuckets(now time.Time) {
	elapsed := now.Sub(l.reqBucket.LastRefillAt).Seconds()
	if elapsed > 0 {
		l.reqBucket.Tokens = math.Min(l.reqBucket.Capacity, l.reqBucket.Tokens+(elapsed*l.reqBucket.RefillRate))
		l.reqBucket.LastRefillAt = now
	}

	elapsedToken := now.Sub(l.tokenBucket.LastRefillAt).Seconds()
	if elapsedToken > 0 {
		l.tokenBucket.Tokens = math.Min(l.tokenBucket.Capacity, l.tokenBucket.Tokens+(elapsedToken*l.tokenBucket.RefillRate))
		l.tokenBucket.LastRefillAt = now
	}
}

// AuditSession analyzes an entire recorded session against throttling policies, detecting rate spikes and SLA breaches.
func AuditSession(session *trace.Session, policy ThrottlePolicy) (*ThrottleAuditReport, error) {
	if session == nil {
		return nil, fmt.Errorf("session cannot be nil")
	}

	report := &ThrottleAuditReport{
		SessionID:       session.ID,
		AgentName:       session.AgentName,
		TotalSteps:      len(session.Events),
		Allocations:     make([]LatencyAllocation, 0),
		Breaches:        make([]string, 0),
		ComplianceScore: 100.0,
	}

	limiter := NewLimiter(policy)
	var currentSpentMs int64

	for idx, ev := range session.Events {
		tokens := 0
		if body, ok := ev.Payload["body"].(map[string]interface{}); ok {
			if usage, ok := body["usage"].(map[string]interface{}); ok {
				if tot, ok := usage["total_tokens"].(float64); ok {
					tokens = int(tot)
				}
			}
		}

		decision := limiter.CheckAndConsume(tokens, ev.Timestamp)
		if !decision.Allowed {
			report.ThrottledRequests++
			delayMs := decision.DelayRequired.Milliseconds()
			report.TotalDelayIncurredMs += delayMs
			report.Breaches = append(report.Breaches, fmt.Sprintf("Step %d: %s", ev.Index, decision.ThrottleReason))
		} else {
			report.AllowedRequests++
		}

		duration := ev.DurationMs
		if duration <= 0 && ev.Type == trace.EventLLMResponse {
			duration = 200
		}
		currentSpentMs += duration
		limiter.RecordStepLatency(duration)

		remainingEst := len(session.Events) - idx
		targetBudget := limiter.DynamicStepAllocation(remainingEst)

		report.Allocations = append(report.Allocations, LatencyAllocation{
			StepIndex:        ev.Index,
			TargetBudgetMs:   targetBudget,
			ActualDurationMs: duration,
			DeltaMs:          duration - targetBudget,
			RemainingTotalMs: policy.TotalLatencyBudgetMs - currentSpentMs,
		})

		if policy.PerStepBudgetMs > 0 && duration > policy.PerStepBudgetMs {
			report.LatencySLABreached = true
			report.Breaches = append(report.Breaches, fmt.Sprintf("Step %d: Exceeded per-step latency SLA (%dms > %dms ceiling)", ev.Index, duration, policy.PerStepBudgetMs))
		}
	}

	report.TotalDurationMs = currentSpentMs

	if policy.TotalLatencyBudgetMs > 0 && currentSpentMs > policy.TotalLatencyBudgetMs {
		report.LatencySLABreached = true
		report.Breaches = append(report.Breaches, fmt.Sprintf("Total trajectory latency (%dms) exceeded budget SLA (%dms)", currentSpentMs, policy.TotalLatencyBudgetMs))
	}

	// Compute compliance score
	deductions := float64(len(report.Breaches)) * 15.0
	if deductions > 100.0 {
		deductions = 100.0
	}
	report.ComplianceScore = math.Round((100.0-deductions)*100) / 100

	return report, nil
}

// RenderThrottleScorecard generates a clean ANSI terminal visualization of throttle compliance and latency allocations.
func RenderThrottleScorecard(report *ThrottleAuditReport, policy ThrottlePolicy) string {
	var sb strings.Builder

	sb.WriteString("\n\033[1;36m========================================================================================\033[0m\n")
	sb.WriteString(fmt.Sprintf("\033[1;37m   AGENTVCR DYNAMIC LATENCY BUDGET & TOKEN RATE-LIMITER AUDIT REPORT\033[0m\n"))
	sb.WriteString("\033[1;36m========================================================================================\033[0m\n")

	sb.WriteString(fmt.Sprintf("  \033[1;33mSession ID:\033[0m          %s (%s)\n", report.SessionID, report.AgentName))
	sb.WriteString(fmt.Sprintf("  \033[1;33mTotal Trajectory:\033[0m   %d steps | Total Wall-Clock: %dms (Budget: %dms)\n", report.TotalSteps, report.TotalDurationMs, policy.TotalLatencyBudgetMs))
	sb.WriteString(fmt.Sprintf("  \033[1;33mRate Limit Policy:\033[0m  Max %d RPM | Max %d TPM | Per-Step SLA: %dms\n", policy.MaxRPM, policy.MaxTPM, policy.PerStepBudgetMs))

	if len(report.Breaches) == 0 {
		sb.WriteString("  \033[1;33mSLA Enforcement:\033[0m   \033[1;32m[PASS] ZERO RATE SPIKES & ALL LATENCY SLAS RESPECTED (100%)\033[0m\n")
	} else {
		sb.WriteString(fmt.Sprintf("  \033[1;33mSLA Enforcement:\033[0m   \033[1;31m[FAIL] %d BREACHES DETECTED (Compliance Score: %.1f%%)\033[0m\n", len(report.Breaches), report.ComplianceScore))
	}
	sb.WriteString("\033[1;34m----------------------------------------------------------------------------------------\033[0m\n")

	// Render Step Latency Allocation Table
	sb.WriteString("\033[1;37m  STEP LATENCY BUDGET & RUNWAY ALLOCATION TABLE:\033[0m\n")
	sb.WriteString("  \033[1;30mSTEP   ACTUAL (MS)   TARGET (MS)   DELTA (MS)   REMAINING RUNWAY (MS)   STATUS\033[0m\n")

	limit := len(report.Allocations)
	if limit > 10 {
		limit = 10
	}
	for i := 0; i < limit; i++ {
		alloc := report.Allocations[i]
		status := "\033[1;32mON-BUDGET\033[0m"
		if alloc.DeltaMs > 0 {
			status = "\033[1;31mOVER-BUDGET\033[0m"
		}
		sb.WriteString(fmt.Sprintf("  #%-4d  %-13d %-13d %-+12d %-23d %s\n",
			alloc.StepIndex, alloc.ActualDurationMs, alloc.TargetBudgetMs, alloc.DeltaMs, alloc.RemainingTotalMs, status))
	}
	if len(report.Allocations) > limit {
		sb.WriteString(fmt.Sprintf("  ... [%d additional steps hidden]\n", len(report.Allocations)-limit))
	}

	if len(report.Breaches) > 0 {
		sb.WriteString("\033[1;34m----------------------------------------------------------------------------------------\033[0m\n")
		sb.WriteString("\033[1;31m  DETECTED THROTTLE & LATENCY SLA BREACHES:\033[0m\n")
		for _, b := range report.Breaches {
			sb.WriteString(fmt.Sprintf("  \033[1;31m[!] %s\033[0m\n", b))
		}
	}

	sb.WriteString("\033[1;36m========================================================================================\033[0m\n\n")
	return sb.String()
}
