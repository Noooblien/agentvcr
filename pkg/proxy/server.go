package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"github.com/Noooblien/agentvcr/pkg/storage"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

// Server is a high-speed reverse proxy that intercepts and records LLM & tool calls.
type Server struct {
	targetURL *url.URL
	store     *storage.Store
	session   *trace.Session
	mu        sync.Mutex
	proxy     *httputil.ReverseProxy
}

// NewServer creates a new AgentVCR proxy recording server.
func NewServer(target string, store *storage.Store, sessionID, agentName string) (*Server, error) {
	u, err := url.Parse(target)
	if err != nil {
		return nil, fmt.Errorf("invalid upstream target URL: %w", err)
	}

	session := &trace.Session{
		ID:        sessionID,
		AgentName: agentName,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Events:    make([]trace.TraceEvent, 0),
		Status:    "recording",
	}

	srv := &Server{
		targetURL: u,
		store:     store,
		session:   session,
	}

	rp := httputil.NewSingleHostReverseProxy(u)
	rp.ModifyResponse = srv.interceptResponse
	srv.proxy = rp

	return srv, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	// Intercept and record request payload
	var reqBody []byte
	if r.Body != nil {
		reqBody, _ = io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewBuffer(reqBody))
	}

	s.mu.Lock()
	reqIndex := len(s.session.Events)
	var parsedReq map[string]interface{}
	_ = json.Unmarshal(reqBody, &parsedReq)

	event := trace.TraceEvent{
		ID:        fmt.Sprintf("evt_%d_%d", time.Now().UnixNano(), reqIndex),
		Index:     reqIndex,
		Timestamp: startTime,
		Type:      trace.EventLLMRequest,
		Payload: map[string]interface{}{
			"method":  r.Method,
			"path":    r.URL.Path,
			"headers": sanitizeHeaders(r.Header),
			"body":    parsedReq,
		},
	}
	s.session.Events = append(s.session.Events, event)
	_ = s.store.SaveSession(s.session)
	s.mu.Unlock()

	// Update host header for target
	r.Host = s.targetURL.Host
	s.proxy.ServeHTTP(w, r)
}

func (s *Server) interceptResponse(res *http.Response) error {
	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	res.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	s.mu.Lock()
	defer s.mu.Unlock()

	respIndex := len(s.session.Events)
	var parsedResp map[string]interface{}
	_ = json.Unmarshal(bodyBytes, &parsedResp)

	event := trace.TraceEvent{
		ID:        fmt.Sprintf("evt_%d_%d", time.Now().UnixNano(), respIndex),
		Index:     respIndex,
		Timestamp: time.Now(),
		Type:      trace.EventLLMResponse,
		Payload: map[string]interface{}{
			"status_code": res.StatusCode,
			"body":        parsedResp,
		},
	}
	s.session.Events = append(s.session.Events, event)
	s.session.UpdatedAt = time.Now()

	if err := s.store.SaveSession(s.session); err != nil {
		log.Printf("[agentvcr] warning: failed to flush trace to disk: %v", err)
	}

	return nil
}

func sanitizeHeaders(h http.Header) map[string]string {
	clean := make(map[string]string)
	for k, v := range h {
		if len(v) > 0 {
			if k == "Authorization" {
				clean[k] = "[REDACTED_API_KEY]"
			} else {
				clean[k] = v[0]
			}
		}
	}
	return clean
}
