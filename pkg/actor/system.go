package actor

import (
	"fmt"
	"sync"
	"time"
)

// ActorSystem is the root coordinator managing actor registration, messaging, and supervision.
type ActorSystem struct {
	id          string
	startedAt   time.Time
	mu          sync.RWMutex
	actors      map[string]*ActorCell
	supervisors map[string]*Supervisor
	deadLetters *DeadLetterOffice
	topics      map[string][]string // topic -> []actorID

	eventMu     sync.RWMutex
	events      []ActorEventRecord
	eventSeq    int64
	totalMsgs   int64
	stopped     bool
}

// NewActorSystem creates a new actor system instance.
func NewActorSystem(id string) *ActorSystem {
	return &ActorSystem{
		id:          id,
		startedAt:   time.Now().UTC(),
		actors:      make(map[string]*ActorCell),
		supervisors: make(map[string]*Supervisor),
		deadLetters: NewDeadLetterOffice(1000),
		topics:      make(map[string][]string),
		events:      make([]ActorEventRecord, 0),
	}
}

// DeadLetters returns the system dead letter office.
func (sys *ActorSystem) DeadLetters() *DeadLetterOffice {
	return sys.deadLetters
}

// SpawnSupervisor creates and registers a new supervisor.
func (sys *ActorSystem) SpawnSupervisor(id string, strategy SupervisionStrategy, policy RestartPolicy) *Supervisor {
	sys.mu.Lock()
	defer sys.mu.Unlock()

	sup := NewSupervisor(id, strategy, policy, sys)
	sys.supervisors[id] = sup
	return sup
}

// SpawnActor creates an actor under an optional supervisor and starts its processing loop.
func (sys *ActorSystem) SpawnActor(id, role string, behavior BehaviorFunc, mbox MailboxConfig, sup *Supervisor) (*ActorCell, error) {
	sys.mu.Lock()
	defer sys.mu.Unlock()

	if _, exists := sys.actors[id]; exists {
		return nil, fmt.Errorf("actor with ID '%s' already exists", id)
	}

	cell := NewActorCell(id, role, behavior, mbox, sys, sup)
	sys.actors[id] = cell

	if sup != nil {
		sup.RegisterChild(cell)
	}

	cell.Start()
	sys.recordEventLocked("SPAWN", id, "", map[string]interface{}{
		"role": role,
	})
	return cell, nil
}

// Subscribe adds an actor to receive messages on a broadcast topic.
func (sys *ActorSystem) Subscribe(actorID, topic string) {
	sys.mu.Lock()
	defer sys.mu.Unlock()

	sys.topics[topic] = append(sys.topics[topic], actorID)
}

// Send delivers an asynchronous message to the recipient's mailbox.
func (sys *ActorSystem) Send(msg Message) error {
	sys.mu.RLock()
	cell, exists := sys.actors[msg.Recipient]
	sys.mu.RUnlock()

	sys.mu.Lock()
	sys.totalMsgs++
	sys.mu.Unlock()

	if !exists {
		sys.deadLetters.Record(msg, fmt.Sprintf("recipient_not_found: %s", msg.Recipient))
		sys.recordEvent("DEAD_LETTER", msg.Sender, msg.Recipient, map[string]interface{}{
			"reason": "recipient_not_found",
			"msg_id": msg.ID,
		})
		return fmt.Errorf("actor '%s' not found", msg.Recipient)
	}

	sys.recordEvent("MSG_SENT", msg.Sender, msg.Recipient, map[string]interface{}{
		"msg_id":   msg.ID,
		"priority": int(msg.Priority),
	})

	if !cell.mailbox.Push(msg) {
		return fmt.Errorf("message dropped by actor '%s' mailbox policy", msg.Recipient)
	}
	return nil
}

