package trace

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// EventType defines the category of an agent trajectory event.
type EventType string

const (
	EventLLMRequest   EventType = "llm_request"
	EventLLMResponse  EventType = "llm_response"
	EventToolCall     EventType = "tool_call"
	EventToolResult   EventType = "tool_result"
	EventStateChange  EventType = "state_change"
	EventError        EventType = "error"
)

// TraceEvent represents a single atomic, deterministic step in an agent run.
type TraceEvent struct {
	ID        string                 `json:"id"`
	Index     int                    `json:"index"`
	Timestamp time.Time              `json:"timestamp"`
	Type      EventType              `json:"type"`
	DurationMs int64                 `json:"duration_ms,omitempty"`
	Payload   map[string]interface{} `json:"payload"`
	Checksum  string                 `json:"checksum"`
}

// ComputeChecksum calculates a deterministic SHA-256 hash of event contents.
func ComputeChecksum(prevChecksum string, index int, eventType EventType, payloadBytes []byte) string {
	h := sha256.New()
	h.Write([]byte(prevChecksum))
	h.Write([]byte(string(eventType)))
	h.Write(payloadBytes)
	return hex.EncodeToString(h.Sum(nil))
}

// Session represents an execution trace recording of an agent run.
type Session struct {
	ID         string       `json:"id"`
	AgentName  string       `json:"agent_name"`
	CreatedAt  time.Time    `json:"created_at"`
	UpdatedAt  time.Time    `json:"updated_at"`
	ParentID   string       `json:"parent_id,omitempty"`   // For forked runs
	ForkAtStep int          `json:"fork_at_step,omitempty"` // Step where fork happened
	Metadata   map[string]string `json:"metadata,omitempty"`
	Events     []TraceEvent `json:"events"`
	Status     string       `json:"status"` // "recording", "completed", "failed", "replayed"
}
