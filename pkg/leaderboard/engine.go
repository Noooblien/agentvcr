package leaderboard

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/Noooblien/agentvcr/pkg/metrics"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

// EvaluateSessionScorecard analyzes a session's trajectory to generate comprehensive benchmark metrics.
func EvaluateSessionScorecard(session *trace.Session) AgentScorecard {
	if session == nil {
		return AgentScorecard{}
	}

	sessionMetrics := metrics.ComputeSessionMetrics(session)

	model := "gpt-4o"
	if m, ok := session.Metadata["model"]; ok && m != "" {
		model = m
	} else {
		for _, ev := range session.Events {
			if ev.Payload != nil {
				if mdl, ok := ev.Payload["model"].(string); ok && mdl != "" {
					model = mdl
					break
				}
				if body, ok := ev.Payload["body"].(map[string]interface{}); ok {
					if mdl, ok := body["model"].(string); ok && mdl != "" {
						model = mdl
						break
					}
				}
			}
		}
	}

	version := "v1.0.0"
	if v, ok := session.Metadata["version"]; ok && v != "" {
		version = v
	} else if c, ok := session.Metadata["commit"]; ok && c != "" {
		version = c
	}

	// Calculate latency percentiles
	var latencies []int64
	for _, ev := range session.Events {
		if ev.DurationMs > 0 {
			latencies = append(latencies, ev.DurationMs)
		}
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })

	var p50, p95 int64
	if len(latencies) > 0 {
		p50Index := int(math.Floor(float64(len(latencies)-1) * 0.50))
		p95Index := int(math.Floor(float64(len(latencies)-1) * 0.95))
		p50 = latencies[p50Index]
		p95 = latencies[p95Index]
	}

	// Track tool calls, errors, and safety
	toolCalls := 0
	toolErrors := 0
	generalErrors := 0

	for _, ev := range session.Events {
		if ev.Type == trace.EventToolCall {
			toolCalls++
		}
		if ev.Type == trace.EventError {
			generalErrors++
		}
		if ev.Type == trace.EventToolResult {
			if errVal, exists := ev.Payload["error"]; exists && errVal != nil && errVal != "" {
				toolErrors++
			} else if status, exists := ev.Payload["status"]; exists && status == "error" {
				toolErrors++
			}
		}
	}

	// Success rate calculation
	totalSteps := len(session.Events)
	successRate := 100.0
	if totalSteps > 0 {
		failedSteps := generalErrors + toolErrors
		if failedSteps > totalSteps {
			failedSteps = totalSteps
		}
		successRate = (float64(totalSteps-failedSteps) / float64(totalSteps)) * 100.0
	}

	// Tool accuracy rate
	toolAccuracy := 100.0
	if toolCalls > 0 {
		toolSuccess := toolCalls - toolErrors
		if toolSuccess < 0 {
			toolSuccess = 0
		}
		toolAccuracy = (float64(toolSuccess) / float64(toolCalls)) * 100.0
	}

	// Safety score: deduct 25 points per error or breach
	safetyScore := 100.0 - float64(generalErrors*25)
	if safetyScore < 0.0 {
		safetyScore = 0.0
	}

	// Composite benchmark score calculation:
	// 40% Success Rate + 20% Tool Accuracy + 20% Safety + 10% Efficiency + 10% Speed
	efficiencyIndex := 100.0 - (float64(sessionMetrics.TotalTokens) / 150.0)
	if efficiencyIndex < 0.0 {
		efficiencyIndex = 0.0
	} else if efficiencyIndex > 100.0 {
		efficiencyIndex = 100.0
	}

	latencyIndex := 100.0 - (float64(p95) / 50.0)
	if latencyIndex < 0.0 {
		latencyIndex = 0.0
	} else if latencyIndex > 100.0 {
		latencyIndex = 100.0
	}

	composite := (successRate * 0.40) +
		(toolAccuracy * 0.20) +
		(safetyScore * 0.20) +
		(efficiencyIndex * 0.10) +
		(latencyIndex * 0.10)

	if composite > 100.0 {
		composite = 100.0
	} else if composite < 0.0 {
		composite = 0.0
	}

	return AgentScorecard{
		SessionID:        session.ID,
		AgentName:        session.AgentName,
		Model:            model,
		Version:          version,
		SuccessRate:      math.Round(successRate*100) / 100,
		TotalEvents:      totalSteps,
		TotalTokens:      sessionMetrics.TotalTokens,
		PromptTokens:     sessionMetrics.PromptTokens,
		CompletionTokens: sessionMetrics.CompletionTokens,
		EstimatedCostUSD: sessionMetrics.EstimatedCostUSD,
		LatencyP50Ms:     p50,
		LatencyP95Ms:     p95,
		TotalDurationMs:  sessionMetrics.TotalDurationMs,
		ToolCallCount:    toolCalls,
		ToolErrorCount:   toolErrors,
		ToolAccuracyRate: math.Round(toolAccuracy*100) / 100,
		SafetyScore:      math.Round(safetyScore*100) / 100,
		CompositeScore:   math.Round(composite*100) / 100,
	}
}

