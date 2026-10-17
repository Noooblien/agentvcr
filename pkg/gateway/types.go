package gateway

import (
	"time"
)

// Protocol defines the API wire protocol supported by an upstream provider.
type Protocol string

const (
	ProtocolOpenAI    Protocol = "openai"
	ProtocolAnthropic Protocol = "anthropic"
	ProtocolGemini    Protocol = "gemini"
	ProtocolOllama    Protocol = "ollama"
)

// ModelTier classifies the operational tier and reasoning depth required.
type ModelTier string

const (
	TierFast      ModelTier = "fast"      // Low latency, minimal cost (e.g. gpt-4o-mini, haiku, flash)
	TierStandard  ModelTier = "standard"  // General-purpose high performance (e.g. gpt-4o, sonnet)
	TierReasoning ModelTier = "reasoning" // Deep multi-step reasoning (e.g. o1, o3-mini, opus)
)

// CircuitState represents the current resilience state of a provider circuit breaker.
type CircuitState string

const (
	CircuitClosed   CircuitState = "CLOSED"    // Healthy, traffic flowing
	CircuitHalfOpen CircuitState = "HALF_OPEN" // Testing recovery with limited probes
	CircuitOpen     CircuitState = "OPEN"      // Tripped due to failures, traffic diverted
)

// CircuitBreakerConfig tunes the sensitivity of failure detection and self-healing.
type CircuitBreakerConfig struct {
	FailureThreshold   int           `json:"failure_threshold"`    // Consecutive errors to trip circuit
	ErrorRateThreshold float64       `json:"error_rate_threshold"` // Error rate percentage (0.0 - 1.0)
	RollingWindowSize  int           `json:"rolling_window_size"`  // Sliding request sample window
	CooldownDuration   time.Duration `json:"cooldown_duration"`    // Time in OPEN state before HALF_OPEN probe
	HalfOpenProbes     int           `json:"half_open_probes"`     // Successful consecutive probes to recover
}

// ProviderConfig specifies the connection parameters, cost model, and capabilities of an upstream.
type ProviderConfig struct {
	ID                 string               `json:"id"`
	Name               string               `json:"name"`
	Protocol           Protocol             `json:"protocol"`
	Endpoint           string               `json:"endpoint"`
	APIKey             string               `json:"api_key,omitempty"`
	Models             map[ModelTier]string `json:"models"` // Mapping of Tier -> Provider Model Name
	CostPerInput1kUSD  float64              `json:"cost_per_input_1k_usd"`
	CostPerOutput1kUSD float64              `json:"cost_per_output_1k_usd"`
	Timeout            time.Duration        `json:"timeout"`
	MaxRetries         int                  `json:"max_retries"`
	Weight             int                  `json:"weight"`
	CircuitConfig      CircuitBreakerConfig `json:"circuit_config"`
}

// ProviderTelemetry tracks live performance and reliability metrics per upstream.
type ProviderTelemetry struct {
	ProviderID          string       `json:"provider_id"`
	ProviderName        string       `json:"provider_name"`
	CircuitState        CircuitState `json:"circuit_state"`
	TotalRequests       int64        `json:"total_requests"`
	SuccessfulRequests  int64        `json:"successful_requests"`
	FailedRequests      int64        `json:"failed_requests"`
	ConsecutiveFailures int          `json:"consecutive_failures"`
	ConsecutiveSuccess  int          `json:"consecutive_success"`
	AvgLatencyMs        float64      `json:"avg_latency_ms"`
	TotalCostUSD        float64      `json:"total_cost_usd"`
	LastStateChange     time.Time    `json:"last_state_change"`
	LastError           string       `json:"last_error,omitempty"`
}

// ChatMessage represents a single message in a multi-turn conversation.
type ChatMessage struct {
	Role       string     `json:"role"` // system, user, assistant, tool
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

// ToolCall represents an invocation of a tool requested by the model.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"` // "function"
	Function ToolFunction `json:"function"`
}

// ToolFunction defines the target function call and arguments.
type ToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolDefinition describes a tool schema exposed to the agent.
type ToolDefinition struct {
	Type     string                 `json:"type"`
	Function ToolFunctionDefinition `json:"function"`
}

// ToolFunctionDefinition contains function metadata and JSON schema.
type ToolFunctionDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// GatewayRequest encapsulates a standard model invocation passed to the gateway.
type GatewayRequest struct {
	ID             string                 `json:"id"`
	SessionID      string                 `json:"session_id,omitempty"`
	RequestedModel string                 `json:"requested_model,omitempty"`
	Tier           ModelTier              `json:"tier,omitempty"`
	Messages       []ChatMessage          `json:"messages"`
	Tools          []ToolDefinition       `json:"tools,omitempty"`
	Temperature    float64                `json:"temperature"`
	MaxTokens      int                    `json:"max_tokens,omitempty"`
	RequireTool    bool                   `json:"require_tool,omitempty"`
	ForceFallback  bool                   `json:"force_fallback,omitempty"` // For testing failure cascades
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// GatewayChoice models an individual completion option returned.
type GatewayChoice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

// TokenUsage reports prompt and generation token counts.
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// GatewayResponse represents the unified response delivered to the caller.
type GatewayResponse struct {
	ID            string          `json:"id"`
	Model         string          `json:"model"`
	ProviderID    string          `json:"provider_id"`
	Choices       []GatewayChoice `json:"choices"`
	Usage         TokenUsage      `json:"usage"`
	CostUSD       float64         `json:"cost_usd"`
	LatencyMs     float64         `json:"latency_ms"`
	Cached        bool            `json:"cached"`
	FallbackChain []string        `json:"fallback_chain"` // Trace of providers evaluated/attempted
	Timestamp     time.Time       `json:"timestamp"`
}

// SemanticCacheStats summarizes caching efficiency and cost savings.
type SemanticCacheStats struct {
	TotalEntries int     `json:"total_entries"`
	Hits         int64   `json:"hits"`
	Misses       int64   `json:"misses"`
	HitRate      float64 `json:"hit_rate"`
	SavedCostUSD float64 `json:"saved_cost_usd"`
}

// GatewayReport provides a complete diagnostic audit of gateway health and traffic.
type GatewayReport struct {
	SystemID        string              `json:"system_id"`
	Timestamp       time.Time           `json:"timestamp"`
	ActiveProviders int                 `json:"active_providers"`
	TotalRequests   int64               `json:"total_requests"`
	TotalFailovers  int64               `json:"total_failovers"`
	TotalCostUSD    float64             `json:"total_cost_usd"`
	TotalSavingsUSD float64             `json:"total_savings_usd"`
	CacheStats      SemanticCacheStats  `json:"cache_stats"`
	ProviderStats   []ProviderTelemetry `json:"provider_stats"`
}
