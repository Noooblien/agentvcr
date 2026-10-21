package metrics

import (
	"math"
	"strings"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// TokenAuditor analyzes trajectory sessions to extract fine-grained token burn, prompt caching, and cost metrics.
type TokenAuditor struct {
	rates map[string]ModelRateCard
}

// NewTokenAuditor initializes a token auditor with standard rate cards.
func NewTokenAuditor() *TokenAuditor {
	return &TokenAuditor{
		rates: DefaultModelRateCards,
	}
}

// AuditSession computes detailed token metrics across all trajectory events.
func (a *TokenAuditor) AuditSession(session *trace.Session) *TokenAuditReport {
	report := &TokenAuditReport{
		SessionID:   session.ID,
		AgentName:   session.AgentName,
		StepMetrics: make([]StepTokenMetric, 0),
		AnalyzedAt:  time.Now(),
	}

	var totalPrompt, totalComp, totalCached, totalReasoning int
	var totalDurationMs int64
	currentModel := "default"

	for _, evt := range session.Events {
		if evt.DurationMs > 0 {
			totalDurationMs += evt.DurationMs
		}

		if evt.Type == trace.EventLLMRequest {
			if m, ok := evt.Payload["model"].(string); ok && m != "" {
				currentModel = strings.ToLower(m)
			}
		}

		if evt.Type == trace.EventLLMResponse {
			pTokens, cTokens, cachedTokens, rTokens := extractTokensFromPayload(evt.Payload)
			totalPrompt += pTokens
			totalComp += cTokens
			totalCached += cachedTokens
			totalReasoning += rTokens

			rate := a.lookupRateCard(currentModel)
			stepCost := (float64(pTokens-cachedTokens)/1000000.0)*rate.PromptPerMillion +
				(float64(cachedTokens)/1000000.0)*rate.CachedPerMillion +
				(float64(cTokens)/1000000.0)*rate.CompPerMillion

			report.StepMetrics = append(report.StepMetrics, StepTokenMetric{
				StepIndex:        evt.Index,
				Model:            currentModel,
				PromptTokens:     pTokens,
				CompletionTokens: cTokens,
				CachedTokens:     cachedTokens,
				DurationMs:       evt.DurationMs,
				CostUSD:          stepCost,
			})
		}
	}

	report.TotalSteps = len(report.StepMetrics)
	totalTokens := totalPrompt + totalComp

	var cacheHitRatio float64
	if totalPrompt > 0 {
		cacheHitRatio = float64(totalCached) / float64(totalPrompt)
	}

	var tokensPerSec float64
	if totalDurationMs > 0 {
		tokensPerSec = float64(totalTokens) / (float64(totalDurationMs) / 1000.0)
	}

	rate := a.lookupRateCard(currentModel)
	nonCachedPrompt := totalPrompt - totalCached
	if nonCachedPrompt < 0 {
		nonCachedPrompt = 0
	}

	costPrompt := (float64(nonCachedPrompt) / 1000000.0) * rate.PromptPerMillion
	costCached := (float64(totalCached) / 1000000.0) * rate.CachedPerMillion
	costComp := (float64(totalComp) / 1000000.0) * rate.CompPerMillion
	totalCost := costPrompt + costCached + costComp

	// Calculate savings realized by prompt cache hits
	fullPromptCostWithoutCache := (float64(totalPrompt) / 1000000.0) * rate.PromptPerMillion
	savingsUSD := fullPromptCostWithoutCache - (costPrompt + costCached)
	if savingsUSD < 0 {
		savingsUSD = 0
	}

	report.Breakdown = GranularTokenBreakdown{
		PromptTokens:          totalPrompt,
		CompletionTokens:      totalComp,
		CachedPromptTokens:    totalCached,
		ReasoningTokens:       totalReasoning,
		TotalTokens:           totalTokens,
		CacheHitRatio:         math.Round(cacheHitRatio*1000) / 1000,
		TokensPerSecond:       math.Round(tokensPerSec*10) / 10,
		EstimatedPromptCost:   costPrompt + costCached,
		EstimatedCompCost:     costComp,
		EstimatedSavingsUSD:   savingsUSD,
		TotalEstimatedCostUSD: totalCost,
	}

	// Calculate Token Efficiency Score (0.0 to 100.0)
	// Base score of 100, penalized by excessive token bloat per step or zero cache efficiency
	efficiencyScore := 100.0
	if report.TotalSteps > 0 {
		avgTokensPerStep := float64(totalTokens) / float64(report.TotalSteps)
		if avgTokensPerStep > 5000 {
			efficiencyScore -= 20.0
		}
	}
	// Bonus for prompt caching
	efficiencyScore += cacheHitRatio * 15.0
	if efficiencyScore > 100.0 {
		efficiencyScore = 100.0
	}
	if efficiencyScore < 0.0 {
		efficiencyScore = 0.0
	}
	report.TokenEfficiencyScore = math.Round(efficiencyScore*10) / 10

	verdict := "NOMINAL"
	if report.TokenEfficiencyScore >= 90.0 {
		verdict = "OPTIMIZED"
	} else if report.TokenEfficiencyScore < 60.0 {
		verdict = "TOKEN_BLOATED"
	}
	report.EfficiencyVerdict = verdict

	return report
}

func (a *TokenAuditor) lookupRateCard(model string) ModelRateCard {
	for k, card := range a.rates {
		if strings.Contains(strings.ToLower(model), k) {
			return card
		}
	}
	return a.rates["default"]
}

func extractTokensFromPayload(payload map[string]interface{}) (prompt, comp, cached, reasoning int) {
	if payload == nil {
		return
	}

	// 1. Direct usage
	if usage, ok := payload["usage"].(map[string]interface{}); ok {
		prompt, comp, cached, reasoning = parseUsageMap(usage)
		return
	}

	// 2. usage inside body
	if body, ok := payload["body"].(map[string]interface{}); ok {
		if usage, ok := body["usage"].(map[string]interface{}); ok {
			prompt, comp, cached, reasoning = parseUsageMap(usage)
			return
		}
	}

	return
}

func parseUsageMap(usage map[string]interface{}) (prompt, comp, cached, reasoning int) {
	if p, ok := usage["prompt_tokens"].(float64); ok {
		prompt = int(p)
	} else if p, ok := usage["input_tokens"].(float64); ok {
		prompt = int(p)
	}

	if c, ok := usage["completion_tokens"].(float64); ok {
		comp = int(c)
	} else if c, ok := usage["output_tokens"].(float64); ok {
		comp = int(c)
	}

	// Anthropic prompt caching / OpenAI cache read
	if cachedVal, ok := usage["cache_read_input_tokens"].(float64); ok {
		cached = int(cachedVal)
	} else if details, ok := usage["prompt_tokens_details"].(map[string]interface{}); ok {
		if cachedVal, ok := details["cached_tokens"].(float64); ok {
			cached = int(cachedVal)
		}
	}

	// OpenAI / Anthropic reasoning tokens
	if details, ok := usage["completion_tokens_details"].(map[string]interface{}); ok {
		if r, ok := details["reasoning_tokens"].(float64); ok {
			reasoning = int(r)
		}
	}

	return
}
