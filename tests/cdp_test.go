package tests

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/cdp"
)

func TestCDP_FrameInterceptionAndNavigation(t *testing.T) {
	rec := cdp.NewCDPRecorder("test-cdp-session-1")

	navParams, _ := json.Marshal(map[string]interface{}{
		"url": "https://app.agentvcr.dev/dashboard",
	})
	err := rec.InterceptFrame(&cdp.CDPFrame{
		ID:        1,
		Method:    cdp.MethodPageNavigate,
		Params:    navParams,
		Timestamp: time.Now().UTC(),
		Direction: "client_to_browser",
	})
	if err != nil {
		t.Fatalf("InterceptFrame failed: %v", err)
	}

	loadParams, _ := json.Marshal(map[string]interface{}{
		"timestamp": 12345.678,
	})
	_ = rec.InterceptFrame(&cdp.CDPFrame{
		Method:    cdp.MethodPageLoadEventFired,
		Params:    loadParams,
		Timestamp: time.Now().UTC().Add(120 * time.Millisecond),
		Direction: "browser_to_client",
	})

	audit := rec.FinalizeAudit(cdp.DefaultCDPAuditConfig())
	if audit.TargetURL != "https://app.agentvcr.dev/dashboard" {
		t.Fatalf("expected TargetURL https://app.agentvcr.dev/dashboard, got %s", audit.TargetURL)
	}
	if audit.TotalFramesCaptured != 2 {
		t.Fatalf("expected 2 frames captured, got %d", audit.TotalFramesCaptured)
	}
}

func TestCDP_NetworkLifecycleAndHARExport(t *testing.T) {
	rec := cdp.NewCDPRecorder("test-cdp-har-session")

	// 1. Request will be sent
	reqData, _ := json.Marshal(map[string]interface{}{
		"requestId": "req-101",
		"request": map[string]interface{}{
			"url":    "https://api.agentvcr.dev/v1/telemetry",
			"method": "POST",
			"headers": map[string]string{
				"Authorization": "Bearer mock-token",
				"Content-Type":  "application/json",
			},
		},
		"initiator": map[string]interface{}{
			"type": "fetch",
		},
	})
	_ = rec.InterceptFrame(&cdp.CDPFrame{
		Method:    cdp.MethodNetworkRequestWillSent,
		Params:    reqData,
		Timestamp: time.Now().UTC(),
	})

	// 2. Response received
	respData, _ := json.Marshal(map[string]interface{}{
		"requestId": "req-101",
		"response": map[string]interface{}{
			"url":        "https://api.agentvcr.dev/v1/telemetry",
			"status":     200,
			"statusText": "OK",
			"mimeType":   "application/json",
			"headers": map[string]string{
				"Content-Type": "application/json",
			},
			"timing": map[string]interface{}{
				"dnsStart":           1.0,
				"dnsEnd":             5.0,
				"connectStart":       5.0,
				"connectEnd":         12.0,
				"sendStart":          13.0,
				"sendEnd":            14.0,
				"receiveHeadersEnd": 45.0,
			},
		},
	})
	_ = rec.InterceptFrame(&cdp.CDPFrame{
		Method:    cdp.MethodNetworkResponseRecv,
		Params:    respData,
		Timestamp: time.Now().UTC().Add(50 * time.Millisecond),
	})

	// 3. Loading finished
	finishData, _ := json.Marshal(map[string]interface{}{
		"requestId":         "req-101",
		"encodedDataLength": 1024,
	})
	_ = rec.InterceptFrame(&cdp.CDPFrame{
		Method:    cdp.MethodNetworkLoadingFinish,
		Params:    finishData,
		Timestamp: time.Now().UTC().Add(75 * time.Millisecond),
	})

	// Export HAR 1.2
	harContainer, harJSON, err := rec.ExportHAR()
	if err != nil {
		t.Fatalf("ExportHAR failed: %v", err)
	}
	if harContainer.Log.Version != "1.2" {
		t.Fatalf("expected HAR version 1.2, got %s", harContainer.Log.Version)
	}
	if len(harContainer.Log.Entries) != 1 {
		t.Fatalf("expected 1 HAR entry, got %d", len(harContainer.Log.Entries))
	}
	entry := harContainer.Log.Entries[0]
	if entry.Request.Method != "POST" || entry.Response.Status != 200 {
		t.Fatalf("unexpected HAR entry values: %v", entry)
	}
	if !strings.Contains(string(harJSON), "https://api.agentvcr.dev/v1/telemetry") {
		t.Fatalf("expected URL in HAR JSON output")
	}
}

