package tests_test

import (
	"testing"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

func TestInterceptMCPFrame(t *testing.T) {
	reqJSON := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"bash","arguments":{"command":"ls -la"}}}`)
	event, err := trace.InterceptMCPFrame(0, reqJSON, true)
	if err != nil {
		t.Fatalf("unexpected error parsing mcp frame: %v", err)
	}

	if event.Type != trace.EventToolCall {
		t.Errorf("expected EventToolCall, got %s", event.Type)
	}

	resJSON := []byte(`{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"total 0\n"}]}}`)
	resEvent, err := trace.InterceptMCPFrame(1, resJSON, false)
	if err != nil {
		t.Fatalf("unexpected error parsing mcp response: %v", err)
	}

	if resEvent.Type != trace.EventToolResult {
		t.Errorf("expected EventToolResult, got %s", resEvent.Type)
	}
}