// BuildLeaderboard aggregates and ranks multiple agent sessions into a unified competitive benchmark table.
func BuildLeaderboard(sessions []*trace.Session, suiteName string) *Leaderboard {
	if suiteName == "" {
		suiteName = "General Autonomous Engineering Benchmark Suite"
	}

	var scorecards []AgentScorecard
	for _, sess := range sessions {
		scorecards = append(scorecards, EvaluateSessionScorecard(sess))
	}

	// Sort scorecards by CompositeScore descending
	sort.Slice(scorecards, func(i, j int) bool {
		return scorecards[i].CompositeScore > scorecards[j].CompositeScore
	})

	var totalSuccess float64
	var totalTokens int
	var totalCost float64
	var totalDuration int64

	for i := range scorecards {
		scorecards[i].Rank = i + 1
		totalSuccess += scorecards[i].SuccessRate
		totalTokens += scorecards[i].TotalTokens
		totalCost += scorecards[i].EstimatedCostUSD
		totalDuration += scorecards[i].TotalDurationMs
	}

	count := len(scorecards)
	topAgent := "N/A"
	if count > 0 {
		topAgent = scorecards[0].AgentName
	}

	avgSuccess := 0.0
	avgTokens := 0
	avgCost := 0.0
	avgDuration := int64(0)
	if count > 0 {
		avgSuccess = math.Round((totalSuccess/float64(count))*100) / 100
		avgTokens = totalTokens / count
		avgCost = totalCost / float64(count)
		avgDuration = totalDuration / int64(count)
	}

	return &Leaderboard{
		SuiteName:          suiteName,
		GeneratedAt:        time.Now(),
		TotalEntries:       count,
		TopAgent:           topAgent,
		AverageSuccessRate: avgSuccess,
		AverageTokens:      avgTokens,
		AverageCostUSD:     avgCost,
		AverageDurationMs:  avgDuration,
		Entries:            scorecards,
	}
}

// DiffSessions performs regression analysis between a baseline trajectory and a candidate trajectory.
func DiffSessions(baseline, candidate *trace.Session, gateConfig *GateConfig) *RegressionDiff {
	cfg := DefaultGateConfig()
	if gateConfig != nil {
		cfg = *gateConfig
	}

	baseCard := EvaluateSessionScorecard(baseline)
	candCard := EvaluateSessionScorecard(candidate)

	deltaSuccess := math.Round((candCard.SuccessRate-baseCard.SuccessRate)*100) / 100
	deltaTokens := candCard.TotalTokens - baseCard.TotalTokens
	deltaCost := candCard.EstimatedCostUSD - baseCard.EstimatedCostUSD
	deltaDuration := candCard.TotalDurationMs - baseCard.TotalDurationMs
	deltaP95 := candCard.LatencyP95Ms - baseCard.LatencyP95Ms
	deltaToolErrors := candCard.ToolErrorCount - baseCard.ToolErrorCount
	deltaComposite := math.Round((candCard.CompositeScore-baseCard.CompositeScore)*100) / 100

	var reasons []string

	if deltaSuccess < -cfg.MaxSuccessRateDropPercent {
		reasons = append(reasons, fmt.Sprintf("Success rate regression: dropped by %.2f%% (allowed: %.2f%%)", -deltaSuccess, cfg.MaxSuccessRateDropPercent))
	}

	if baseCard.TotalTokens > 0 {
		tokenInflationRatio := float64(deltaTokens) / float64(baseCard.TotalTokens)
		if tokenInflationRatio > cfg.MaxTokenCostIncreaseRatio {
			reasons = append(reasons, fmt.Sprintf("Token cost inflation: increased by %.1f%% (ceiling: %.1f%%)", tokenInflationRatio*100, cfg.MaxTokenCostIncreaseRatio*100))
		}
	}

	if deltaP95 > cfg.MaxLatencyIncreaseMs {
		reasons = append(reasons, fmt.Sprintf("Latency degradation: P95 latency increased by %dms (ceiling: %dms)", deltaP95, cfg.MaxLatencyIncreaseMs))
	}

	if candCard.ToolErrorCount > baseCard.ToolErrorCount+cfg.MaxAllowedToolErrors {
		reasons = append(reasons, fmt.Sprintf("Tool call failure increase: %d additional tool failures detected", deltaToolErrors))
	}

	if cfg.RequireZeroSafetyBreaches && candCard.SafetyScore < baseCard.SafetyScore {
		reasons = append(reasons, fmt.Sprintf("Safety regression: safety score dropped from %.1f%% to %.1f%%", baseCard.SafetyScore, candCard.SafetyScore))
	}

	if deltaComposite < -5.0 {
		reasons = append(reasons, fmt.Sprintf("Composite benchmark index declined by %.2f points", -deltaComposite))
	}

	regressionDetected := len(reasons) > 0

	return &RegressionDiff{
		BaselineSessionID:   baseline.ID,
		CandidateSessionID:  candidate.ID,
		BaselineAgent:       baseline.AgentName,
		CandidateAgent:      candidate.AgentName,
		BaselineScorecard:   baseCard,
		CandidateScorecard:  candCard,
		DeltaSuccessRate:    deltaSuccess,
		DeltaTotalTokens:    deltaTokens,
		DeltaCostUSD:        deltaCost,
		DeltaDurationMs:     deltaDuration,
		DeltaLatencyP95Ms:   deltaP95,
		DeltaToolErrors:     deltaToolErrors,
		DeltaCompositeScore: deltaComposite,
		RegressionDetected:  regressionDetected,
		RegressionReasons:   reasons,
		PassGate:            !regressionDetected,
	}
}

