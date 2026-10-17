package cdp

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

// CDPRecorder captures, synchronizes, and audits Chrome DevTools Protocol streams.
type CDPRecorder struct {
	mu           sync.RWMutex
	sessionID    string
	targetURL    string
	startTime    time.Time
	pageLoadedAt time.Time
	frames       []*CDPFrame

	// Network State
	transactions map[string]*NetworkTransaction
	orderedTx    []*NetworkTransaction

	// DOM State
	domNodes  map[int64]*DOMNode
	rootNode  *DOMNode
	mutations []*DOMMutationEvent

	// Console & Interaction State
	consoleLogs  []ConsoleMessage
	interactions []UserInteraction
}

// NewCDPRecorder initializes a new thread-safe Chrome DevTools Protocol flight recorder.
func NewCDPRecorder(sessionID string) *CDPRecorder {
	return &CDPRecorder{
		sessionID:    sessionID,
		startTime:    time.Now().UTC(),
		transactions: make(map[string]*NetworkTransaction),
		domNodes:     make(map[int64]*DOMNode),
	}
}

// InterceptFrame parses and routes an incoming or outgoing CDP JSON-RPC frame.
func (r *CDPRecorder) InterceptFrame(frame *CDPFrame) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if frame.Timestamp.IsZero() {
		frame.Timestamp = time.Now().UTC()
	}
	r.frames = append(r.frames, frame)

	// Process method-specific events
	switch frame.Method {
	case MethodPageNavigate:
		r.handlePageNavigate(frame)
	case MethodPageLoadEventFired:
		r.handlePageLoad(frame)
	case MethodNetworkRequestWillSent:
		r.handleNetworkRequestWillBeSent(frame)
	case MethodNetworkResponseRecv:
		r.handleNetworkResponseReceived(frame)
	case MethodNetworkLoadingFinish:
		r.handleNetworkLoadingFinished(frame)
	case MethodNetworkLoadingFailed:
		r.handleNetworkLoadingFailed(frame)
	case MethodDOMGetDocument:
		r.handleDOMGetDocument(frame)
	case MethodDOMChildNodeInserted:
		r.handleDOMChildNodeInserted(frame)
	case MethodDOMAttributeModified:
		r.handleDOMAttributeModified(frame)
	case MethodDOMNodeRemoved:
		r.handleDOMChildNodeRemoved(frame)
	case MethodRuntimeConsoleAPI:
		r.handleRuntimeConsoleAPI(frame)
	case MethodRuntimeException:
		r.handleRuntimeException(frame)
	case MethodInputDispatchMouse:
		r.handleInputMouseEvent(frame)
	case MethodInputDispatchKey:
		r.handleInputKeyEvent(frame)
	}

	return nil
}

func (r *CDPRecorder) handlePageNavigate(frame *CDPFrame) {
	var payload struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(frame.Params, &payload); err == nil && payload.URL != "" {
		r.targetURL = payload.URL
	}
}

func (r *CDPRecorder) handlePageLoad(frame *CDPFrame) {
	r.pageLoadedAt = frame.Timestamp
}

func (r *CDPRecorder) handleNetworkRequestWillBeSent(frame *CDPFrame) {
	var payload struct {
		RequestID string `json:"requestId"`
		Request   struct {
			URL     string            `json:"url"`
			Method  string            `json:"method"`
			Headers map[string]string `json:"headers"`
		} `json:"request"`
		Initiator struct {
			Type string `json:"type"`
		} `json:"initiator"`
	}
	if err := json.Unmarshal(frame.Params, &payload); err != nil || payload.RequestID == "" {
		return
	}

	tx := &NetworkTransaction{
		RequestID:      payload.RequestID,
		URL:            payload.Request.URL,
		Method:         payload.Request.Method,
		RequestHeaders: payload.Request.Headers,
		Initiator:      payload.Initiator.Type,
		StartTime:      frame.Timestamp,
	}
	if tx.Method == "" {
		tx.Method = "GET"
	}

	r.transactions[payload.RequestID] = tx
	r.orderedTx = append(r.orderedTx, tx)
}

