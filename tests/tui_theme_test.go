package tests

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
	"github.com/Noooblien/agentvcr/pkg/tui"
)

func TestTUI_ThemeRegistryAndEnv(t *testing.T) {
	themes := tui.ListThemes()
	if len(themes) < 5 {
		t.Errorf("expected at least 5 registered themes, got %d", len(themes))
	}

	dracula := tui.GetTheme("dracula")
	if dracula.Name != "dracula" {
		t.Errorf("expected theme name dracula, got %s", dracula.Name)
	}

	monokai := tui.GetTheme("monokai")
	if monokai.Name != "monokai" {
		t.Errorf("expected theme name monokai, got %s", monokai.Name)
	}

	fallback := tui.GetTheme("nonexistent-theme-xyz")
	if fallback.Name != "default" {
		t.Errorf("expected fallback to default, got %s", fallback.Name)
	}

	// Test environment variable loading
	os.Setenv("AGENTVCR_THEME", "gruvbox")
	defer os.Unsetenv("AGENTVCR_THEME")

	envTheme := tui.LoadThemeFromEnv()
	if envTheme.Name != "gruvbox" {
		t.Errorf("expected env theme gruvbox, got %s", envTheme.Name)
	}
}

func TestTUI_ThemeColorApplication(t *testing.T) {
	ev := trace.TraceEvent{
		Index:      1,
		Timestamp:  time.Now(),
		Type:       trace.EventError,
		DurationMs: 15,
		Payload: map[string]interface{}{
			"error": "database connection timeout",
		},
	}

	var buf bytes.Buffer
	player := tui.NewPlayer(&buf)
	player.SetThemeByName("dracula")

	card := player.RenderEventCard(ev, 1)
	if !strings.Contains(card, "RUNTIME EXCEPTION") {
		t.Errorf("expected RUNTIME EXCEPTION in card")
	}
	if !strings.Contains(card, tui.DraculaRed) {
		t.Errorf("expected Dracula red ANSI escape in error card")
	}
}

func TestTUI_InteractiveViKeybindings(t *testing.T) {
	session := &trace.Session{
		ID:        "vi-debug-session",
		AgentName: "code-debugger",
		Status:    "running",
		CreatedAt: time.Now(),
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventLLMRequest, DurationMs: 100},
			{Index: 2, Type: trace.EventToolCall, DurationMs: 50},
			{Index: 3, Type: trace.EventToolResult, DurationMs: 25},
			{Index: 4, Type: trace.EventLLMResponse, DurationMs: 200},
		},
	}

	var out bytes.Buffer
	ip := tui.NewInteractivePlayer(session, nil, nil, &out)

	// Step forward with 'j'
	action, quit := ip.HandleKey("j")
	if quit || action != "step_forward:1" || ip.CurrentIndex != 1 {
		t.Fatalf("expected step_forward:1, got action=%s quit=%v idx=%d", action, quit, ip.CurrentIndex)
	}

	// Step forward with 'n'
	action, _ = ip.HandleKey("n")
	if action != "step_forward:2" || ip.CurrentIndex != 2 {
		t.Fatalf("expected step_forward:2, got %s", action)
	}

	// Step backward with 'k'
	action, _ = ip.HandleKey("k")
	if action != "step_backward:1" || ip.CurrentIndex != 1 {
		t.Fatalf("expected step_backward:1, got %s", action)
	}

	// Jump to end with 'G'
	action, _ = ip.HandleKey("G")
	if action != "jump_end" || ip.CurrentIndex != 3 {
		t.Fatalf("expected jump_end at idx 3, got %s idx=%d", action, ip.CurrentIndex)
	}

	// Attempt stepping past end
	action, _ = ip.HandleKey("j")
	if action != "at_end" || ip.CurrentIndex != 3 {
		t.Fatalf("expected at_end at idx 3, got %s", action)
	}

	// Jump to start with 'g'
	action, _ = ip.HandleKey("g")
	if action != "jump_start" || ip.CurrentIndex != 0 {
		t.Fatalf("expected jump_start at idx 0, got %s", action)
	}

	// Attempt stepping before start
	action, _ = ip.HandleKey("k")
	if action != "at_start" || ip.CurrentIndex != 0 {
		t.Fatalf("expected at_start at idx 0, got %s", action)
	}

	// Toggle help with '?'
	action, _ = ip.HandleKey("?")
	if action != "toggle_help" || !ip.ShowHelp {
		t.Fatalf("expected help toggled on")
	}

	// Cycle theme with 't'
	action, _ = ip.HandleKey("t")
	if !strings.HasPrefix(action, "theme:") {
		t.Fatalf("expected theme cycle action, got %s", action)
	}

	// Fork trajectory at current step with 'f'
	action, _ = ip.HandleKey("f")
	if !strings.HasPrefix(action, "forked:") || ip.LastForkedSession == nil {
		t.Fatalf("expected forked session, got %s", action)
	}
	if len(ip.LastForkedSession.Events) != 1 {
		t.Fatalf("expected forked session to contain 1 event, got %d", len(ip.LastForkedSession.Events))
	}
	if ip.ForkCount != 1 {
		t.Fatalf("expected ForkCount 1, got %d", ip.ForkCount)
	}

	// Quit with 'q'
	action, quit = ip.HandleKey("q")
	if !quit || action != "quit" {
		t.Fatalf("expected quit=true, got quit=%v action=%s", quit, action)
	}
}

func TestTUI_RunInteractive_ScriptedStream(t *testing.T) {
	session := &trace.Session{
		ID:        "scripted-session-42",
		AgentName: "auto-agent",
		Status:    "completed",
		CreatedAt: time.Now(),
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventLLMRequest, DurationMs: 120},
			{Index: 2, Type: trace.EventLLMResponse, DurationMs: 250},
		},
	}

	// Script: step next, fork, toggle help, quit
	inputScript := "j\nf\n?\nq\n"
	in := strings.NewReader(inputScript)
	var out bytes.Buffer

	ip := tui.NewInteractivePlayer(session, nil, in, &out)
	err := ip.RunInteractive()
	if err != nil {
		t.Fatalf("unexpected error running interactive debugger: %v", err)
	}

	outStr := out.String()
	if !strings.Contains(outStr, "[AgentVCR Interactive Debugger]") {
		t.Errorf("expected debugger header in output")
	}
	if !strings.Contains(outStr, "Keyboard Shortcuts (vi-mode)") {
		t.Errorf("expected help screen in output")
	}
	if !strings.Contains(outStr, "Quitting debugger.") {
		t.Errorf("expected quitting message in output")
	}
	if ip.ForkCount != 1 {
		t.Errorf("expected 1 fork created, got %d", ip.ForkCount)
	}
	if ip.LastForkedSession == nil || len(ip.LastForkedSession.Events) != 2 {
		t.Errorf("expected forked session with 2 events")
	}
}
