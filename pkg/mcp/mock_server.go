package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"
)

// MockToolConfig allows custom mock configuration per tool.
type MockToolConfig struct {
	Tool           Tool
	DefaultResult  *CallToolResult
	DefaultError   *JSONRPCError
	SimulatedDelay time.Duration
	InjectedError  *JSONRPCError // Force fault injection
}

// VirtualMCPServer implements a deterministic virtual MCP server.
type VirtualMCPServer struct {
	mu            sync.RWMutex
	serverInfo    ImplementationInfo
	tools         map[string]Tool
	recordedCalls map[string]MCPRecordedCall // key: toolName:argHash
	toolConfigs   map[string]MockToolConfig
	strictMatch   bool
	globalDelay   time.Duration
}

// NewVirtualMCPServer initializes a virtual MCP server.
func NewVirtualMCPServer(name, version string) *VirtualMCPServer {
	return &VirtualMCPServer{
		serverInfo: ImplementationInfo{
			Name:    name,
			Version: version,
		},
		tools:         make(map[string]Tool),
		recordedCalls: make(map[string]MCPRecordedCall),
		toolConfigs:   make(map[string]MockToolConfig),
		strictMatch:   false,
	}
}

// SetStrictMatching enforces that tool calls only match if arguments hash identically.
func (s *VirtualMCPServer) SetStrictMatching(strict bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.strictMatch = strict
}

// SetGlobalDelay injects artificial latency into all tool responses.
func (s *VirtualMCPServer) SetGlobalDelay(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.globalDelay = d
}

// RegisterTool adds a tool schema definition to the virtual server.
func (s *VirtualMCPServer) RegisterTool(tool Tool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools[tool.Name] = tool
}

// SeedRecordedCalls loads past recorded calls into the virtual server's lookup table.
func (s *VirtualMCPServer) SeedRecordedCalls(calls []MCPRecordedCall) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, call := range calls {
		key := fmt.Sprintf("%s:%s", call.ToolName, call.ArgumentsHash)
		s.recordedCalls[key] = call

		// Auto-register tool if not explicitly registered
		if _, exists := s.tools[call.ToolName]; !exists {
			s.tools[call.ToolName] = Tool{
				Name:        call.ToolName,
				Description: fmt.Sprintf("Recorded tool: %s", call.ToolName),
				InputSchema: ToolInputSchema{
					Type: "object",
				},
			}
		}
	}
}

// ConfigureTool sets mock or fault injection rules for a specific tool.
func (s *VirtualMCPServer) ConfigureTool(config MockToolConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools[config.Tool.Name] = config.Tool
	s.toolConfigs[config.Tool.Name] = config
}

// HandleMessage processes a single JSON-RPC message and produces a response.
func (s *VirtualMCPServer) HandleMessage(reqBytes []byte) ([]byte, error) {
	var msg JSONRPCMessage
	if err := json.Unmarshal(reqBytes, &msg); err != nil {
		errResp := JSONRPCMessage{
			JSONRPC: JSONRPCVersion,
			Error: &JSONRPCError{
				Code:    CodeParseError,
				Message: fmt.Sprintf("Parse error: %v", err),
			},
		}
		return json.Marshal(errResp)
	}

	// Notifications don't require responses
	if msg.ID == nil && msg.Method == MethodInitialized {
		return nil, nil
	}

	resp := JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		ID:      msg.ID,
	}

	switch msg.Method {
	case MethodInitialize:
		initResult := InitializeResult{
			ProtocolVersion: "2024-11-05",
			ServerInfo:      s.serverInfo,
			Capabilities: ServerCapabilities{
				Tools: &ToolsCapability{ListChanged: false},
			},
			Instructions: "AgentVCR Virtual Deterministic Mock MCP Server",
		}
		resBytes, _ := json.Marshal(initResult)
		resp.Result = resBytes

	case MethodPing:
		resp.Result = json.RawMessage(`{}`)

	case MethodToolsList:
		s.mu.RLock()
		toolsList := make([]Tool, 0, len(s.tools))
		for _, t := range s.tools {
			toolsList = append(toolsList, t)
		}
		s.mu.RUnlock()

		res := ToolsListResult{Tools: toolsList}
		resBytes, _ := json.Marshal(res)
		resp.Result = resBytes

	case MethodToolsCall:
		var callParams CallToolParams
		if err := json.Unmarshal(msg.Params, &callParams); err != nil {
			resp.Error = &JSONRPCError{
				Code:    CodeInvalidParams,
				Message: fmt.Sprintf("Invalid tools/call params: %v", err),
			}
			break
		}

		callRes, callErr := s.resolveToolCall(callParams)
		if callErr != nil {
			resp.Error = callErr
		} else {
			resBytes, _ := json.Marshal(callRes)
			resp.Result = resBytes
		}

	default:
		resp.Error = &JSONRPCError{
			Code:    CodeMethodNotFound,
			Message: fmt.Sprintf("Method '%s' not found on virtual server", msg.Method),
		}
	}

	return json.Marshal(resp)
}

// resolveToolCall looks up recorded results or mock configurations.
func (s *VirtualMCPServer) resolveToolCall(params CallToolParams) (*CallToolResult, *JSONRPCError) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 1. Check for tool-specific delay or global delay
	delay := s.globalDelay
	if cfg, ok := s.toolConfigs[params.Name]; ok && cfg.SimulatedDelay > 0 {
		delay = cfg.SimulatedDelay
	}
	if delay > 0 {
		time.Sleep(delay)
	}

	// 2. Check for configured fault injection
	if cfg, ok := s.toolConfigs[params.Name]; ok && cfg.InjectedError != nil {
		return nil, cfg.InjectedError
	}

	// 3. Exact match via toolName:argHash
	argHash := HashArguments(params.Arguments)
	key := fmt.Sprintf("%s:%s", params.Name, argHash)

	if recorded, found := s.recordedCalls[key]; found {
		if recorded.Error != nil {
			return nil, recorded.Error
		}
		return &recorded.Result, nil
	}

	// 4. Non-strict match: look for any recorded call with the same tool name
	if !s.strictMatch {
		for _, recorded := range s.recordedCalls {
			if recorded.ToolName == params.Name {
				if recorded.Error != nil {
					return nil, recorded.Error
				}
				return &recorded.Result, nil
			}
		}
	}

	// 5. Configured default fallback
	if cfg, ok := s.toolConfigs[params.Name]; ok && cfg.DefaultResult != nil {
		return cfg.DefaultResult, nil
	}

	return nil, &JSONRPCError{
		Code:    CodeInternalError,
		Message: fmt.Sprintf("No recorded trace or mock response found for tool '%s' (hash: %s)", params.Name, argHash),
	}
}

// ServeStream reads newline-delimited JSON-RPC from reader and writes responses to writer.
func (s *VirtualMCPServer) ServeStream(r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		respBytes, err := s.HandleMessage(line)
		if err != nil {
			return err
		}

		if respBytes != nil {
			if _, err := w.Write(append(respBytes, '\n')); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}
