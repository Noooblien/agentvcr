package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
	"github.com/Noooblien/agentvcr/pkg/watermark"
)

func TestWatermark_EmbedAndExtractZeroWidth(t *testing.T) {
	cfg := watermark.WatermarkConfig{
		Strategy:  watermark.StrategyZeroWidthUnicode,
		SecretKey: "super-secret-test-key",
		OrgID:     "org-enterprise-corp",
	}
	engine := watermark.NewEngine(cfg)

	payload := engine.GeneratePayload("sess-100", "agent-alpha", "claude-3-5-sonnet")

	originalText := "The database migration script completed successfully. All 5,000 records have been partitioned."
	watermarkedText, err := engine.EmbedWatermark(originalText, payload)
	if err != nil {
		t.Fatalf("Failed to embed watermark: %v", err)
	}

	// Text should visually look like the original (zero-width characters are invisible)
	cleanVisible := watermark.StripWatermarks(watermarkedText)
	if cleanVisible != originalText {
		t.Fatalf("Cleaned visible text does not match original: got '%s'", cleanVisible)
	}

	// Extract and verify
	result := engine.ExtractWatermark(watermarkedText)
	if !result.Detected {
		t.Fatalf("Expected watermark to be detected in text")
	}
	if result.Tampered {
		t.Fatalf("Expected watermark signature to be valid, got tampered=true")
	}
	if result.Payload == nil {
		t.Fatalf("Extracted payload is nil")
	}
	if result.Payload.SessionID != "sess-100" || result.Payload.AgentID != "agent-alpha" {
		t.Fatalf("Payload mismatch: %+v", result.Payload)
	}
	if result.BitConfidence != 1.0 {
		t.Fatalf("Expected confidence 1.0, got: %.2f", result.BitConfidence)
	}
}

func TestWatermark_TamperingDetection(t *testing.T) {
	cfg := watermark.WatermarkConfig{
		Strategy:  watermark.StrategyZeroWidthUnicode,
		SecretKey: "key-a",
		OrgID:     "org-1",
	}
	engineA := watermark.NewEngine(cfg)

	payload := engineA.GeneratePayload("sess-auth-1", "agent-1", "gpt-4o")
	text, _ := engineA.EmbedWatermark("System authorization granted.", payload)

	// Receiver with mismatched key (or altered payload) tries to verify
	cfgWrongKey := watermark.WatermarkConfig{
		Strategy:  watermark.StrategyZeroWidthUnicode,
		SecretKey: "key-wrong-intruder",
		OrgID:     "org-1",
	}
	engineWrong := watermark.NewEngine(cfgWrongKey)

	result := engineWrong.ExtractWatermark(text)
	if !result.Detected {
		t.Fatalf("Watermark should be detected")
	}
	if !result.Tampered {
		t.Fatalf("Expected watermark to be flagged as TAMPERED due to wrong key/signature mismatch")
	}
}

func TestWatermark_HeaderSignatureStrategy(t *testing.T) {
	cfg := watermark.WatermarkConfig{
		Strategy:  watermark.StrategyHeaderSignature,
		SecretKey: "header-secret-key",
		OrgID:     "acme-ai",
	}
	engine := watermark.NewEngine(cfg)

	payload := engine.GeneratePayload("sess-header-99", "agent-planner", "gemini-1.5-pro")
	text, _ := engine.EmbedWatermark("Execution plan initialized.", payload)

	if !strings.Contains(text, "<!-- AGENTVCR-WATERMARK:") {
		t.Fatalf("Expected header signature in text: %s", text)
	}

	result := engine.ExtractWatermark(text)
	if !result.Detected {
		t.Fatalf("Expected header watermark to be detected")
	}
	if result.Tampered {
		t.Fatalf("Expected valid signature on header watermark")
	}
	if result.Payload.SessionID != "sess-header-99" {
		t.Fatalf("Session ID mismatch: %s", result.Payload.SessionID)
	}
}

func TestWatermark_TrajectoryAuditScorecardAndMermaid(t *testing.T) {
	cfg := watermark.WatermarkConfig{
		Strategy:  watermark.StrategyZeroWidthUnicode,
		SecretKey: "audit-secret-key",
		OrgID:     "audit-org",
	}
	engine := watermark.NewEngine(cfg)

	payload := engine.GeneratePayload("sess-audit-1", "agent-audit", "gpt-4o")
	text1, _ := engine.EmbedWatermark("Step 1 execution output.", payload)
	text2, _ := engine.EmbedWatermark("Step 2 verified completion.", payload)

	session := &trace.Session{
		ID:        "sess-audit-1",
		AgentName: "audited-agent",
		CreatedAt: time.Now(),
		Events: []trace.TraceEvent{
			{
				ID:    "evt-1",
				Index: 0,
				Type:  trace.EventLLMResponse,
				Payload: map[string]interface{}{
					"content": text1,
				},
			},
			{
				ID:    "evt-2",
				Index: 1,
				Type:  trace.EventLLMResponse,
				Payload: map[string]interface{}{
					"content": text2,
				},
			},
		},
	}

	report := engine.AuditTrajectory(session)
	if !report.AttestationValid {
		t.Fatalf("Expected valid attestation")
	}
	if report.OverallIntegrityScore != 10.0 {
		t.Fatalf("Expected 10.0 integrity score, got: %.1f", report.OverallIntegrityScore)
	}
	if report.WatermarksExtracted != 2 {
		t.Fatalf("Expected 2 extracted watermarks, got %d", report.WatermarksExtracted)
	}

	scorecard := watermark.RenderWatermarkScorecard(report)
	if !strings.Contains(scorecard, "AGENTVCR // STEGANOGRAPHIC PROVENANCE WATERMARK") {
		t.Fatalf("Scorecard missing header: %s", scorecard)
	}
	if !strings.Contains(scorecard, "[VALID]") {
		t.Fatalf("Scorecard missing [VALID] status: %s", scorecard)
	}

	mermaid := watermark.RenderWatermarkMermaid(report)
	if !strings.Contains(mermaid, "graph TD") || !strings.Contains(mermaid, "ATTESTATION_PASS") {
		t.Fatalf("Mermaid diagram missing expected structure: %s", mermaid)
	}
}
