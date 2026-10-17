package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/pty"
)

func TestPTY_RecorderLifecycle(t *testing.T) {
	rec := pty.NewRecorder("pty-test-1", "bash", []string{"-i"}, pty.WindowSize{Cols: 80, Rows: 24})

	rec.RecordInput([]byte("echo 'hello agent'\n"))
	rec.RecordOutput([]byte("hello agent\r\n"))
	rec.RecordResize(120, 35)
	rec.RecordSignal("SIGWINCH")

	session := rec.Finish(0)

	if session.Metadata.ID != "pty-test-1" {
		t.Fatalf("expected ID pty-test-1, got %s", session.Metadata.ID)
	}
	if session.Metadata.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", session.Metadata.ExitCode)
	}
	if len(session.Events) != 4 {
		t.Fatalf("expected 4 events, got %d", len(session.Events))
	}

	// Verify JSON export and import
	jsonBytes, err := rec.ExportJSON()
	if err != nil {
		t.Fatalf("failed to export JSON: %v", err)
	}

	imported, err := pty.ImportJSON(jsonBytes)
	if err != nil {
		t.Fatalf("failed to import JSON: %v", err)
	}

	if len(imported.Events) != len(session.Events) {
		t.Fatalf("event length mismatch after import: %d vs %d", len(imported.Events), len(session.Events))
	}
}

func TestPTY_ScreenGrid_Basics(t *testing.T) {
	grid := pty.NewScreenGrid(40, 10)

	grid.Feed("Line 1\r\nLine 2\r\nLine 3")
	text := grid.RenderText()

	expected := "Line 1\nLine 2\nLine 3"
	if text != expected {
		t.Fatalf("expected text:\n%q\ngot:\n%q", expected, text)
	}

	// Test backspace
	grid.Feed("\r\nABC\b\bD")
	text2 := grid.RenderText()
	if !strings.Contains(text2, "Line 3\nAD") {
		t.Fatalf("backspace failed, got:\n%s", text2)
	}

	// Test tab
	grid2 := pty.NewScreenGrid(40, 5)
	grid2.Feed("A\tB")
	textTab := grid2.RenderText()
	if textTab != "A       B" {
		t.Fatalf("tab failed, expected 'A       B', got %q", textTab)
	}
}

func TestPTY_ScreenGrid_ANSIEscapes(t *testing.T) {
	grid := pty.NewScreenGrid(80, 24)

	// Feed cursor jump to row 5, col 10
	grid.Feed("\033[5;10HTEST")
	if grid.CursorY != 4 || grid.CursorX != 13 {
		t.Fatalf("expected cursor at (13, 4), got (%d, %d)", grid.CursorX, grid.CursorY)
	}

	// Feed Clear Line
	grid.Feed("\033[2K")
	row4 := strings.TrimSpace(grid.RenderText())
	if row4 != "" {
		t.Fatalf("expected cleared line, got %q", row4)
	}

	// Feed full screen clear
	grid.Feed("Some text\033[2J")
	if strings.TrimSpace(grid.RenderText()) != "" {
		t.Fatalf("expected empty screen after \\033[2J")
	}
	if grid.CursorX != 0 || grid.CursorY != 0 {
		t.Fatalf("expected cursor home (0,0), got (%d, %d)", grid.CursorX, grid.CursorY)
	}

	// Cursor movement: up, down, forward, backward
	grid.Feed("A") // (1, 0)
	grid.Feed("\033[2C") // forward 2 -> col 3
	if grid.CursorX != 3 {
		t.Fatalf("expected cursorX 3 after forward, got %d", grid.CursorX)
	}
	grid.Feed("\033[1D") // back 1 -> col 2
	if grid.CursorX != 2 {
		t.Fatalf("expected cursorX 2 after back, got %d", grid.CursorX)
	}
	grid.Feed("\033[3B") // down 3 -> row 3
	if grid.CursorY != 3 {
		t.Fatalf("expected cursorY 3 after down, got %d", grid.CursorY)
	}
	grid.Feed("\033[2A") // up 2 -> row 1
	if grid.CursorY != 1 {
		t.Fatalf("expected cursorY 1 after up, got %d", grid.CursorY)
	}
}

