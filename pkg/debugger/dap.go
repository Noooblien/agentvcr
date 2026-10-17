package debugger

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
)

// DAPMessageType defines the standard category of DAP protocol messages.
type DAPMessageType string

const (
	DAPTypeRequest  DAPMessageType = "request"
	DAPTypeResponse DAPMessageType = "response"
	DAPTypeEvent    DAPMessageType = "event"
)

// ProtocolMessage is the base structure for all DAP JSON-RPC messages.
type ProtocolMessage struct {
	Seq  int            `json:"seq"`
	Type DAPMessageType `json:"type"`
}

// DAPRequest represents a request sent from an IDE/client to the debugger.
type DAPRequest struct {
	ProtocolMessage
	Command   string                 `json:"command"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

// DAPResponse represents the debugger's reply to an IDE request.
type DAPResponse struct {
	ProtocolMessage
	RequestSeq int         `json:"request_seq"`
	Success    bool        `json:"success"`
	Command    string      `json:"command"`
	Message    string      `json:"message,omitempty"`
	Body       interface{} `json:"body,omitempty"`
}

// DAPEvent represents an asynchronous event notification emitted to the IDE.
type DAPEvent struct {
	ProtocolMessage
	Event string      `json:"event"`
	Body  interface{} `json:"body,omitempty"`
}

// DAPStoppedBody contains the payload for a 'stopped' event in DAP.
type DAPStoppedBody struct {
	Reason            string `json:"reason"`
	Description       string `json:"description,omitempty"`
	ThreadID          int    `json:"threadId"`
	PreserveFocusHint bool   `json:"preserveFocusHint,omitempty"`
	Text              string `json:"text,omitempty"`
	AllThreadsStopped bool   `json:"allThreadsStopped"`
}

// DAPAdapter manages DAP protocol sequences and serializes responses/events.
type DAPAdapter struct {
	seqCounter int64
}

// NewDAPAdapter creates a new protocol adapter.
func NewDAPAdapter() *DAPAdapter {
	return &DAPAdapter{
		seqCounter: 0,
	}
}

func (a *DAPAdapter) nextSeq() int {
	return int(atomic.AddInt64(&a.seqCounter, 1))
}

// CreateResponse formats a successful response to a DAP request.
func (a *DAPAdapter) CreateResponse(req *DAPRequest, body interface{}) *DAPResponse {
	return &DAPResponse{
		ProtocolMessage: ProtocolMessage{
			Seq:  a.nextSeq(),
			Type: DAPTypeResponse,
		},
		RequestSeq: req.Seq,
		Success:    true,
		Command:    req.Command,
		Body:       body,
	}
}

// CreateErrorResponse formats an error response to a DAP request.
func (a *DAPAdapter) CreateErrorResponse(req *DAPRequest, message string) *DAPResponse {
	return &DAPResponse{
		ProtocolMessage: ProtocolMessage{
			Seq:  a.nextSeq(),
			Type: DAPTypeResponse,
		},
		RequestSeq: req.Seq,
		Success:    false,
		Command:    req.Command,
		Message:    message,
	}
}

// CreateStoppedEvent generates a 'stopped' DAP notification.
func (a *DAPAdapter) CreateStoppedEvent(reason StopReason, description string, step int) *DAPEvent {
	return &DAPEvent{
		ProtocolMessage: ProtocolMessage{
			Seq:  a.nextSeq(),
			Type: DAPTypeEvent,
		},
		Event: "stopped",
		Body: DAPStoppedBody{
			Reason:            string(reason),
			Description:       description,
			ThreadID:          1,
			Text:              fmt.Sprintf("Paused at step %d: %s", step, description),
			AllThreadsStopped: true,
		},
	}
}

// ParseDAPRequest decodes raw JSON-RPC bytes into a DAPRequest.
func ParseDAPRequest(data []byte) (*DAPRequest, error) {
	var req DAPRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("failed to parse DAP request: %w", err)
	}
	return &req, nil
}
