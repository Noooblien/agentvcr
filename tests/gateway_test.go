package tests

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/gateway"
)

func createTestGateway() *gateway.GatewayEngine {
	gw := gateway.NewGatewayEngine("test-gateway-system")

	gw.RegisterProvider(&gateway.ProviderConfig{
		ID:       "openai-primary",
		Name:     "OpenAI",
		Protocol: gateway.ProtocolOpenAI,
		Endpoint: "https://api.openai.com/v1/chat/completions",
		Models: map[gateway.ModelTier]string{
			gateway.TierFast:      "gpt-4o-mini",
			gateway.TierStandard:  "gpt-4o",
			gateway.TierReasoning: "o1-preview",
		},
		CostPerInput1kUSD:  0.005,
		CostPerOutput1kUSD: 0.015,
		CircuitConfig: gateway.CircuitBreakerConfig{
			FailureThreshold: 2,
			CooldownDuration: 50 * time.Millisecond,
		},
	})

	gw.RegisterProvider(&gateway.ProviderConfig{
		ID:       "anthropic-backup",
		Name:     "Anthropic",
		Protocol: gateway.ProtocolAnthropic,
		Endpoint: "https://api.anthropic.com/v1/messages",
		Models: map[gateway.ModelTier]string{
			gateway.TierFast:      "claude-3-5-haiku",
			gateway.TierStandard:  "claude-3-7-sonnet",
			gateway.TierReasoning: "claude-3-opus",
		},
		CostPerInput1kUSD:  0.003,
		CostPerOutput1kUSD: 0.015,
		CircuitConfig: gateway.CircuitBreakerConfig{
			FailureThreshold: 3,
			CooldownDuration: 50 * time.Millisecond,
		},
	})

	gw.RegisterProvider(&gateway.ProviderConfig{
		ID:       "gemini-fallback",
		Name:     "Gemini",
		Protocol: gateway.ProtocolGemini,
		Endpoint: "https://generativelanguage.googleapis.com/v1beta/models",
		Models: map[gateway.ModelTier]string{
			gateway.TierFast:      "gemini-2.0-flash",
			gateway.TierStandard:  "gemini-1.5-pro",
			gateway.TierReasoning: "gemini-1.5-pro",
		},
		CostPerInput1kUSD:  0.001,
		CostPerOutput1kUSD: 0.005,
		CircuitConfig: gateway.CircuitBreakerConfig{
			FailureThreshold: 3,
			CooldownDuration: 50 * time.Millisecond,
		},
	})

	gw.Router().SetCascade(gateway.TierFast, []string{"openai-primary", "anthropic-backup", "gemini-fallback"})
	gw.Router().SetCascade(gateway.TierStandard, []string{"openai-primary", "anthropic-backup", "gemini-fallback"})
	gw.Router().SetCascade(gateway.TierReasoning, []string{"openai-primary", "anthropic-backup", "gemini-fallback"})

	return gw
}

func TestGateway_SemanticClassification_Tiers(t *testing.T) {
	gw := createTestGateway()

	// 1. Simple fast prompt
	reqFast := &gateway.GatewayRequest{
		Messages: []gateway.ChatMessage{{Role: "user", Content: "Hello world"}},
	}
	if tier := gw.Router().ClassifyTier(reqFast); tier != gateway.TierFast {
		t.Fatalf("Expected TierFast, got %s", tier)
	}

	// 2. Tool requiring prompt
	reqStd := &gateway.GatewayRequest{
		Messages: []gateway.ChatMessage{{Role: "user", Content: "Check git status"}},
		Tools: []gateway.ToolDefinition{
			{Type: "function", Function: gateway.ToolFunctionDefinition{Name: "git_status"}},
		},
	}
	if tier := gw.Router().ClassifyTier(reqStd); tier != gateway.TierStandard {
		t.Fatalf("Expected TierStandard, got %s", tier)
	}

	// 3. Deep reasoning prompt
	reqReason := &gateway.GatewayRequest{
		Messages: []gateway.ChatMessage{{Role: "user", Content: "Provide a formal mathematical proof for the Byzantine consensus threshold."}},
	}
	if tier := gw.Router().ClassifyTier(reqReason); tier != gateway.TierReasoning {
		t.Fatalf("Expected TierReasoning, got %s", tier)
	}
}

