package gateway

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// SemanticReplayCache provides zero-cost deterministic replay caching for idempotent LLM turns.
type SemanticReplayCache struct {
	mu           sync.RWMutex
	maxEntries   int
	ttl          time.Duration
	entries      map[string]*CacheItem
	hits         int64
	misses       int64
	savedCostUSD float64
}

// CacheItem holds the cached response payload and access metadata.
type CacheItem struct {
	Key          string
	Response     GatewayResponse
	CreatedAt    time.Time
	ExpiresAt    time.Time
	Hits         int64
	SavedCostUSD float64
}

// NewSemanticReplayCache instantiates a thread-safe replay cache with capacity and TTL eviction.
func NewSemanticReplayCache(maxEntries int, ttl time.Duration) *SemanticReplayCache {
	if maxEntries <= 0 {
		maxEntries = 1000
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &SemanticReplayCache{
		maxEntries: maxEntries,
		ttl:        ttl,
		entries:    make(map[string]*CacheItem),
	}
}

// ComputeKey generates a deterministic SHA-256 fingerprint for a gateway request.
func (c *SemanticReplayCache) ComputeKey(req *GatewayRequest) string {
	h := sha256.New()

	// Normalize model/tier
	h.Write([]byte(fmt.Sprintf("tier:%s|model:%s|temp:%.2f\n", req.Tier, req.RequestedModel, req.Temperature)))

	// Normalize messages
	for idx, msg := range req.Messages {
		h.Write([]byte(fmt.Sprintf("msg:%d:role:%s:name:%s:content:%s\n", idx, msg.Role, msg.Name, msg.Content)))
		for _, tc := range msg.ToolCalls {
			h.Write([]byte(fmt.Sprintf("tc:%s:%s:%s\n", tc.ID, tc.Function.Name, tc.Function.Arguments)))
		}
	}

	// Normalize tools
	if len(req.Tools) > 0 {
		toolsBytes, _ := json.Marshal(req.Tools)
		h.Write([]byte("tools:"))
		h.Write(toolsBytes)
		h.Write([]byte("\n"))
	}

	return hex.EncodeToString(h.Sum(nil))
}

// Get attempts to retrieve a deterministic cached response.
func (c *SemanticReplayCache) Get(req *GatewayRequest) (*GatewayResponse, bool) {
	key := c.ComputeKey(req)

	c.mu.Lock()
	defer c.mu.Unlock()

	item, exists := c.entries[key]
	if !exists {
		c.misses++
		return nil, false
	}

	// Check TTL expiration
	if time.Now().After(item.ExpiresAt) {
		delete(c.entries, key)
		c.misses++
		return nil, false
	}

	item.Hits++
	c.hits++
	c.savedCostUSD += item.Response.CostUSD
	item.SavedCostUSD += item.Response.CostUSD

	// Clone response and stamp metadata
	cachedResp := item.Response
	cachedResp.Cached = true
	cachedResp.LatencyMs = 0.5 // Sub-millisecond instant memory hit
	cachedResp.Timestamp = time.Now()

	return &cachedResp, true
}

// Put saves a newly computed response into the replay cache.
func (c *SemanticReplayCache) Put(req *GatewayRequest, resp *GatewayResponse) {
	key := c.ComputeKey(req)

	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict one entry if capacity is exceeded
	if len(c.entries) >= c.maxEntries {
		c.evictOldest()
	}

	item := &CacheItem{
		Key:       key,
		Response:  *resp,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(c.ttl),
		Hits:      0,
	}
	c.entries[key] = item
}

// Clear purges all entries in the cache.
func (c *SemanticReplayCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*CacheItem)
	c.hits = 0
	c.misses = 0
	c.savedCostUSD = 0
}

// Stats returns a snapshot of caching metrics.
func (c *SemanticReplayCache) Stats() SemanticCacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := c.hits + c.misses
	rate := 0.0
	if total > 0 {
		rate = float64(c.hits) / float64(total)
	}

	return SemanticCacheStats{
		TotalEntries: len(c.entries),
		Hits:         c.hits,
		Misses:       c.misses,
		HitRate:      rate,
		SavedCostUSD: c.savedCostUSD,
	}
}

func (c *SemanticReplayCache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	first := true
	for k, v := range c.entries {
		if first || v.CreatedAt.Before(oldestTime) {
			oldestKey = k
			oldestTime = v.CreatedAt
			first = false
		}
	}

	if oldestKey != "" {
		delete(c.entries, oldestKey)
	}
}
