package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// PendingCall tracks an in-flight tool invocation awaiting response.
type PendingCall struct {
	ID        interface{}
	ToolName  string
	Arguments map[string]interface{}
	ArgHash   string
	StartTime time.Time
	StepIndex int
}

// InterceptorMetrics aggregates real-time telemetry from intercepted MCP exchanges.
type InterceptorMetrics struct {
	TotalToolCalls  int64            `json:"total_tool_calls"`
	SuccessCalls    int64            `json:"success_calls"`
	ErrorCalls      int64            `json:"error_calls"`
	ToolsInvoked    map[string]int64 `json:"tools_invoked"`
	TotalLatencyMs  int64            `json:"total_latency_ms"`
	MaxLatencyMs    int64            `json:"max_latency_ms"`
	AverageLatency  float64          `json:"average_latency_ms"`
}

// Interceptor manages in-flight interception, hashing, and recording of MCP protocol exchanges.
type Interceptor struct {
	mu           sync.Mutex
	pendingCalls map[string]*PendingCall
	recorded     []MCPRecordedCall
	events       []trace.TraceEvent
	metrics      InterceptorMetrics
	stepCounter  int
	prevChecksum string
}

// NewInterceptor initializes a fresh MCP protocol interceptor.
func NewInterceptor() *Interceptor {
	return &Interceptor{
		pendingCalls: make(map[string]*PendingCall),
		recorded:     make([]MCPRecordedCall, 0),
		events:       make([]trace.TraceEvent, 0),
		metrics: InterceptorMetrics{
			ToolsInvoked: make(map[string]int64),
		},
		prevChecksum: "genesis_mcp",
	}
}

// HashArguments generates a deterministic canonical SHA-256 hash of tool argument values.
func HashArguments(args map[string]interface{}) string {
	if len(args) == 0 {
		return "empty_args"
	}

	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	canonicalMap := make(map[string]interface{}, len(args))
	for _, k := range keys {
		canonicalMap[k] = args[k]
	}

	raw, err := json.Marshal(canonicalMap)
	if err != nil {
		return "unhashable_args"
	}

	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// makeKey converts an ID (int, float64, string) to a reliable map key.
func makeKey(id interface{}) string {
	return fmt.Sprintf("%v", id)
}

// ProcessClientFrame inspects and records outbound client JSON-RPC messages.
func (i *Interceptor) ProcessClientFrame(raw []byte) (*trace.TraceEvent, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	var msg JSONRPCMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return nil, fmt.Errorf("invalid json-rpc message: %w", err)
	}

	if msg.Method == MethodToolsCall && msg.ID != nil {
		var params CallToolParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return nil, fmt.Errorf("malformed tools/call params: %w", err)
		}

		argHash := HashArguments(params.Arguments)
		now := time.Now()
		step := i.stepCounter
		i.stepCounter++

		key := makeKey(msg.ID)
		i.pendingCalls[key] = &PendingCall{
			ID:        msg.ID,
			ToolName:  params.Name,
			Arguments: params.Arguments,
			ArgHash:   argHash,
			StartTime: now,
			StepIndex: step,
		}

		payload := map[string]interface{}{
			"jsonrpc":        msg.JSONRPC,
			"id":             msg.ID,
			"method":         msg.Method,
			"tool_name":      params.Name,
			"arguments":      params.Arguments,
			"arguments_hash": argHash,
		}

		rawPayload, _ := json.Marshal(payload)
		checksum := trace.ComputeChecksum(i.prevChecksum, step, trace.EventToolCall, rawPayload)
		i.prevChecksum = checksum

		event := trace.TraceEvent{
			ID:        fmt.Sprintf("mcp_call_%d_%d", now.UnixNano(), step),
			Index:     step,
			Timestamp: now,
			Type:      trace.EventToolCall,
			Payload:   payload,
			Checksum:  checksum,
		}

		i.events = append(i.events, event)
		return &event, nil
	}

	return nil, nil
}

