package mcp

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// LoadBalancingPolicy defines the routing strategy across pool servers.
type LoadBalancingPolicy string

const (
	PolicyRoundRobin        LoadBalancingPolicy = "round_robin"
	PolicyLeastConnections  LoadBalancingPolicy = "least_connections"
	PolicyLatencyOptimized  LoadBalancingPolicy = "latency_optimized"
	PolicyRandom            LoadBalancingPolicy = "random"
)

// CircuitBreakerState represents the operational status of an upstream server circuit breaker.
type CircuitBreakerState string

const (
	CircuitClosed   CircuitBreakerState = "closed"   // Normal operation
	CircuitOpen     CircuitBreakerState = "open"     // Failing, traffic blocked
	CircuitHalfOpen CircuitBreakerState = "half_open" // Testing recovery
)

// UpstreamServer represents a managed backend MCP server instance.
type UpstreamServer struct {
	mu             sync.RWMutex
	ID             string             `json:"id"`
	Name           string             `json:"name"`
	Endpoint       string             `json:"endpoint"`
	Weight         int                `json:"weight"`
	ServerInstance *VirtualMCPServer  `json:"-"`
	Healthy        bool               `json:"healthy"`
	ActiveRequests int64              `json:"active_requests"`
	TotalRequests  int64              `json:"total_requests"`
	FailedRequests int64              `json:"failed_requests"`
	AverageRTTMs   float64            `json:"average_rtt_ms"`
	CircuitState   CircuitBreakerState `json:"circuit_state"`
	FailuresStreak int                `json:"failures_streak"`
	LastFailure    time.Time          `json:"last_failure"`
}

// RouterConfig defines configuration for the MCP multi-server router and load balancer.
type RouterConfig struct {
	Policy               LoadBalancingPolicy `json:"policy"`
	CircuitThreshold     int                 `json:"circuit_threshold"`      // Consecutive failures to open circuit
	CircuitCooldown      time.Duration       `json:"circuit_cooldown"`       // Time before half-open probe
	MaxRetries           int                 `json:"max_retries"`            // Failover retry attempts
	HealthCheckInterval  time.Duration       `json:"health_check_interval"`
	StripPrefix          bool                `json:"strip_prefix"`
}

// DefaultRouterConfig returns production defaults for the MCP load balancer.
func DefaultRouterConfig() RouterConfig {
	return RouterConfig{
		Policy:              PolicyRoundRobin,
		CircuitThreshold:    3,
		CircuitCooldown:     2 * time.Second,
		MaxRetries:          2,
		HealthCheckInterval: 10 * time.Second,
		StripPrefix:         true,
	}
}

// ToolRoute maps a tool namespace or pattern to target upstream server pool.
type ToolRoute struct {
	Pattern       string   `json:"pattern"`        // Tool name or prefix (e.g. "github_*", "bash", "*")
	TargetServers []string `json:"target_servers"` // Upstream server IDs
}

// Router dispatches and load-balances MCP JSON-RPC frames across virtual/remote MCP servers.
type Router struct {
	mu           sync.RWMutex
	Config       RouterConfig
	servers      map[string]*UpstreamServer
	routes       []ToolRoute
	roundRobinIdx uint64
}

// NewRouter creates an initialized MCP Router and Load Balancer.
func NewRouter(cfg RouterConfig) *Router {
	if cfg.CircuitThreshold <= 0 {
		cfg.CircuitThreshold = 3
	}
	if cfg.CircuitCooldown <= 0 {
		cfg.CircuitCooldown = 2 * time.Second
	}
	if cfg.MaxRetries < 0 {
		cfg.MaxRetries = 2
	}
	if cfg.Policy == "" {
		cfg.Policy = PolicyRoundRobin
	}
	return &Router{
		Config:  cfg,
		servers: make(map[string]*UpstreamServer),
		routes:  make([]ToolRoute, 0),
	}
}

// RegisterServer adds a backend MCP server instance to the pool.
func (r *Router) RegisterServer(server *UpstreamServer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if server.Weight <= 0 {
		server.Weight = 1
	}
	if server.CircuitState == "" {
		server.CircuitState = CircuitClosed
		server.Healthy = true
	}
	r.servers[server.ID] = server
}

