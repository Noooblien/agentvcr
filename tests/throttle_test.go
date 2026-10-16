package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/throttle"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func TestThrottle_TokenBucketAndRPM(t *testing.T) {
	policy := throttle.ThrottlePolicy{
		MaxRPM:               60, // 1 req/sec
		MaxTPM:               1000,
		BurstCapacity:        2,
		TotalLatencyBudgetMs: 5000,
	}

	limiter := throttle.NewLimiter(policy)
	now := time.Now()

	// 1st request should pass (burns 1 of 2 burst tokens)
	d1 := limiter.CheckAndConsume(100, now)
	if !d1.Allowed {
		t.Fatalf("Expected 1st request to pass, got throttled: %s", d1.ThrottleReason)
	}

	// 2nd request should pass (burns remaining burst token)
	d2 := limiter.CheckAndConsume(100, now)
	if !d2.Allowed {
		t.Fatalf("Expected 2nd request to pass, got throttled: %s", d2.ThrottleReason)
	}

	// 3rd immediate request should be throttled (empty bucket)
	d3 := limiter.CheckAndConsume(100, now)
	if d3.Allowed {
		t.Fatalf("Expected 3rd request to be throttled due to empty burst bucket")
	}
	if d3.DelayRequired <= 0 {
		t.Errorf("Expected positive delay required, got %v", d3.DelayRequired)
	}

	// After 1 second refill, request should be allowed again
	d4 := limiter.CheckAndConsume(100, now.Add(1100*time.Millisecond))
	if !d4.Allowed {
		t.Fatalf("Expected request to pass after 1.1s refill, got throttled: %s", d4.ThrottleReason)
	}
}

func TestThrottle_DynamicLatencyAllocation(t *testing.T) {
	policy := throttle.ThrottlePolicy{
		TotalLatencyBudgetMs: 10000, // 10 seconds total
		PerStepBudgetMs:      3000,  // max 3s per step
	}

	limiter := throttle.NewLimiter(policy)

	// With 5 steps remaining and 10,000ms budget, each gets 2,000ms
	alloc1 := limiter.DynamicStepAllocation(5)
	if alloc1 != 2000 {
		t.Errorf("Expected 2000ms allocation, got %dms", alloc1)
	}

	// Record that step 1 was fast (took only 500ms)
	limiter.RecordStepLatency(500)

	// Now 9500ms remaining for 4 steps => 9500 / 4 = 2375ms
	alloc2 := limiter.DynamicStepAllocation(4)
	if alloc2 != 2375 {
		t.Errorf("Expected 2375ms allocation, got %dms", alloc2)
	}

	// Now burn 9000ms (total spent: 9500ms, 500ms remaining)
	limiter.RecordStepLatency(9000)
	alloc3 := limiter.DynamicStepAllocation(2)
	if alloc3 != 250 {
		t.Errorf("Expected 250ms allocation, got %dms", alloc3)
	}
}

func TestThrottle_AuditSessionAndBreaches(t *testing.T) {
	now := time.Now()
	session := &trace.Session{
		ID:        "throttle-test-session",
		AgentName: "fast-agent",
		Events: []trace.TraceEvent{
			{
				Index:      1,
				Timestamp:  now,
				Type:       trace.EventLLMRequest,
				DurationMs: 400,
			},
			{
				Index:      2,
				Timestamp:  now.Add(100 * time.Millisecond), // Fast rapid burst
				Type:       trace.EventLLMResponse,
				DurationMs: 3500, // Exceeds per-step budget of 3000ms
			},
			{
				Index:      3,
				Timestamp:  now.Add(200 * time.Millisecond),
				Type:       trace.EventToolCall,
				DurationMs: 2000,
			},
		},
	}

	policy := throttle.ThrottlePolicy{
		MaxRPM:               10,
		MaxTPM:               10000,
		BurstCapacity:        1, // Will throttle on 2nd and 3rd events
		TotalLatencyBudgetMs: 5000,
		PerStepBudgetMs:      3000,
	}

	report, err := throttle.AuditSession(session, policy)
	if err != nil {
		t.Fatalf("AuditSession failed: %v", err)
	}

	if report.ThrottledRequests == 0 {
		t.Errorf("Expected throttled requests due to burst capacity of 1")
	}
	if !report.LatencySLABreached {
		t.Errorf("Expected LatencySLABreached to be true (3500ms > 3000ms, and total > 5000ms)")
	}
	if report.ComplianceScore >= 100.0 {
		t.Errorf("Expected compliance score < 100%%, got %.1f%%", report.ComplianceScore)
	}

	rendered := throttle.RenderThrottleScorecard(report, policy)
	if !strings.Contains(rendered, "AGENTVCR DYNAMIC LATENCY BUDGET") {
		t.Errorf("Expected rendered scorecard header, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "[FAIL]") {
		t.Errorf("Expected failing status in scorecard, got:\n%s", rendered)
	}
}
