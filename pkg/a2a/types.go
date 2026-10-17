package a2a

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// MessageType defines the nature of the inter-agent wire frame.
type MessageType string

const (
	TypeHandshake  MessageType = "HANDSHAKE"
	TypeDiscover   MessageType = "DISCOVER"
	TypeDelegate   MessageType = "DELEGATE"
	TypeResult     MessageType = "RESULT"
	TypeCancel     MessageType = "CANCEL"
	TypeHeartbeat  MessageType = "HEARTBEAT"
	TypeInterrupt  MessageType = "INTERRUPT"
	TypeAck        MessageType = "ACK"
)

// Capability defines a specific skill or tool exposed by an agent card.
type Capability struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema,omitempty"`
	TimeoutMs   int64                  `json:"timeout_ms,omitempty"`
}

// AgentCard advertises an agent's identity, role, and capabilities.
type AgentCard struct {
	AgentID      string       `json:"agent_id"`
	Name         string       `json:"name"`
	Role         string       `json:"role"`
	Endpoint     string       `json:"endpoint"`
	Capabilities []Capability `json:"capabilities"`
	PublicKey    string       `json:"public_key,omitempty"`
	RegisteredAt time.Time    `json:"registered_at"`
}

// MessageEnvelope represents the standard wire protocol packet for A2A communication.
type MessageEnvelope struct {
	ID             string                 `json:"id"`
	ConversationID string                 `json:"conversation_id"`
	Type           MessageType            `json:"type"`
	SenderID       string                 `json:"sender_id"`
	RecipientID    string                 `json:"recipient_id"`
	ReplyToID      string                 `json:"reply_to_id,omitempty"`
	Timestamp      time.Time              `json:"timestamp"`
	Payload        map[string]interface{} `json:"payload"`
	Checksum       string                 `json:"checksum"`
	Hops           int                    `json:"hops"`
	Signature      string                 `json:"signature,omitempty"`
}

// ComputeEnvelopeChecksum generates a deterministic SHA-256 hash of envelope contents.
func ComputeEnvelopeChecksum(id, convID, sender, recipient string, msgType MessageType, payload map[string]interface{}) string {
	h := sha256.New()
	h.Write([]byte(id))
	h.Write([]byte(convID))
	h.Write([]byte(sender))
	h.Write([]byte(recipient))
	h.Write([]byte(string(msgType)))
	payloadBytes, _ := json.Marshal(payload)
	h.Write(payloadBytes)
	return hex.EncodeToString(h.Sum(nil))
}

// DelegationTask is a typed payload for TypeDelegate messages.
type DelegationTask struct {
	Goal            string                 `json:"goal"`
	Capability      string                 `json:"capability"`
	Arguments       map[string]interface{} `json:"arguments"`
	TimeoutSeconds  int                    `json:"timeout_seconds"`
	RequiresReply   bool                   `json:"requires_reply"`
	ApprovedActions []string               `json:"approved_actions,omitempty"`
}

// DelegationResult is a typed payload for TypeResult messages.
type DelegationResult struct {
	TaskID   string                 `json:"task_id"`
	Status   string                 `json:"status"` // "done", "partial", "blocked", "failed"
	Result   interface{}            `json:"result"`
	Evidence string                 `json:"evidence,omitempty"`
	Error    string                 `json:"error,omitempty"`
	DurationMs int64                `json:"duration_ms"`
}

// A2AAuditReport summarizes inter-agent traffic and delivery SLAs.
type A2AAuditReport struct {
	TotalMessages      int                    `json:"total_messages"`
	DeliveredMessages  int                    `json:"delivered_messages"`
	PendingMessages    int                    `json:"pending_messages"`
	RegisteredAgents   int                    `json:"registered_agents"`
	ActiveDelegations  int                    `json:"active_delegations"`
	FailedDelegations  int                    `json:"failed_delegations"`
	AvgLatencyMs       float64                `json:"avg_latency_ms"`
	AgentDirectory     []AgentCard            `json:"agent_directory"`
	MessageLog         []MessageEnvelope      `json:"message_log"`
	SLAPercent         float64                `json:"sla_percent"`
}
