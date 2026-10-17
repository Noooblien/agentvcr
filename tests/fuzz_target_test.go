package tests

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/compact"
	"github.com/Noooblien/agentvcr/pkg/mcp"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

// FuzzMCPJSONRPCFrame continuously fuzzes MCP JSON-RPC 2.0 payload decoding and argument hashing.
func FuzzMCPJSONRPCFrame(f *testing.F) {
	// Seed corpus
	f.Add([]byte(`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"bash","arguments":{"command":"echo hello"}},"id":1}`))
	f.Add([]byte(`{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"command completed successfully"}]}}`))
	f.Add([]byte(`{"jsonrpc":"2.0","id":3,"error":{"code":-32601,"message":"Method not found"}}`))
	f.Add([]byte(`{"jsonrpc":"2.0","method":"tools/list","params":{}}`))

	validator := mcp.NewValidator()
	sampleSchema := mcp.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"command": map[string]interface{}{"type": "string"},
		},
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		var raw map[string]interface{}
		if err := json.Unmarshal(data, &raw); err != nil {
			return // Ignore unparseable non-JSON mutations
		}

		// Verify argument hashing never panics on arbitrary structures
		_ = mcp.HashArguments(raw)

		// Verify schema validator handles arbitrary nested types gracefully
		_ = validator.ValidateArguments(sampleSchema, raw)
	})
}

// FuzzSessionDeserialization tests that arbitrary corrupt trace JSON files never cause crashes or panics.
func FuzzSessionDeserialization(f *testing.F) {
	f.Add([]byte(`{"id":"sess_1","agent_name":"test","events":[{"index":1,"type":"llm_request","payload":{"model":"gpt-4"}}]}`))
	f.Add([]byte(`{"id":"","agent_name":"","events":[]}`))
	f.Add([]byte(`{"events":[{"index":-100,"type":"unknown","duration_ms":-999}]}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		var session trace.Session
		if err := json.Unmarshal(data, &session); err != nil {
			return
		}

		for _, ev := range session.Events {
			// Checksum calculation must be totally crash-safe for any fuzzed payload
			payloadBytes, _ := json.Marshal(ev.Payload)
			_ = trace.ComputeChecksum("genesis", ev.Index, ev.Type, payloadBytes)
		}
	})
}

// FuzzCompactorDeduplication fuzzes the context compactor engine with arbitrary mutated payloads.
func FuzzCompactorDeduplication(f *testing.F) {
	f.Add("first tool execution output", "second identical tool output")
	f.Add("", "non-empty content")
	f.Add(strings.Repeat("A", 1000), strings.Repeat("B", 1000))

	cfg := compact.DefaultCompactorConfig()
	comp := compact.NewCompactor(cfg)

	f.Fuzz(func(t *testing.T, payloadA, payloadB string) {
		now := time.Now()
		session := &trace.Session{
			ID:        "fuzz-compaction-session",
			AgentName: "fuzzer",
			Events: []trace.TraceEvent{
				{Index: 1, Timestamp: now, Type: trace.EventToolCall, Payload: map[string]interface{}{"tool": "read", "arg": payloadA}},
				{Index: 2, Timestamp: now.Add(time.Millisecond), Type: trace.EventToolResult, Payload: map[string]interface{}{"output": payloadA}},
				{Index: 3, Timestamp: now.Add(2 * time.Millisecond), Type: trace.EventToolCall, Payload: map[string]interface{}{"tool": "read", "arg": payloadB}},
				{Index: 4, Timestamp: now.Add(3 * time.Millisecond), Type: trace.EventToolResult, Payload: map[string]interface{}{"output": payloadB}},
			},
		}

		// Compactor must never panic regardless of string contents or byte values
		compacted, report, err := comp.CompactSession(session, "fuzzed-result")
		if err == nil && compacted != nil {
			_ = report.TokensSaved
		}
	})
}