func TestCDP_VirtualDOMAndMutations(t *testing.T) {
	rec := cdp.NewCDPRecorder("test-cdp-dom-session")

	// Initial DOM tree
	rootDoc := &cdp.DOMNode{
		NodeID:         1,
		NodeType:       9,
		NodeName:       "#document",
		ChildNodeCount: 1,
		Children: []*cdp.DOMNode{
			{
				NodeID:         2,
				NodeType:       1,
				NodeName:       "HTML",
				ChildNodeCount: 1,
				Children: []*cdp.DOMNode{
					{
						NodeID:         3,
						NodeType:       1,
						NodeName:       "BODY",
						ChildNodeCount: 0,
					},
				},
			},
		},
	}
	docBytes, _ := json.Marshal(map[string]interface{}{
		"root": rootDoc,
	})
	_ = rec.InterceptFrame(&cdp.CDPFrame{
		Method:    cdp.MethodDOMGetDocument,
		Result:    docBytes,
		Timestamp: time.Now().UTC(),
	})

	// Insert child button into BODY
	insertBytes, _ := json.Marshal(map[string]interface{}{
		"parentNodeId": 3,
		"node": &cdp.DOMNode{
			NodeID:     4,
			NodeType:   1,
			NodeName:   "BUTTON",
			Attributes: map[string]string{"id": "pay-now", "class": "btn action-btn"},
		},
	})
	_ = rec.InterceptFrame(&cdp.CDPFrame{
		Method:    cdp.MethodDOMChildNodeInserted,
		Params:    insertBytes,
		Timestamp: time.Now().UTC(),
	})

	// Query by ID
	nodeByID, err := rec.QuerySelector("#pay-now")
	if err != nil || nodeByID == nil {
		t.Fatalf("QuerySelector #pay-now failed: %v", err)
	}
	if nodeByID.NodeID != 4 || nodeByID.NodeName != "BUTTON" {
		t.Fatalf("expected button node 4, got %v", nodeByID)
	}

	// Query by class
	nodeByClass, err := rec.QuerySelector(".action-btn")
	if err != nil || nodeByClass == nil {
		t.Fatalf("QuerySelector .action-btn failed: %v", err)
	}
	if nodeByClass.NodeID != 4 {
		t.Fatalf("expected button node 4, got %v", nodeByClass)
	}

	// Modify attribute
	attrBytes, _ := json.Marshal(map[string]interface{}{
		"nodeId": 4,
		"name":   "disabled",
		"value":  "true",
	})
	_ = rec.InterceptFrame(&cdp.CDPFrame{
		Method:    cdp.MethodDOMAttributeModified,
		Params:    attrBytes,
		Timestamp: time.Now().UTC(),
	})

	if nodeByID.Attributes["disabled"] != "true" {
		t.Fatalf("expected attribute disabled=true, got %s", nodeByID.Attributes["disabled"])
	}

	// Remove child
	removeBytes, _ := json.Marshal(map[string]interface{}{
		"parentNodeId": 3,
		"nodeId":       4,
	})
	_ = rec.InterceptFrame(&cdp.CDPFrame{
		Method:    cdp.MethodDOMNodeRemoved,
		Params:    removeBytes,
		Timestamp: time.Now().UTC(),
	})

	_, err = rec.QuerySelector("#pay-now")
	if err == nil {
		t.Fatalf("expected node to be removed and not found")
	}
}

func TestCDP_ConsoleAndRuntimeExceptions(t *testing.T) {
	rec := cdp.NewCDPRecorder("test-cdp-console-session")

	// Console log
	logBytes, _ := json.Marshal(map[string]interface{}{
		"type": "log",
		"args": []map[string]interface{}{
			{"value": "Agent initialized successfully"},
		},
	})
	_ = rec.InterceptFrame(&cdp.CDPFrame{
		Method:    cdp.MethodRuntimeConsoleAPI,
		Params:    logBytes,
		Timestamp: time.Now().UTC(),
	})

	// Runtime exception
	excBytes, _ := json.Marshal(map[string]interface{}{
		"exceptionDetails": map[string]interface{}{
			"text": "Uncaught TypeError: Cannot read properties of null",
			"url":  "https://app.agentvcr.dev/main.js",
			"lineNumber": 42,
			"columnNumber": 10,
		},
	})
	_ = rec.InterceptFrame(&cdp.CDPFrame{
		Method:    cdp.MethodRuntimeException,
		Params:    excBytes,
		Timestamp: time.Now().UTC(),
	})

	audit := rec.FinalizeAudit(cdp.DefaultCDPAuditConfig())
	if audit.ConsoleErrorsCount != 1 {
		t.Fatalf("expected 1 console error, got %d", audit.ConsoleErrorsCount)
	}
	if audit.SLAPassed {
		t.Fatalf("expected audit to fail due to uncaught JS exception")
	}
}

