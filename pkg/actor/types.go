package actor

import (
	"time"
)

// ActorStatus represents the lifecycle state of an actor.
type ActorStatus string

const (
	StatusInitializing ActorStatus = "INITIALIZING"
	StatusReady        ActorStatus = "READY"
	StatusBusy         ActorStatus = "BUSY"
	StatusRestarting   ActorStatus = "RESTARTING"
	StatusSuspended    ActorStatus = "SUSPENDED"
	StatusTerminated   ActorStatus = "TERMINATED"
)

// MessagePriority defines the processing priority of an actor message.
type MessagePriority int

const (
	PriorityLow    MessagePriority = 0
	PriorityNormal MessagePriority = 1
	PriorityHigh   MessagePriority = 2
	PrioritySystem MessagePriority = 3
)

// Message encapsulates an asynchronous unit of communication between actors.
type Message struct {
	ID        string                 `json:"id"`
	Sender    string                 `json:"sender"`
	Recipient string                 `json:"recipient"`
	Topic     string                 `json:"topic,omitempty"`
	Priority  MessagePriority        `json:"priority"`
	Payload   map[string]interface{} `json:"payload"`
	Timestamp time.Time              `json:"timestamp"`
}

// DeadLetter represents an unroutable or dropped message.
type DeadLetter struct {
	Message   Message   `json:"message"`
	Reason    string    `json:"reason"`
	Timestamp time.Time `json:"timestamp"`
}

// SupervisionStrategy defines how a supervisor handles child failures.
type SupervisionStrategy string

const (
	StrategyOneForOne  SupervisionStrategy = "ONE_FOR_ONE"  // Restart only the failed actor
	StrategyOneForAll  SupervisionStrategy = "ONE_FOR_ALL"  // Restart all sibling actors under supervisor
	StrategyRestForOne SupervisionStrategy = "REST_FOR_ONE" // Restart failed actor and subsequent siblings
)

// RestartPolicy specifies limits and backoff for actor recovery.
type RestartPolicy struct {
	MaxRestarts int           `json:"max_restarts"` // Max restarts before escalating failure
	TimeWindow  time.Duration `json:"time_window"`  // Time window for MaxRestarts
	Backoff     time.Duration `json:"backoff"`      // Wait before reviving
}

// DefaultRestartPolicy provides standard production-grade resilience defaults.
func DefaultRestartPolicy() RestartPolicy {
	return RestartPolicy{
		MaxRestarts: 3,
		TimeWindow:  10 * time.Second,
		Backoff:     5 * time.Millisecond,
	}
}

// MailboxDropPolicy defines behavior when an actor's mailbox capacity is reached.
type MailboxDropPolicy string

const (
	DropOldest     MailboxDropPolicy = "DROP_OLDEST"
	DropNewest     MailboxDropPolicy = "DROP_NEWEST"
	DropToDeadLett MailboxDropPolicy = "DEAD_LETTER"
)

// MailboxConfig specifies queue capacity and overflow behavior.
type MailboxConfig struct {
	Capacity   int               `json:"capacity"`
	DropPolicy MailboxDropPolicy `json:"drop_policy"`
}

// DefaultMailboxConfig returns standard mailbox parameters.
func DefaultMailboxConfig() MailboxConfig {
	return MailboxConfig{
		Capacity:   500,
		DropPolicy: DropToDeadLett,
	}
}

// ActorTelemetry contains live performance and health metrics for an actor.
type ActorTelemetry struct {
	ActorID          string      `json:"actor_id"`
	Status           ActorStatus `json:"status"`
	Role             string      `json:"role"`
	ProcessedCount   int64       `json:"processed_count"`
	DroppedCount     int64       `json:"dropped_count"`
	RestartsCount    int         `json:"restarts_count"`
	ErrorsCount      int         `json:"errors_count"`
	LastHeartbeat    time.Time   `json:"last_heartbeat"`
	AvgLatencyUs     float64     `json:"avg_latency_us"`
	CurrentQueueSize int         `json:"current_queue_size"`
}

// ActorEventRecord captures deterministic trace events of the actor mesh.
type ActorEventRecord struct {
	Index       int64                  `json:"index"`
	EventType   string                 `json:"event_type"` // e.g. "MSG_SENT", "MSG_RECV", "RESTART", "CRASH"
	ActorID     string                 `json:"actor_id"`
	TargetID    string                 `json:"target_id,omitempty"`
	Details     map[string]interface{} `json:"details,omitempty"`
	TimestampUs int64                  `json:"timestamp_us"`
}

// SystemAuditReport holds aggregated health and resilience metrics for the actor system.
type SystemAuditReport struct {
	SystemID         string           `json:"system_id"`
	StartedAt        time.Time        `json:"started_at"`
	TotalActors      int              `json:"total_actors"`
	ActiveActors     int              `json:"active_actors"`
	TotalMessages    int64            `json:"total_messages"`
	TotalRestarts    int              `json:"total_restarts"`
	DeadLettersCount int              `json:"dead_letters_count"`
	DeadLetters      []DeadLetter     `json:"dead_letters,omitempty"`
	Actors           []ActorTelemetry `json:"actors"`
	Events           []ActorEventRecord `json:"events,omitempty"`
	AllResilient     bool             `json:"all_resilient"`
}
