package tests

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/mcp"
)

func TestMCP_Router_RoundRobinSelection(t *testing.T) {
	cfg := mcp.DefaultRouterConfig()
	cfg.Policy = mcp.PolicyRoundRobin
	router := mcp.NewRouter(cfg)

	srv1 := &mcp.UpstreamServer{ID: "srv-github", Name: "GitHub Tool Server", Weight: 1}
	srv2 := &mcp.UpstreamServer{ID: "srv-bash", Name: "Bash Tool Server", Weight: 1}
	router.RegisterServer(srv1)
	router.RegisterServer(srv2)

	// Round-robin alternate selections
	s1, err := router.SelectServer("any_tool")
	if err != nil {
		t.Fatalf("Unexpected error selecting server: %v", err)
	}
	s2, err := router.SelectServer("any_tool")
	if err != nil {
		t.Fatalf("Unexpected error selecting server: %v", err)
	}

	if s1.ID == s2.ID {
		t.Errorf("Expected round-robin alternation, got same server: %s", s1.ID)
	}
}

func TestMCP_Router_PrefixRoutingAndFailover(t *testing.T) {
	cfg := mcp.DefaultRouterConfig()
	cfg.CircuitThreshold = 2
	cfg.CircuitCooldown = 100 * time.Millisecond
	router := mcp.NewRouter(cfg)

	// Server A (Primary GitHub)
	vsA := mcp.NewVirtualMCPServer("GitHub-Primary", "1.0.0")
	srvA := &mcp.UpstreamServer{
		ID:             "gh-primary",
		Name:           "GitHub Primary",
		ServerInstance: vsA,
	}

	// Server B (Secondary/Backup GitHub)
	vsB := mcp.NewVirtualMCPServer("GitHub-Secondary", "1.0.0")
	srvB := &mcp.UpstreamServer{
		ID:             "gh-secondary",
		Name:           "GitHub Secondary",
		ServerInstance: vsB,
	}

	// Register tools and mock response
	ghTool := mcp.Tool{
		Name:        "github_create_issue",
		Description: "Create a GitHub issue",
	}
	vsA.RegisterTool(ghTool)
	vsB.RegisterTool(ghTool)

	defaultResult := &mcp.CallToolResult{
		Content: []mcp.ToolContent{
			{Type: "text", Text: `{"issue_id": 101, "status": "created"}`},
		},
	}
	vsA.ConfigureTool(mcp.MockToolConfig{
		Tool:          ghTool,
		DefaultResult: defaultResult,
	})
	vsB.ConfigureTool(mcp.MockToolConfig{
		Tool:          ghTool,
		DefaultResult: defaultResult,
	})

	router.RegisterServer(srvA)
	router.RegisterServer(srvB)

	// Map github_* to gh-primary, then gh-secondary
	router.AddRoute("github_*", "gh-primary", "gh-secondary")

	// Call tool
	callFrame := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"github_create_issue","arguments":{"title":"Bug in auth"}}}`)
	resp, srv, err := router.DispatchMessage(callFrame)
	if err != nil {
		t.Fatalf("Dispatch error: %v", err)
	}
	if srv.ID != "gh-primary" {
		t.Errorf("Expected primary server, got: %s", srv.ID)
	}

	var jsonResp mcp.JSONRPCMessage
	if err := json.Unmarshal(resp, &jsonResp); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if jsonResp.Error != nil {
		t.Errorf("Unexpected tool error: %v", jsonResp.Error)
	}
}

func TestMCP_Router_CircuitBreakerTrippingAndRecovery(t *testing.T) {
	cfg := mcp.DefaultRouterConfig()
	cfg.MaxRetries = 0 // Disable retry loop to test per-attempt failure transitions
	cfg.CircuitThreshold = 2
	cfg.CircuitCooldown = 50 * time.Millisecond
	router := mcp.NewRouter(cfg)

	// Failing server (no server instance attached)
	failingSrv := &mcp.UpstreamServer{
		ID:   "unstable-server",
		Name: "Unstable Worker",
	}
	router.RegisterServer(failingSrv)

	callFrame := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"any_tool"}}`)

	// 1st failure
	_, _, _ = router.DispatchMessage(callFrame)
	if failingSrv.CircuitState != mcp.CircuitClosed {
		t.Errorf("Circuit should still be closed after 1 failure")
	}

	// 2nd failure -> Trips circuit breaker to OPEN
	_, _, _ = router.DispatchMessage(callFrame)
	if failingSrv.CircuitState != mcp.CircuitOpen {
		t.Errorf("Circuit should be open after %d failures, got: %s", cfg.CircuitThreshold, failingSrv.CircuitState)
	}

	// Wait for cooldown to expire
	time.Sleep(70 * time.Millisecond)

	// Probing server when circuit cools down sets state to HALF-OPEN
	srv, err := router.SelectServer("any_tool")
	if err != nil {
		t.Fatalf("Expected half-open server to be selectable: %v", err)
	}
	if srv.CircuitState != mcp.CircuitHalfOpen {
		t.Errorf("Expected circuit state half-open, got: %s", srv.CircuitState)
	}
}

func TestMCP_Router_ScorecardAndMermaid(t *testing.T) {
	cfg := mcp.DefaultRouterConfig()
	router := mcp.NewRouter(cfg)

	srv1 := &mcp.UpstreamServer{ID: "srv-01", Name: "Server Alpha", Healthy: true, CircuitState: mcp.CircuitClosed}
	srv2 := &mcp.UpstreamServer{ID: "srv-02", Name: "Server Beta", Healthy: false, CircuitState: mcp.CircuitOpen}
	router.RegisterServer(srv1)
	router.RegisterServer(srv2)

	report := router.Audit()
	if report.TotalUpstreams != 2 {
		t.Errorf("Expected 2 upstreams, got %d", report.TotalUpstreams)
	}
	if report.HealthyUpstreams != 1 {
		t.Errorf("Expected 1 healthy upstream, got %d", report.HealthyUpstreams)
	}

	scorecard := mcp.RenderRouterScorecard(report)
	if !strings.Contains(scorecard, "AGENTVCR // MODEL CONTEXT PROTOCOL (MCP) ROUTER") {
		t.Errorf("Expected scorecard header, got: %s", scorecard)
	}

	mermaid := mcp.RenderRouterMermaid(report)
	if !strings.Contains(mermaid, "graph TD") {
		t.Errorf("Expected mermaid diagram, got: %s", mermaid)
	}
}
