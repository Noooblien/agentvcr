package actor

import (
	"fmt"
	"sync"
	"time"
)

// BehaviorFunc defines the handler function for incoming messages.
type BehaviorFunc func(ctx *ActorContext, msg Message) error

// ActorContext provides execution metadata and message dispatch capabilities to an actor.
type ActorContext struct {
	system       *ActorSystem
	cell         *ActorCell
	currentMsg   *Message
	mu           sync.RWMutex
	state        interface{}
}

// Self returns the actor's unique identifier.
func (ctx *ActorContext) Self() string {
	return ctx.cell.id
}

// Role returns the role/type assigned to this actor.
func (ctx *ActorContext) Role() string {
	return ctx.cell.role
}

// Sender returns the identifier of the actor that sent the current message.
func (ctx *ActorContext) Sender() string {
	if ctx.currentMsg == nil {
		return ""
	}
	return ctx.currentMsg.Sender
}

// CurrentMessage returns the active message being processed.
func (ctx *ActorContext) CurrentMessage() *Message {
	return ctx.currentMsg
}

// State returns the actor's current internal state.
func (ctx *ActorContext) State() interface{} {
	ctx.mu.RLock()
	defer ctx.mu.RUnlock()
	return ctx.state
}

// SetState updates the actor's internal state.
func (ctx *ActorContext) SetState(s interface{}) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.state = s
}

// Tell sends an asynchronous message to the target actor.
func (ctx *ActorContext) Tell(target string, payload map[string]interface{}) error {
	return ctx.system.Send(Message{
		ID:        fmt.Sprintf("msg_%d", time.Now().UnixNano()),
		Sender:    ctx.Self(),
		Recipient: target,
		Priority:  PriorityNormal,
		Payload:   payload,
		Timestamp: time.Now().UTC(),
	})
}

// TellWithPriority sends an asynchronous message with specified priority.
func (ctx *ActorContext) TellWithPriority(target string, payload map[string]interface{}, priority MessagePriority) error {
	return ctx.system.Send(Message{
		ID:        fmt.Sprintf("msg_%d", time.Now().UnixNano()),
		Sender:    ctx.Self(),
		Recipient: target,
		Priority:  priority,
		Payload:   payload,
		Timestamp: time.Now().UTC(),
	})
}

// Ask sends a message and synchronously awaits a reply within timeout.
func (ctx *ActorContext) Ask(target string, payload map[string]interface{}, timeout time.Duration) (Message, error) {
	return ctx.system.Ask(ctx.Self(), target, payload, timeout)
}

// ActorCell manages the runtime execution, state, and lifecycle of a single actor.
type ActorCell struct {
	id         string
	role       string
	behavior   BehaviorFunc
	mailbox    *Mailbox
	system     *ActorSystem
	context    *ActorContext
	supervisor *Supervisor

	mu            sync.RWMutex
	status        ActorStatus
	restartsCount int
	errorsCount   int
	procCount     int64
	dropCount     int64
	totalLatUs    int64
	lastHeartbeat time.Time
	stopChan      chan struct{}
	stopped       bool
}

// NewActorCell initializes an actor cell.
func NewActorCell(id, role string, behavior BehaviorFunc, mboxConfig MailboxConfig, sys *ActorSystem, sup *Supervisor) *ActorCell {
	c := &ActorCell{
		id:            id,
		role:          role,
		behavior:      behavior,
		system:        sys,
		supervisor:    sup,
		status:        StatusInitializing,
		lastHeartbeat: time.Now().UTC(),
		stopChan:      make(chan struct{}),
	}
	c.mailbox = NewMailbox(mboxConfig, sys.DeadLetters())
	c.context = &ActorContext{
		system: sys,
		cell:   c,
	}
	return c
}

// Start begins the message consumer loop in a background goroutine.
func (c *ActorCell) Start() {
	c.mu.Lock()
	c.status = StatusReady
	c.mu.Unlock()

	go c.runLoop()
}

func (c *ActorCell) runLoop() {
	for {
		select {
		case <-c.stopChan:
			c.mu.Lock()
			c.status = StatusTerminated
			c.mu.Unlock()
			return
		default:
			msg, ok := c.mailbox.Pop()
			if !ok {
				c.mu.Lock()
				c.status = StatusTerminated
				c.mu.Unlock()
				return
			}

			c.processMessage(msg)
		}
	}
}

func (c *ActorCell) processMessage(msg Message) {
	c.mu.Lock()
	c.status = StatusBusy
	c.lastHeartbeat = time.Now().UTC()
	c.context.currentMsg = &msg
	c.mu.Unlock()

	start := time.Now()
	var err error

	// Safe execution with recover to catch panics in user-defined actor behavior
	func() {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("actor panic: %v", r)
			}
		}()
		err = c.behavior(c.context, msg)
	}()

	durationUs := time.Since(start).Microseconds()

	c.mu.Lock()
	c.procCount++
	c.totalLatUs += durationUs
	c.lastHeartbeat = time.Now().UTC()
	c.context.currentMsg = nil

	if err != nil {
		c.errorsCount++
		c.status = StatusReady
		c.mu.Unlock()

		// Record crash event
		c.system.recordEvent("CRASH", c.id, "", map[string]interface{}{
			"error":  err.Error(),
			"msg_id": msg.ID,
		})

		// Notify supervisor of failure
		if c.supervisor != nil {
			c.supervisor.HandleFailure(c, err)
		}
		return
	}

	c.status = StatusReady
	c.mu.Unlock()

	c.system.recordEvent("MSG_RECV", c.id, msg.Sender, map[string]interface{}{
		"msg_id":      msg.ID,
		"duration_us": durationUs,
	})
}

// Stop gracefully stops the actor and closes its mailbox.
func (c *ActorCell) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.stopped {
		return
	}
	c.stopped = true
	c.status = StatusTerminated
	close(c.stopChan)
	c.mailbox.Close()
}

// Telemetry returns a snapshot of runtime metrics for this actor.
func (c *ActorCell) Telemetry() ActorTelemetry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	avgLat := 0.0
	if c.procCount > 0 {
		avgLat = float64(c.totalLatUs) / float64(c.procCount)
	}

	return ActorTelemetry{
		ActorID:          c.id,
		Status:           c.status,
		Role:             c.role,
		ProcessedCount:   c.procCount,
		DroppedCount:     c.dropCount,
		RestartsCount:    c.restartsCount,
		ErrorsCount:      c.errorsCount,
		LastHeartbeat:    c.lastHeartbeat,
		AvgLatencyUs:     avgLat,
		CurrentQueueSize: c.mailbox.Len(),
	}
}