func TestGateway_CircuitBreaker_StateTransitions(t *testing.T) {
	cb := gateway.NewCircuitBreaker("test-provider", gateway.CircuitBreakerConfig{
		FailureThreshold: 2,
		CooldownDuration: 30 * time.Millisecond,
		HalfOpenProbes:   1,
	})

	if cb.State() != gateway.CircuitClosed {
		t.Fatalf("Expected initial state CLOSED, got %s", cb.State())
	}
	if !cb.Allow() {
		t.Fatalf("Expected request to be allowed in CLOSED state")
	}

	// 1 failure (below threshold)
	cb.RecordFailure(fmt.Errorf("temporary timeout"))
	if cb.State() != gateway.CircuitClosed {
		t.Fatalf("Expected state CLOSED after 1 failure, got %s", cb.State())
	}

	// 2nd failure (threshold reached) -> Trip OPEN
	cb.RecordFailure(fmt.Errorf("HTTP 429 rate limit"))
	if cb.State() != gateway.CircuitOpen {
		t.Fatalf("Expected state OPEN after 2 failures, got %s", cb.State())
	}

	// Immediate check should be rejected
	if cb.Allow() {
		t.Fatalf("Expected Allow() to return false when circuit is OPEN")
	}

	// Wait for cooldown
	time.Sleep(35 * time.Millisecond)

	// Now Allow() should transition to HALF_OPEN to test a probe
	if !cb.Allow() {
		t.Fatalf("Expected Allow() to permit probe after cooldown")
	}
	if cb.State() != gateway.CircuitHalfOpen {
		t.Fatalf("Expected state HALF_OPEN during probe, got %s", cb.State())
	}

	// Successful probe heals circuit back to CLOSED
	cb.RecordSuccess()
	if cb.State() != gateway.CircuitClosed {
		t.Fatalf("Expected state CLOSED after successful probe, got %s", cb.State())
	}
}

func TestGateway_SemanticReplayCache_HitAndSavings(t *testing.T) {
	gw := createTestGateway()

	gw.SetDispatcher(func(ctx context.Context, p *gateway.ProviderConfig, targetModel string, payload []byte) ([]byte, int, error) {
		return []byte(`{
			"id": "chatcmpl_test",
			"choices": [{"index": 0, "message": {"role": "assistant", "content": "42"}, "finish_reason": "stop"}],
			"usage": {"prompt_tokens": 100, "completion_tokens": 50, "total_tokens": 150}
		}`), 200, nil
	})

	ctx := context.Background()
	req := &gateway.GatewayRequest{
		ID:          "q1",
		Messages:    []gateway.ChatMessage{{Role: "user", Content: "What is the answer to everything?"}},
		Temperature: 0.0,
	}

	// Turn 1: Cache Miss
	resp1, err := gw.Handle(ctx, req)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if resp1.Cached {
		t.Fatalf("Expected Turn 1 to be a cache miss")
	}
	if resp1.CostUSD <= 0 {
		t.Fatalf("Expected non-zero cost for Turn 1")
	}

	// Turn 2: Cache Hit
	resp2, err := gw.Handle(ctx, req)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if !resp2.Cached {
		t.Fatalf("Expected Turn 2 to be a CACHE HIT")
	}
	if resp2.Choices[0].Message.Content != "42" {
		t.Fatalf("Unexpected cached content: %s", resp2.Choices[0].Message.Content)
	}

	stats := gw.Cache().Stats()
	if stats.Hits != 1 || stats.Misses != 1 {
		t.Fatalf("Unexpected cache stats: hits=%d, misses=%d", stats.Hits, stats.Misses)
	}
	if stats.SavedCostUSD <= 0 {
		t.Fatalf("Expected positive saved cost, got $%.4f", stats.SavedCostUSD)
	}
}

func TestGateway_Cascade_Failover_Automatic(t *testing.T) {
	gw := createTestGateway()

	// Dispatcher: Primary OpenAI fails with HTTP 429, Anthropic succeeds
	gw.SetDispatcher(func(ctx context.Context, p *gateway.ProviderConfig, targetModel string, payload []byte) ([]byte, int, error) {
		if p.ID == "openai-primary" {
			return nil, 429, fmt.Errorf("rate limit exceeded (429)")
		}
		if p.ID == "anthropic-backup" {
			return []byte(`{
				"id": "msg_anthropic_ok",
				"content": [{"type": "text", "text": "Claude fallback response"}],
				"usage": {"input_tokens": 100, "output_tokens": 40}
			}`), 200, nil
		}
		return nil, 500, fmt.Errorf("should not be reached")
	})

	ctx := context.Background()
	req := &gateway.GatewayRequest{
		ID:          "cascade-test",
		Messages:    []gateway.ChatMessage{{Role: "user", Content: "Complex architecture query"}},
		Temperature: 0.5,
	}

	resp, err := gw.Handle(ctx, req)
	if err != nil {
		t.Fatalf("Cascade handling failed: %v", err)
	}

	if resp.ProviderID != "anthropic-backup" {
		t.Fatalf("Expected failover to anthropic-backup, got %s", resp.ProviderID)
	}
	if resp.Choices[0].Message.Content != "Claude fallback response" {
		t.Fatalf("Unexpected response payload: %s", resp.Choices[0].Message.Content)
	}
	if len(resp.FallbackChain) < 2 {
		t.Fatalf("Expected fallback trace of at least 2 entries, got: %v", resp.FallbackChain)
	}

	// Verify primary circuit breaker recorded failure
	cbOpenAI := gw.GetCircuitBreaker("openai-primary")
	if cbOpenAI.State() != gateway.CircuitClosed { // 1 failure out of 2 threshold
		// Check fail count
		_, fails, _, _, _ := cbOpenAI.Snapshot()
		if fails != 1 {
			t.Fatalf("Expected 1 failure on openai-primary, got %d", fails)
		}
	}
}

