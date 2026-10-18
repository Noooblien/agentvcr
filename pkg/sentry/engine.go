package sentry

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// SentryEngine performs runtime and post-hoc trajectory anomaly detection.
type SentryEngine struct {
	cfg SentryConfig
}

// NewEngine initializes a SentryEngine with the specified configuration.
func NewEngine(cfg SentryConfig) *SentryEngine {
	return &SentryEngine{cfg: cfg}
}

// AnalyzeTrajectory inspects an entire sequence of TraceEvents and generates an evaluation verdict.
func (e *SentryEngine) AnalyzeTrajectory(events []trace.TraceEvent) SentryVerdict {
	start := time.Now()

	if len(events) == 0 {
		return SentryVerdict{
			Healthy:         true,
			Action:          ActionPass,
			RiskScore:       0.0,
			Summary:         "Empty trajectory (zero events recorded)",
			Anomalies:       nil,
			Metrics:         SentryMetrics{},
			EvaluatedAt:     time.Now().UTC(),
			EvaluationDurUs: time.Since(start).Microseconds(),
		}
	}

	var anomalies []AnomalyEvent
	var metrics SentryMetrics
	metrics.TotalSteps = len(events)

	// --- 1. Latency Statistical Distribution ---
	var latencies []float64
	var sumLatency float64
	var maxLatency float64

	for _, ev := range events {
		lat := float64(ev.DurationMs)
		latencies = append(latencies, lat)
		sumLatency += lat
		if lat > maxLatency {
			maxLatency = lat
		}
	}

	meanLatency := sumLatency / float64(len(events))
	metrics.AvgStepLatencyMs = meanLatency
	metrics.MaxStepLatencyMs = maxLatency

	var varianceSum float64
	for _, lat := range latencies {
		diff := lat - meanLatency
		varianceSum += diff * diff
	}
	stdDevLatency := math.Sqrt(varianceSum / float64(len(events)))
	metrics.StdDevLatencyMs = stdDevLatency

	// Check latency Z-score outliers (> LatencyZScoreThreshold & > 1500ms)
	if stdDevLatency > 10.0 {
		for i, lat := range latencies {
			zScore := (lat - meanLatency) / stdDevLatency
			if zScore >= e.cfg.LatencyZScoreThreshold && lat > 1500.0 {
				anomalies = append(anomalies, AnomalyEvent{
					Type:        AnomalyLatencySpike,
					Severity:    SeverityMedium,
					StepIndex:   events[i].Index,
					ToolName:    extractToolName(events[i]),
					Message:     fmt.Sprintf("Step %d execution latency (%.1f ms) breached +%.2fσ threshold (mean=%.1f ms, σ=%.1f ms)", events[i].Index, lat, zScore, meanLatency, stdDevLatency),
					ObservedVal: lat,
					Threshold:   meanLatency + e.cfg.LatencyZScoreThreshold*stdDevLatency,
					Timestamp:   events[i].Timestamp,
				})
			}
		}
	}

	// --- 2. Tool Repetition & Oscillation Cycles ---
	var toolCalls []string
	toolIndices := make(map[string][]int)
	consecutiveTool := ""
	consecutiveCount := 0

	for i, ev := range events {
		tName := extractToolName(ev)
		if tName == "" {
			continue
		}

		toolCalls = append(toolCalls, tName)
		toolIndices[tName] = append(toolIndices[tName], ev.Index)

		// Check consecutive identical tool invocations
		if tName == consecutiveTool {
			consecutiveCount++
			if consecutiveCount >= e.cfg.MaxToolRepetition {
				anomalies = append(anomalies, AnomalyEvent{
					Type:        AnomalyToolLoop,
					Severity:    SeverityHigh,
					StepIndex:   ev.Index,
					ToolName:    tName,
					Message:     fmt.Sprintf("Detected recursive tool loop: tool '%s' called %d consecutive times", tName, consecutiveCount),
					ObservedVal: float64(consecutiveCount),
					Threshold:   float64(e.cfg.MaxToolRepetition),
					Timestamp:   ev.Timestamp,
				})
			}
		} else {
			consecutiveTool = tName
			consecutiveCount = 1
		}

		// Check alternating oscillation (A -> B -> A -> B)
		if len(toolCalls) >= 4 {
			n := len(toolCalls)
			if toolCalls[n-1] == toolCalls[n-3] && toolCalls[n-2] == toolCalls[n-4] && toolCalls[n-1] != toolCalls[n-2] {
				anomalies = append(anomalies, AnomalyEvent{
					Type:        AnomalyOscillation,
					Severity:    SeverityHigh,
					StepIndex:   ev.Index,
					ToolName:    tName,
					Message:     fmt.Sprintf("Detected bipartite oscillation trap between '%s' and '%s'", toolCalls[n-2], toolCalls[n-1]),
					ObservedVal: 2.0,
					Threshold:   float64(e.cfg.MaxOscillationCycles),
					Timestamp:   ev.Timestamp,
				})
			}
		}
		_ = i
	}

	if len(toolCalls) > 0 {
		uniqueTools := make(map[string]struct{})
		for _, t := range toolCalls {
			uniqueTools[t] = struct{}{}
		}
		metrics.RepetitionIndex = 1.0 - (float64(len(uniqueTools)) / float64(len(toolCalls)))
	}

	// --- 3. Token Velocity & Consumption Bursts ---
	var totalTokens int
	for _, ev := range events {
		toks := extractTokenCount(ev)
		totalTokens += toks

		// Per-step velocity check
		if ev.DurationMs > 0 && toks > 0 {
			durSec := float64(ev.DurationMs) / 1000.0
			stepVelocity := float64(toks) / durSec
			if stepVelocity > e.cfg.MaxTokenVelocity && toks > 500 {
				anomalies = append(anomalies, AnomalyEvent{
					Type:        AnomalyTokenVelocity,
					Severity:    SeverityCritical,
					StepIndex:   ev.Index,
					ToolName:    extractToolName(ev),
					Message:     fmt.Sprintf("Token burn rate spike at step %d: %.1f tokens/sec (burned %d tokens in %d ms)", ev.Index, stepVelocity, toks, ev.DurationMs),
					ObservedVal: stepVelocity,
					Threshold:   e.cfg.MaxTokenVelocity,
					Timestamp:   ev.Timestamp,
				})
			}
		}
	}
	metrics.TotalTokens = totalTokens

	if len(events) >= 2 {
		firstTime := events[0].Timestamp
		lastTime := events[len(events)-1].Timestamp
		totalSec := lastTime.Sub(firstTime).Seconds()
		if totalSec > 0.05 {
			metrics.TokenBurnRate = float64(totalTokens) / totalSec
		}
	}

	// --- 4. Error Cascades & Failure Storms ---
	consecutiveErrors := 0
	maxConsecutiveErrors := 0
	totalErrors := 0

	for _, ev := range events {
		isErr := isErrorEvent(ev)
		if isErr {
			totalErrors++
			consecutiveErrors++
			if consecutiveErrors > maxConsecutiveErrors {
				maxConsecutiveErrors = consecutiveErrors
			}
			if consecutiveErrors >= e.cfg.MaxConsecutiveErrors {
				anomalies = append(anomalies, AnomalyEvent{
					Type:        AnomalyErrorCascade,
					Severity:    SeverityCritical,
					StepIndex:   ev.Index,
					ToolName:    extractToolName(ev),
					Message:     fmt.Sprintf("Error cascade detected: %d consecutive failed tool steps without recovery", consecutiveErrors),
					ObservedVal: float64(consecutiveErrors),
					Threshold:   float64(e.cfg.MaxConsecutiveErrors),
					Timestamp:   ev.Timestamp,
				})
			}
		} else {
			consecutiveErrors = 0
		}
	}
	metrics.ConsecutiveErrors = maxConsecutiveErrors
	metrics.ErrorCascadeCount = totalErrors

	// --- 5. Risk Score Aggregation & Circuit Breaker ---
	riskScore := 0.0
	for _, a := range anomalies {
		switch a.Severity {
		case SeverityCritical:
			riskScore += 35.0
		case SeverityHigh:
			riskScore += 20.0
		case SeverityMedium:
			riskScore += 10.0
		case SeverityLow:
			riskScore += 5.0
		}
	}
	if riskScore > 100.0 {
		riskScore = 100.0
	}

	action := ActionPass
	healthy := true
	summary := "Trajectory health nominal. Zero critical anomalies detected."

	if riskScore >= e.cfg.TripRiskThreshold {
		action = ActionTripCircuitBreaker
		healthy = false
		summary = fmt.Sprintf("CIRCUIT BREAKER TRIPPED (Risk: %.1f/100). Halting agent execution to prevent runaway degradation.", riskScore)
	} else if riskScore >= e.cfg.ThrottleRiskThreshold {
		action = ActionThrottle
		healthy = false
		summary = fmt.Sprintf("TRAJECTORY THROTTLED (Risk: %.1f/100). Throttling execution velocity and flagging telemetry.", riskScore)
	} else if len(anomalies) > 0 {
		action = ActionPass
		healthy = true
		summary = fmt.Sprintf("Minor anomalies detected (Risk: %.1f/100). Execution permitted under observation.", riskScore)
	}

	return SentryVerdict{
		Healthy:         healthy,
		Action:          action,
		RiskScore:       riskScore,
		Summary:         summary,
		Anomalies:       anomalies,
		Metrics:         metrics,
		EvaluatedAt:     time.Now().UTC(),
		EvaluationDurUs: time.Since(start).Microseconds(),
	}
}