// Publish broadcasts a message to all actors subscribed to the topic.
func (sys *ActorSystem) Publish(sender, topic string, payload map[string]interface{}) int {
	sys.mu.RLock()
	subs := append([]string{}, sys.topics[topic]...)
	sys.mu.RUnlock()

	count := 0
	for _, subID := range subs {
		msg := Message{
			ID:        fmt.Sprintf("pub_%d_%d", time.Now().UnixNano(), count),
			Sender:    sender,
			Recipient: subID,
			Topic:     topic,
			Priority:  PriorityNormal,
			Payload:   payload,
			Timestamp: time.Now().UTC(),
		}
		if err := sys.Send(msg); err == nil {
			count++
		}
	}
	return count
}

// Ask sends a message and waits for a response from the target actor within timeout.
func (sys *ActorSystem) Ask(sender, target string, payload map[string]interface{}, timeout time.Duration) (Message, error) {
	replyInbox := make(chan Message, 1)
	tempActorID := fmt.Sprintf("temp_ask_%d", time.Now().UnixNano())

	tempBehavior := func(ctx *ActorContext, msg Message) error {
		select {
		case replyInbox <- msg:
		default:
		}
		return nil
	}

	tempCell, err := sys.SpawnActor(tempActorID, "ephemeral_ask", tempBehavior, DefaultMailboxConfig(), nil)
	if err != nil {
		return Message{}, err
	}
	defer func() {
		tempCell.Stop()
		sys.mu.Lock()
		delete(sys.actors, tempActorID)
		sys.mu.Unlock()
	}()

	askMsg := Message{
		ID:        fmt.Sprintf("ask_%d", time.Now().UnixNano()),
		Sender:    tempActorID,
		Recipient: target,
		Priority:  PriorityHigh,
		Payload:   payload,
		Timestamp: time.Now().UTC(),
	}

	if err := sys.Send(askMsg); err != nil {
		return Message{}, err
	}

	select {
	case reply := <-replyInbox:
		return reply, nil
	case <-time.After(timeout):
		return Message{}, fmt.Errorf("ask timeout after %v waiting for response from '%s'", timeout, target)
	}
}

// Audit evaluates the health, telemetry, and resilience of all actors in the system.
func (sys *ActorSystem) Audit() *SystemAuditReport {
	sys.mu.RLock()
	defer sys.mu.RUnlock()

	report := &SystemAuditReport{
		SystemID:         sys.id,
		StartedAt:        sys.startedAt,
		TotalActors:      len(sys.actors),
		TotalMessages:    sys.totalMsgs,
		DeadLettersCount: sys.deadLetters.Count(),
		DeadLetters:      sys.deadLetters.GetAll(),
		Actors:           make([]ActorTelemetry, 0, len(sys.actors)),
	}

	allResilient := true
	totalRestarts := 0
	activeActors := 0

	for _, cell := range sys.actors {
		telem := cell.Telemetry()
		report.Actors = append(report.Actors, telem)
		totalRestarts += telem.RestartsCount

		if telem.Status == StatusReady || telem.Status == StatusBusy {
			activeActors++
		}
		if telem.Status == StatusTerminated || telem.ErrorsCount > 5 {
			allResilient = false
		}
	}

	report.ActiveActors = activeActors
	report.TotalRestarts = totalRestarts
	report.AllResilient = allResilient

	sys.eventMu.RLock()
	report.Events = append([]ActorEventRecord{}, sys.events...)
	sys.eventMu.RUnlock()

	return report
}

// Shutdown stops all actors and terminates the system.
func (sys *ActorSystem) Shutdown() {
	sys.mu.Lock()
	defer sys.mu.Unlock()

	if sys.stopped {
		return
	}
	sys.stopped = true

	for _, cell := range sys.actors {
		cell.Stop()
	}
}

func (sys *ActorSystem) recordEvent(eventType, actorID, targetID string, details map[string]interface{}) {
	sys.eventMu.Lock()
	defer sys.eventMu.Unlock()
	sys.recordEventLocked(eventType, actorID, targetID, details)
}

func (sys *ActorSystem) recordEventLocked(eventType, actorID, targetID string, details map[string]interface{}) {
	sys.eventSeq++
	sys.events = append(sys.events, ActorEventRecord{
		Index:       sys.eventSeq,
		EventType:   eventType,
		ActorID:     actorID,
		TargetID:    targetID,
		Details:     details,
		TimestampUs: time.Now().UnixMicro(),
	})
}
