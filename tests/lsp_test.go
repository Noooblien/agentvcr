package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Noooblien/agentvcr/pkg/lsp"
	"github.com/Noooblien/agentvcr/pkg/speculate"
)

func TestLSP_InitializeAndHandshake(t *testing.T) {
	server := lsp.NewServer(nil)

	req := []byte(`{
		"jsonrpc": "2.0",
		"id": 1,
		"method": "initialize",
		"params": {
			"processId": 1234,
			"clientInfo": {"name": "Neovim", "version": "0.10"},
			"rootUri": "file:///workspace"
		}
	}`)

	respRaw, err := server.HandleMessage(req)
	if err != nil {
		t.Fatalf("HandleMessage failed: %v", err)
	}

	var resp lsp.Response
	if err := json.Unmarshal(respRaw, &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("Unexpected error in initialize: %v", resp.Error)
	}

	resultBytes, _ := json.Marshal(resp.Result)
	var initRes lsp.InitializeResult
	_ = json.Unmarshal(resultBytes, &initRes)

	if !initRes.Capabilities.HoverProvider {
		t.Error("Expected HoverProvider to be true")
	}
	if !initRes.Capabilities.AgentVCRFastTabProvider {
		t.Error("Expected AgentVCRFastTabProvider to be true")
	}
	if initRes.Capabilities.CompletionProvider == nil {
		t.Error("Expected CompletionProvider to be configured")
	}

	// Test initialized notification (returns nil)
	initNotify := []byte(`{"jsonrpc": "2.0", "method": "initialized", "params": {}}`)
	notifyResp, err := server.HandleMessage(initNotify)
	if err != nil || notifyResp != nil {
		t.Errorf("Expected nil response for initialized notification, got: %s", string(notifyResp))
	}
}

func TestLSP_DocumentLifecycleAndIncrementalUpdate(t *testing.T) {
	server := lsp.NewServer(nil)

	// Open document
	openFrame := []byte(`{
		"jsonrpc": "2.0",
		"method": "textDocument/didOpen",
		"params": {
			"textDocument": {
				"uri": "file:///workspace/main.go",
				"languageId": "go",
				"version": 1,
				"text": "package main\n\nfunc main() {\n\terr := doWork()\n}"
			}
		}
	}`)
	_, _ = server.HandleMessage(openFrame)

	// Check status
	statusFrame := []byte(`{"jsonrpc": "2.0", "id": 2, "method": "agentvcr/status"}`)
	statusRespRaw, _ := server.HandleMessage(statusFrame)

	var resp lsp.Response
	_ = json.Unmarshal(statusRespRaw, &resp)
	resBytes, _ := json.Marshal(resp.Result)
	var status lsp.ServerStatusResult
	_ = json.Unmarshal(resBytes, &status)

	if status.ActiveBuffers != 1 {
		t.Errorf("Expected 1 active buffer, got %d", status.ActiveBuffers)
	}

	// Incremental change
	changeFrame := []byte(`{
		"jsonrpc": "2.0",
		"method": "textDocument/didChange",
		"params": {
			"textDocument": {"uri": "file:///workspace/main.go", "version": 2},
			"contentChanges": [
				{
					"range": {"start": {"line": 3, "character": 17}, "end": {"line": 3, "character": 17}},
					"text": "\n\t"
				}
			]
		}
	}`)
	_, _ = server.HandleMessage(changeFrame)

	// Close document
	closeFrame := []byte(`{
		"jsonrpc": "2.0",
		"method": "textDocument/didClose",
		"params": {
			"textDocument": {"uri": "file:///workspace/main.go"}
		}
	}`)
	_, _ = server.HandleMessage(closeFrame)

	statusRespRaw, _ = server.HandleMessage(statusFrame)
	_ = json.Unmarshal(statusRespRaw, &resp)
	resBytes, _ = json.Marshal(resp.Result)
	_ = json.Unmarshal(resBytes, &status)

	if status.ActiveBuffers != 0 {
		t.Errorf("Expected 0 active buffers after close, got %d", status.ActiveBuffers)
	}
}

func TestLSP_CompletionAndFastTab(t *testing.T) {
	predictor := speculate.NewASTPredictor(speculate.DefaultPredictorConfig())
	server := lsp.NewServer(predictor)

	openFrame := []byte(`{
		"jsonrpc": "2.0",
		"method": "textDocument/didOpen",
		"params": {
			"textDocument": {
				"uri": "file:///workspace/handler.go",
				"languageId": "go",
				"version": 1,
				"text": "package main\n\nfunc HandleRequest() error {\n\terr := db.Query()\n\treturn nil\n}"
			}
		}
	}`)
	_, _ = server.HandleMessage(openFrame)

	// Request Completion at line 3 (err := db.Query())
	compFrame := []byte(`{
		"jsonrpc": "2.0",
		"id": 10,
		"method": "textDocument/completion",
		"params": {
			"textDocument": {"uri": "file:///workspace/handler.go"},
			"position": {"line": 3, "character": 18}
		}
	}`)

	compRaw, err := server.HandleMessage(compFrame)
	if err != nil {
		t.Fatalf("Completion failed: %v", err)
	}

	var compResp lsp.Response
	_ = json.Unmarshal(compRaw, &compResp)
	if compResp.Error != nil {
		t.Fatalf("Unexpected completion error: %v", compResp.Error)
	}

	compBytes, _ := json.Marshal(compResp.Result)
	var compList lsp.CompletionList
	_ = json.Unmarshal(compBytes, &compList)

	if len(compList.Items) == 0 {
		t.Errorf("Expected at least one completion item for Go err pattern")
	} else {
		item := compList.Items[0]
		if !strings.Contains(item.InsertText, "if err != nil") {
			t.Errorf("Expected completion to contain 'if err != nil', got: %s", item.InsertText)
		}
	}

	// Request custom agentvcr/fastTab
	fastTabFrame := []byte(`{
		"jsonrpc": "2.0",
		"id": 11,
		"method": "agentvcr/fastTab",
		"params": {
			"textDocument": {"uri": "file:///workspace/handler.go"},
			"position": {"line": 3, "character": 18}
		}
	}`)

	fastTabRaw, err := server.HandleMessage(fastTabFrame)
	if err != nil {
		t.Fatalf("FastTab failed: %v", err)
	}

	var ftResp lsp.Response
	_ = json.Unmarshal(fastTabRaw, &ftResp)
	ftBytes, _ := json.Marshal(ftResp.Result)
	var ftResult lsp.FastTabResult
	_ = json.Unmarshal(ftBytes, &ftResult)

	if !ftResult.Available {
		t.Errorf("Expected FastTab to be available")
	}
	if ftResult.GhostText == "" {
		t.Errorf("Expected non-empty GhostText")
	}
	if ftResult.Confidence < 0.60 {
		t.Errorf("Expected confidence >= 0.60, got %.2f", ftResult.Confidence)
	}
}