func TestPTY_AsciinemaV2_ExportAndImport(t *testing.T) {
	now := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	session := &pty.PTYSession{
		Metadata: pty.SessionMetadata{
			ID:          "pty-asciinema-1",
			Command:     "/bin/bash",
			StartTime:   now,
			InitialSize: pty.WindowSize{Cols: 80, Rows: 24},
			Env: map[string]string{
				"TERM":  "xterm-256color",
				"SHELL": "/bin/bash",
			},
		},
		Events: []pty.PTYEvent{
			{
				Timestamp: now.Add(100 * time.Millisecond),
				OffsetSec: 0.1,
				Type:      pty.EventOutput,
				Data:      "user@host:~$ ",
			},
			{
				Timestamp: now.Add(500 * time.Millisecond),
				OffsetSec: 0.5,
				Type:      pty.EventInput,
				Data:      "ls -la\r",
			},
			{
				Timestamp: now.Add(800 * time.Millisecond),
				OffsetSec: 0.8,
				Type:      pty.EventOutput,
				Data:      "\r\ntotal 0\r\nuser@host:~$ ",
			},
		},
	}

	castBytes, err := pty.ExportAsciinemaV2(session)
	if err != nil {
		t.Fatalf("failed to export asciinema: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(castBytes)), "\n")
	if len(lines) != 4 { // 1 header + 3 events
		t.Fatalf("expected 4 lines in cast output, got %d", len(lines))
	}

	if !strings.Contains(lines[0], `"version":2`) {
		t.Fatalf("header missing version 2: %s", lines[0])
	}
	if !strings.Contains(lines[1], `"user@host:~$ "`) {
		t.Fatalf("event 1 mismatch: %s", lines[1])
	}

	// Re-import
	imported, err := pty.ImportAsciinemaV2(castBytes)
	if err != nil {
		t.Fatalf("failed to import asciinema v2: %v", err)
	}

	if len(imported.Events) != 3 {
		t.Fatalf("expected 3 imported events, got %d", len(imported.Events))
	}
	if imported.Events[0].Data != "user@host:~$ " {
		t.Fatalf("imported data mismatch: %s", imported.Events[0].Data)
	}
}

func TestPTY_Player_TimeTravelAndCommands(t *testing.T) {
	now := time.Now().UTC()
	session := &pty.PTYSession{
		Metadata: pty.SessionMetadata{
			ID:          "pty-player-1",
			Command:     "zsh",
			StartTime:   now,
			InitialSize: pty.WindowSize{Cols: 60, Rows: 10},
		},
		Events: []pty.PTYEvent{
			{OffsetSec: 0.1, Type: pty.EventOutput, Data: "Welcome to Agent Shell\r\n> "},
			{OffsetSec: 0.5, Type: pty.EventInput, Data: "git status\r"},
			{OffsetSec: 0.8, Type: pty.EventOutput, Data: "On branch main\r\n> "},
			{OffsetSec: 1.2, Type: pty.EventInput, Data: "pytest\r"},
			{OffsetSec: 1.5, Type: pty.EventOutput, Data: "100 passed in 0.2s\r\n> "},
		},
	}

	player := pty.NewPlayer(session)

	// Test command extraction
	commands := player.ExtractCommands()
	if len(commands) != 2 || commands[0] != "git status" || commands[1] != "pytest" {
		t.Fatalf("unexpected extracted commands: %v", commands)
	}

	// Test seek to t=0.3s (before git status output)
	gridAt03 := player.SeekTo(0.3)
	textAt03 := gridAt03.RenderText()
	if !strings.Contains(textAt03, "Welcome to Agent Shell") || strings.Contains(textAt03, "On branch main") {
		t.Fatalf("unexpected screen at t=0.3:\n%s", textAt03)
	}

	// Test seek to t=1.0s (after git status, before pytest output)
	gridAt10 := player.SeekTo(1.0)
	textAt10 := gridAt10.RenderText()
	if !strings.Contains(textAt10, "On branch main") || strings.Contains(textAt10, "100 passed") {
		t.Fatalf("unexpected screen at t=1.0:\n%s", textAt10)
	}

	// Test seek to end
	gridEnd := player.SeekTo(2.0)
	textEnd := gridEnd.RenderText()
	if !strings.Contains(textEnd, "100 passed in 0.2s") {
		t.Fatalf("unexpected final screen:\n%s", textEnd)
	}

	// Test HTML output rendering
	htmlOutput := gridEnd.RenderHTML()
	if !strings.Contains(htmlOutput, "<pre style=") || !strings.Contains(htmlOutput, "100 passed") {
		t.Fatalf("invalid HTML output: %s", htmlOutput)
	}

	// Test step forward from reset
	player.Reset()
	ev1 := player.StepForward()
	if ev1 == nil || ev1.Data != "Welcome to Agent Shell\r\n> " {
		t.Fatalf("unexpected step 1 event: %v", ev1)
	}
}