// AddRoute maps tool names or prefixes to target server IDs.
func (r *Router) AddRoute(pattern string, targetServerIDs ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.routes = append(r.routes, ToolRoute{
		Pattern:       pattern,
		TargetServers: targetServerIDs,
	})
}

// GetServer returns a registered upstream server by ID.
func (r *Router) GetServer(id string) (*UpstreamServer, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	srv, ok := r.servers[id]
	return srv, ok
}

// SelectServer chooses the optimal upstream server for a given tool call according to load balancing policy and health.
func (r *Router) SelectServer(toolName string) (*UpstreamServer, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var candidates []*UpstreamServer

	// 1. Check explicit routing rules
	for _, route := range r.routes {
		if route.Pattern == "*" || route.Pattern == toolName || matchesPrefix(route.Pattern, toolName) {
			for _, srvID := range route.TargetServers {
				if srv, ok := r.servers[srvID]; ok {
					candidates = append(candidates, srv)
				}
			}
			break
		}
	}

	// If no explicit route, consider all registered servers in deterministic order
	if len(candidates) == 0 {
		var ids []string
		for id := range r.servers {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			candidates = append(candidates, r.servers[id])
		}
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no upstream servers available for tool '%s'", toolName)
	}

	// 2. Filter available healthy candidates (considering circuit breaker)
	now := time.Now()
	var available []*UpstreamServer
	for _, srv := range candidates {
		srv.mu.Lock()
		if srv.CircuitState == CircuitOpen {
			if now.Sub(srv.LastFailure) > r.Config.CircuitCooldown {
				srv.CircuitState = CircuitHalfOpen
				available = append(available, srv)
			}
		} else if srv.Healthy {
			available = append(available, srv)
		}
		srv.mu.Unlock()
	}

	if len(available) == 0 {
		return nil, fmt.Errorf("all %d candidate upstream servers for tool '%s' are unavailable (circuit open)", len(candidates), toolName)
	}

	// 3. Apply load-balancing policy
	switch r.Config.Policy {
	case PolicyLeastConnections:
		var best *UpstreamServer
		var minConn int64 = 1<<62 - 1
		for _, srv := range available {
			conns := atomic.LoadInt64(&srv.ActiveRequests)
			if conns < minConn {
				minConn = conns
				best = srv
			}
		}
		return best, nil

	case PolicyLatencyOptimized:
		var best *UpstreamServer
		var minRTT float64 = 1e9
		for _, srv := range available {
			srv.mu.RLock()
			rtt := srv.AverageRTTMs
			srv.mu.RUnlock()
			if rtt < minRTT {
				minRTT = rtt
				best = srv
			}
		}
		if best == nil {
			best = available[0]
		}
		return best, nil

	case PolicyRoundRobin:
		fallthrough
	default:
		idx := atomic.AddUint64(&r.roundRobinIdx, 1)
		chosen := available[(idx-1)%uint64(len(available))]
		return chosen, nil
	}
}