// RenderLeaderboardTable renders an ANSI-formatted leaderboard ranking scorecard.
func RenderLeaderboardTable(lb *Leaderboard) string {
	var sb strings.Builder

	sb.WriteString("\n\033[1;36m========================================================================================\033[0m\n")
	sb.WriteString(fmt.Sprintf("\033[1;37m   AGENTVCR AGENT BENCHMARK LEADERBOARD & PERFORMANCE RANKINGS\033[0m\n"))
	sb.WriteString("\033[1;36m========================================================================================\033[0m\n")
	sb.WriteString(fmt.Sprintf("  \033[1;33mSuite Name:\033[0m       %s\n", lb.SuiteName))
	sb.WriteString(fmt.Sprintf("  \033[1;33mContenders:\033[0m       %d agents evaluated\n", lb.TotalEntries))
	sb.WriteString(fmt.Sprintf("  \033[1;33mTop Ranked:\033[0m       \033[1;32m%s\033[0m\n", lb.TopAgent))
	sb.WriteString(fmt.Sprintf("  \033[1;33mSuite Averages:\033[0m   Success: %.1f%% | Avg Tokens: %d | Avg Cost: $%.5f\n", lb.AverageSuccessRate, lb.AverageTokens, lb.AverageCostUSD))
	sb.WriteString("\033[1;34m----------------------------------------------------------------------------------------\033[0m\n")

	sb.WriteString("\033[1;37m  RANK  AGENT NAME           MODEL          SUCCESS   TOKENS   P95 LATENCY  SAFETY   SCORE\033[0m\n")

	for _, e := range lb.Entries {
		badge := fmt.Sprintf("#%d", e.Rank)
		if e.Rank == 1 {
			badge = "🥇 #1"
		} else if e.Rank == 2 {
			badge = "🥈 #2"
		} else if e.Rank == 3 {
			badge = "🥉 #3"
		}

		agentName := e.AgentName
		if len(agentName) > 18 {
			agentName = agentName[:15] + "..."
		}
		modelName := e.Model
		if len(modelName) > 14 {
			modelName = modelName[:11] + "..."
		}

		sb.WriteString(fmt.Sprintf("  %-5s %-20s %-14s %6.1f%%   %-8d %6dms    %5.1f%%  \033[1;32m%5.1f\033[0m\n",
			badge, agentName, modelName, e.SuccessRate, e.TotalTokens, e.LatencyP95Ms, e.SafetyScore, e.CompositeScore))
	}

	sb.WriteString("\033[1;36m========================================================================================\033[0m\n\n")
	return sb.String()
}

