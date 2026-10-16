package otel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// Standard OTLP Environment Variables
const (
	EnvOTLPEndpoint       = "OTEL_EXPORTER_OTLP_ENDPOINT"
	EnvOTLPTracesEndpoint = "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"
	EnvOTLPHeaders        = "OTEL_EXPORTER_OTLP_HEADERS"
	EnvOTLPTracesHeaders  = "OTEL_EXPORTER_OTLP_TRACES_HEADERS"
	EnvOTLPInsecure       = "OTEL_EXPORTER_OTLP_INSECURE"
)

// CollectorConfig parameterizes connection and batching parameters for OTLP collectors.
type CollectorConfig struct {
	Endpoint       string            `json:"endpoint"`
	Headers        map[string]string `json:"headers"`
	BatchSize      int               `json:"batch_size"`
	FlushInterval  time.Duration     `json:"flush_interval"`
	MaxQueueSize   int               `json:"max_queue_size"`
	MaxRetries     int               `json:"max_retries"`
	InitialBackoff time.Duration     `json:"initial_backoff"`
	MaxBackoff     time.Duration     `json:"max_backoff"`
	Timeout        time.Duration     `json:"timeout"`
	Insecure       bool              `json:"insecure"`
	ServiceName    string            `json:"service_name"`
}

// DefaultCollectorConfig returns standard production defaults.
func DefaultCollectorConfig() CollectorConfig {
	return CollectorConfig{
		Endpoint:       "http://localhost:4318/v1/traces",
		Headers:        make(map[string]string),
		BatchSize:      100,
		FlushInterval:  1 * time.Second,
		MaxQueueSize:   10000,
		MaxRetries:     5,
		InitialBackoff: 50 * time.Millisecond,
		MaxBackoff:     2 * time.Second,
		Timeout:        10 * time.Second,
		Insecure:       false,
		ServiceName:    "agentvcr-collector",
	}
}

// LoadConfigFromEnv builds configuration from standard OpenTelemetry environment variables.
func LoadConfigFromEnv() CollectorConfig {
	cfg := DefaultCollectorConfig()

	if ep := os.Getenv(EnvOTLPTracesEndpoint); ep != "" {
		cfg.Endpoint = ep
	} else if ep := os.Getenv(EnvOTLPEndpoint); ep != "" {
		if !strings.HasSuffix(ep, "/v1/traces") && !strings.Contains(ep, ":4317") {
			cfg.Endpoint = strings.TrimSuffix(ep, "/") + "/v1/traces"
		} else {
			cfg.Endpoint = ep
		}
	}

	headersStr := os.Getenv(EnvOTLPTracesHeaders)
	if headersStr == "" {
		headersStr = os.Getenv(EnvOTLPHeaders)
	}
	if headersStr != "" {
		cfg.Headers = ParseHeaders(headersStr)
	}

	if insec := os.Getenv(EnvOTLPInsecure); strings.EqualFold(insec, "true") || insec == "1" {
		cfg.Insecure = true
	}

	return cfg
}