func (r *CDPRecorder) handleNetworkResponseReceived(frame *CDPFrame) {
	var payload struct {
		RequestID string `json:"requestId"`
		Response  struct {
			URL         string            `json:"url"`
			Status      int               `json:"status"`
			StatusText  string            `json:"statusText"`
			Headers     map[string]string `json:"headers"`
			MimeType    string            `json:"mimeType"`
			EncodedSize int64             `json:"encodedDataLength"`
			Timing      *struct {
				DNSStart     float64 `json:"dnsStart"`
				DNSEnd       float64 `json:"dnsEnd"`
				ConnectStart float64 `json:"connectStart"`
				ConnectEnd   float64 `json:"connectEnd"`
				SSLStart     float64 `json:"sslStart"`
				SSLEnd       float64 `json:"sslEnd"`
				SendStart    float64 `json:"sendStart"`
				SendEnd      float64 `json:"sendEnd"`
				ReceiveReady float64 `json:"receiveHeadersEnd"`
			} `json:"timing,omitempty"`
		} `json:"response"`
	}
	if err := json.Unmarshal(frame.Params, &payload); err != nil || payload.RequestID == "" {
		return
	}

	tx, exists := r.transactions[payload.RequestID]
	if !exists {
		tx = &NetworkTransaction{
			RequestID: payload.RequestID,
			StartTime: frame.Timestamp,
		}
		r.transactions[payload.RequestID] = tx
		r.orderedTx = append(r.orderedTx, tx)
	}

	tx.StatusCode = payload.Response.Status
	tx.StatusText = payload.Response.StatusText
	tx.MimeType = payload.Response.MimeType
	tx.ResponseHeaders = payload.Response.Headers
	if payload.Response.URL != "" {
		tx.URL = payload.Response.URL
	}

	if t := payload.Response.Timing; t != nil {
		tx.Timing.DNSLookupMs = math.Max(0, t.DNSEnd-t.DNSStart)
		tx.Timing.ConnectMs = math.Max(0, t.ConnectEnd-t.ConnectStart)
		if t.SSLStart > 0 && t.SSLEnd > 0 {
			tx.Timing.TLSHandshakeMs = math.Max(0, t.SSLEnd-t.SSLStart)
		}
		tx.Timing.TTFBMs = math.Max(0, t.ReceiveReady-t.SendStart)
	}
}

func (r *CDPRecorder) handleNetworkLoadingFinished(frame *CDPFrame) {
	var payload struct {
		RequestID         string  `json:"requestId"`
		EncodedDataLength int64   `json:"encodedDataLength"`
		FinishTime        float64 `json:"finishTime"`
	}
	if err := json.Unmarshal(frame.Params, &payload); err != nil || payload.RequestID == "" {
		return
	}

	if tx, exists := r.transactions[payload.RequestID]; exists {
		tx.ResponseSize = payload.EncodedDataLength
		tx.EndTime = frame.Timestamp
		if !tx.StartTime.IsZero() {
			tx.Timing.TotalDurationMs = float64(tx.EndTime.Sub(tx.StartTime).Milliseconds())
			if tx.Timing.TTFBMs > 0 && tx.Timing.TotalDurationMs >= tx.Timing.TTFBMs {
				tx.Timing.DownloadMs = tx.Timing.TotalDurationMs - tx.Timing.TTFBMs
			}
		}
	}
}

func (r *CDPRecorder) handleNetworkLoadingFailed(frame *CDPFrame) {
	var payload struct {
		RequestID string `json:"requestId"`
		ErrorText string `json:"errorText"`
		Canceled  bool   `json:"canceled"`
	}
	if err := json.Unmarshal(frame.Params, &payload); err != nil || payload.RequestID == "" {
		return
	}

	if tx, exists := r.transactions[payload.RequestID]; exists {
		tx.Failed = true
		tx.FailureText = payload.ErrorText
		tx.EndTime = frame.Timestamp
	}
}

