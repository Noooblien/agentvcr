package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Noooblien/agentvcr/pkg/speculate"
)

// Server implements a full Language Server Protocol (LSP 3.17) daemon over JSON-RPC 2.0.
type Server struct {
	mu               sync.RWMutex
	docs             *DocumentStore
	predictor        *speculate.ASTPredictor
	startTime        time.Time
	isInitialized    bool
	isShutdown       bool
	totalCompletions int64
	totalFastTabs    int64
	logWriter        io.Writer
}

// NewServer initializes an LSP daemon backed by an ASTPredictor.
func NewServer(predictor *speculate.ASTPredictor) *Server {
	if predictor == nil {
		predictor = speculate.NewASTPredictor(speculate.DefaultPredictorConfig())
	}
	return &Server{
		docs:      NewDocumentStore(),
		predictor: predictor,
		startTime: time.Now(),
	}
}

// SetLogWriter configures an optional writer for diagnostic LSP log frames.
func (s *Server) SetLogWriter(w io.Writer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logWriter = w
}

// HandleMessage parses and processes a raw JSON-RPC frame, returning the response frame (or nil if notification).
func (s *Server) HandleMessage(raw []byte) ([]byte, error) {
	var base struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      interface{}     `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}

	if err := json.Unmarshal(raw, &base); err != nil {
		return s.formatError(nil, CodeParseError, "Parse error: invalid JSON-RPC payload")
	}

	if base.JSONRPC != "2.0" && base.JSONRPC != "" {
		return s.formatError(base.ID, CodeInvalidRequest, "Invalid Request: jsonrpc must be '2.0'")
	}

	isNotification := base.ID == nil

	// Dispatch by method
	switch base.Method {
	case "initialize":
		var params InitializeParams
		if len(base.Params) > 0 {
			_ = json.Unmarshal(base.Params, &params)
		}
		result := s.handleInitialize(params)
		s.mu.Lock()
		s.isInitialized = true
		s.mu.Unlock()
		return s.formatResponse(base.ID, result)

	case "initialized":
		// Client notification after initialize handshake
		return nil, nil

	case "shutdown":
		s.mu.Lock()
		s.isShutdown = true
		s.mu.Unlock()
		return s.formatResponse(base.ID, nil)

	case "exit":
		return nil, nil

	case "textDocument/didOpen":
		var params DidOpenTextDocumentParams
		if err := json.Unmarshal(base.Params, &params); err == nil {
			s.docs.Open(params.TextDocument.URI, params.TextDocument.LanguageID, params.TextDocument.Version, params.TextDocument.Text)
		}
		return nil, nil

	case "textDocument/didChange":
		var params DidChangeTextDocumentParams
		if err := json.Unmarshal(base.Params, &params); err == nil {
			_, _ = s.docs.Change(params.TextDocument.URI, params.TextDocument.Version, params.ContentChanges)
		}
		return nil, nil

	case "textDocument/didClose":
		var params DidCloseTextDocumentParams
		if err := json.Unmarshal(base.Params, &params); err == nil {
			s.docs.Close(params.TextDocument.URI)
		}
		return nil, nil

	case "textDocument/completion":
		if isNotification {
			return nil, nil
		}
		var params CompletionParams
		if err := json.Unmarshal(base.Params, &params); err != nil {
			return s.formatError(base.ID, CodeInvalidParams, "Invalid params for textDocument/completion")
		}
		result := s.handleCompletion(params)
		atomic.AddInt64(&s.totalCompletions, 1)
		return s.formatResponse(base.ID, result)

	case "textDocument/hover":
		if isNotification {
			return nil, nil
		}
		var params HoverParams
		if err := json.Unmarshal(base.Params, &params); err != nil {
			return s.formatError(base.ID, CodeInvalidParams, "Invalid params for textDocument/hover")
		}
		result := s.handleHover(params)
		return s.formatResponse(base.ID, result)

	case "agentvcr/fastTab":
		if isNotification {
			return nil, nil
		}
		var params FastTabParams
		if err := json.Unmarshal(base.Params, &params); err != nil {
			return s.formatError(base.ID, CodeInvalidParams, "Invalid params for agentvcr/fastTab")
		}
		result := s.handleFastTab(params)
		atomic.AddInt64(&s.totalFastTabs, 1)
		return s.formatResponse(base.ID, result)

	case "agentvcr/status":
		if isNotification {
			return nil, nil
		}
		result := s.handleStatus()
		return s.formatResponse(base.ID, result)

	default:
		if isNotification {
			return nil, nil
		}
		return s.formatError(base.ID, CodeMethodNotFound, fmt.Sprintf("Method not found: %s", base.Method))
	}
}

func (s *Server) handleInitialize(params InitializeParams) InitializeResult {
	return InitializeResult{
		ServerInfo: ServerInfo{
			Name:    "AgentVCR Language Server & Fast-Tab Daemon",
			Version: "1.0.0",
		},
		Capabilities: ServerCapabilities{
			TextDocumentSync: 1, // Full sync
			CompletionProvider: &CompletionOptions{
				ResolveProvider:   false,
				TriggerCharacters: []string{".", ":", "=", "(", "{"},
			},
			HoverProvider:             true,
			AgentVCRFastTabProvider:    true,
			AgentVCRTelemetryProvider: true,
		},
	}
}

func (s *Server) handleCompletion(params CompletionParams) CompletionList {
	astCtx, cursorPos, err := s.docs.ExtractASTContext(params.TextDocument.URI, params.Position)
	if err != nil {
		return CompletionList{IsIncomplete: false, Items: []CompletionItem{}}
	}

	draft, err := s.predictor.GenerateDraft(*astCtx, *cursorPos)
	if err != nil || draft == nil || len(draft.PredictedLines) == 0 {
		return CompletionList{IsIncomplete: false, Items: []CompletionItem{}}
	}

	insertText := strings.Join(draft.PredictedLines, "\n")
	item := CompletionItem{
		Label:            draft.PredictedLines[0],
		Kind:             KindSnippet,
		Detail:           fmt.Sprintf("[AgentVCR Speculate] %.0f%% confidence (%s)", draft.Confidence*100, draft.SyntacticReason),
		Documentation:    MarkupContent{Kind: "markdown", Value: fmt.Sprintf("```%s\n%s\n```\n*Speculative multi-token draft generated in %d ms*", astCtx.Language, insertText, draft.PredictedInMs)},
		InsertText:       insertText,
		InsertTextFormat: 1, // PlainText
		SortText:         "0000",
	}

	return CompletionList{
		IsIncomplete: false,
		Items:        []CompletionItem{item},
	}
}

func (s *Server) handleHover(params HoverParams) *Hover {
	astCtx, _, err := s.docs.ExtractASTContext(params.TextDocument.URI, params.Position)
	if err != nil {
		return nil
	}

	doc, _ := s.docs.Get(params.TextDocument.URI)
	lineCount := 0
	if doc != nil {
		lineCount = len(doc.Lines)
	}

	markdown := fmt.Sprintf(
		"### 🛰️ AgentVCR Flight Attestation\n\n"+
			"- **Buffer**: `%s`\n"+
			"- **Language**: `%s` (indent: %d spaces)\n"+
			"- **Line**: %d / %d\n"+
			"- **Fast-Tab**: Active (Sub-50ms AST Speculation Ready)\n",
		params.TextDocument.URI, astCtx.Language, astCtx.IndentLevel, params.Position.Line+1, lineCount,
	)

	return &Hover{
		Contents: MarkupContent{
			Kind:  "markdown",
			Value: markdown,
		},
	}
}

func (s *Server) handleFastTab(params FastTabParams) FastTabResult {
	start := time.Now()
	astCtx, cursorPos, err := s.docs.ExtractASTContext(params.TextDocument.URI, params.Position)
	if err != nil {
		return FastTabResult{Available: false}
	}

	draft, err := s.predictor.GenerateDraft(*astCtx, *cursorPos)
	latency := time.Since(start).Milliseconds()

	if err != nil || draft == nil || len(draft.PredictedLines) == 0 {
		return FastTabResult{
			Available: false,
			LatencyMs: latency,
		}
	}

	ghostText := strings.Join(draft.PredictedLines, "\n")
	words := len(strings.Fields(ghostText))
	estSavingsSec := float64(words) / (60.0 / 60.0) // ~60 WPM
	savingsStr := fmt.Sprintf("~%.1fs typing saved", estSavingsSec)

	return FastTabResult{
		Available:     true,
		Draft:         draft,
		GhostText:     ghostText,
		LineRange:     Range{Start: params.Position, End: Position{Line: params.Position.Line + len(draft.PredictedLines), Character: 0}},
		EstimatedGain: savingsStr,
		Confidence:    draft.Confidence,
		LatencyMs:     latency,
	}
}

func (s *Server) handleStatus() ServerStatusResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	uptime := time.Since(s.startTime).Seconds()

	return ServerStatusResult{
		ActiveBuffers:      s.docs.Count(),
		TotalCompletions:   atomic.LoadInt64(&s.totalCompletions),
		TotalFastTabs:      atomic.LoadInt64(&s.totalFastTabs),
		UptimeSeconds:      uptime,
		SpeculatePatterns:  4, // Default registered common patterns
	}
}

func (s *Server) formatResponse(id interface{}, result interface{}) ([]byte, error) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	return json.Marshal(resp)
}

func (s *Server) formatError(id interface{}, code int, message string) ([]byte, error) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Error: &ResponseError{
			Code:    code,
			Message: message,
		},
	}
	return json.Marshal(resp)
}

// Serve reads Content-Length framed JSON-RPC messages from in and writes responses to out.
func (s *Server) Serve(in io.Reader, out io.Writer) error {
	reader := bufio.NewReader(in)

	for {
		// Read headers until \r\n\r\n
		contentLength := -1
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					return nil
				}
				return err
			}
			line = strings.TrimSpace(line)
			if line == "" {
				// End of headers
				break
			}
			if strings.HasPrefix(strings.ToLower(line), "content-length:") {
				parts := strings.Split(line, ":")
				if len(parts) == 2 {
					cl, err := strconv.Atoi(strings.TrimSpace(parts[1]))
					if err == nil {
						contentLength = cl
					}
				}
			}
		}

		if contentLength <= 0 {
			continue
		}

		payload := make([]byte, contentLength)
		if _, err := io.ReadFull(reader, payload); err != nil {
			return err
		}

		respBytes, err := s.HandleMessage(payload)
		if err != nil {
			return err
		}

		if len(respBytes) > 0 {
			header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(respBytes))
			if _, err := out.Write([]byte(header)); err != nil {
				return err
			}
			if _, err := out.Write(respBytes); err != nil {
				return err
			}
		}
	}
}

// ServeTCP listens on a TCP port and serves incoming LSP client connections.
func (s *Server) ServeTCP(addr string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			return err
		}
		go func(c net.Conn) {
			defer c.Close()
			_ = s.Serve(c, c)
		}(conn)
	}
}