// ParseHeaders parses key=value comma-delimited headers per OTel specification.
func ParseHeaders(raw string) map[string]string {
	headers := make(map[string]string)
	pairs := strings.Split(raw, ",")
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 2 {
			headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return headers
}

// CollectorMetrics monitors telemetry pipeline throughput, drops, and backoff health.
type CollectorMetrics struct {
	ExportedSpans  int64     `json:"exported_spans"`
	DroppedSpans   int64     `json:"dropped_spans"`
	BatchesSent    int64     `json:"batches_sent"`
	FailedAttempts int64     `json:"failed_attempts"`
	LastFlush      time.Time `json:"last_flush"`
	LastError      string    `json:"last_error,omitempty"`
}

// AsyncCollectorExporter batches and ships OTel spans asynchronously to remote collectors with backoff.
type AsyncCollectorExporter struct {
	cfg        CollectorConfig
	queue      chan OTelSpan
	httpClient *http.Client
	stopChan   chan struct{}
	doneChan   chan struct{}
	exporter   *Exporter

	mu       sync.RWMutex
	metrics  CollectorMetrics
	isClosed uint32
}

// NewAsyncCollectorExporter instantiates and boots the background worker thread.
func NewAsyncCollectorExporter(cfg CollectorConfig) *AsyncCollectorExporter {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 100
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = 1 * time.Second
	}
	if cfg.MaxQueueSize <= 0 {
		cfg.MaxQueueSize = 10000
	}
	if cfg.InitialBackoff <= 0 {
		cfg.InitialBackoff = 50 * time.Millisecond
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = 2 * time.Second
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}

	exp := &AsyncCollectorExporter{
		cfg:        cfg,
		queue:      make(chan OTelSpan, cfg.MaxQueueSize),
		stopChan:   make(chan struct{}),
		doneChan:   make(chan struct{}),
		exporter:   NewExporter(cfg.ServiceName),
		httpClient: &http.Client{Timeout: cfg.Timeout},
	}

	go exp.worker()
	return exp
}

// Enqueue places a span into the asynchronous buffer. Returns false if dropped due to queue saturation.
func (a *AsyncCollectorExporter) Enqueue(spans ...OTelSpan) bool {
	if atomic.LoadUint32(&a.isClosed) == 1 {
		return false
	}

	allAccepted := true
	for _, span := range spans {
		select {
		case a.queue <- span:
		default:
			atomic.AddInt64(&a.metrics.DroppedSpans, 1)
			allAccepted = false
		}
	}
	return allAccepted
}

// EnqueueSession transforms an AgentVCR trace session into OTel spans and queues them.
func (a *AsyncCollectorExporter) EnqueueSession(session *trace.Session) int {
	spans := a.exporter.ExportToOTelSpans(session)
	accepted := 0
	for _, sp := range spans {
		if a.Enqueue(sp) {
			accepted++
		}
	}
	return accepted
}

// worker coordinates batch collection, ticker flushes, and background delivery.
func (a *AsyncCollectorExporter) worker() {
	defer close(a.doneChan)

	ticker := time.NewTicker(a.cfg.FlushInterval)
	defer ticker.Stop()

	batch := make([]OTelSpan, 0, a.cfg.BatchSize)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		toSend := make([]OTelSpan, len(batch))
		copy(toSend, batch)
		batch = batch[:0]

		ctx, cancel := context.WithTimeout(context.Background(), a.cfg.Timeout)
		_ = a.sendBatchWithBackoff(ctx, toSend)
		cancel()
	}

	for {
		select {
		case <-a.stopChan:
			// Drain remaining elements in queue before exit
			for {
				select {
				case item := <-a.queue:
					batch = append(batch, item)
					if len(batch) >= a.cfg.BatchSize {
						flush()
					}
				default:
					flush()
					return
				}
			}

		case item := <-a.queue:
			batch = append(batch, item)
			if len(batch) >= a.cfg.BatchSize {
				flush()
			}

		case <-ticker.C:
			flush()
		}
	}
}