func (r *CDPRecorder) handleDOMGetDocument(frame *CDPFrame) {
	var payload struct {
		Root *DOMNode `json:"root"`
	}
	// Try frame.Result first (browser response) then frame.Params
	data := frame.Result
	if len(data) == 0 {
		data = frame.Params
	}
	if err := json.Unmarshal(data, &payload); err == nil && payload.Root != nil {
		r.rootNode = payload.Root
		r.indexDOMNode(payload.Root)
	}
}

func (r *CDPRecorder) indexDOMNode(node *DOMNode) {
	if node == nil {
		return
	}
	r.domNodes[node.NodeID] = node
	for _, child := range node.Children {
		child.ParentID = node.NodeID
		r.indexDOMNode(child)
	}
}

func (r *CDPRecorder) handleDOMChildNodeInserted(frame *CDPFrame) {
	var payload struct {
		ParentNodeID int64    `json:"parentNodeId"`
		Node         *DOMNode `json:"node"`
	}
	if err := json.Unmarshal(frame.Params, &payload); err != nil || payload.Node == nil {
		return
	}

	payload.Node.ParentID = payload.ParentNodeID
	r.indexDOMNode(payload.Node)

	if parent, ok := r.domNodes[payload.ParentNodeID]; ok {
		parent.Children = append(parent.Children, payload.Node)
		parent.ChildNodeCount = len(parent.Children)
	}

	r.mutations = append(r.mutations, &DOMMutationEvent{
		Type:         "insert",
		NodeID:       payload.Node.NodeID,
		ParentNodeID: payload.ParentNodeID,
		NodeName:     payload.Node.NodeName,
		Timestamp:    frame.Timestamp,
	})
}

func (r *CDPRecorder) handleDOMAttributeModified(frame *CDPFrame) {
	var payload struct {
		NodeID int64  `json:"nodeId"`
		Name   string `json:"name"`
		Value  string `json:"value"`
	}
	if err := json.Unmarshal(frame.Params, &payload); err != nil {
		return
	}

	if node, ok := r.domNodes[payload.NodeID]; ok {
		if node.Attributes == nil {
			node.Attributes = make(map[string]string)
		}
		node.Attributes[payload.Name] = payload.Value
	}

	r.mutations = append(r.mutations, &DOMMutationEvent{
		Type:           "attribute",
		NodeID:         payload.NodeID,
		AttributeName:  payload.Name,
		AttributeValue: payload.Value,
		Timestamp:      frame.Timestamp,
	})
}

func (r *CDPRecorder) handleDOMChildNodeRemoved(frame *CDPFrame) {
	var payload struct {
		ParentNodeID int64 `json:"parentNodeId"`
		NodeID       int64 `json:"nodeId"`
	}
	if err := json.Unmarshal(frame.Params, &payload); err != nil {
		return
	}

	if parent, ok := r.domNodes[payload.ParentNodeID]; ok {
		var filtered []*DOMNode
		for _, child := range parent.Children {
			if child.NodeID != payload.NodeID {
				filtered = append(filtered, child)
			}
		}
		parent.Children = filtered
		parent.ChildNodeCount = len(filtered)
	}
	delete(r.domNodes, payload.NodeID)

	r.mutations = append(r.mutations, &DOMMutationEvent{
		Type:         "remove",
		NodeID:       payload.NodeID,
		ParentNodeID: payload.ParentNodeID,
		Timestamp:    frame.Timestamp,
	})
}

