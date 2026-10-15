package otel

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// OTelSpan represents an OpenTelemetry trace span compatible with W3C Trace Context and OTLP.
type OTelSpan struct {
	TraceID      string            `json:"trace_id"`
	SpanID       string            `json:"span_id"`
	ParentSpanID string            `json:"parent_span_id,omitempty"`
	Name         string            `json:"name"`
	Kind         string            `json:"kind"` // "INTERNAL", "CLIENT", "SERVER"
	StartTime    string            `json:"start_time"`
	EndTime      string            `json:"end_time"`
	DurationMs   int64             `json:"duration_ms"`
	Attributes   map[string]string `json:"attributes"`
	Status       OTelStatus        `json:"status"`
}

// OTelStatus denotes span execution state.
type OTelStatus struct {
	Code        string `json:"code"` // "OK", "ERROR", "UNSET"
	Description string `json:"description,omitempty"`
}

// OTelResourceSpans represents the top-level OTLP payload envelope.
type OTelResourceSpans struct {
	Resource struct {
		Attributes map[string]string `json:"attributes"`
	} `json:"resource"`
	ScopeSpans []struct {
		Scope struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"scope"`
		Spans []OTelSpan `json:"spans"`
	} `json:"scope_spans"`
}

// Exporter converts AgentVCR sessions into standards-compliant OpenTelemetry trace structures.
type Exporter struct {
	ServiceName string
}

// NewExporter initializes an OpenTelemetry exporter for AgentVCR.
func NewExporter(serviceName string) *Exporter {
	if serviceName == "" {
		serviceName = "agentvcr"
	}
	return &Exporter{ServiceName: serviceName}
}

// ExportToOTelSpans transforms an AgentVCR trace.Session into an array of OTelSpan objects.
func (e *Exporter) ExportToOTelSpans(session *trace.Session) []OTelSpan {
	if session == nil {
		return nil
	}

	traceID := generateTraceID(session.ID)
	rootSpanID := generateSpanID(session.ID + "_root")

	var spans []OTelSpan

	// Root Agent Session Span
	rootStart := session.CreatedAt
	rootEnd := session.UpdatedAt
	if rootEnd.Before(rootStart) {
		rootEnd = rootStart.Add(time.Second)
	}

	rootSpan := OTelSpan{
		TraceID:    traceID,
		SpanID:     rootSpanID,
		Name:       fmt.Sprintf("agent.session: %s", session.AgentName),
		Kind:       "SERVER",
		StartTime:  rootStart.Format(time.RFC3339Nano),
		EndTime:    rootEnd.Format(time.RFC3339Nano),
		DurationMs: rootEnd.Sub(rootStart).Milliseconds(),
		Attributes: map[string]string{
			"agent.name":       session.AgentName,
			"agent.session_id": session.ID,
			"agent.status":     session.Status,
		},
		Status: OTelStatus{Code: "OK"},
	}

	if session.Status == "failed" {
		rootSpan.Status = OTelStatus{Code: "ERROR", Description: "session execution failed"}
	}
	spans = append(spans, rootSpan)

	// Sub-spans for individual events
	for _, ev := range session.Events {
		spanID := generateSpanID(fmt.Sprintf("%s_%d_%s", session.ID, ev.Index, ev.ID))
		evStart := ev.Timestamp
		dur := ev.DurationMs
		if dur <= 0 {
			dur = 10
		}
		evEnd := evStart.Add(time.Duration(dur) * time.Millisecond)

		spanName := fmt.Sprintf("event.%s", ev.Type)
		kind := "INTERNAL"
		attrs := map[string]string{
			"event.type":       string(ev.Type),
			"event.step_index": fmt.Sprintf("%d", ev.Index),
			"agent.name":       session.AgentName,
		}

		switch ev.Type {
		case trace.EventLLMRequest, trace.EventLLMResponse:
			kind = "CLIENT"
			spanName = fmt.Sprintf("llm.%s", ev.Type)
			if model, ok := ev.Payload["model"].(string); ok {
				attrs["gen_ai.request.model"] = model
			}
		case trace.EventToolCall, trace.EventToolResult:
			kind = "CLIENT"
			spanName = fmt.Sprintf("tool.%s", ev.Type)
			if name, ok := ev.Payload["tool_name"].(string); ok {
				attrs["gen_ai.tool.name"] = name
			}
		case trace.EventError:
			spanName = "runtime.error"
		}

		spanStatus := OTelStatus{Code: "OK"}
		if ev.Type == trace.EventError {
			spanStatus = OTelStatus{Code: "ERROR", Description: "event error occurred"}
		}

		spans = append(spans, OTelSpan{
			TraceID:      traceID,
			SpanID:       spanID,
			ParentSpanID: rootSpanID,
			Name:         spanName,
			Kind:         kind,
			StartTime:    evStart.Format(time.RFC3339Nano),
			EndTime:      evEnd.Format(time.RFC3339Nano),
			DurationMs:   dur,
			Attributes:   attrs,
			Status:       spanStatus,
		})
	}

	return spans
}

// ExportOTLPJSON formats the session into the standard OTLP JSON trace export payload.
func (e *Exporter) ExportOTLPJSON(session *trace.Session) ([]byte, error) {
	spans := e.ExportToOTelSpans(session)

	var payload OTelResourceSpans
	payload.Resource.Attributes = map[string]string{
		"service.name":        e.ServiceName,
		"telemetry.sdk.name":  "agentvcr-otel",
		"agent.runtime":       "openbot",
	}

	scopeSpan := struct {
		Scope struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"scope"`
		Spans []OTelSpan `json:"spans"`
	}{
		Spans: spans,
	}
	scopeSpan.Scope.Name = "agentvcr"
	scopeSpan.Scope.Version = "v1.0.0"

	payload.ScopeSpans = append(payload.ScopeSpans, scopeSpan)
	return json.MarshalIndent(payload, "", "  ")
}

func generateTraceID(input string) string {
	h := sha256.Sum256([]byte(input + "_trace"))
	return hex.EncodeToString(h[:16]) // 128-bit / 32 hex chars
}

func generateSpanID(input string) string {
	h := sha256.Sum256([]byte(input + "_span"))
	return hex.EncodeToString(h[:8]) // 64-bit / 16 hex chars
}
