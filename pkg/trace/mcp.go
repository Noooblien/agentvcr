package trace

import (
	"encoding/json"
	"fmt"
	"time"
)

// MCPMessage represents a Model Context Protocol JSON-RPC frame.
type MCPMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *MCPError       `json:"error,omitempty"`
}

// MCPError represents an error payload in JSON-RPC.
type MCPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// InterceptMCPFrame processes and returns a structured TraceEvent for an MCP protocol exchange.
func InterceptMCPFrame(stepIndex int, rawFrame []byte, isRequest bool) (*TraceEvent, error) {
	var msg MCPMessage
	if err := json.Unmarshal(rawFrame, &msg); err != nil {
		return nil, fmt.Errorf("invalid json-rpc frame: %w", err)
	}

	eventType := EventToolResult
	if isRequest {
		eventType = EventToolCall
	}

	payload := map[string]interface{}{
		"jsonrpc": msg.JSONRPC,
		"id":      msg.ID,
		"method":  msg.Method,
	}

	if isRequest && len(msg.Params) > 0 {
		var params interface{}
		_ = json.Unmarshal(msg.Params, &params)
		payload["params"] = params
	} else if !isRequest {
		if msg.Error != nil {
			payload["error"] = msg.Error
		}
		if len(msg.Result) > 0 {
			var result interface{}
			_ = json.Unmarshal(msg.Result, &result)
			payload["result"] = result
		}
	}

	event := &TraceEvent{
		ID:        fmt.Sprintf("mcp_%d_%d", time.Now().UnixNano(), stepIndex),
		Index:     stepIndex,
		Timestamp: time.Now(),
		Type:      eventType,
		Payload:   payload,
	}

	return event, nil
}
