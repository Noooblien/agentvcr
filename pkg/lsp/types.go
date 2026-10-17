package lsp

import (
	"encoding/json"

	"github.com/Noooblien/agentvcr/pkg/speculate"
)

// JSON-RPC 2.0 base protocol structures

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"` // string or int
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string       `json:"jsonrpc"`
	ID      interface{}  `json:"id,omitempty"`
	Result  interface{}  `json:"result,omitempty"`
	Error   *ResponseError `json:"error,omitempty"`
}

type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type ResponseError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Standard JSON-RPC and LSP Error Codes
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
	CodeServerNotInitialized = -32002
)

// LSP 3.17 Core Structures

type InitializeParams struct {
	ProcessID             int                `json:"processId,omitempty"`
	RootURI               string             `json:"rootUri,omitempty"`
	ClientInfo            *ClientInfo        `json:"clientInfo,omitempty"`
	Capabilities          ClientCapabilities `json:"capabilities,omitempty"`
	InitializationOptions interface{}        `json:"initializationOptions,omitempty"`
}

type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type ClientCapabilities struct {
	TextDocument interface{} `json:"textDocument,omitempty"`
	Workspace    interface{} `json:"workspace,omitempty"`
}

type InitializeResult struct {
	Capabilities ServerCapabilities `json:"capabilities"`
	ServerInfo   ServerInfo         `json:"serverInfo"`
}

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type ServerCapabilities struct {
	TextDocumentSync           int                `json:"textDocumentSync"` // 1 = Full, 2 = Incremental
	CompletionProvider         *CompletionOptions `json:"completionProvider,omitempty"`
	HoverProvider              bool               `json:"hoverProvider"`
	AgentVCRFastTabProvider     bool               `json:"agentvcrFastTabProvider"`
	AgentVCRTelemetryProvider  bool               `json:"agentvcrTelemetryProvider"`
}

type CompletionOptions struct {
	ResolveProvider   bool     `json:"resolveProvider"`
	TriggerCharacters []string `json:"triggerCharacters"`
}

// Document Synchronization Structures

type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

type DidOpenTextDocumentParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

type VersionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
}

type TextDocumentContentChangeEvent struct {
	Range       *Range `json:"range,omitempty"`
	RangeLength int    `json:"rangeLength,omitempty"`
	Text        string `json:"text"`
}

type DidChangeTextDocumentParams struct {
	TextDocument   VersionedTextDocumentIdentifier  `json:"textDocument"`
	ContentChanges []TextDocumentContentChangeEvent `json:"contentChanges"`
}

type DidCloseTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

// Position and Range Structures

type Position struct {
	Line      int `json:"line"`      // 0-indexed in LSP
	Character int `json:"character"` // 0-indexed in LSP
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Completion Structures

type CompletionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
	Context      *CompletionContext     `json:"context,omitempty"`
}

type CompletionContext struct {
	TriggerKind      int    `json:"triggerKind"`
	TriggerCharacter string `json:"triggerCharacter,omitempty"`
}

type CompletionList struct {
	IsIncomplete bool             `json:"isIncomplete"`
	Items        []CompletionItem `json:"items"`
}

type CompletionItemKind int

const (
	KindText          CompletionItemKind = 1
	KindMethod        CompletionItemKind = 2
	KindFunction      CompletionItemKind = 3
	KindConstructor   CompletionItemKind = 4
	KindField         CompletionItemKind = 5
	KindVariable      CompletionItemKind = 6
	KindClass         CompletionItemKind = 7
	KindInterface     CompletionItemKind = 8
	KindModule        CompletionItemKind = 9
	KindProperty      CompletionItemKind = 10
	KindUnit          CompletionItemKind = 11
	KindValue         CompletionItemKind = 12
	KindEnum          CompletionItemKind = 13
	KindKeyword       CompletionItemKind = 14
	KindSnippet       CompletionItemKind = 15
	KindColor         CompletionItemKind = 16
	KindFile          CompletionItemKind = 17
	KindReference     CompletionItemKind = 18
)

type CompletionItem struct {
	Label            string             `json:"label"`
	Kind             CompletionItemKind `json:"kind,omitempty"`
	Detail           string             `json:"detail,omitempty"`
	Documentation    interface{}        `json:"documentation,omitempty"` // string or MarkupContent
	InsertText       string             `json:"insertText,omitempty"`
	InsertTextFormat int                `json:"insertTextFormat,omitempty"` // 1 = PlainText, 2 = Snippet
	TextEdit         *TextEdit          `json:"textEdit,omitempty"`
	SortText         string             `json:"sortText,omitempty"`
}

type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

// Hover Structures

type HoverParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

type Hover struct {
	Contents MarkupContent `json:"contents"`
	Range    *Range        `json:"range,omitempty"`
}

type MarkupContent struct {
	Kind  string `json:"kind"` // "plaintext" or "markdown"
	Value string `json:"value"`
}

// AgentVCR Custom Extensions

type FastTabParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

type FastTabResult struct {
	Available     bool                      `json:"available"`
	Draft         *speculate.MultiTokenDraft `json:"draft,omitempty"`
	GhostText     string                    `json:"ghost_text,omitempty"`
	LineRange     Range                     `json:"line_range"`
	EstimatedGain string                    `json:"estimated_gain,omitempty"`
	Confidence    float64                   `json:"confidence"`
	LatencyMs     int64                     `json:"latency_ms"`
}

type RecordStepParams struct {
	SessionID string      `json:"session_id"`
	Action    string      `json:"action"`
	Payload   interface{} `json:"payload"`
}

type ServerStatusResult struct {
	ActiveBuffers      int     `json:"active_buffers"`
	TotalCompletions   int64   `json:"total_completions"`
	TotalFastTabs      int64   `json:"total_fast_tabs"`
	UptimeSeconds      float64 `json:"uptime_seconds"`
	SpeculatePatterns  int     `json:"speculate_patterns"`
}
