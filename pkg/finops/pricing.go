package finops

import (
	"fmt"
	"strings"
	"sync"
)

// PricingCalculator calculates precise microdollar LLM API costs based on model rate cards.
type PricingCalculator struct {
	mu     sync.RWMutex
	rates  map[string]ModelPricing
}

// NewPricingCalculator initializes calculator with standard default pricing.
func NewPricingCalculator() *PricingCalculator {
	return &PricingCalculator{
		rates: DefaultModelPricings(),
	}
}

// SetPricing registers or overrides a model pricing configuration.
func (pc *PricingCalculator) SetPricing(pricing ModelPricing) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	pc.rates[strings.ToLower(pricing.ModelName)] = pricing
}

// CalculateCost computes total USD expenditure for an individual token usage event.
func (pc *PricingCalculator) CalculateCost(model string, promptTokens, completionTokens, cachedTokens int) (float64, error) {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	modelKey := strings.ToLower(model)
	pricing, ok := pc.rates[modelKey]
	if !ok {
		// Fallback to closest prefix match or reasonable standard default
		for k, p := range pc.rates {
			if strings.HasPrefix(modelKey, k) {
				pricing = p
				ok = true
				break
			}
		}
		if !ok {
			// Default generic pricing: $0.002 input, $0.008 output per 1k
			pricing = ModelPricing{
				ModelName:           model,
				InputUSDPer1k:       0.002,
				OutputUSDPer1k:      0.008,
				CachedInputUSDPer1k: 0.001,
			}
		}
	}

	uncachedPrompt := promptTokens - cachedTokens
	if uncachedPrompt < 0 {
		uncachedPrompt = 0
	}

	costUncached := (float64(uncachedPrompt) / 1000.0) * pricing.InputUSDPer1k
	costCached := (float64(cachedTokens) / 1000.0) * pricing.CachedInputUSDPer1k
	costOutput := (float64(completionTokens) / 1000.0) * pricing.OutputUSDPer1k

	total := costUncached + costCached + costOutput
	return total, nil
}

// CalculateSavings estimates dollar savings achieved when downgrading from baseline to cheap target model.
func (pc *PricingCalculator) CalculateSavings(originalModel, downgradedModel string, promptTokens, completionTokens int) float64 {
	origCost, _ := pc.CalculateCost(originalModel, promptTokens, completionTokens, 0)
	downCost, _ := pc.CalculateCost(downgradedModel, promptTokens, completionTokens, 0)
	diff := origCost - downCost
	if diff < 0 {
		return 0
	}
	return diff
}

func (pc *PricingCalculator) String() string {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return fmt.Sprintf("PricingCalculator[SupportedModels=%d]", len(pc.rates))
}
