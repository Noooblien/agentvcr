package shadow

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// ShadowEngine coordinates continuous asynchronous shadowing, live comparison, and promotion gating.
type ShadowEngine struct {
	mu            sync.RWMutex
	Config        ShadowConfig
	BaselineModel ModelCandidate
	CandidateModel ModelCandidate
	Comparisons   []TrajectoryComparison
}

// NewShadowEngine initializes a production ShadowEngine.
func NewShadowEngine(cfg ShadowConfig, baseline, candidate ModelCandidate) *ShadowEngine {
	if cfg.MaxDriftThreshold <= 0 {
		cfg.MaxDriftThreshold = 0.25
	}
	if cfg.MaxLatencyRatio <= 0 {
		cfg.MaxLatencyRatio = 1.30
	}
	if cfg.MaxCostIncreaseRatio <= 0 {
		cfg.MaxCostIncreaseRatio = 1.20
	}
	if cfg.MaxAllowedErrorRate <= 0 {
		cfg.MaxAllowedErrorRate = 0.05
	}
	if cfg.MinSampleCount <= 0 {
		cfg.MinSampleCount = 1
	}

	return &ShadowEngine{
		Config:         cfg,
		BaselineModel:  baseline,
		CandidateModel: candidate,
		Comparisons:    make([]TrajectoryComparison, 0),
	}
}

// CompareTrajectories evaluates a baseline session against a shadowed candidate session.
func (se *ShadowEngine) CompareTrajectories(baseSession, candSession *trace.Session) TrajectoryComparison {
	se.mu.Lock()
	defer se.mu.Unlock()

	totalSteps := int(math.Min(float64(len(baseSession.Events)), float64(len(candSession.Events))))
	stepComps := make([]StepComparison, 0, totalSteps)

	var totalSimilarity float64
	var totalDrift float64
	var maxDrift float64
	var baseLatency int64
	var candLatency int64
	var baseTokens int
	var candTokens int
	var toolDivergence int

	for i := 0; i < totalSteps; i++ {
		comp := CompareEvents(i, baseSession.Events[i], candSession.Events[i])
		stepComps = append(stepComps, comp)

		totalSimilarity += comp.SemanticSimilarity
		totalDrift += comp.DriftScore
		if comp.DriftScore > maxDrift {
			maxDrift = comp.DriftScore
		}
		baseLatency += comp.BaselineLatencyMs
		candLatency += comp.CandidateLatencyMs
		baseTokens += comp.BaselineTokens
		candTokens += comp.CandidateTokens

		if comp.ToolCallDiverged {
			toolDivergence++
		}
	}

	avgSimilarity := 1.0
	avgDrift := 0.0
	if totalSteps > 0 {
		avgSimilarity = totalSimilarity / float64(totalSteps)
		avgDrift = totalDrift / float64(totalSteps)
	}

	latencyRatio := 1.0
	if baseLatency > 0 {
		latencyRatio = float64(candLatency) / float64(baseLatency)
	}

	tokenDeltaPercent := 0.0
	if baseTokens > 0 {
		tokenDeltaPercent = ((float64(candTokens) - float64(baseTokens)) / float64(baseTokens)) * 100.0
	}

	baseCost := (float64(baseTokens) / 1000.0) * se.BaselineModel.CostPer1kOutput
	candCost := (float64(candTokens) / 1000.0) * se.CandidateModel.CostPer1kOutput
	costDeltaPercent := 0.0
	if baseCost > 0 {
		costDeltaPercent = ((candCost - baseCost) / baseCost) * 100.0
	}

	// Gate evaluations
	rejectionReasons := make([]string, 0)
	if avgDrift > se.Config.MaxDriftThreshold {
		rejectionReasons = append(rejectionReasons,
			fmt.Sprintf("Semantic drift exceeded threshold (%.2f > %.2f)", avgDrift, se.Config.MaxDriftThreshold))
	}
	if latencyRatio > se.Config.MaxLatencyRatio {
		rejectionReasons = append(rejectionReasons,
			fmt.Sprintf("Latency ratio degraded beyond allowance (%.2fx > %.2fx)", latencyRatio, se.Config.MaxLatencyRatio))
	}
	if baseCost > 0 && (candCost/baseCost) > se.Config.MaxCostIncreaseRatio {
		rejectionReasons = append(rejectionReasons,
			fmt.Sprintf("Cost increase ratio exceeded budget (%.2fx > %.2fx)", candCost/baseCost, se.Config.MaxCostIncreaseRatio))
	}
	if toolDivergence > 0 && toolDivergence > totalSteps/2 {
		rejectionReasons = append(rejectionReasons,
			fmt.Sprintf("Severe tool invocation divergence (%d / %d steps diverged)", toolDivergence, totalSteps))
	}

	passed := len(rejectionReasons) == 0
	promotable := passed && avgDrift <= se.Config.MaxDriftThreshold

	comparison := TrajectoryComparison{
		SessionID:              baseSession.ID,
		BaselineModel:          se.BaselineModel.ModelName,
		CandidateModel:         se.CandidateModel.ModelName,
		TotalSteps:             totalSteps,
		AverageSemanticScore:   avgSimilarity,
		AverageDriftScore:      avgDrift,
		MaxStepDrift:           maxDrift,
		BaselineTotalLatencyMs: baseLatency,
		CandidateTotalLatencyMs: candLatency,
		LatencyRatio:           latencyRatio,
		BaselineTotalTokens:    baseTokens,
		CandidateTotalTokens:   candTokens,
		TokenDeltaPercent:      tokenDeltaPercent,
		BaselineEstimatedCost:  baseCost,
		CandidateEstimatedCost: candCost,
		CostDeltaPercent:       costDeltaPercent,
		ToolDivergenceCount:    toolDivergence,
		StepComparisons:        stepComps,
		Passed:                 passed,
		PromotionRecommended:   promotable,
		RejectionReasons:       rejectionReasons,
	}

	se.Comparisons = append(se.Comparisons, comparison)
	return comparison
}