// ProcessServerFrame inspects and records inbound server JSON-RPC responses.
func (i *Interceptor) ProcessServerFrame(raw []byte) (*trace.TraceEvent, *MCPRecordedCall, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	var msg JSONRPCMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return nil, nil, fmt.Errorf("invalid json-rpc response: %w", err)
	}

	if msg.ID == nil {
		return nil, nil, nil // Notification or unsolicited frame
	}

	key := makeKey(msg.ID)
	pending, exists := i.pendingCalls[key]
	if !exists {
		return nil, nil, nil // Not a tracked tool call (e.g. initialize, ping)
	}
	delete(i.pendingCalls, key)

	now := time.Now()
	latencyMs := now.Sub(pending.StartTime).Milliseconds()
	if latencyMs < 0 {
		latencyMs = 0
	}

	step := i.stepCounter
	i.stepCounter++

	var toolResult CallToolResult
	var recorded MCPRecordedCall
	isError := false

	if msg.Error != nil {
		isError = true
		recorded = MCPRecordedCall{
			ToolName:      pending.ToolName,
			Arguments:     pending.Arguments,
			ArgumentsHash: pending.ArgHash,
			Error:         msg.Error,
			LatencyMs:     latencyMs,
			Timestamp:     now,
		}
	} else {
		_ = json.Unmarshal(msg.Result, &toolResult)
		if toolResult.IsError {
			isError = true
		}
		recorded = MCPRecordedCall{
			ToolName:      pending.ToolName,
			Arguments:     pending.Arguments,
			ArgumentsHash: pending.ArgHash,
			Result:        toolResult,
			LatencyMs:     latencyMs,
			Timestamp:     now,
		}
	}

	// Update Metrics
	i.metrics.TotalToolCalls++
	if isError {
		i.metrics.ErrorCalls++
	} else {
		i.metrics.SuccessCalls++
	}
	i.metrics.ToolsInvoked[pending.ToolName]++
	i.metrics.TotalLatencyMs += latencyMs
	if latencyMs > i.metrics.MaxLatencyMs {
		i.metrics.MaxLatencyMs = latencyMs
	}
	if i.metrics.TotalToolCalls > 0 {
		i.metrics.AverageLatency = float64(i.metrics.TotalLatencyMs) / float64(i.metrics.TotalToolCalls)
	}

	i.recorded = append(i.recorded, recorded)

	payload := map[string]interface{}{
		"jsonrpc":        msg.JSONRPC,
		"id":             msg.ID,
		"tool_name":      pending.ToolName,
		"arguments_hash": pending.ArgHash,
		"latency_ms":     latencyMs,
	}
	if msg.Error != nil {
		payload["error"] = msg.Error
	} else {
		payload["result"] = toolResult
	}

	rawPayload, _ := json.Marshal(payload)
	checksum := trace.ComputeChecksum(i.prevChecksum, step, trace.EventToolResult, rawPayload)
	i.prevChecksum = checksum

	event := trace.TraceEvent{
		ID:         fmt.Sprintf("mcp_result_%d_%d", now.UnixNano(), step),
		Index:      step,
		Timestamp:  now,
		Type:       trace.EventToolResult,
		DurationMs: latencyMs,
		Payload:    payload,
		Checksum:   checksum,
	}

	i.events = append(i.events, event)
	return &event, &recorded, nil
}

// GetRecordedCalls returns all captured tool calls.
func (i *Interceptor) GetRecordedCalls() []MCPRecordedCall {
	i.mu.Lock()
	defer i.mu.Unlock()
	copied := make([]MCPRecordedCall, len(i.recorded))
	copy(copied, i.recorded)
	return copied
}

// GetTraceEvents returns all generated TraceEvents.
func (i *Interceptor) GetTraceEvents() []trace.TraceEvent {
	i.mu.Lock()
	defer i.mu.Unlock()
	copied := make([]trace.TraceEvent, len(i.events))
	copy(copied, i.events)
	return copied
}

// GetMetrics returns snapshot of aggregated execution metrics.
func (i *Interceptor) GetMetrics() InterceptorMetrics {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.metrics
}
