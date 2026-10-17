package mcp

import (
	"encoding/json"
	"time"
)

// JSON-RPC 2.0 protocol constants
const (
	JSONRPCVersion = "2.0"

	// Standard JSON-RPC error codes
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603

	// Standard MCP protocol methods
	MethodInitialize     = "initialize"
	MethodInitialized    = "notifications/initialized"
	MethodToolsList      = "tools/list"
	MethodToolsCall      = "tools/call"
	MethodResourcesList  = "resources/list"
	MethodResourcesRead  = "resources/read"
	MethodPromptsList    = "prompts/list"
	MethodPromptsGet     = "prompts/get"
	MethodPing           = "ping"
	MethodCancelNotify   = "notifications/cancelled"
)

// JSONRPCMessage is a generic top-level JSON-RPC 2.0 message envelope.
type JSONRPCMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError represents a structured JSON-RPC error object.
type JSONRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *JSONRPCError) Error() string {
	return e.Message
}

// ImplementationInfo identifies client/server implementation.
type ImplementationInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ClientCapabilities declares supported MCP client features.
type ClientCapabilities struct {
	Experimental map[string]interface{} `json:"experimental,omitempty"`
	Roots        *RootsCapability       `json:"roots,omitempty"`
	Sampling     map[string]interface{} `json:"sampling,omitempty"`
}

// RootsCapability represents client roots capability.
type RootsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ServerCapabilities declares supported MCP server features.
type ServerCapabilities struct {
	Experimental map[string]interface{} `json:"experimental,omitempty"`
	Logging      map[string]interface{} `json:"logging,omitempty"`
	Prompts      *PromptsCapability     `json:"prompts,omitempty"`
	Resources    *ResourcesCapability   `json:"resources,omitempty"`
	Tools        *ToolsCapability       `json:"tools,omitempty"`
}

// ToolsCapability indicates tool support.
type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ResourcesCapability indicates resource support.
type ResourcesCapability struct {
	Subscribe   bool `json:"subscribe,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
}

// PromptsCapability indicates prompt template support.
type PromptsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// InitializeParams is sent by client on connection initialization.
type InitializeParams struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ClientCapabilities `json:"capabilities"`
	ClientInfo      ImplementationInfo `json:"clientInfo"`
}

// InitializeResult is returned by server on connection initialization.
type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      ImplementationInfo `json:"serverInfo"`
	Instructions    string             `json:"instructions,omitempty"`
}

// ToolInputSchema defines JSON schema for tool arguments.
type ToolInputSchema struct {
	Type                 string                 `json:"type"`
	Properties           map[string]interface{} `json:"properties,omitempty"`
	Required             []string               `json:"required,omitempty"`
	AdditionalProperties interface{}            `json:"additionalProperties,omitempty"`
}

// Tool represents a Model Context Protocol tool definition.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema ToolInputSchema `json:"inputSchema"`
}

// ToolsListResult holds the list of available tools.
type ToolsListResult struct {
	Tools      []Tool  `json:"tools"`
	NextCursor *string `json:"nextCursor,omitempty"`
}

// CallToolParams contains tool name and runtime arguments.
type CallToolParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

// ToolContent represents a single content block in a tool result.
type ToolContent struct {
	Type     string `json:"type"` // "text", "image", "resource"
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
}

// CallToolResult contains content and execution status returned by a tool.
type CallToolResult struct {
	Content []ToolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// MCPRecordedCall preserves a paired tool invocation and result for deterministic replay.
type MCPRecordedCall struct {
	ToolName      string                 `json:"tool_name"`
	Arguments     map[string]interface{} `json:"arguments"`
	ArgumentsHash string                 `json:"arguments_hash"`
	Result        CallToolResult         `json:"result"`
	Error         *JSONRPCError          `json:"error,omitempty"`
	LatencyMs     int64                  `json:"latency_ms"`
	Timestamp     time.Time              `json:"timestamp"`
}
