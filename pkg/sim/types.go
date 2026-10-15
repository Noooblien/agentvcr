package sim

import (
	"sync"
)

// MockRule defines a declarative mock tool matching rule and response synthesis.
type MockRule struct {
	ID           string                 `json:"id"`
	ToolName     string                 `json:"tool_name"`               // Exact tool name, wildcard "*", or regex pattern
	ArgMatch     map[string]interface{} `json:"arg_match,omitempty"`      // Predicate matching on tool arguments
	Response     interface{}            `json:"response"`                // Synthesized mock response payload
	Status       string                 `json:"status"`                  // "success", "error", "timeout"
	ErrorMessage string                 `json:"error_message,omitempty"` // Error string if Status == "error"
	LatencyMs    int64                  `json:"latency_ms,omitempty"`    // Simulated tool execution delay in ms
	StatefulOp   string                 `json:"stateful_op,omitempty"`   // "", "get", "set", "delete", "append"
	StateKey     string                 `json:"state_key,omitempty"`     // Key in SandboxState for stateful tools
}

// SimulatedResult represents the outcome of executing a simulated mock tool.
type SimulatedResult struct {
	RuleID      string      `json:"rule_id,omitempty"`
	ToolName    string      `json:"tool_name"`
	Status      string      `json:"status"` // "success", "error", "timeout"
	Response    interface{} `json:"response"`
	LatencyMs   int64       `json:"latency_ms"`
	IsSynthetic bool        `json:"is_synthetic"`
}

// SimulationReport summarizes the results of an offline trajectory simulation.
type SimulationReport struct {
	SessionID          string `json:"session_id"`
	BaseSessionID      string `json:"base_session_id"`
	TotalEvents        int    `json:"total_events"`
	ToolCallsSimulated int    `json:"tool_calls_simulated"`
	RulesMatched       int    `json:"rules_matched"`
	FallbacksUsed      int    `json:"fallbacks_used"`
	SimulatedLatencyMs int64  `json:"simulated_latency_ms"`
	ErrorsInjected     int    `json:"errors_injected"`
}

// SandboxState provides an isolated, thread-safe in-memory state store for stateful mock tools.
type SandboxState struct {
	mu    sync.RWMutex
	store map[string]interface{}
}

// NewSandboxState creates an empty sandbox state store.
func NewSandboxState() *SandboxState {
	return &SandboxState{
		store: make(map[string]interface{}),
	}
}

// Get retrieves a key from the sandbox state.
func (s *SandboxState) Get(key string) (interface{}, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	val, ok := s.store[key]
	return val, ok
}

// Set stores a key-value pair in the sandbox state.
func (s *SandboxState) Set(key string, val interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.store[key] = val
}

// Delete removes a key from the sandbox state.
func (s *SandboxState) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.store, key)
}

// Append appends an item to a slice stored at the key.
func (s *SandboxState) Append(key string, val interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.store[key]
	if !ok {
		s.store[key] = []interface{}{val}
		return
	}
	if slice, ok := existing.([]interface{}); ok {
		s.store[key] = append(slice, val)
	} else {
		s.store[key] = []interface{}{existing, val}
	}
}

// Reset clears all stored state.
func (s *SandboxState) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.store = make(map[string]interface{})
}

// Snapshot returns a shallow copy of the state map.
func (s *SandboxState) Snapshot() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]interface{}, len(s.store))
	for k, v := range s.store {
		out[k] = v
	}
	return out
}
