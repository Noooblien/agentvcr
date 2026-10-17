package gateway

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// DispatcherFunc allows mocking or virtualizing upstream HTTP transport for offline deterministic tests.
type DispatcherFunc func(ctx context.Context, provider *ProviderConfig, targetModel string, payload []byte) ([]byte, int, error)

// GatewayEngine is the high-performance model routing, resilience, and caching gateway.
type GatewayEngine struct {
	mu             sync.RWMutex
	systemID       string
	router         *Router
	cache          *SemanticReplayCache
	normalizer     *Normalizer
	breakers       map[string]*CircuitBreaker
	telemetry      map[string]*ProviderTelemetry
	providers      map[string]*ProviderConfig
	dispatcher     DispatcherFunc
	httpClient     *http.Client
	totalRequests  int64
	totalFailovers int64
}

// NewGatewayEngine creates a new gateway engine with default components.
func NewGatewayEngine(systemID string) *GatewayEngine {
	if systemID == "" {
		systemID = "agentvcr-gateway"
	}

	return &GatewayEngine{
		systemID:   systemID,
		router:     NewRouter(),
		cache:      NewSemanticReplayCache(5000, 24*time.Hour),
		normalizer: NewNormalizer(),
		breakers:   make(map[string]*CircuitBreaker),
		telemetry:  make(map[string]*ProviderTelemetry),
		providers:  make(map[string]*ProviderConfig),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// SetDispatcher overrides the network transport dispatcher (crucial for deterministic unit tests).
func (ge *GatewayEngine) SetDispatcher(fn DispatcherFunc) {
	ge.mu.Lock()
	defer ge.mu.Unlock()
	ge.dispatcher = fn
}

// Router returns the underlying semantic router.
func (ge *GatewayEngine) Router() *Router {
	return ge.router
}

// Cache returns the underlying semantic cache.
func (ge *GatewayEngine) Cache() *SemanticReplayCache {
	return ge.cache
}

// RegisterProvider wires a new upstream provider into the router, circuit breakers, and telemetry.
func (ge *GatewayEngine) RegisterProvider(cfg *ProviderConfig) {
	ge.mu.Lock()
	defer ge.mu.Unlock()

	ge.providers[cfg.ID] = cfg
	ge.router.RegisterProvider(cfg)
	ge.breakers[cfg.ID] = NewCircuitBreaker(cfg.ID, cfg.CircuitConfig)
	ge.telemetry[cfg.ID] = &ProviderTelemetry{
		ProviderID:      cfg.ID,
		ProviderName:    cfg.Name,
		CircuitState:    CircuitClosed,
		LastStateChange: time.Now(),
	}
}

// GetCircuitBreaker returns the circuit breaker instance for a provider.
func (ge *GatewayEngine) GetCircuitBreaker(providerID string) *CircuitBreaker {
	ge.mu.RLock()
	defer ge.mu.RUnlock()
	return ge.breakers[providerID]
}

// Handle processes an incoming agent model request through the semantic cache, router, and resilient cascades.
func (ge *GatewayEngine) Handle(ctx context.Context, req *GatewayRequest) (*GatewayResponse, error) {
	atomic.AddInt64(&ge.totalRequests, 1)

	// 1. Check deterministic semantic replay cache
	if !req.ForceFallback {
		if cachedResp, hit := ge.cache.Get(req); hit {
			return cachedResp, nil
		}
	}

	// 2. Classify task complexity and determine model tier
	tier := ge.router.ClassifyTier(req)
	cascade := ge.router.ResolveCascade(tier)
	if len(cascade) == 0 {
		return nil, fmt.Errorf("no upstream providers configured for tier: %s", tier)
	}

	var fallbackChain []string
	startOverall := time.Now()

	// 3. Iterate through provider cascade with circuit-breaker protection
	for i, provider := range cascade {
		cb := ge.GetCircuitBreaker(provider.ID)
		if cb == nil {
			continue
		}

		// Verify circuit breaker allows traffic
		if !cb.Allow() {
			fallbackChain = append(fallbackChain, fmt.Sprintf("%s [SKIPPED: CIRCUIT_OPEN]", provider.ID))
			continue
		}

		targetModel := provider.Models[tier]
		if targetModel == "" {
			// Fallback to requested model or first available model
			targetModel = req.RequestedModel
			for _, m := range provider.Models {
				targetModel = m
				break
			}
		}

		payload, err := ge.normalizer.FormatRequest(req, provider, targetModel)
		if err != nil {
			fallbackChain = append(fallbackChain, fmt.Sprintf("%s [ERR: FORMAT %v]", provider.ID, err))
			continue
		}

		// Check simulated failure injection
		if i == 0 && req.ForceFallback {
			err = fmt.Errorf("simulated upstream outage: HTTP 429 Too Many Requests")
		} else {
			startReq := time.Now()
			var rawResp []byte
			var statusCode int
			rawResp, statusCode, err = ge.dispatch(ctx, provider, targetModel, payload)
			latency := time.Since(startReq).Seconds() * 1000.0

			if err == nil && (statusCode < 200 || statusCode >= 300) {
				err = fmt.Errorf("upstream returned HTTP error status %d", statusCode)
			}

			if err == nil {
				// Success! Record breaker status and update telemetry
				cb.RecordSuccess()

				parsedResp, parseErr := ge.normalizer.ParseResponse(rawResp, provider, targetModel)
				if parseErr != nil {
					cb.RecordFailure(parseErr)
					fallbackChain = append(fallbackChain, fmt.Sprintf("%s [ERR: PARSE %v]", provider.ID, parseErr))
					continue
				}

				parsedResp.LatencyMs = latency
				parsedResp.CostUSD = ge.router.EstimateCost(provider, parsedResp.Usage)
				if i > 0 {
					atomic.AddInt64(&ge.totalFailovers, 1)
				}
				fallbackChain = append(fallbackChain, fmt.Sprintf("%s (%s) [OK: %.1fms]", provider.ID, targetModel, latency))
				parsedResp.FallbackChain = fallbackChain

				// Update provider telemetry
				ge.recordSuccessTelemetry(provider.ID, latency, parsedResp.CostUSD)

				// Cache idempotent response
				ge.cache.Put(req, parsedResp)

				return parsedResp, nil
			}
		}

		// Failure handling: trip breaker and escalate to next cascade candidate
		cb.RecordFailure(err)
		ge.recordFailureTelemetry(provider.ID, err)
		fallbackChain = append(fallbackChain, fmt.Sprintf("%s (%s) [FAIL: %v]", provider.ID, targetModel, err))
	}

	totalDuration := time.Since(startOverall).Seconds() * 1000.0
	return nil, fmt.Errorf("gateway cascade exhausted after %.1fms across all providers: %v", totalDuration, fallbackChain)
}

func (ge *GatewayEngine) dispatch(ctx context.Context, provider *ProviderConfig, targetModel string, payload []byte) ([]byte, int, error) {
	ge.mu.RLock()
	customDispatcher := ge.dispatcher
	ge.mu.RUnlock()

	if customDispatcher != nil {
		return customDispatcher(ctx, provider, targetModel, payload)
	}

	// Live HTTP dispatch
	req, err := http.NewRequestWithContext(ctx, "POST", provider.Endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if provider.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+provider.APIKey)
	}

	resp, err := ge.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}

	return body, resp.StatusCode, nil
}

