package gateway

import (
	"strings"
	"sync"
)

// Router handles semantic task classification, tier resolution, and provider cascade selection.
type Router struct {
	mu           sync.RWMutex
	providers    map[string]*ProviderConfig
	tierCascades map[ModelTier][]string // Tier -> Ordered list of Provider IDs
}

// NewRouter creates an initialized semantic router.
func NewRouter() *Router {
	return &Router{
		providers: make(map[string]*ProviderConfig),
		tierCascades: map[ModelTier][]string{
			TierFast:      {},
			TierStandard:  {},
			TierReasoning: {},
		},
	}
}

// RegisterProvider registers a provider configuration with the router.
func (r *Router) RegisterProvider(cfg *ProviderConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.providers[cfg.ID] = cfg

	// Register in default cascades if not present
	for tier := range cfg.Models {
		list := r.tierCascades[tier]
		found := false
		for _, id := range list {
			if id == cfg.ID {
				found = true
				break
			}
		}
		if !found {
			r.tierCascades[tier] = append(r.tierCascades[tier], cfg.ID)
		}
	}
}

// SetCascade configures an explicit ordered list of provider IDs for a specific tier.
func (r *Router) SetCascade(tier ModelTier, providerIDs []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tierCascades[tier] = providerIDs
}

// ClassifyTier analyzes prompt contents, token size, and function requirements to resolve optimal tier.
func (r *Router) ClassifyTier(req *GatewayRequest) ModelTier {
	if req.Tier != "" {
		return req.Tier
	}

	// 1. Check if model name explicitly indicates a tier
	reqModelLower := strings.ToLower(req.RequestedModel)
	if strings.Contains(reqModelLower, "mini") || strings.Contains(reqModelLower, "haiku") || strings.Contains(reqModelLower, "flash") {
		return TierFast
	}
	if strings.Contains(reqModelLower, "o1") || strings.Contains(reqModelLower, "o3") || strings.Contains(reqModelLower, "opus") || strings.Contains(reqModelLower, "r1") {
		return TierReasoning
	}

	// 2. Estimate token count and scan for reasoning keywords
	totalChars := 0
	hasDeepReasoningKeyword := false
	reasoningKeywords := []string{
		"prove", "formal verification", "tlaplus", "mathematical proof",
		"architectural refactor", "security exploit", "zero-day", "byzantine fault",
		"step-by-step reasoning", "deep analysis", "derivation",
	}

	for _, msg := range req.Messages {
		totalChars += len(msg.Content)
		contentLower := strings.ToLower(msg.Content)
		for _, kw := range reasoningKeywords {
			if strings.Contains(contentLower, kw) {
				hasDeepReasoningKeyword = true
				break
			}
		}
	}

	approxTokens := totalChars / 4

	if hasDeepReasoningKeyword || approxTokens > 6000 {
		return TierReasoning
	}

	if len(req.Tools) > 0 || req.RequireTool || approxTokens > 1500 {
		return TierStandard
	}

	return TierFast
}

// ResolveCascade returns the ordered list of provider configurations to attempt for a request.
func (r *Router) ResolveCascade(tier ModelTier) []*ProviderConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	providerIDs := r.tierCascades[tier]
	var cascade []*ProviderConfig

	for _, id := range providerIDs {
		if p, ok := r.providers[id]; ok {
			cascade = append(cascade, p)
		}
	}

	// If cascade is empty, fallback to any available provider supporting the tier
	if len(cascade) == 0 {
		for _, p := range r.providers {
			if _, ok := p.Models[tier]; ok {
				cascade = append(cascade, p)
			}
		}
	}

	return cascade
}

// EstimateCost calculates approximate cost in USD for a given token usage and provider.
func (r *Router) EstimateCost(provider *ProviderConfig, usage TokenUsage) float64 {
	inputCost := (float64(usage.PromptTokens) / 1000.0) * provider.CostPerInput1kUSD
	outputCost := (float64(usage.CompletionTokens) / 1000.0) * provider.CostPerOutput1kUSD
	return inputCost + outputCost
}
