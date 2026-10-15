package tests

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/otel"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func TestOTelExporter_ExportToOTelSpans(t *testing.T) {
	now := time.Now()
	session := &trace.Session{
		ID:        "session-otel-test",
		AgentName: "telemetry-agent",
		Status:    "completed",
		CreatedAt: now,
		UpdatedAt: now.Add(5 * time.Second),
		Events: []trace.TraceEvent{
			{
				ID:         "ev-1",
				Index:      1,
				Timestamp:  now.Add(1 * time.Second),
				Type:       trace.EventLLMRequest,
				DurationMs: 150,
				Payload: map[string]interface{}{
					"model": "gpt-4o",
				},
			},
			{
				ID:         "ev-2",
				Index:      2,
				Timestamp:  now.Add(2 * time.Second),
				Type:       trace.EventToolCall,
				DurationMs: 45,
				Payload: map[string]interface{}{
					"tool_name": "fetch_metric",
				},
			},
		},
	}

	exp := otel.NewExporter("agentvcr-service")
	spans := exp.ExportToOTelSpans(session)

	// 1 root span + 2 event sub-spans = 3 spans
	if len(spans) != 3 {
		t.Fatalf("expected 3 spans, got %d", len(spans))
	}

	rootSpan := spans[0]
	if !strings.Contains(rootSpan.Name, "agent.session") {
		t.Errorf("expected root span to be agent.session, got %s", rootSpan.Name)
	}
	if len(rootSpan.TraceID) != 32 {
		t.Errorf("expected 32-char hex TraceID, got %s", rootSpan.TraceID)
	}

	// Sub-spans should reference root span ID as ParentSpanID
	for i := 1; i < len(spans); i++ {
		if spans[i].ParentSpanID != rootSpan.SpanID {
			t.Errorf("span %d ParentSpanID mismatch: expected %s, got %s", i, rootSpan.SpanID, spans[i].ParentSpanID)
		}
		if spans[i].TraceID != rootSpan.TraceID {
			t.Errorf("span %d TraceID mismatch: expected %s, got %s", i, rootSpan.TraceID, spans[i].TraceID)
		}
	}

	// Verify OTLP JSON serialization
	otlpBytes, err := exp.ExportOTLPJSON(session)
	if err != nil {
		t.Fatalf("failed to serialize OTLP JSON: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(otlpBytes, &parsed); err != nil {
		t.Fatalf("invalid json generated: %v", err)
	}
	if parsed["resource"] == nil || parsed["scope_spans"] == nil {
		t.Errorf("expected OTLP resource and scope_spans in root envelope")
	}
}
