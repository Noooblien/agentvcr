package tests

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/otel"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func TestOTel_LoadConfigFromEnvAndHeaders(t *testing.T) {
	os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "https://collector.internal:4318/v1/traces")
	os.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "api-key=secret123,x-tenant-id=acme-corp")
	os.Setenv("OTEL_EXPORTER_OTLP_INSECURE", "true")
	defer func() {
		os.Unsetenv("OTEL_EXPORTER_OTLP_ENDPOINT")
		os.Unsetenv("OTEL_EXPORTER_OTLP_HEADERS")
		os.Unsetenv("OTEL_EXPORTER_OTLP_INSECURE")
	}()

	cfg := otel.LoadConfigFromEnv()
	if cfg.Endpoint != "https://collector.internal:4318/v1/traces" {
		t.Errorf("unexpected endpoint: %s", cfg.Endpoint)
	}
	if cfg.Headers["api-key"] != "secret123" {
		t.Errorf("expected api-key=secret123, got: %s", cfg.Headers["api-key"])
	}
	if cfg.Headers["x-tenant-id"] != "acme-corp" {
		t.Errorf("expected x-tenant-id=acme-corp, got: %s", cfg.Headers["x-tenant-id"])
	}
	if !cfg.Insecure {
		t.Errorf("expected Insecure=true")
	}
}

func TestOTel_W3CTraceContext(t *testing.T) {
	tc := otel.TraceContext{
		TraceID:      "4bf92f3577b34da6a3ce929d0e0e4736",
		ParentSpanID: "00f067aa0ba902b7",
		Sampled:      true,
	}

	header := tc.FormatTraceparent()
	expected := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	if header != expected {
		t.Fatalf("expected header %s, got %s", expected, header)
	}

	parsed, err := otel.ParseTraceparent(header)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if parsed.TraceID != tc.TraceID {
		t.Errorf("TraceID mismatch: %s vs %s", parsed.TraceID, tc.TraceID)
	}
	if parsed.ParentSpanID != tc.ParentSpanID {
		t.Errorf("ParentSpanID mismatch: %s vs %s", parsed.ParentSpanID, tc.ParentSpanID)
	}
	if !parsed.Sampled {
		t.Errorf("expected Sampled=true")
	}

	// Test malformed header
	_, err = otel.ParseTraceparent("invalid-header")
	if err == nil {
		t.Errorf("expected error for malformed header")
	}
}

func TestOTel_MultiAgentCorrelator(t *testing.T) {
	now := time.Now()
	parentSession := &trace.Session{
		ID:        "parent-session-100",
		AgentName: "orchestrator-agent",
		Status:    "completed",
		CreatedAt: now,
		UpdatedAt: now.Add(10 * time.Second),
		Events: []trace.TraceEvent{
			{
				ID:         "ev-subtask",
				Index:      1,
				Timestamp:  now.Add(1 * time.Second),
				Type:       trace.EventToolCall,
				DurationMs: 200,
				Payload: map[string]interface{}{
					"tool_name": "subagent_researcher",
				},
			},
		},
	}

	childSession := &trace.Session{
		ID:        "child-session-200",
		AgentName: "subagent_researcher",
		Status:    "completed",
		CreatedAt: now.Add(2 * time.Second),
		UpdatedAt: now.Add(5 * time.Second),
		Events: []trace.TraceEvent{
			{
				ID:         "ev-query",
				Index:      1,
				Timestamp:  now.Add(3 * time.Second),
				Type:       trace.EventLLMRequest,
				DurationMs: 150,
				Payload: map[string]interface{}{
					"model": "gemini-2.5-pro",
				},
			},
		},
	}

	correlator := otel.NewMultiAgentCorrelator("agentvcr-fleet-test")
	fleetTrace := correlator.CorrelateFleet(parentSession, childSession)

	if fleetTrace == nil {
		t.Fatalf("expected non-nil fleet trace")
	}
	if fleetTrace.TotalAgents != 2 {
		t.Errorf("expected 2 agents, got %d", fleetTrace.TotalAgents)
	}

	// Verify all spans in fleet share the primary agent's root TraceID
	rootTraceID := fleetTrace.RootTraceID
	for i, sp := range fleetTrace.Spans {
		if sp.TraceID != rootTraceID {
			t.Errorf("span %d (%s) traceID mismatch: expected %s, got %s", i, sp.Name, rootTraceID, sp.TraceID)
		}
	}

	// Find child root span
	var childRoot *otel.OTelSpan
	for _, sp := range fleetTrace.Spans {
		if sp.Attributes["agent.name"] == "subagent_researcher" && sp.Kind == "SERVER" {
			childRoot = &sp
			break
		}
	}
	if childRoot == nil {
		t.Fatalf("child root span not found")
	}
	if childRoot.Attributes["agent.parent_session_id"] != "parent-session-100" {
		t.Errorf("expected parent session ID parent-session-100, got %s", childRoot.Attributes["agent.parent_session_id"])
	}
	if childRoot.Attributes["w3c.traceparent"] == "" {
		t.Errorf("expected w3c.traceparent attribute on correlated child span")
	}
}