func (ge *GatewayEngine) recordSuccessTelemetry(providerID string, latencyMs, costUSD float64) {
	ge.mu.Lock()
	defer ge.mu.Unlock()

	tel := ge.telemetry[providerID]
	if tel == nil {
		return
	}

	tel.TotalRequests++
	tel.SuccessfulRequests++
	tel.ConsecutiveFailures = 0
	tel.ConsecutiveSuccess++
	tel.TotalCostUSD += costUSD
	if tel.AvgLatencyMs == 0 {
		tel.AvgLatencyMs = latencyMs
	} else {
		tel.AvgLatencyMs = (tel.AvgLatencyMs * 0.8) + (latencyMs * 0.2) // Exponential moving average
	}
	if cb := ge.breakers[providerID]; cb != nil {
		tel.CircuitState = cb.State()
	}
}

func (ge *GatewayEngine) recordFailureTelemetry(providerID string, err error) {
	ge.mu.Lock()
	defer ge.mu.Unlock()

	tel := ge.telemetry[providerID]
	if tel == nil {
		return
	}

	tel.TotalRequests++
	tel.FailedRequests++
	tel.ConsecutiveFailures++
	tel.ConsecutiveSuccess = 0
	if err != nil {
		tel.LastError = err.Error()
	}
	if cb := ge.breakers[providerID]; cb != nil {
		tel.CircuitState = cb.State()
	}
}

// GenerateReport produces a complete audit scorecard of gateway routing and reliability.
func (ge *GatewayEngine) GenerateReport() *GatewayReport {
	ge.mu.RLock()
	defer ge.mu.RUnlock()

	var providerStats []ProviderTelemetry
	var totalCostUSD float64

	for _, p := range ge.providers {
		tel := ge.telemetry[p.ID]
		if tel != nil {
			stat := *tel
			if cb := ge.breakers[p.ID]; cb != nil {
				stat.CircuitState = cb.State()
			}
			providerStats = append(providerStats, stat)
			totalCostUSD += stat.TotalCostUSD
		}
	}

	cacheStats := ge.cache.Stats()

	return &GatewayReport{
		SystemID:        ge.systemID,
		Timestamp:       time.Now(),
		ActiveProviders: len(ge.providers),
		TotalRequests:   atomic.LoadInt64(&ge.totalRequests),
		TotalFailovers:  atomic.LoadInt64(&ge.totalFailovers),
		TotalCostUSD:    totalCostUSD,
		TotalSavingsUSD: cacheStats.SavedCostUSD,
		CacheStats:      cacheStats,
		ProviderStats:   providerStats,
	}
}