func extractToolName(ev trace.TraceEvent) string {
	if ev.Type != trace.EventToolCall && ev.Type != trace.EventToolResult {
		return ""
	}
	if val, ok := ev.Payload["tool_name"].(string); ok && val != "" {
		return val
	}
	if val, ok := ev.Payload["tool"].(string); ok && val != "" {
		return val
	}
	if val, ok := ev.Payload["name"].(string); ok && val != "" {
		return val
	}
	return ""
}

func extractTokenCount(ev trace.TraceEvent) int {
	if val, ok := ev.Payload["tokens"].(float64); ok {
		return int(val)
	}
	if val, ok := ev.Payload["tokens"].(int); ok {
		return val
	}
	if val, ok := ev.Payload["total_tokens"].(float64); ok {
		return int(val)
	}
	if val, ok := ev.Payload["total_tokens"].(int); ok {
		return val
	}
	if usage, ok := ev.Payload["usage"].(map[string]interface{}); ok {
		if val, ok := usage["total_tokens"].(float64); ok {
			return int(val)
		}
	}
	// Fallback to text length heuristic (1 token ~= 4 chars)
	if content, ok := ev.Payload["content"].(string); ok && len(content) > 0 {
		return len(content) / 4
	}
	if prompt, ok := ev.Payload["prompt"].(string); ok && len(prompt) > 0 {
		return len(prompt) / 4
	}
	return 0
}

func isErrorEvent(ev trace.TraceEvent) bool {
	if ev.Type == trace.EventError {
		return true
	}
	if errVal, ok := ev.Payload["error"]; ok && errVal != nil {
		if str, isStr := errVal.(string); isStr && str != "" {
			return true
		}
		if b, isBool := errVal.(bool); isBool && b {
			return true
		}
	}
	if status, ok := ev.Payload["status"].(string); ok && (strings.EqualFold(status, "error") || strings.EqualFold(status, "failed")) {
		return true
	}
	return false
}