func TestOTel_AsyncCollectorExporter_EndToEnd(t *testing.T) {
	var receivedPayloads int32
	var receivedSpans int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "secret-agent-key" {
			t.Errorf("missing or incorrect X-Api-Key header: %s", r.Header.Get("X-Api-Key"))
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		var payload otel.OTelResourceSpans
		if err := json.Unmarshal(body, &payload); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}

		for _, scope := range payload.ScopeSpans {
			atomic.AddInt32(&receivedSpans, int32(len(scope.Spans)))
		}
		atomic.AddInt32(&receivedPayloads, 1)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"success"}`))
	}))
	defer server.Close()

	cfg := otel.CollectorConfig{
		Endpoint:       server.URL,
		Headers:        map[string]string{"X-Api-Key": "secret-agent-key"},
		BatchSize:      2,
		FlushInterval:  100 * time.Millisecond,
		MaxQueueSize:   100,
		MaxRetries:     2,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     50 * time.Millisecond,
		Timeout:        2 * time.Second,
		ServiceName:    "test-service",
	}

	exporter := otel.NewAsyncCollectorExporter(cfg)

	now := time.Now()
	session := &trace.Session{
		ID:        "test-session-batch",
		AgentName: "batch-agent",
		Status:    "completed",
		CreatedAt: now,
		UpdatedAt: now.Add(2 * time.Second),
		Events: []trace.TraceEvent{
			{
				ID:         "ev-1",
				Index:      1,
				Timestamp:  now.Add(500 * time.Millisecond),
				Type:       trace.EventToolCall,
				DurationMs: 50,
			},
		},
	}

	queued := exporter.EnqueueSession(session)
	if queued == 0 {
		t.Fatalf("expected queued spans > 0")
	}

	// Give background worker time to flush batch
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = exporter.Flush(ctx)

	err := exporter.Close()
	if err != nil {
		t.Fatalf("failed to close exporter: %v", err)
	}

	metrics := exporter.GetMetrics()
	if metrics.ExportedSpans <= 0 {
		t.Errorf("expected ExportedSpans > 0, got %d", metrics.ExportedSpans)
	}
	if metrics.BatchesSent <= 0 {
		t.Errorf("expected BatchesSent > 0, got %d", metrics.BatchesSent)
	}
	if atomic.LoadInt32(&receivedSpans) == 0 {
		t.Errorf("server received 0 spans")
	}
}

func TestOTel_AsyncCollectorExporter_ExponentialBackoffRetry(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur := atomic.AddInt32(&attempts, 1)
		if cur < 3 {
			http.Error(w, "internal server error (simulated network partition)", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"recovered"}`))
	}))
	defer server.Close()

	cfg := otel.CollectorConfig{
		Endpoint:       server.URL,
		BatchSize:      1,
		FlushInterval:  50 * time.Millisecond,
		MaxQueueSize:   50,
		MaxRetries:     4,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     50 * time.Millisecond,
		Timeout:        2 * time.Second,
		ServiceName:    "retry-test-service",
	}

	exporter := otel.NewAsyncCollectorExporter(cfg)
	span := otel.OTelSpan{
		TraceID:    "trace12345678901234567890123456",
		SpanID:     "span123456789012",
		Name:       "retry.span",
		Kind:       "INTERNAL",
		StartTime:  time.Now().Format(time.RFC3339Nano),
		EndTime:    time.Now().Add(10 * time.Millisecond).Format(time.RFC3339Nano),
		DurationMs: 10,
	}

	exporter.Enqueue(span)

	// Wait for backoff retries to succeed on attempt 3
	time.Sleep(300 * time.Millisecond)

	_ = exporter.Close()

	metrics := exporter.GetMetrics()
	if atomic.LoadInt32(&attempts) < 3 {
		t.Errorf("expected at least 3 attempts with backoff, got %d", atomic.LoadInt32(&attempts))
	}
	if metrics.ExportedSpans != 1 {
		t.Errorf("expected 1 exported span after recovery, got %d", metrics.ExportedSpans)
	}
	if metrics.FailedAttempts < 2 {
		t.Errorf("expected at least 2 failed attempts recorded, got %d", metrics.FailedAttempts)
	}
}
