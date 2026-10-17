package pty

import (
	"fmt"
	"strings"
	"sync"
)

// Player provides time-travel seeking and step playback over a recorded PTYSession.
type Player struct {
	mu           sync.RWMutex
	session      *PTYSession
	grid         *ScreenGrid
	currentIndex int
}

// NewPlayer initializes a player bound to a recorded session.
func NewPlayer(session *PTYSession) *Player {
	initSize := session.Metadata.InitialSize
	if initSize.Cols <= 0 {
		initSize.Cols = 80
	}
	if initSize.Rows <= 0 {
		initSize.Rows = 24
	}

	p := &Player{
		session: session,
		grid:    NewScreenGrid(initSize.Cols, initSize.Rows),
	}
	return p
}

// Reset clears the screen buffer and resets cursor to event index 0.
func (p *Player) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()

	initSize := p.session.Metadata.InitialSize
	p.grid = NewScreenGrid(initSize.Cols, initSize.Rows)
	p.currentIndex = 0
}

// SeekTo reconstructs the exact terminal screen state at a specific second offset.
func (p *Player) SeekTo(offsetSec float64) *ScreenGrid {
	p.mu.Lock()
	defer p.mu.Unlock()

	initSize := p.session.Metadata.InitialSize
	p.grid = NewScreenGrid(initSize.Cols, initSize.Rows)
	p.currentIndex = 0

	for i, ev := range p.session.Events {
		if ev.OffsetSec > offsetSec {
			p.currentIndex = i
			break
		}
		p.applyEvent(ev)
		p.currentIndex = i + 1
	}

	return p.grid
}

// StepForward applies the next event in the stream and returns it.
func (p *Player) StepForward() *PTYEvent {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.currentIndex >= len(p.session.Events) {
		return nil
	}

	ev := &p.session.Events[p.currentIndex]
	p.applyEvent(*ev)
	p.currentIndex++
	return ev
}

// CurrentGrid returns the current state of the virtual screen grid.
func (p *Player) CurrentGrid() *ScreenGrid {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.grid
}

// ExtractCommands reconstructs the full list of typed shell commands from EventInput chunks.
func (p *Player) ExtractCommands() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var commands []string
	var current strings.Builder

	for _, ev := range p.session.Events {
		if ev.Type == EventInput {
			for _, ch := range ev.Data {
				if ch == '\r' || ch == '\n' {
					cmd := strings.TrimSpace(current.String())
					if cmd != "" {
						commands = append(commands, cmd)
					}
					current.Reset()
				} else if ch == '\b' || ch == 0x7f { // Backspace
					s := current.String()
					if len(s) > 0 {
						current.Reset()
						current.WriteString(s[:len(s)-1])
					}
				} else if ch >= 32 && ch <= 126 { // Printable ASCII
					current.WriteRune(ch)
				}
			}
		}
	}

	last := strings.TrimSpace(current.String())
	if last != "" {
		commands = append(commands, last)
	}

	return commands
}

func (p *Player) applyEvent(ev PTYEvent) {
	switch ev.Type {
	case EventOutput:
		p.grid.Feed(ev.Data)
	case EventResize:
		var cols, rows int
		if _, err := fmt.Sscanf(ev.Data, "%dx%d", &cols, &rows); err == nil && cols > 0 && rows > 0 {
			p.grid.Resize(cols, rows)
		}
	}
}