// DispatchMessage routes an MCP JSON-RPC request frame to an upstream server with automatic failover.
func (r *Router) DispatchMessage(rawJSON []byte) ([]byte, *UpstreamServer, error) {
	var msg JSONRPCMessage
	if err := json.Unmarshal(rawJSON, &msg); err != nil {
		return nil, nil, fmt.Errorf("malformed JSON-RPC message: %w", err)
	}

	toolName := "unknown"
	if msg.Method == MethodToolsCall && len(msg.Params) > 0 {
		var callParams CallToolParams
		if err := json.Unmarshal(msg.Params, &callParams); err == nil {
			toolName = callParams.Name
		}
	}

	// Try selected server with automatic failover retry
	var lastErr error
	for attempt := 0; attempt <= r.Config.MaxRetries; attempt++ {
		srv, err := r.SelectServer(toolName)
		if err != nil {
			return nil, nil, err
		}

		atomic.AddInt64(&srv.ActiveRequests, 1)
		atomic.AddInt64(&srv.TotalRequests, 1)
		start := time.Now()

		var resp []byte
		if srv.ServerInstance != nil {
			resp, err = srv.ServerInstance.HandleMessage(rawJSON)
		} else {
			err = fmt.Errorf("upstream server instance not connected")
		}

		duration := time.Since(start).Milliseconds()
		atomic.AddInt64(&srv.ActiveRequests, -1)

		srv.mu.Lock()
		if srv.AverageRTTMs == 0 {
			srv.AverageRTTMs = float64(duration)
		} else {
			srv.AverageRTTMs = (srv.AverageRTTMs * 0.8) + (float64(duration) * 0.2)
		}

		if err != nil {
			srv.FailuresStreak++
			srv.LastFailure = time.Now()
			atomic.AddInt64(&srv.FailedRequests, 1)
			if srv.FailuresStreak >= r.Config.CircuitThreshold {
				srv.CircuitState = CircuitOpen
				srv.Healthy = false
			}
			srv.mu.Unlock()
			lastErr = err
			continue // Failover to next available candidate
		}

		// Success resets circuit breaker
		if srv.CircuitState == CircuitHalfOpen {
			srv.CircuitState = CircuitClosed
			srv.Healthy = true
		}
		srv.FailuresStreak = 0
		srv.mu.Unlock()

		return resp, srv, nil
	}

	return nil, nil, fmt.Errorf("failed after %d retries: %w", r.Config.MaxRetries, lastErr)
}

// RouterAuditReport summarizes load balancer statistics across all upstreams.
type RouterAuditReport struct {
	Policy              LoadBalancingPolicy `json:"policy"`
	TotalUpstreams      int                 `json:"total_upstreams"`
	HealthyUpstreams    int                 `json:"healthy_upstreams"`
	TotalRequestsRouted int64               `json:"total_requests_routed"`
	FailedRequestsTotal int64               `json:"failed_requests_total"`
	Servers             []UpstreamSummary   `json:"servers"`
	Passed              bool                `json:"passed"`
}

// UpstreamSummary provides metrics snapshot for a single upstream server.
type UpstreamSummary struct {
	ID             string              `json:"id"`
	Name           string              `json:"name"`
	Healthy        bool                `json:"healthy"`
	CircuitState   CircuitBreakerState `json:"circuit_state"`
	ActiveRequests int64               `json:"active_requests"`
	TotalRequests  int64               `json:"total_requests"`
	FailedRequests int64               `json:"failed_requests"`
	AverageRTTMs   float64             `json:"average_rtt_ms"`
}

// Audit collects health and performance metrics across all managed upstream servers.
func (r *Router) Audit() *RouterAuditReport {
	r.mu.RLock()
	defer r.mu.RUnlock()

	summaries := make([]UpstreamSummary, 0, len(r.servers))
	var healthyCount int
	var totalRouted int64
	var totalFailed int64

	for _, srv := range r.servers {
		srv.mu.RLock()
		if srv.Healthy && srv.CircuitState != CircuitOpen {
			healthyCount++
		}
		tot := atomic.LoadInt64(&srv.TotalRequests)
		fail := atomic.LoadInt64(&srv.FailedRequests)
		act := atomic.LoadInt64(&srv.ActiveRequests)

		totalRouted += tot
		totalFailed += fail

		summaries = append(summaries, UpstreamSummary{
			ID:             srv.ID,
			Name:           srv.Name,
			Healthy:        srv.Healthy,
			CircuitState:   srv.CircuitState,
			ActiveRequests: act,
			TotalRequests:  tot,
			FailedRequests: fail,
			AverageRTTMs:   srv.AverageRTTMs,
		})
		srv.mu.RUnlock()
	}

	passed := healthyCount > 0

	return &RouterAuditReport{
		Policy:              r.Config.Policy,
		TotalUpstreams:      len(r.servers),
		HealthyUpstreams:    healthyCount,
		TotalRequestsRouted: totalRouted,
		FailedRequestsTotal: totalFailed,
		Servers:             summaries,
		Passed:              passed,
	}
}

func matchesPrefix(pattern, toolName string) bool {
	if len(pattern) > 1 && pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		return len(toolName) >= len(prefix) && toolName[:len(prefix)] == prefix
	}
	return false
}
