package cdp

import (
	"encoding/json"
	"time"
)

// CDPMethod represents standardized Chrome DevTools Protocol method identifiers.
const (
	MethodPageNavigate           = "Page.navigate"
	MethodPageLoadEventFired     = "Page.loadEventFired"
	MethodPageDOMContentEvent    = "Page.domContentEventFired"
	MethodDOMGetDocument         = "DOM.getDocument"
	MethodDOMQuerySelector       = "DOM.querySelector"
	MethodDOMChildNodeInserted   = "DOM.childNodeInserted"
	MethodDOMAttributeModified   = "DOM.attributeModified"
	MethodDOMNodeRemoved         = "DOM.childNodeRemoved"
	MethodNetworkRequestWillSent = "Network.requestWillBeSent"
	MethodNetworkResponseRecv    = "Network.responseReceived"
	MethodNetworkLoadingFinish   = "Network.loadingFinished"
	MethodNetworkLoadingFailed   = "Network.loadingFailed"
	MethodRuntimeConsoleAPI      = "Runtime.consoleAPICalled"
	MethodRuntimeException       = "Runtime.exceptionThrown"
	MethodRuntimeEvaluate        = "Runtime.evaluate"
	MethodInputDispatchMouse     = "Input.dispatchMouseEvent"
	MethodInputDispatchKey       = "Input.dispatchKeyEvent"
)

// CDPError represents an error response in JSON-RPC 2.0 format.
type CDPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data,omitempty"`
}

// CDPFrame represents a captured bidirectional WebSocket frame in Chrome DevTools Protocol.
type CDPFrame struct {
	ID        int64           `json:"id,omitempty"`
	Method    string          `json:"method,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
	Params    json.RawMessage `json:"params,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *CDPError       `json:"error,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
	Direction string          `json:"direction"` // "client_to_browser" or "browser_to_client"
}

// DOMNode represents a node in the reconstructed in-memory virtual DOM tree.
type DOMNode struct {
	NodeID         int64             `json:"nodeId"`
	ParentID       int64             `json:"parentId,omitempty"`
	NodeType       int               `json:"nodeType"` // 1: Element, 3: Text, 9: Document
	NodeName       string            `json:"nodeName"` // e.g. "DIV", "BUTTON", "A", "#text"
	Attributes     map[string]string `json:"attributes,omitempty"`
	TextContent    string            `json:"textContent,omitempty"`
	ChildNodeCount int               `json:"childNodeCount"`
	Children       []*DOMNode        `json:"children,omitempty"`
}

// DOMMutationEvent represents an incremental DOM state transition.
type DOMMutationEvent struct {
	Type           string    `json:"type"` // "insert", "remove", "attribute", "text"
	NodeID         int64     `json:"nodeId"`
	ParentNodeID   int64     `json:"parentNodeId,omitempty"`
	NodeName       string    `json:"nodeName,omitempty"`
	AttributeName  string    `json:"attributeName,omitempty"`
	AttributeValue string    `json:"attributeValue,omitempty"`
	Timestamp      time.Time `json:"timestamp"`
}

// NetworkTiming contains high-precision waterfall latency metrics in milliseconds.
type NetworkTiming struct {
	DNSLookupMs    float64 `json:"dnsLookupMs"`
	ConnectMs      float64 `json:"connectMs"`
	TLSHandshakeMs float64 `json:"tlsHandshakeMs"`
	TTFBMs         float64 `json:"ttfbMs"` // Time to First Byte
	DownloadMs     float64 `json:"downloadMs"`
	TotalDurationMs float64 `json:"totalDurationMs"`
}

// NetworkTransaction represents a correlated HTTP/WebSocket network transaction captured via CDP.
type NetworkTransaction struct {
	RequestID       string            `json:"requestId"`
	URL             string            `json:"url"`
	Method          string            `json:"method"`
	StatusCode      int               `json:"statusCode"`
	StatusText      string            `json:"statusText"`
	MimeType        string            `json:"mimeType"`
	RequestHeaders  map[string]string `json:"requestHeaders"`
	ResponseHeaders map[string]string `json:"responseHeaders"`
	RequestSize     int64             `json:"requestSize"`
	ResponseSize    int64             `json:"responseSize"`
	Timing          NetworkTiming     `json:"timing"`
	Initiator       string            `json:"initiator,omitempty"`
	Failed          bool              `json:"failed"`
	FailureText     string            `json:"failureText,omitempty"`
	StartTime       time.Time         `json:"startTime"`
	EndTime         time.Time         `json:"endTime"`
}

// ConsoleMessage represents a JavaScript console message or uncaught runtime exception.
type ConsoleMessage struct {
	Level     string    `json:"level"` // "log", "info", "warn", "error"
	Text      string    `json:"text"`
	URL       string    `json:"url,omitempty"`
	Line      int       `json:"line,omitempty"`
	Column    int       `json:"column,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// UserInteraction represents an agent-driven user input event (click, type, scroll).
type UserInteraction struct {
	Type        string    `json:"type"` // "click", "move", "keyDown", "keyUp"
	Target      string    `json:"target,omitempty"`
	X           float64   `json:"x,omitempty"`
	Y           float64   `json:"y,omitempty"`
	Key         string    `json:"key,omitempty"`
	Button      string    `json:"button,omitempty"`
	Modifiers   int       `json:"modifiers,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
}