func (r *CDPRecorder) handleRuntimeConsoleAPI(frame *CDPFrame) {
	var payload struct {
		Type string `json:"type"` // "log", "warning", "error", "info"
		Args []struct {
			Value interface{} `json:"value"`
		} `json:"args"`
		StackTrace *struct {
			CallFrames []struct {
				URL        string `json:"url"`
				LineNumber int    `json:"lineNumber"`
				Column     int    `json:"columnNumber"`
			} `json:"callFrames"`
		} `json:"stackTrace"`
	}
	if err := json.Unmarshal(frame.Params, &payload); err != nil {
		return
	}

	var parts []string
	for _, a := range payload.Args {
		parts = append(parts, fmt.Sprintf("%v", a.Value))
	}
	text := strings.Join(parts, " ")

	level := payload.Type
	if level == "warning" {
		level = "warn"
	}

	msg := ConsoleMessage{
		Level:     level,
		Text:      text,
		Timestamp: frame.Timestamp,
	}
	if payload.StackTrace != nil && len(payload.StackTrace.CallFrames) > 0 {
		top := payload.StackTrace.CallFrames[0]
		msg.URL = top.URL
		msg.Line = top.LineNumber
		msg.Column = top.Column
	}

	r.consoleLogs = append(r.consoleLogs, msg)
}

