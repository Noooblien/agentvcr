package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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

	// Test 4: GET / (Dashboard HTML index)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK for html, got %d", rec.Code)
	}
}