// GenerateAuditReport aggregates all trajectory comparisons into an executive audit report.
func (se *ShadowEngine) GenerateAuditReport() ShadowAuditReport {
	se.mu.RLock()
	defer se.mu.RUnlock()

	var totalDrift float64
	var totalBaseLatency int64
	var totalCandLatency int64
	var totalBaseCost float64
	var totalCandCost float64
	var totalSteps int
	var errorCount int

	for _, c := range se.Comparisons {
		totalDrift += c.AverageDriftScore
		totalBaseLatency += c.BaselineTotalLatencyMs
		totalCandLatency += c.CandidateTotalLatencyMs
		totalBaseCost += c.BaselineEstimatedCost
		totalCandCost += c.CandidateEstimatedCost
		totalSteps += c.TotalSteps

		for _, s := range c.StepComparisons {
			if s.CandidateError {
				errorCount++
			}
		}
	}

	n := len(se.Comparisons)
	globalAvgDrift := 0.0
	globalLatencyRatio := 1.0
	globalCostDeltaPercent := 0.0
	overallErrorRate := 0.0

	if n > 0 {
		globalAvgDrift = totalDrift / float64(n)
	}
	if totalBaseLatency > 0 {
		globalLatencyRatio = float64(totalCandLatency) / float64(totalBaseLatency)
	}
	if totalBaseCost > 0 {
		globalCostDeltaPercent = ((totalCandCost - totalBaseCost) / totalBaseCost) * 100.0
	}
	if totalSteps > 0 {
		overallErrorRate = float64(errorCount) / float64(totalSteps)
	}

	circuitBreaker := false
	circuitReason := ""
	status := "PROMOTABLE"

	if overallErrorRate > se.Config.MaxAllowedErrorRate {
		circuitBreaker = true
		circuitReason = fmt.Sprintf("Candidate error rate %.2f%% exceeded ceiling %.2f%%",
			overallErrorRate*100.0, se.Config.MaxAllowedErrorRate*100.0)
		status = "ROLLED_BACK"
	} else if globalAvgDrift > se.Config.MaxDriftThreshold {
		circuitBreaker = true
		circuitReason = fmt.Sprintf("Global semantic drift %.2f exceeded threshold %.2f",
			globalAvgDrift, se.Config.MaxDriftThreshold)
		status = "BLOCKED"
	} else if n < se.Config.MinSampleCount {
		status = "OBSERVING"
	}

	recommendations := make([]string, 0)
	if status == "PROMOTABLE" {
		recommendations = append(recommendations,
			fmt.Sprintf("Candidate %s passed all safety, latency, and cost gates. Eligible for traffic ramp.", se.CandidateModel.ModelName))
		if globalCostDeltaPercent < 0 {
			recommendations = append(recommendations,
				fmt.Sprintf("Estimated cost reduction of %.1f%% with candidate model.", math.Abs(globalCostDeltaPercent)))
		}
	} else if circuitBreaker {
		recommendations = append(recommendations,
			fmt.Sprintf("Auto-rollback engaged: %s. Revert live canary split to 0%% immediately.", circuitReason))
	} else {
		recommendations = append(recommendations,
			fmt.Sprintf("Accumulate additional sample trajectories (%d / %d completed) before promotion.", n, se.Config.MinSampleCount))
	}

	return ShadowAuditReport{
		AuditID:                fmt.Sprintf("shadow-audit-%d", time.Now().Unix()),
		Timestamp:              time.Now(),
		Mode:                   se.Config.Mode,
		BaselineModel:          se.BaselineModel,
		CandidateModel:         se.CandidateModel,
		TotalSessionsShadowed:  n,
		TotalStepsEvaluated:    totalSteps,
		GlobalAverageDrift:     globalAvgDrift,
		GlobalLatencyRatio:     globalLatencyRatio,
		GlobalCostDeltaPercent: globalCostDeltaPercent,
		OverallErrorRate:       overallErrorRate,
		CircuitBreakerTripped:  circuitBreaker,
		CircuitBreakerReason:   circuitReason,
		Status:                 status,
		Sessions:               se.Comparisons,
		Recommendations:        recommendations,
	}
}