// HAREntry represents a single entry in the W3C HAR 1.2 specification.
type HAREntry struct {
	StartedDateTime time.Time      `json:"startedDateTime"`
	Time            float64        `json:"time"` // Total time in ms
	Request         HARRequest     `json:"request"`
	Response        HARResponse    `json:"response"`
	Timings         HARTimings     `json:"timings"`
	ServerIPAddress string         `json:"serverIPAddress,omitempty"`
}

// HARRequest represents the HTTP request object in HAR format.
type HARRequest struct {
	Method      string         `json:"method"`
	URL         string         `json:"url"`
	HTTPVersion string         `json:"httpVersion"`
	Headers     []HARHeader    `json:"headers"`
	QueryString []HARHeader    `json:"queryString"`
	HeadersSize int64          `json:"headersSize"`
	BodySize    int64          `json:"bodySize"`
}

// HARResponse represents the HTTP response object in HAR format.
type HARResponse struct {
	Status      int            `json:"status"`
	StatusText  string         `json:"statusText"`
	HTTPVersion string         `json:"httpVersion"`
	Headers     []HARHeader    `json:"headers"`
	Content     HARContent     `json:"content"`
	RedirectURL string         `json:"redirectURL"`
	HeadersSize int64          `json:"headersSize"`
	BodySize    int64          `json:"bodySize"`
}

// HARHeader represents an HTTP key-value header pair.
type HARHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// HARContent describes response content characteristics.
type HARContent struct {
	Size     int64  `json:"size"`
	MimeType string `json:"mimeType"`
	Text     string `json:"text,omitempty"`
}

// HARTimings describes detailed connection and transfer timing breakdown in milliseconds.
type HARTimings struct {
	DNS     float64 `json:"dns"`
	Connect float64 `json:"connect"`
	SSL     float64 `json:"ssl"`
	Send    float64 `json:"send"`
	Wait    float64 `json:"wait"` // TTFB
	Receive float64 `json:"receive"`
}

// HARLog represents the root container of the W3C HAR 1.2 log specification.
type HARLog struct {
	Version string     `json:"version"`
	Creator HARCreator `json:"creator"`
	Pages   []HARPage  `json:"pages"`
	Entries []HAREntry `json:"entries"`
}

// HARCreator holds metadata about the tool generating the HAR log.
type HARCreator struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// HARPage holds page-level metadata.
type HARPage struct {
	StartedDateTime time.Time `json:"startedDateTime"`
	ID              string    `json:"id"`
	Title           string    `json:"title"`
}

// HARContainer wraps the root "log" field per HAR 1.2 schema.
type HARContainer struct {
	Log HARLog `json:"log"`
}

// CDPAuditConfig defines SLA thresholds for automated browser agent audits.
type CDPAuditConfig struct {
	MaxPageLoadTimeMs    int64   `json:"maxPageLoadTimeMs"`    // Default: 3000ms
	MaxNetworkRequests   int     `json:"maxNetworkRequests"`   // Default: 100
	MaxP95TTFBMs         float64 `json:"maxP95TTFBMs"`         // Default: 500ms
	AllowHTTPClientError bool    `json:"allowHttpClientError"` // 4xx errors allowed?
	AllowHTTPServerError bool    `json:"allowHttpServerError"` // 5xx errors allowed?
	AllowJSError         bool    `json:"allowJsError"`         // Uncaught JS exceptions allowed?
}

// CDPAuditReport holds comprehensive telemetry and SLA validation for a recorded CDP browser session.
type CDPAuditReport struct {
	SessionID             string                `json:"sessionId"`
	TargetURL             string                `json:"targetUrl"`
	TotalFramesCaptured   int                   `json:"totalFramesCaptured"`
	DurationMs            int64                 `json:"durationMs"`
	PageLoadTimeMs        int64                 `json:"pageLoadTimeMs"`
	TotalNetworkRequests  int                   `json:"totalNetworkRequests"`
	FailedNetworkRequests int                   `json:"failedNetworkRequests"`
	TotalBytesTransferred int64                 `json:"totalBytesTransferred"`
	P50TTFBMs             float64               `json:"p50TtfbMs"`
	P95TTFBMs             float64               `json:"p95TtfbMs"`
	HTTP4xxCount          int                   `json:"http4xxCount"`
	HTTP5xxCount          int                   `json:"http5xxCount"`
	DOMNodeCount          int                   `json:"domNodeCount"`
	DOMMutationCount      int                   `json:"domMutationCount"`
	ConsoleErrorsCount    int                   `json:"consoleErrorsCount"`
	UserInteractionsCount int                   `json:"userInteractionsCount"`
	Transactions          []*NetworkTransaction `json:"transactions"`
	ConsoleMessages       []ConsoleMessage      `json:"consoleMessages"`
	SLAPassed             bool                  `json:"slaPassed"`
	SLAViolations         []string              `json:"slaViolations"`
}

// DefaultCDPAuditConfig returns standard production SLA configurations for browser agents.
func DefaultCDPAuditConfig() CDPAuditConfig {
	return CDPAuditConfig{
		MaxPageLoadTimeMs:    3000,
		MaxNetworkRequests:   100,
		MaxP95TTFBMs:         500.0,
		AllowHTTPClientError: false,
		AllowHTTPServerError: false,
		AllowJSError:         false,
	}
}
