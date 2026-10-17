package pty

import (
	"time"
)

// EventType denotes the nature of a terminal I/O or control event.
type EventType string

const (
	// EventOutput represents terminal stdout/stderr stream data ("o" in asciinema v2).
	EventOutput EventType = "o"
	// EventInput represents terminal stdin keyboard data ("i" in asciinema v2).
	EventInput EventType = "i"
	// EventResize represents terminal window dimensions change ("r" in asciinema v2).
	EventResize EventType = "r"
	// EventSignal represents an intercepted POSIX process signal ("s").
	EventSignal EventType = "s"
)

// WindowSize holds terminal row and column dimensions.
type WindowSize struct {
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}

// DefaultWindowSize provides standard 80x24 terminal dimensions.
func DefaultWindowSize() WindowSize {
	return WindowSize{Cols: 80, Rows: 24}
}

// PTYEvent records an atomic I/O chunk or terminal geometry update.
type PTYEvent struct {
	Timestamp time.Time `json:"timestamp"`
	OffsetSec float64   `json:"offset_sec"`
	Type      EventType `json:"type"`
	Data      string    `json:"data"`
}

// SessionMetadata captures runtime context for an agent terminal execution.
type SessionMetadata struct {
	ID          string            `json:"id"`
	Command     string            `json:"command"`
	Args        []string          `json:"args,omitempty"`
	WorkingDir  string            `json:"working_dir,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	StartTime   time.Time         `json:"start_time"`
	EndTime     time.Time         `json:"end_time"`
	ExitCode    int               `json:"exit_code"`
	InitialSize WindowSize        `json:"initial_size"`
}

// PTYSession encapsulates a complete recorded interactive terminal session.
type PTYSession struct {
	Metadata SessionMetadata `json:"metadata"`
	Events   []PTYEvent      `json:"events"`
}

// TotalDuration returns the session elapsed time in seconds.
func (s *PTYSession) TotalDuration() float64 {
	if len(s.Events) == 0 {
		return 0
	}
	return s.Events[len(s.Events)-1].OffsetSec
}
