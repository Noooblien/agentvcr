package cdp

import (
	"encoding/json"
	"fmt"
	"sync"
)

// VirtualCDPServer simulates a live Chrome DevTools Protocol endpoint for offline agent replay.
type VirtualCDPServer struct {
	mu        sync.RWMutex
	recorder  *CDPRecorder
	frameSeq  int64
	navURL    string
	evalMocks map[string]interface{}
}

// NewVirtualCDPServer constructs a virtual mock CDP server hooked into an optional recorder.
func NewVirtualCDPServer(recorder *CDPRecorder) *VirtualCDPServer {
	if recorder == nil {
		recorder = NewCDPRecorder("virtual-cdp-replay")
	}
	return &VirtualCDPServer{
		recorder:  recorder,
		evalMocks: make(map[string]interface{}),
	}
}

// RegisterMockEvaluation registers a predetermined return value for a JS expression.
func (s *VirtualCDPServer) RegisterMockEvaluation(expression string, result interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evalMocks[expression] = result
}

// DispatchJSONRPC processes an incoming client-to-browser JSON-RPC 2.0 request and returns the response.
func (s *VirtualCDPServer) DispatchJSONRPC(reqBytes []byte) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var req struct {
		ID     int64           `json:"id"`
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(reqBytes, &req); err != nil {
		return nil, fmt.Errorf("invalid json-rpc request: %w", err)
	}

	// Capture outbound client request in recorder
	_ = s.recorder.InterceptFrame(&CDPFrame{
		ID:        req.ID,
		Method:    req.Method,
		Params:    req.Params,
		Direction: "client_to_browser",
	})

	var resultObj interface{}
	var cdpErr *CDPError

	switch req.Method {
	case MethodPageNavigate:
		var params struct {
			URL string `json:"url"`
		}
		_ = json.Unmarshal(req.Params, &params)
		s.navURL = params.URL
		resultObj = map[string]interface{}{
			"frameId":  "virtual-frame-001",
			"loaderId": "virtual-loader-001",
		}

	case MethodDOMGetDocument:
		if s.recorder.rootNode != nil {
			resultObj = map[string]interface{}{
				"root": s.recorder.rootNode,
			}
		} else {
			// Synthesize default basic HTML document root
			defaultRoot := &DOMNode{
				NodeID:         1,
				NodeType:       9, // Document
				NodeName:       "#document",
				ChildNodeCount: 1,
				Children: []*DOMNode{
					{
						NodeID:         2,
						ParentID:       1,
						NodeType:       1,
						NodeName:       "HTML",
						ChildNodeCount: 2,
						Children: []*DOMNode{
							{
								NodeID:   3,
								ParentID: 2,
								NodeType: 1,
								NodeName: "HEAD",
							},
							{
								NodeID:         4,
								ParentID:       2,
								NodeType:       1,
								NodeName:       "BODY",
								ChildNodeCount: 1,
								Children: []*DOMNode{
									{
										NodeID:     5,
										ParentID:   4,
										NodeType:   1,
										NodeName:   "BUTTON",
										Attributes: map[string]string{"id": "submit-btn", "class": "btn primary"},
									},
								},
							},
						},
					},
				},
			}
			s.recorder.rootNode = defaultRoot
			s.recorder.indexDOMNode(defaultRoot)
			resultObj = map[string]interface{}{
				"root": defaultRoot,
			}
		}

	case MethodDOMQuerySelector:
		var params struct {
			Selector string `json:"selector"`
		}
		_ = json.Unmarshal(req.Params, &params)
		node, err := s.recorder.QuerySelector(params.Selector)
		if err != nil {
			resultObj = map[string]interface{}{"nodeId": 0}
		} else {
			resultObj = map[string]interface{}{"nodeId": node.NodeID}
		}

	case MethodInputDispatchMouse, MethodInputDispatchKey:
		resultObj = map[string]interface{}{}

	case MethodRuntimeEvaluate:
		var params struct {
			Expression string `json:"expression"`
		}
		_ = json.Unmarshal(req.Params, &params)
		if val, exists := s.evalMocks[params.Expression]; exists {
			resultObj = map[string]interface{}{
				"result": map[string]interface{}{
					"type":  "string",
					"value": val,
				},
			}
		} else {
			resultObj = map[string]interface{}{
				"result": map[string]interface{}{
					"type":  "undefined",
					"value": nil,
				},
			}
		}

	default:
		// Generic ack for standard CDP protocols
		resultObj = map[string]interface{}{}
	}

	resultBytes, _ := json.Marshal(resultObj)

	// Record browser-to-client response
	_ = s.recorder.InterceptFrame(&CDPFrame{
		ID:        req.ID,
		Result:    resultBytes,
		Error:     cdpErr,
		Direction: "browser_to_client",
	})

	resp := map[string]interface{}{
		"id": req.ID,
	}
	if cdpErr != nil {
		resp["error"] = cdpErr
	} else {
		resp["result"] = resultObj
	}

	return json.Marshal(resp)
}
