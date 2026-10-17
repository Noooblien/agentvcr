package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/Noooblien/agentvcr/pkg/metrics"
	"github.com/Noooblien/agentvcr/pkg/replay"
	"github.com/Noooblien/agentvcr/pkg/storage"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

// BroadcastEventType defines categories of real-time stream messages.
type BroadcastEventType string

const (
	EventSessionStarted  BroadcastEventType = "session_started"
	EventTraceCaptured   BroadcastEventType = "trace_captured"
	EventSessionFinished BroadcastEventType = "session_finished"
	EventReplayStep      BroadcastEventType = "replay_step"
	EventFleetHeartbeat  BroadcastEventType = "fleet_heartbeat"
)

// TelemetryFrame represents an in-flight real-time event sent over SSE and WebSocket.
type TelemetryFrame struct {
	Type      BroadcastEventType     `json:"type"`
	Timestamp int64                  `json:"timestamp"`
	SessionID string                 `json:"session_id,omitempty"`
	Event     *trace.TraceEvent      `json:"event,omitempty"`
	Metrics   *metrics.SessionMetrics `json:"metrics,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

// ClientChannel represents an active real-time subscriber.
type ClientChannel chan TelemetryFrame

// TelemetryHub orchestrates real-time fan-out pub/sub to SSE and streaming clients.
type TelemetryHub struct {
	mu         sync.RWMutex
	clients    map[ClientChannel]bool
	register   chan ClientChannel
	unregister chan ClientChannel
	broadcast  chan TelemetryFrame
	done       chan struct{}
}

// NewTelemetryHub creates an active Pub/Sub telemetry event hub.
func NewTelemetryHub() *TelemetryHub {
	hub := &TelemetryHub{
		clients:    make(map[ClientChannel]bool),
		register:   make(chan ClientChannel, 32),
		unregister: make(chan ClientChannel, 32),
		broadcast:  make(chan TelemetryFrame, 1024),
		done:       make(chan struct{}),
	}
	go hub.run()
	return hub
}

func (h *TelemetryHub) run() {
	for {
		select {
		case <-h.done:
			h.mu.Lock()
			for ch := range h.clients {
				close(ch)
			}
			h.clients = make(map[ClientChannel]bool)
			h.mu.Unlock()
			return

		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client)
			}
			h.mu.Unlock()

		case frame := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client <- frame:
				default:
					// Drop slow consumer frame to prevent head-of-line blocking
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Close gracefully terminates the telemetry hub.
func (h *TelemetryHub) Close() {
	close(h.done)
}

// Broadcast sends a telemetry frame to all active stream subscribers.
func (h *TelemetryHub) Broadcast(frame TelemetryFrame) {
	if frame.Timestamp == 0 {
		frame.Timestamp = time.Now().UnixNano()
	}
	select {
	case h.broadcast <- frame:
	default:
		// Queue full, drop non-blocking
	}
}

// SubscriberCount returns the current number of active streaming consumers.
func (h *TelemetryHub) SubscriberCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// Server coordinates HTTP API, interactive web console, and live streaming telemetry.
type Server struct {
	store  *storage.Store
	replay *replay.Engine
	hub    *TelemetryHub
	mu     sync.RWMutex
	live   map[string]*trace.Session
}

// NewServer initializes a dashboard server instance with storage and pub/sub integration.
func NewServer(store *storage.Store) *Server {
	return &Server{
		store:  store,
		replay: replay.NewEngine(store),
		hub:    NewTelemetryHub(),
		live:   make(map[string]*trace.Session),
	}
}

// Hub returns the underlying pub/sub event broadcaster.
func (s *Server) Hub() *TelemetryHub {
	return s.hub
}

// RegisterLiveSession registers an actively running trajectory session and broadcasts notice.
func (s *Server) RegisterLiveSession(sess *trace.Session) {
	s.mu.Lock()
	s.live[sess.ID] = sess
	s.mu.Unlock()

	s.hub.Broadcast(TelemetryFrame{
		Type:      EventSessionStarted,
		Timestamp: time.Now().UnixNano(),
		SessionID: sess.ID,
		Data: map[string]interface{}{
			"agent_name": sess.AgentName,
			"status":     sess.Status,
		},
	})
}

// RecordEvent records an in-flight trace event and broadcasts it instantaneously to live subscribers.
func (s *Server) RecordEvent(sessionID string, evt trace.TraceEvent) {
	s.mu.Lock()
	if sess, ok := s.live[sessionID]; ok {
		sess.Events = append(sess.Events, evt)
		sess.UpdatedAt = time.Now()
		_ = s.store.SaveSession(sess)
	}
	s.mu.Unlock()

	s.hub.Broadcast(TelemetryFrame{
		Type:      EventTraceCaptured,
		Timestamp: time.Now().UnixNano(),
		SessionID: sessionID,
		Event:     &evt,
	})
}

// Handler returns the HTTP mux for the dashboard, REST API, SSE streaming, and interactive fork controller.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/sessions", s.handleListSessions)
	mux.HandleFunc("/api/sessions/", s.handleGetSession)
	mux.HandleFunc("/api/fork", s.handleForkSession)
	mux.HandleFunc("/api/metrics", s.handleMetrics)
	mux.HandleFunc("/api/live/stream", s.handleLiveStream)
	mux.HandleFunc("/api/stream/events", s.handleSSEHubStream)
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/", s.handleIndex)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":      "ok",
		"subscribers": s.hub.SubscriberCount(),
		"timestamp":   time.Now().UTC(),
	})
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

	s.mu.RLock()
	if sess, ok := s.live[sessionID]; ok {
		s.mu.RUnlock()
		m := metrics.ComputeSessionMetrics(sess)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"session": sess,
			"metrics": m,
			"is_live": true,
		})
		return
	}
	s.mu.RUnlock()

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
		"is_live": false,
	})
}

func (s *Server) handleForkSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ParentID     string `json:"parent_id"`
		ForkStep     int    `json:"fork_step"`
		NewSessionID string `json:"new_session_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.NewSessionID == "" {
		req.NewSessionID = fmt.Sprintf("%s_fork_%d", req.ParentID, time.Now().Unix())
	}

	branch, err := s.replay.ForkSession(req.ParentID, req.ForkStep, req.NewSessionID)
	if err != nil {
		http.Error(w, fmt.Sprintf("fork failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "forked",
		"session": branch,
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

// handleLiveStream provides legacy SSE stream compatibility.
func (s *Server) handleLiveStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	s.mu.RLock()
	activeSessions := len(s.live)
	s.mu.RUnlock()

	data, _ := json.Marshal(map[string]interface{}{
		"timestamp":       time.Now().Unix(),
		"active_sessions": activeSessions,
	})
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}

// handleSSEHubStream implements full Server-Sent Events live telemetry fanout from TelemetryHub.
func (s *Server) handleSSEHubStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	clientChan := make(ClientChannel, 64)
	s.hub.register <- clientChan
	defer func() {
		s.hub.unregister <- clientChan
	}()

	// Send initial connection ACK
	ack, _ := json.Marshal(map[string]interface{}{
		"type":        "connected",
		"timestamp":   time.Now().UnixNano(),
		"subscribers": s.hub.SubscriberCount(),
	})
	fmt.Fprintf(w, "event: ack\ndata: %s\n\n", ack)
	flusher.Flush()

	notify := r.Context().Done()
	for {
		select {
		case <-notify:
			return
		case frame, ok := <-clientChan:
			if !ok {
				return
			}
			data, err := json.Marshal(frame)
			if err == nil {
				fmt.Fprintf(w, "event: %s\ndata: %s\n\n", frame.Type, data)
				flusher.Flush()
			}
		}
	}
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>AgentVCR Flight Control — Autonomous Trajectory Inspector</title>
  <style>
    :root {
      --bg: #0b0e14;
      --card-bg: #151b23;
      --border: #30363d;
      --accent: #58a6ff;
      --green: #3fb950;
      --yellow: #d29922;
      --red: #f85149;
      --text: #c9d1d9;
      --text-muted: #8b949e;
    }
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body { font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; background: var(--bg); color: var(--text); padding: 2rem; }
    header { display: flex; justify-content: space-between; align-items: center; border-bottom: 1px solid var(--border); padding-bottom: 1rem; margin-bottom: 2rem; }
    h1 { color: var(--accent); font-size: 1.5rem; display: flex; align-items: center; gap: 0.5rem; }
    .badge { background: #1f6feb22; border: 1px solid #1f6feb; color: var(--accent); padding: 2px 8px; border-radius: 12px; font-size: 0.75rem; }
    .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); gap: 1rem; margin-bottom: 2rem; }
    .card { background: var(--card-bg); border: 1px solid var(--border); border-radius: 8px; padding: 1.25rem; }
    .card-title { color: var(--text-muted); font-size: 0.8rem; text-transform: uppercase; letter-spacing: 0.05em; margin-bottom: 0.5rem; }
    .card-value { font-size: 1.75rem; font-weight: bold; color: #fff; }
    .pulse { display: inline-block; width: 8px; height: 8px; border-radius: 50%; background: var(--green); box-shadow: 0 0 8px var(--green); margin-right: 6px; }
    table { width: 100%; border-collapse: collapse; margin-top: 1rem; }
    th, td { text-align: left; padding: 0.75rem 1rem; border-bottom: 1px solid var(--border); font-size: 0.85rem; }
    th { color: var(--text-muted); background: #161b22; }
    tr:hover { background: #161b2288; }
    .code-pill { background: #21262d; padding: 2px 6px; border-radius: 4px; font-size: 0.8rem; }
    #live-feed { max-height: 300px; overflow-y: auto; background: #07090d; border: 1px solid var(--border); border-radius: 6px; padding: 1rem; font-size: 0.8rem; line-height: 1.6; }
    .feed-entry { border-bottom: 1px solid #1c2128; padding: 4px 0; }
  </style>
</head>
<body>
  <header>
    <h1><span>📼</span> AgentVCR Flight Control <span class="badge">v1.0 Live</span></h1>
    <div><span class="pulse"></span> <strong>STATUS:</strong> <span style="color:var(--green)">REPLAY ENGINE ACTIVE</span></div>
  </header>

  <div class="grid">
    <div class="card">
      <div class="card-title">Recorded Trajectories</div>
      <div class="card-value" id="val-sessions">—</div>
    </div>
    <div class="card">
      <div class="card-title">Total Intercepted Events</div>
      <div class="card-value" id="val-events">—</div>
    </div>
    <div class="card">
      <div class="card-title">Total Token Footprint</div>
      <div class="card-value" id="val-tokens">—</div>
    </div>
    <div class="card">
      <div class="card-title">Estimated Cost (USD)</div>
      <div class="card-value" id="val-cost" style="color:var(--yellow)">—</div>
    </div>
  </div>

  <div class="card" style="margin-bottom: 2rem;">
    <div class="card-title">⚡ Real-Time Flight Telemetry Stream (SSE /pub-sub)</div>
    <div id="live-feed">
      <div class="feed-entry" style="color:var(--text-muted)">Connecting to live telemetry bus...</div>
    </div>
  </div>

  <div class="card">
    <div class="card-title">📁 Recent Agent Flight Traces</div>
    <table>
      <thead>
        <tr>
          <th>Session ID</th>
          <th>Status</th>
          <th>Actions</th>
        </tr>
      </thead>
      <tbody id="sessions-table-body">
        <tr><td colspan="3" style="color:var(--text-muted)">Loading traces...</td></tr>
      </tbody>
    </table>
  </div>

  <script>
    async function loadMetrics() {
      try {
        const res = await fetch('/api/metrics');
        const data = await res.json();
        document.getElementById('val-sessions').innerText = data.total_sessions || 0;
        document.getElementById('val-events').innerText = data.total_events || 0;
        document.getElementById('val-tokens').innerText = (data.total_tokens || 0).toLocaleString();
        document.getElementById('val-cost').innerText = '$' + (data.total_cost_usd || 0).toFixed(4);
      } catch(e) { console.error(e); }
    }

    async function loadSessions() {
      try {
        const res = await fetch('/api/sessions');
        const data = await res.json();
        const tbody = document.getElementById('sessions-table-body');
        if (!data.sessions || data.sessions.length === 0) {
          tbody.innerHTML = '<tr><td colspan="3" style="color:var(--text-muted)">No traces recorded yet. Run <code>agentvcr record</code>.</td></tr>';
          return;
        }
        tbody.innerHTML = data.sessions.map(function(id) {
          return '<tr>' +
            '<td><code class="code-pill">' + id + '</code></td>' +
            '<td><span style="color:var(--green)">Recorded</span></td>' +
            '<td><a href="/api/sessions/' + id + '" target="_blank" style="color:var(--accent); text-decoration:none;">[Inspect JSON]</a></td>' +
          '</tr>';
        }).join('');
      } catch(e) { console.error(e); }
    }

    function initStream() {
      const feed = document.getElementById('live-feed');
      const es = new EventSource('/api/stream/events');
      es.addEventListener('ack', function(e) {
        feed.innerHTML += '<div class="feed-entry" style="color:var(--green)">⚡ Connected to AgentVCR telemetry bus</div>';
      });
      es.addEventListener('trace_captured', function(e) {
        const frame = JSON.parse(e.data);
        const div = document.createElement('div');
        div.className = 'feed-entry';
        div.innerHTML = '<span style="color:var(--accent)">[' + new Date().toLocaleTimeString() + ']</span> <strong>' + frame.session_id + '</strong>: Event #' + frame.event.index + ' (' + frame.event.type + ')';
        feed.prepend(div);
      });
      es.onerror = function() {
        setTimeout(initStream, 3000);
      };
    }

    loadMetrics();
    loadSessions();
    initStream();
  </script>
</body>
</html>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(html))
}
