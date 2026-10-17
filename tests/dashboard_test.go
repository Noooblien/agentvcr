package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/dashboard"
	"github.com/Noooblien/agentvcr/pkg/storage"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func TestDashboardServer_APIs(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agentvcr_dash_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store, err := storage.NewStore(tempDir)
	if err != nil {
		t.Fatalf("failed to init store: %v", err)
	}

	session := &trace.Session{
		ID:        "dash-test-1",
		AgentName: "flight-bot",
		CreatedAt: time.Now(),
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventToolCall, Payload: map[string]interface{}{"tool": "verify"}},
		},
	}
	if err := store.SaveSession(session); err != nil {
		t.Fatalf("failed to save session: %v", err)
	}

	srv := dashboard.NewServer(store)
	defer srv.Hub().Close()
	handler := srv.Handler()

	// Test 1: GET /api/sessions
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/sessions", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rec.Code)
	}
	var listResp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if int(listResp["count"].(float64)) != 1 {
		t.Errorf("expected 1 session in count, got %v", listResp["count"])
	}

	// Test 2: GET /api/sessions/dash-test-1
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/api/sessions/dash-test-1", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK for get session, got %d", rec.Code)
	}

	// Test 3: GET /api/metrics
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/api/metrics", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK for metrics, got %d", rec.Code)
	}

	// Test 4: GET /healthz
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/healthz", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK for healthz, got %d", rec.Code)
	}

	// Test 5: POST /api/fork
	forkPayload, _ := json.Marshal(map[string]interface{}{
		"parent_id":      "dash-test-1",
		"fork_step":      1,
		"new_session_id": "dash-test-1-fork",
	})
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/fork", bytes.NewBuffer(forkPayload))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201 Created for fork, got %d: %s", rec.Code, rec.Body.String())
	}

	// Test 6: GET / (Dashboard HTML index)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK for html, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "AgentVCR Flight Control") {
		t.Errorf("expected html to contain AgentVCR Flight Control")
	}
}

func TestTelemetryHub_PubSubAndSSEStream(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agentvcr_pubsub_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store, _ := storage.NewStore(tempDir)
	srv := dashboard.NewServer(store)
	defer srv.Hub().Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/stream/events", nil).WithContext(ctx)

	go func() {
		// Wait for subscriber registration
		time.Sleep(50 * time.Millisecond)

		// Broadcast a test frame
		srv.Hub().Broadcast(dashboard.TelemetryFrame{
			Type:      dashboard.EventTraceCaptured,
			SessionID: "sess-live-99",
			Event: &trace.TraceEvent{
				Index: 1,
				Type:  trace.EventToolCall,
			},
		})

		time.Sleep(50 * time.Millisecond)
		cancel() // Terminate SSE stream
	}()

	srv.Handler().ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "event: ack") {
		t.Errorf("expected event: ack in SSE output, got: %s", body)
	}
	if !strings.Contains(body, "trace_captured") {
		t.Errorf("expected trace_captured event in SSE output, got: %s", body)
	}
}