func (r *CDPRecorder) handleRuntimeException(frame *CDPFrame) {
	var payload struct {
		ExceptionDetails struct {
			Text      string `json:"text"`
			URL       string `json:"url"`
			Line      int    `json:"lineNumber"`
			Column    int    `json:"columnNumber"`
			Exception struct {
				Description string `json:"description"`
			} `json:"exception"`
		} `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(frame.Params, &payload); err != nil {
		return
	}

	desc := payload.ExceptionDetails.Exception.Description
	if desc == "" {
		desc = payload.ExceptionDetails.Text
	}

	r.consoleLogs = append(r.consoleLogs, ConsoleMessage{
		Level:     "error",
		Text:      desc,
		URL:       payload.ExceptionDetails.URL,
		Line:      payload.ExceptionDetails.Line,
		Column:    payload.ExceptionDetails.Column,
		Timestamp: frame.Timestamp,
	})
}

func (r *CDPRecorder) handleInputMouseEvent(frame *CDPFrame) {
	var payload struct {
		Type   string  `json:"type"` // "mousePressed", "mouseReleased", "mouseMoved"
		X      float64 `json:"x"`
		Y      float64 `json:"y"`
		Button string  `json:"button"`
	}
	if err := json.Unmarshal(frame.Params, &payload); err != nil {
		return
	}

	r.interactions = append(r.interactions, UserInteraction{
		Type:      payload.Type,
		X:         payload.X,
		Y:         payload.Y,
		Button:    payload.Button,
		Timestamp: frame.Timestamp,
	})
}

func (r *CDPRecorder) handleInputKeyEvent(frame *CDPFrame) {
	var payload struct {
		Type string `json:"type"` // "keyDown", "keyUp"
		Key  string `json:"key"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(frame.Params, &payload); err != nil {
		return
	}

	key := payload.Key
	if key == "" {
		key = payload.Text
	}
	r.interactions = append(r.interactions, UserInteraction{
		Type:      payload.Type,
		Key:       key,
		Timestamp: frame.Timestamp,
	})
}

// QuerySelector executes an in-memory CSS selector search against the virtual DOM tree.
// Supports tag names (e.g. "button"), IDs (e.g. "#submit-btn"), and classes (e.g. ".primary").
func (r *CDPRecorder) QuerySelector(selector string) (*DOMNode, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.rootNode == nil {
		return nil, fmt.Errorf("no DOM tree available")
	}

	selector = strings.TrimSpace(selector)
	var matchFn func(n *DOMNode) bool

	if strings.HasPrefix(selector, "#") {
		idVal := strings.TrimPrefix(selector, "#")
		matchFn = func(n *DOMNode) bool {
			return n.Attributes != nil && n.Attributes["id"] == idVal
		}
	} else if strings.HasPrefix(selector, ".") {
		classVal := strings.TrimPrefix(selector, ".")
		matchFn = func(n *DOMNode) bool {
			if n.Attributes == nil {
				return false
			}
			classes := strings.Fields(n.Attributes["class"])
			for _, c := range classes {
				if c == classVal {
					return true
				}
			}
			return false
		}
	} else {
		tagUpper := strings.ToUpper(selector)
		matchFn = func(n *DOMNode) bool {
			return strings.ToUpper(n.NodeName) == tagUpper
		}
	}

	var found *DOMNode
	var walk func(n *DOMNode)
	walk = func(n *DOMNode) {
		if n == nil || found != nil {
			return
		}
		if matchFn(n) {
			found = n
			return
		}
		for _, child := range n.Children {
			walk(child)
			if found != nil {
				return
			}
		}
	}

	walk(r.rootNode)
	if found == nil {
		return nil, fmt.Errorf("node not found for selector: %s", selector)
	}
	return found, nil
}

// ExportHAR synthesizes standard W3C HAR 1.2 JSON log representation.
func (r *CDPRecorder) ExportHAR() (*HARContainer, []byte, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entries := make([]HAREntry, 0, len(r.orderedTx))
	for _, tx := range r.orderedTx {
		var reqHeaders []HARHeader
		for k, v := range tx.RequestHeaders {
			reqHeaders = append(reqHeaders, HARHeader{Name: k, Value: v})
		}
		var respHeaders []HARHeader
		for k, v := range tx.ResponseHeaders {
			respHeaders = append(respHeaders, HARHeader{Name: k, Value: v})
		}

		duration := tx.Timing.TotalDurationMs
		if duration <= 0 {
			duration = 10.0
		}

		entry := HAREntry{
			StartedDateTime: tx.StartTime,
			Time:            duration,
			Request: HARRequest{
				Method:      tx.Method,
				URL:         tx.URL,
				HTTPVersion: "HTTP/1.1",
				Headers:     reqHeaders,
				HeadersSize: -1,
				BodySize:    tx.RequestSize,
			},
			Response: HARResponse{
				Status:      tx.StatusCode,
				StatusText:  tx.StatusText,
				HTTPVersion: "HTTP/1.1",
				Headers:     respHeaders,
				Content: HARContent{
					Size:     tx.ResponseSize,
					MimeType: tx.MimeType,
				},
				HeadersSize: -1,
				BodySize:    tx.ResponseSize,
			},
			Timings: HARTimings{
				DNS:     tx.Timing.DNSLookupMs,
				Connect: tx.Timing.ConnectMs,
				SSL:     tx.Timing.TLSHandshakeMs,
				Send:    1.0,
				Wait:    math.Max(1.0, tx.Timing.TTFBMs),
				Receive: math.Max(1.0, tx.Timing.DownloadMs),
			},
		}
		entries = append(entries, entry)
	}

	harLog := HARLog{
		Version: "1.2",
		Creator: HARCreator{
			Name:    "AgentVCR CDP Flight Recorder",
			Version: "1.0",
		},
		Pages: []HARPage{
			{
				StartedDateTime: r.startTime,
				ID:              r.sessionID,
				Title:           r.targetURL,
			},
		},
		Entries: entries,
	}

	container := &HARContainer{Log: harLog}
	data, err := json.MarshalIndent(container, "", "  ")
	return container, data, err
}

// FinalizeAudit performs comprehensive browser agent SLA audit and scorecard computation.
func (r *CDPRecorder) FinalizeAudit(cfg CDPAuditConfig) *CDPAuditReport {
	r.mu.RLock()
	defer r.mu.RUnlock()

	totalFrames := len(r.frames)
	totalRequests := len(r.orderedTx)
	failedRequests := 0
	totalBytes := int64(0)
	http4xx := 0
	http5xx := 0
	consoleErrors := 0

	var ttfbList []float64

	for _, tx := range r.orderedTx {
		if tx.Failed {
			failedRequests++
		}
		if tx.StatusCode >= 400 && tx.StatusCode < 500 {
			http4xx++
		}
		if tx.StatusCode >= 500 {
			http5xx++
		}
		totalBytes += tx.ResponseSize
		if tx.Timing.TTFBMs > 0 {
			ttfbList = append(ttfbList, tx.Timing.TTFBMs)
		}
	}

	for _, c := range r.consoleLogs {
		if c.Level == "error" {
			consoleErrors++
		}
	}

	p50 := 0.0
	p95 := 0.0
	if len(ttfbList) > 0 {
		sort.Float64s(ttfbList)
		idx50 := int(float64(len(ttfbList)) * 0.50)
		idx95 := int(float64(len(ttfbList)) * 0.95)
		if idx95 >= len(ttfbList) {
			idx95 = len(ttfbList) - 1
		}
		p50 = ttfbList[idx50]
		p95 = ttfbList[idx95]
	}

	durationMs := int64(0)
	if totalFrames > 0 {
		lastFrame := r.frames[totalFrames-1]
		durationMs = lastFrame.Timestamp.Sub(r.startTime).Milliseconds()
		if durationMs < 0 {
			durationMs = 0
		}
	}

	pageLoadMs := int64(0)
	if !r.pageLoadedAt.IsZero() {
		pageLoadMs = r.pageLoadedAt.Sub(r.startTime).Milliseconds()
		if pageLoadMs < 0 {
			pageLoadMs = 0
		}
	}

	// Validate against SLA bounds
	var violations []string
	if cfg.MaxPageLoadTimeMs > 0 && pageLoadMs > cfg.MaxPageLoadTimeMs {
		violations = append(violations, fmt.Sprintf("Page load latency %dms breached SLA (<%dms)", pageLoadMs, cfg.MaxPageLoadTimeMs))
	}
	if cfg.MaxNetworkRequests > 0 && totalRequests > cfg.MaxNetworkRequests {
		violations = append(violations, fmt.Sprintf("Network requests count %d exceeded max limit (%d)", totalRequests, cfg.MaxNetworkRequests))
	}
	if cfg.MaxP95TTFBMs > 0 && p95 > cfg.MaxP95TTFBMs {
		violations = append(violations, fmt.Sprintf("P95 TTFB %.1fms breached latency SLA (<%.1fms)", p95, cfg.MaxP95TTFBMs))
	}
	if !cfg.AllowHTTPClientError && http4xx > 0 {
		violations = append(violations, fmt.Sprintf("%d HTTP 4xx client errors detected in browser traffic", http4xx))
	}
	if !cfg.AllowHTTPServerError && http5xx > 0 {
		violations = append(violations, fmt.Sprintf("%d HTTP 5xx server errors detected in browser traffic", http5xx))
	}
	if !cfg.AllowJSError && consoleErrors > 0 {
		violations = append(violations, fmt.Sprintf("%d uncaught JS runtime exceptions detected in console", consoleErrors))
	}

	return &CDPAuditReport{
		SessionID:             r.sessionID,
		TargetURL:             r.targetURL,
		TotalFramesCaptured:   totalFrames,
		DurationMs:            durationMs,
		PageLoadTimeMs:        pageLoadMs,
		TotalNetworkRequests:  totalRequests,
		FailedNetworkRequests: failedRequests,
		TotalBytesTransferred: totalBytes,
		P50TTFBMs:             p50,
		P95TTFBMs:             p95,
		HTTP4xxCount:          http4xx,
		HTTP5xxCount:          http5xx,
		DOMNodeCount:          len(r.domNodes),
		DOMMutationCount:      len(r.mutations),
		ConsoleErrorsCount:    consoleErrors,
		UserInteractionsCount: len(r.interactions),
		Transactions:          r.orderedTx,
		ConsoleMessages:       r.consoleLogs,
		SLAPassed:             len(violations) == 0,
		SLAViolations:         violations,
	}
}