// sendBatchWithBackoff transmits the span batch with exponential backoff on transient errors.
func (a *AsyncCollectorExporter) sendBatchWithBackoff(ctx context.Context, batch []OTelSpan) error {
	if len(batch) == 0 {
		return nil
	}

	var lastErr error
	backoff := a.cfg.InitialBackoff

	for attempt := 0; attempt <= a.cfg.MaxRetries; attempt++ {
		err := a.postOTLP(ctx, batch)
		if err == nil {
			a.mu.Lock()
			a.metrics.ExportedSpans += int64(len(batch))
			a.metrics.BatchesSent++
			a.metrics.LastFlush = time.Now()
			a.metrics.LastError = ""
			a.mu.Unlock()
			return nil
		}

		lastErr = err
		a.mu.Lock()
		a.metrics.FailedAttempts++
		a.metrics.LastError = err.Error()
		a.mu.Unlock()

		if attempt == a.cfg.MaxRetries {
			break
		}

		// Calculate exponential backoff with jitter
		jitter := time.Duration(rand.Float64() * float64(backoff) * 0.2)
		sleepDuration := backoff + jitter
		if sleepDuration > a.cfg.MaxBackoff {
			sleepDuration = a.cfg.MaxBackoff
		}

		select {
		case <-time.After(sleepDuration):
			backoff = time.Duration(math.Min(float64(backoff*2), float64(a.cfg.MaxBackoff)))
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return fmt.Errorf("failed to export batch of %d spans after %d attempts: %w", len(batch), a.cfg.MaxRetries+1, lastErr)
}

// postOTLP performs HTTP POST of the OTLP trace payload.
func (a *AsyncCollectorExporter) postOTLP(ctx context.Context, spans []OTelSpan) error {
	var payload OTelResourceSpans
	payload.Resource.Attributes = map[string]string{
		"service.name":       a.cfg.ServiceName,
		"telemetry.sdk.name": "agentvcr-collector-exporter",
		"agent.runtime":      "openbot",
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
	scopeSpan.Scope.Name = "agentvcr.collector"
	scopeSpan.Scope.Version = "v1.0.0"
	payload.ScopeSpans = append(payload.ScopeSpans, scopeSpan)

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal OTLP payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.Endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create http request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range a.cfg.Headers {
		req.Header.Set(k, v)
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("collector returned non-2xx status code: %d", resp.StatusCode)
	}

	return nil
}

// Flush triggers immediate export of any currently buffered spans.
func (a *AsyncCollectorExporter) Flush(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		for len(a.queue) > 0 {
			time.Sleep(10 * time.Millisecond)
		}
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close gracefully flushes pending spans and stops background workers.
func (a *AsyncCollectorExporter) Close() error {
	if !atomic.CompareAndSwapUint32(&a.isClosed, 0, 1) {
		return nil
	}
	close(a.stopChan)
	<-a.doneChan
	return nil
}

// GetMetrics returns a snapshot of collector export telemetry.
func (a *AsyncCollectorExporter) GetMetrics() CollectorMetrics {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.metrics
}

// --- W3C Distributed Trace Context and Multi-Agent Correlation ---

// TraceContext encapsulates W3C Trace Context parameters (traceparent).
type TraceContext struct {
	TraceID      string `json:"trace_id"`
	ParentSpanID string `json:"parent_span_id"`
	Sampled      bool   `json:"sampled"`
}

// FormatTraceparent formats the W3C traceparent header string (00-<trace_id>-<span_id>-01).
func (tc TraceContext) FormatTraceparent() string {
	sampledFlag := "00"
	if tc.Sampled {
		sampledFlag = "01"
	}
	return fmt.Sprintf("00-%s-%s-%s", tc.TraceID, tc.ParentSpanID, sampledFlag)
}

// ParseTraceparent decodes a W3C traceparent string into TraceContext.
func ParseTraceparent(header string) (*TraceContext, error) {
	header = strings.TrimSpace(header)
	parts := strings.Split(header, "-")
	if len(parts) != 4 {
		return nil, errors.New("invalid traceparent format: expected 4 segments")
	}
	if parts[0] != "00" {
		return nil, fmt.Errorf("unsupported traceparent version: %s", parts[0])
	}
	if len(parts[1]) != 32 {
		return nil, fmt.Errorf("invalid trace_id length: %d (expected 32 hex chars)", len(parts[1]))
	}
	if len(parts[2]) != 16 {
		return nil, fmt.Errorf("invalid span_id length: %d (expected 16 hex chars)", len(parts[2]))
	}

	flags, err := strconv.ParseUint(parts[3], 16, 8)
	if err != nil {
		return nil, fmt.Errorf("invalid trace flags: %w", err)
	}

	return &TraceContext{
		TraceID:      parts[1],
		ParentSpanID: parts[2],
		Sampled:      (flags & 0x01) == 1,
	}, nil
}

// MultiAgentCorrelator correlates parent agent sessions and subordinate multi-agent subroutines.
type MultiAgentCorrelator struct {
	ServiceName string
}

// NewMultiAgentCorrelator initializes the correlator.
func NewMultiAgentCorrelator(serviceName string) *MultiAgentCorrelator {
	if serviceName == "" {
		serviceName = "agentvcr-fleet"
	}
	return &MultiAgentCorrelator{ServiceName: serviceName}
}

// CorrelatedFleetTrace bundles unified multi-agent trace spans with hierarchical correlation.
type CorrelatedFleetTrace struct {
	RootTraceID   string            `json:"root_trace_id"`
	PrimaryAgent  string            `json:"primary_agent"`
	TotalAgents   int               `json:"total_agents"`
	TotalSpans    int               `json:"total_spans"`
	Spans         []OTelSpan        `json:"spans"`
	AgentBranches map[string]string `json:"agent_branches"` // agent -> parent_span_id
}

// CorrelateFleet builds a single unified trace tree connecting a primary agent and all subordinate subagents.
func (c *MultiAgentCorrelator) CorrelateFleet(parentSession *trace.Session, childSessions ...*trace.Session) *CorrelatedFleetTrace {
	if parentSession == nil {
		return nil
	}

	exp := NewExporter(c.ServiceName)
	parentSpans := exp.ExportToOTelSpans(parentSession)
	if len(parentSpans) == 0 {
		return nil
	}

	rootTraceID := parentSpans[0].TraceID
	rootSpanID := parentSpans[0].SpanID

	// Map invocation events to span IDs
	invocationSpanMap := make(map[string]string)
	for _, sp := range parentSpans {
		if sp.Kind == "CLIENT" {
			invocationSpanMap[sp.Name] = sp.SpanID
		}
	}

	allSpans := append([]OTelSpan{}, parentSpans...)
	branches := make(map[string]string)
	branches[parentSession.AgentName] = rootSpanID

	for depth, child := range childSessions {
		if child == nil {
			continue
		}

		childSpans := exp.ExportToOTelSpans(child)
		if len(childSpans) == 0 {
			continue
		}

		// Find parent anchor span or default to parent root
		targetParentSpanID := rootSpanID
		for _, pSpan := range parentSpans {
			// If parent had a tool or subagent call mentioning child agent name
			if strings.Contains(strings.ToLower(pSpan.Name), strings.ToLower(child.AgentName)) ||
				strings.Contains(strings.ToLower(pSpan.Attributes["agent.name"]), strings.ToLower(child.AgentName)) {
				targetParentSpanID = pSpan.SpanID
				break
			}
		}

		branches[child.AgentName] = targetParentSpanID

		// Re-root child spans under the parent's unified trace ID and link root
		childRootID := childSpans[0].SpanID
		for i := range childSpans {
			childSpans[i].TraceID = rootTraceID // Unified 128-bit trace ID
			if i == 0 {
				childSpans[i].ParentSpanID = targetParentSpanID // Link child root to parent invocation
				childSpans[i].Attributes["agent.hierarchy_depth"] = fmt.Sprintf("%d", depth+1)
				childSpans[i].Attributes["agent.parent_session_id"] = parentSession.ID
				childSpans[i].Attributes["w3c.traceparent"] = TraceContext{
					TraceID:      rootTraceID,
					ParentSpanID: targetParentSpanID,
					Sampled:      true,
				}.FormatTraceparent()
			} else if childSpans[i].ParentSpanID == childSpans[0].SpanID || childSpans[i].ParentSpanID == "" {
				childSpans[i].ParentSpanID = childRootID
			}
			childSpans[i].Attributes["telemetry.fleet_correlated"] = "true"
		}

		allSpans = append(allSpans, childSpans...)
	}

	return &CorrelatedFleetTrace{
		RootTraceID:   rootTraceID,
		PrimaryAgent:  parentSession.AgentName,
		TotalAgents:   1 + len(childSessions),
		TotalSpans:    len(allSpans),
		Spans:         allSpans,
		AgentBranches: branches,
	}
}
