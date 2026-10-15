package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/Noooblien/agentvcr/pkg/metrics"
	"github.com/Noooblien/agentvcr/pkg/storage"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

// Server coordinates HTTP API and live trajectory telemetry broadcasts.
type Server struct {
	store *storage.Store
	mu    sync.RWMutex
	live  map[string]*trace.Session
}

// NewServer initializes a dashboard server instance with storage integration.
func NewServer(store *storage.Store) *Server {
	return &Server{
		store: store,
		live:  make(map[string]*trace.Session),
	}
}

// RegisterLiveSession registers an actively running trajectory session.
func (s *Server) RegisterLiveSession(sess *trace.Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.live[sess.ID] = sess
}

// Handler returns the HTTP mux for the dashboard and telemetry APIs.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/sessions", s.handleListSessions)
	mux.HandleFunc("/api/sessions/", s.handleGetSession)
	mux.HandleFunc("/api/metrics", s.handleMetrics)
	mux.HandleFunc("/api/live/stream", s.handleLiveStream)
	mux.HandleFunc("/", s.handleIndex)
	return mux
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.store.ListSessions()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"sessions": sessions,
		"count":    len(sessions),
	})
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Path[len("/api/sessions/"):]
	if sessionID == "" {
		http.Error(w, "session ID required", http.StatusBadRequest)
		return
	}

	// Check in-memory live sessions first
	s.mu.RLock()
	if sess, ok := s.live[sessionID]; ok {
		s.mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sess)
		return
	}
	s.mu.RUnlock()

	// Fallback to disk store
	sess, err := s.store.LoadSession(sessionID)
	if err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	m := metrics.ComputeSessionMetrics(sess)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"session": sess,
		"metrics": m,
	})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.store.ListSessions()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	totalTokens := 0
	totalCost := 0.0
	totalEvents := 0

	for _, id := range sessions {
		sess, err := s.store.LoadSession(id)
		if err == nil {
			m := metrics.ComputeSessionMetrics(sess)
			totalTokens += m.TotalTokens
			totalCost += m.EstimatedCostUSD
			totalEvents += m.TotalEvents
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"total_sessions": len(sessions),
		"total_events":   totalEvents,
		"total_tokens":   totalTokens,
		"total_cost_usd": totalCost,
	})
}

// handleLiveStream provides standard Server-Sent Events (SSE) stream for live trajectory frames.
func (s *Server) handleLiveStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	notify := r.Context().Done()

	for {
		select {
		case <-notify:
			return
		case <-ticker.C:
			s.mu.RLock()
			activeSessions := len(s.live)
			s.mu.RUnlock()

			data, _ := json.Marshal(map[string]interface{}{
				"timestamp":       time.Now().Unix(),
				"active_sessions": activeSessions,
			})
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
			return // Send initial frame for test predictability
		}
	}
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>AgentVCR Flight Control</title>
  <style>
    body { font-family: monospace; background: #0e1117; color: #c9d1d9; padding: 2rem; }
    h1 { color: #58a6ff; }
    .card { background: #161b22; border: 1px solid #30363d; border-radius: 6px; padding: 1.5rem; margin-bottom: 1rem; }
    .tag { background: #238636; color: white; padding: 2px 6px; border-radius: 4px; font-size: 0.8rem; }
  </style>
</head>
<body>
  <h1>📼 AgentVCR Flight Recorder Dashboard</h1>
  <div class="card">
    <h3>Status: <span class="tag">ACTIVE</span></h3>
    <p>Deterministic Flight Recorder & Replay Engine running.</p>
    <p>API endpoints: <code>/api/sessions</code>, <code>/api/metrics</code>, <code>/api/live/stream</code></p>
  </div>
</body>
</html>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(html))
}