// RenderRegressionDiffCard formats the head-to-head regression diff for terminal inspection.
func RenderRegressionDiffCard(diff *RegressionDiff) string {
	var sb strings.Builder

	sb.WriteString("\n\033[1;36m========================================================================================\033[0m\n")
	sb.WriteString(fmt.Sprintf("\033[1;37m   AGENTVCR AGENT REGRESSION DIFF & CI/CD DEPLOYMENT GATE REPORT\033[0m\n"))
	sb.WriteString("\033[1;36m========================================================================================\033[0m\n")

	sb.WriteString(fmt.Sprintf("  \033[1;33mBaseline Trajectory:\033[0m  %s (%s)\n", diff.BaselineSessionID, diff.BaselineAgent))
	sb.WriteString(fmt.Sprintf("  \033[1;33mCandidate Trajectory:\033[0m %s (%s)\n", diff.CandidateSessionID, diff.CandidateAgent))

	if diff.PassGate {
		sb.WriteString("  \033[1;33mCI/CD Gate Status:\033[0m    \033[1;32m[PASS] APPROVED FOR PRODUCTION (NO REGRESSIONS)\033[0m\n")
	} else {
		sb.WriteString(fmt.Sprintf("  \033[1;33mCI/CD Gate Status:\033[0m    \033[1;31m[FAIL] REGRESSION DETECTED (BLOCK DEPLOYMENT)\033[0m\n"))
	}
	sb.WriteString("\033[1;34m----------------------------------------------------------------------------------------\033[0m\n")

	sb.WriteString("\033[1;37m  METRIC                    BASELINE           CANDIDATE          DELTA\033[0m\n")

	// Helper for format deltas
	renderDeltaFloat := func(label string, base, cand, delta float64, suffix string, invertGood bool) {
		color := "\033[1;32m" // green
		if (!invertGood && delta < 0) || (invertGood && delta > 0) {
			color = "\033[1;31m" // red
		}
		sign := "+"
		if delta < 0 {
			sign = ""
		}
		sb.WriteString(fmt.Sprintf("  %-25s %8.2f%-4s       %8.2f%-4s       %s%s%.2f%s\033[0m\n",
			label, base, suffix, cand, suffix, color, sign, delta, suffix))
	}

	renderDeltaInt := func(label string, base, cand int, delta int, suffix string, invertGood bool) {
		color := "\033[1;32m"
		if (!invertGood && delta < 0) || (invertGood && delta > 0) {
			color = "\033[1;31m"
		}
		sign := "+"
		if delta < 0 {
			sign = ""
		}
		sb.WriteString(fmt.Sprintf("  %-25s %8d%-4s       %8d%-4s       %s%s%d%s\033[0m\n",
			label, base, suffix, cand, suffix, color, sign, delta, suffix))
	}

	renderDeltaFloat("Success Rate", diff.BaselineScorecard.SuccessRate, diff.CandidateScorecard.SuccessRate, diff.DeltaSuccessRate, "%", false)
	renderDeltaInt("Total Tokens", diff.BaselineScorecard.TotalTokens, diff.CandidateScorecard.TotalTokens, diff.DeltaTotalTokens, "", true)
	renderDeltaFloat("Estimated Cost", diff.BaselineScorecard.EstimatedCostUSD, diff.CandidateScorecard.EstimatedCostUSD, diff.DeltaCostUSD, "$", true)
	renderDeltaInt("Latency P95", int(diff.BaselineScorecard.LatencyP95Ms), int(diff.CandidateScorecard.LatencyP95Ms), int(diff.DeltaLatencyP95Ms), "ms", true)
	renderDeltaInt("Tool Errors", diff.BaselineScorecard.ToolErrorCount, diff.CandidateScorecard.ToolErrorCount, diff.DeltaToolErrors, "", true)
	renderDeltaFloat("Composite Score", diff.BaselineScorecard.CompositeScore, diff.CandidateScorecard.CompositeScore, diff.DeltaCompositeScore, "", false)

	if len(diff.RegressionReasons) > 0 {
		sb.WriteString("\033[1;34m----------------------------------------------------------------------------------------\033[0m\n")
		sb.WriteString("\033[1;31m  REGRESSION VIOLATIONS DETECTED:\033[0m\n")
		for _, r := range diff.RegressionReasons {
			sb.WriteString(fmt.Sprintf("  \033[1;31m[!] %s\033[0m\n", r))
		}
	}

	sb.WriteString("\033[1;36m========================================================================================\033[0m\n\n")
	return sb.String()
}
