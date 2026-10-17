package pty

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Recorder captures live terminal I/O streams and window events.
type Recorder struct {
	mu        sync.RWMutex
	session   PTYSession
	startTime time.Time
	running   bool
}

// NewRecorder creates a new recorder instance initialized with metadata.
func NewRecorder(id, command string, args []string, initialSize WindowSize) *Recorder {
	now := time.Now().UTC()
	if initialSize.Cols <= 0 {
		initialSize.Cols = 80
	}
	if initialSize.Rows <= 0 {
		initialSize.Rows = 24
	}

	return &Recorder{
		startTime: now,
		running:   true,
		session: PTYSession{
			Metadata: SessionMetadata{
				ID:          id,
				Command:     command,
				Args:        args,
				StartTime:   now,
				InitialSize: initialSize,
				Env:         make(map[string]string),
			},
			Events: make([]PTYEvent, 0),
		},
	}
}

// RecordOutput appends standard output / stderr chunks from the spawned process.
func (r *Recorder) RecordOutput(data []byte) {
	r.recordEvent(EventOutput, string(data))
}

// RecordInput appends user or agent keyboard inputs sent to stdin.
func (r *Recorder) RecordInput(data []byte) {
	r.recordEvent(EventInput, string(data))
}

// RecordResize logs a terminal window geometry change (SIGWINCH).
func (r *Recorder) RecordResize(cols, rows int) {
	data := fmt.Sprintf("%dx%d", cols, rows)
	r.recordEvent(EventResize, data)
}

// RecordSignal logs an intercepted POSIX process signal.
func (r *Recorder) RecordSignal(sig string) {
	r.recordEvent(EventSignal, sig)
}

func (r *Recorder) recordEvent(evType EventType, data string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	offset := now.Sub(r.startTime).Seconds()
	if offset < 0 {
		offset = 0
	}

	r.session.Events = append(r.session.Events, PTYEvent{
		Timestamp: now,
		OffsetSec: offset,
		Type:      evType,
		Data:      data,
	})
}

// Finish marks the terminal session as completed and sets the process exit code.
func (r *Recorder) Finish(exitCode int) *PTYSession {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.running = false
	r.session.Metadata.EndTime = time.Now().UTC()
	r.session.Metadata.ExitCode = exitCode

	// Deep copy session for caller
	copied := r.session
	copied.Events = make([]PTYEvent, len(r.session.Events))
	copy(copied.Events, r.session.Events)
	return &copied
}

// Session returns a snapshot of the current recorded session.
func (r *Recorder) Session() *PTYSession {
	r.mu.RLock()
	defer r.mu.RUnlock()

	copied := r.session
	copied.Events = make([]PTYEvent, len(r.session.Events))
	copy(copied.Events, r.session.Events)
	return &copied
}

// EventCount returns the number of captured events so far.
func (r *Recorder) EventCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.session.Events)
}

// ExportJSON serializes the full PTYSession to formatted JSON.
func (r *Recorder) ExportJSON() ([]byte, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return json.MarshalIndent(r.session, "", "  ")
}

// ImportJSON parses a serialized PTYSession from JSON.
func ImportJSON(data []byte) (*PTYSession, error) {
	var s PTYSession
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("failed to parse PTY session JSON: %w", err)
	}
	return &s, nil
}
