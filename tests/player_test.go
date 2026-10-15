package tests

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
	"github.com/Noooblien/agentvcr/pkg/tui"
)

func TestPlayer_RenderEventCard(t *testing.T) {
	ev := trace.TraceEvent{
		Index:      1,
		Timestamp:  time.Now(),
		Type:       trace.EventToolCall,
		DurationMs: 42,
		Payload: map[string]interface{}{
			"tool_name": "search_code",
			"query":     "func main",
		},
	}

	var buf bytes.Buffer
	player := tui.NewPlayer(&buf)
	card := player.RenderEventCard(ev, 5)

	if !strings.Contains(card, "TOOL DISPATCH") {
		t.Errorf("expected card to contain TOOL DISPATCH, got: %s", card)
	}
	if !strings.Contains(card, "search_code") {
		t.Errorf("expected card to contain tool name search_code, got: %s", card)
	}
	if !strings.Contains(card, "Step 1/5") {
		t.Errorf("expected card to contain progress step 1/5, got: %s", card)
	}
}

func TestPlayer_PlaySession(t *testing.T) {
	session := &trace.Session{
		ID:        "play-session-1",
		AgentName: "code-assistant",
		Status:    "completed",
		Events: []trace.TraceEvent{
			{
				Index:      1,
				Timestamp:  time.Now(),
				Type:       trace.EventLLMRequest,
				DurationMs: 10,
				Payload: map[string]interface{}{
					"model": "gpt-4o",
				},
			},
			{
				Index:      2,
				Timestamp:  time.Now(),
				Type:       trace.EventLLMResponse,
				DurationMs: 350,
				Payload: map[string]interface{}{
					"content": "Hello! How can I help you?",
				},
			},
		},
	}

	var buf bytes.Buffer
	player := tui.NewPlayer(&buf)
	player.PlaySession(session, 0)

	output := buf.String()
	if !strings.Contains(output, "Replaying Trajectory: play-session-1") {
		t.Errorf("expected header with session ID, got: %s", output)
	}
	if !strings.Contains(output, "Playback completed.") {
		t.Errorf("expected completion message, got: %s", output)
	}
}