func TestGateway_Normalizer_Protocols(t *testing.T) {
	norm := gateway.NewNormalizer()
	req := &gateway.GatewayRequest{
		Messages: []gateway.ChatMessage{
			{Role: "system", Content: "You are a concise assistant."},
			{Role: "user", Content: "Say hi."},
		},
		Temperature: 0.7,
	}

	// 1. OpenAI Format
	pOpenAI := &gateway.ProviderConfig{Protocol: gateway.ProtocolOpenAI}
	bytesOpenAI, err := norm.FormatRequest(req, pOpenAI, "gpt-4o")
	if err != nil || !strings.Contains(string(bytesOpenAI), "gpt-4o") {
		t.Fatalf("OpenAI formatting failed: %v, body: %s", err, string(bytesOpenAI))
	}

	// 2. Anthropic Format
	pAnthropic := &gateway.ProviderConfig{Protocol: gateway.ProtocolAnthropic}
	bytesAnthropic, err := norm.FormatRequest(req, pAnthropic, "claude-3-7-sonnet")
	if err != nil || !strings.Contains(string(bytesAnthropic), "claude-3-7-sonnet") || !strings.Contains(string(bytesAnthropic), "system") {
		t.Fatalf("Anthropic formatting failed: %v, body: %s", err, string(bytesAnthropic))
	}

	// 3. Gemini Format
	pGemini := &gateway.ProviderConfig{Protocol: gateway.ProtocolGemini}
	bytesGemini, err := norm.FormatRequest(req, pGemini, "gemini-1.5-pro")
	if err != nil || !strings.Contains(string(bytesGemini), "contents") {
		t.Fatalf("Gemini formatting failed: %v, body: %s", err, string(bytesGemini))
	}
}

func TestGateway_ScorecardAndMermaid(t *testing.T) {
	gw := createTestGateway()

	gw.SetDispatcher(func(ctx context.Context, p *gateway.ProviderConfig, targetModel string, payload []byte) ([]byte, int, error) {
		return []byte(`{
			"id": "chatcmpl_scorecard",
			"choices": [{"index": 0, "message": {"role": "assistant", "content": "Done"}, "finish_reason": "stop"}],
			"usage": {"prompt_tokens": 50, "completion_tokens": 25, "total_tokens": 75}
		}`), 200, nil
	})

	ctx := context.Background()
	_, _ = gw.Handle(ctx, &gateway.GatewayRequest{
		Messages: []gateway.ChatMessage{{Role: "user", Content: "Test query"}},
	})

	report := gw.GenerateReport()
	if report.TotalRequests != 1 {
		t.Fatalf("Expected TotalRequests=1, got %d", report.TotalRequests)
	}

	scorecard := gateway.RenderGatewayScorecard(report)
	if !strings.Contains(scorecard, "AGENTVCR // MODEL GATEWAY") {
		t.Fatalf("Scorecard missing header: %s", scorecard)
	}

	mermaid := gateway.RenderGatewayMermaid(report)
	if !strings.Contains(mermaid, "graph TD") || !strings.Contains(mermaid, "openai_primary") {
		t.Fatalf("Mermaid markup invalid: %s", mermaid)
	}
}

func TestGateway_ExhaustedCascade_ErrorHandling(t *testing.T) {
	gw := createTestGateway()

	// All providers fail
	gw.SetDispatcher(func(ctx context.Context, p *gateway.ProviderConfig, targetModel string, payload []byte) ([]byte, int, error) {
		return nil, 503, fmt.Errorf("outage: provider %s unavailable", p.ID)
	})

	ctx := context.Background()
	_, err := gw.Handle(ctx, &gateway.GatewayRequest{
		Messages: []gateway.ChatMessage{{Role: "user", Content: "Will fail across all providers"}},
	})

	if err == nil {
		t.Fatalf("Expected error when all cascade providers fail")
	}
	if !strings.Contains(err.Error(), "gateway cascade exhausted") {
		t.Fatalf("Unexpected error message: %v", err)
	}
}