func TestLSP_HoverAttestation(t *testing.T) {
	server := lsp.NewServer(nil)

	openFrame := []byte(`{
		"jsonrpc": "2.0",
		"method": "textDocument/didOpen",
		"params": {
			"textDocument": {
				"uri": "file:///workspace/app.ts",
				"languageId": "typescript",
				"version": 1,
				"text": "async function fetchData() {\n  try {\n    const res = await fetch('/api');\n  }\n}"
			}
		}
	}`)
	_, _ = server.HandleMessage(openFrame)

	hoverFrame := []byte(`{
		"jsonrpc": "2.0",
		"id": 20,
		"method": "textDocument/hover",
		"params": {
			"textDocument": {"uri": "file:///workspace/app.ts"},
			"position": {"line": 2, "character": 10}
		}
	}`)

	hoverRaw, err := server.HandleMessage(hoverFrame)
	if err != nil {
		t.Fatalf("Hover failed: %v", err)
	}

	var hResp lsp.Response
	_ = json.Unmarshal(hoverRaw, &hResp)
	hBytes, _ := json.Marshal(hResp.Result)
	var hover lsp.Hover
	_ = json.Unmarshal(hBytes, &hover)

	if !strings.Contains(hover.Contents.Value, "AgentVCR Flight Attestation") {
		t.Errorf("Expected hover content to mention AgentVCR Flight Attestation")
	}
	if !strings.Contains(hover.Contents.Value, "typescript") {
		t.Errorf("Expected hover content to detect language typescript")
	}
}

func TestLSP_ErrorHandling(t *testing.T) {
	server := lsp.NewServer(nil)

	// Malformed JSON
	respRaw, _ := server.HandleMessage([]byte(`{not valid json`))
	var resp lsp.Response
	_ = json.Unmarshal(respRaw, &resp)
	if resp.Error == nil || resp.Error.Code != lsp.CodeParseError {
		t.Errorf("Expected CodeParseError, got: %v", resp.Error)
	}

	// Unknown Method
	unknownFrame := []byte(`{"jsonrpc": "2.0", "id": 99, "method": "invalid/method"}`)
	respRaw, _ = server.HandleMessage(unknownFrame)
	_ = json.Unmarshal(respRaw, &resp)
	if resp.Error == nil || resp.Error.Code != lsp.CodeMethodNotFound {
		t.Errorf("Expected CodeMethodNotFound, got: %v", resp.Error)
	}
}

func TestLSP_ContentLengthFraming(t *testing.T) {
	server := lsp.NewServer(nil)

	initPayload := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`
	frame := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(initPayload), initPayload)

	in := bytes.NewBufferString(frame)
	var out bytes.Buffer

	err := server.Serve(in, &out)
	if err != nil {
		t.Fatalf("Serve failed: %v", err)
	}

	respStr := out.String()
	if !strings.Contains(respStr, "Content-Length:") || !strings.Contains(respStr, "capabilities") {
		t.Errorf("Expected framed LSP response, got: %s", respStr)
	}
}

func TestLSP_ScorecardRendering(t *testing.T) {
	status := lsp.ServerStatusResult{
		ActiveBuffers:     3,
		TotalCompletions:  42,
		TotalFastTabs:     18,
		UptimeSeconds:     12.34,
		SpeculatePatterns: 4,
	}

	sampleDraft := &speculate.MultiTokenDraft{
		SyntacticReason: "Go error return checking",
		PredictedLines:  []string{"if err != nil {", "\treturn nil, err", "}"},
		Confidence:      0.92,
	}

	sampleFastTab := &lsp.FastTabResult{
		Available:     true,
		Draft:         sampleDraft,
		Confidence:    0.92,
		LatencyMs:     8,
		EstimatedGain: "~3.0s typing saved",
	}

	card := lsp.RenderLSPScorecard(status, sampleFastTab)
	if !strings.Contains(card, "LANGUAGE SERVER PROTOCOL (LSP 3.17) & FAST-TAB DAEMON") {
		t.Errorf("Expected banner in scorecard")
	}
	if !strings.Contains(card, "Exposed Editor Capabilities") {
		t.Errorf("Expected capabilities section in scorecard")
	}
	if !strings.Contains(card, "Speculative Fast-Tab Ghost Text Preview") {
		t.Errorf("Expected ghost text preview section in scorecard")
	}
}