func TestCDP_VirtualServerMockDispatch(t *testing.T) {
	rec := cdp.NewCDPRecorder("mock-server-session")
	srv := cdp.NewVirtualCDPServer(rec)

	srv.RegisterMockEvaluation("window.innerWidth", 1920)

	// 1. Test Navigation
	navReq, _ := json.Marshal(map[string]interface{}{
		"id":     1,
		"method": cdp.MethodPageNavigate,
		"params": map[string]interface{}{
			"url": "https://example.org",
		},
	})
	resp, err := srv.DispatchJSONRPC(navReq)
	if err != nil {
		t.Fatalf("DispatchJSONRPC navigation failed: %v", err)
	}
	if !strings.Contains(string(resp), "virtual-frame-001") {
		t.Fatalf("expected virtual-frame-001 in response, got %s", string(resp))
	}

	// 2. Test GetDocument
	docReq, _ := json.Marshal(map[string]interface{}{
		"id":     2,
		"method": cdp.MethodDOMGetDocument,
	})
	docResp, err := srv.DispatchJSONRPC(docReq)
	if err != nil {
		t.Fatalf("DispatchJSONRPC getDocument failed: %v", err)
	}
	if !strings.Contains(string(docResp), "BUTTON") {
		t.Fatalf("expected BUTTON in synthetic root document, got %s", string(docResp))
	}

	// 3. Test QuerySelector
	qsReq, _ := json.Marshal(map[string]interface{}{
		"id":     3,
		"method": cdp.MethodDOMQuerySelector,
		"params": map[string]interface{}{
			"selector": "#submit-btn",
		},
	})
	qsResp, err := srv.DispatchJSONRPC(qsReq)
	if err != nil {
		t.Fatalf("DispatchJSONRPC querySelector failed: %v", err)
	}
	if !strings.Contains(string(qsResp), `"nodeId":5`) {
		t.Fatalf("expected nodeId 5 for submit-btn, got %s", string(qsResp))
	}

	// 4. Test Runtime Evaluate
	evalReq, _ := json.Marshal(map[string]interface{}{
		"id":     4,
		"method": cdp.MethodRuntimeEvaluate,
		"params": map[string]interface{}{
			"expression": "window.innerWidth",
		},
	})
	evalResp, err := srv.DispatchJSONRPC(evalReq)
	if err != nil {
		t.Fatalf("DispatchJSONRPC evaluate failed: %v", err)
	}
	if !strings.Contains(string(evalResp), "1920") {
		t.Fatalf("expected 1920 in evaluate response, got %s", string(evalResp))
	}
}

func TestCDP_ScorecardAndSLAAudit(t *testing.T) {
	rec := cdp.NewCDPRecorder("scorecard-session")

	// Add clean transactions
	reqData, _ := json.Marshal(map[string]interface{}{
		"requestId": "req-1",
		"request": map[string]interface{}{
			"url":    "https://example.com/api/data",
			"method": "GET",
		},
	})
	_ = rec.InterceptFrame(&cdp.CDPFrame{
		Method:    cdp.MethodNetworkRequestWillSent,
		Params:    reqData,
		Timestamp: time.Now().UTC(),
	})

	respData, _ := json.Marshal(map[string]interface{}{
		"requestId": "req-1",
		"response": map[string]interface{}{
			"url":        "https://example.com/api/data",
			"status":     200,
			"statusText": "OK",
			"timing": map[string]interface{}{
				"sendStart":          10.0,
				"receiveHeadersEnd": 40.0,
			},
		},
	})
	_ = rec.InterceptFrame(&cdp.CDPFrame{
		Method:    cdp.MethodNetworkResponseRecv,
		Params:    respData,
		Timestamp: time.Now().UTC(),
	})

	report := rec.FinalizeAudit(cdp.DefaultCDPAuditConfig())
	scorecard := cdp.RenderCDPScorecard(report)

	if !strings.Contains(scorecard, "CHROME DEVTOOLS PROTOCOL (CDP) FLIGHT RECORDER") {
		t.Fatalf("scorecard missing header: %s", scorecard)
	}
	if !strings.Contains(scorecard, "PASS") {
		t.Fatalf("expected PASS in clean scorecard: %s", scorecard)
	}
}
