package actor

import (
	"sync"
	"time"
)

// DeadLetterOffice captures unroutable or dropped messages across the actor system.
type DeadLetterOffice struct {
	mu      sync.RWMutex
	letters []DeadLetter
	limit   int
}

// NewDeadLetterOffice creates a new dead letter office with maximum capacity.
func NewDeadLetterOffice(limit int) *DeadLetterOffice {
	if limit <= 0 {
		limit = 1000
	}
	return &DeadLetterOffice{
		letters: make([]DeadLetter, 0),
		limit:   limit,
	}
}

// Record captures an unroutable or rejected message.
func (dlo *DeadLetterOffice) Record(msg Message, reason string) {
	dlo.mu.Lock()
	defer dlo.mu.Unlock()

	dl := DeadLetter{
		Message:   msg,
		Reason:    reason,
		Timestamp: time.Now().UTC(),
	}

	if len(dlo.letters) >= dlo.limit {
		dlo.letters = dlo.letters[1:] // Evict oldest
	}
	dlo.letters = append(dlo.letters, dl)
}

// Count returns the number of captured dead letters.
func (dlo *DeadLetterOffice) Count() int {
	dlo.mu.RLock()
	defer dlo.mu.RUnlock()
	return len(dlo.letters)
}

// GetAll returns a copy of captured dead letters.
func (dlo *DeadLetterOffice) GetAll() []DeadLetter {
	dlo.mu.RLock()
	defer dlo.mu.RUnlock()

	res := make([]DeadLetter, len(dlo.letters))
	copy(res, dlo.letters)
	return res
}

// Mailbox provides a thread-safe, bounded, priority-aware queue for an actor.
type Mailbox struct {
	mu         sync.Mutex
	cond       *sync.Cond
	messages   []Message
	config     MailboxConfig
	deadOffice *DeadLetterOffice
	closed     bool
}

// NewMailbox initializes a mailbox with configured capacity and drop policy.
func NewMailbox(config MailboxConfig, dlo *DeadLetterOffice) *Mailbox {
	m := &Mailbox{
		messages:   make([]Message, 0, config.Capacity),
		config:     config,
		deadOffice: dlo,
	}
	m.cond = sync.NewCond(&m.mu)
	return m
}

// Push enqueues a message, inserting higher-priority messages ahead of lower-priority ones.
// Returns true if the message was pushed, or false if dropped.
func (m *Mailbox) Push(msg Message) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		if m.deadOffice != nil {
			m.deadOffice.Record(msg, "mailbox_closed")
		}
		return false
	}

	// Check capacity
	if len(m.messages) >= m.config.Capacity {
		switch m.config.DropPolicy {
		case DropOldest:
			dropped := m.messages[0]
			m.messages = m.messages[1:]
			if m.deadOffice != nil {
				m.deadOffice.Record(dropped, "mailbox_overflow_evict_oldest")
			}
		case DropNewest:
			if m.deadOffice != nil {
				m.deadOffice.Record(msg, "mailbox_overflow_drop_newest")
			}
			return false
		case DropToDeadLett:
			if m.deadOffice != nil {
				m.deadOffice.Record(msg, "mailbox_overflow_dead_letter")
			}
			return false
		}
	}

	// Priority-aware insertion: maintain descending order of priority
	inserted := false
	for i, existing := range m.messages {
		if msg.Priority > existing.Priority {
			m.messages = append(m.messages[:i], append([]Message{msg}, m.messages[i:]...)...)
			inserted = true
			break
		}
	}
	if !inserted {
		m.messages = append(m.messages, msg)
	}

	m.cond.Signal()
	return true
}

// Pop retrieves the next message, blocking until one arrives or the mailbox closes.
func (m *Mailbox) Pop() (Message, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for len(m.messages) == 0 && !m.closed {
		m.cond.Wait()
	}

	if len(m.messages) == 0 {
		return Message{}, false
	}

	msg := m.messages[0]
	m.messages = m.messages[1:]
	return msg, true
}

// PopTimeout attempts to retrieve a message within the specified duration.
func (m *Mailbox) PopTimeout(timeout time.Duration) (Message, bool) {
	ch := make(chan struct{})
	go func() {
		time.Sleep(timeout)
		m.mu.Lock()
		m.cond.Broadcast()
		m.mu.Unlock()
		close(ch)
	}()

	m.mu.Lock()
	defer m.mu.Unlock()

	deadline := time.Now().Add(timeout)
	for len(m.messages) == 0 && !m.closed && time.Now().Before(deadline) {
		m.cond.Wait()
	}

	if len(m.messages) == 0 {
		return Message{}, false
	}

	msg := m.messages[0]
	m.messages = m.messages[1:]
	return msg, true
}

// Len returns current queue size.
func (m *Mailbox) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.messages)
}

// Close closes the mailbox and wakes waiting consumers.
func (m *Mailbox) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	m.cond.Broadcast()
}
