package tui

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// ANSI color escape codes for terminal rendering.
const (
	Reset   = "\033[0m"
	Bold    = "\033[1m"
	Dim     = "\033[2m"
	Red     = "\033[31m"
	Green   = "\033[32m"
	Yellow  = "\033[33m"
	Blue    = "\033[34m"
	Magenta = "\033[35m"
	Cyan    = "\033[36m"
)

// Player renders animated or buffered step-by-step playback of an AgentVCR trajectory.
type Player struct {
	Out io.Writer
}

// NewPlayer creates a new terminal trajectory player writing to the provided io.Writer.
func NewPlayer(out io.Writer) *Player {
	return &Player{Out: out}
}

// RenderEventCard formats a single trajectory event as a styled terminal card.
func (p *Player) RenderEventCard(ev trace.TraceEvent, totalSteps int) string {
	var b strings.Builder

	icon := "🔹"
	color := Cyan
	typeLabel := string(ev.Type)

	switch ev.Type {
	case trace.EventLLMRequest:
		icon = "🧠"
		color = Magenta
		typeLabel = "LLM INVOCATION REQUEST"
	case trace.EventLLMResponse:
		icon = "💬"
		color = Green
		typeLabel = "LLM COMPLETION RESPONSE"
	case trace.EventToolCall:
		icon = "🛠️ "
		color = Yellow
		typeLabel = "TOOL DISPATCH"
	case trace.EventToolResult:
		icon = "📦"
		color = Blue
		typeLabel = "TOOL RESULT CAPTURE"
	case trace.EventError:
		icon = "🚨"
		color = Red
		typeLabel = "RUNTIME EXCEPTION"
	}

	progress := renderProgressBar(ev.Index, totalSteps, 16)

	b.WriteString(fmt.Sprintf("%s┌── %s %s[%s] %s(Step %d/%d) %s%s\n",
		color, icon, Bold, typeLabel, Dim, ev.Index, totalSteps, progress, Reset))
	b.WriteString(fmt.Sprintf("%s│%s  Timestamp: %s%s | Duration: %d ms\n",
		color, Reset, ev.Timestamp.Format("15:04:05.000"), Dim, ev.DurationMs))

	payloadBytes, _ := json.MarshalIndent(ev.Payload, "", "  ")
	lines := strings.Split(string(payloadBytes), "\n")
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			b.WriteString(fmt.Sprintf("%s│%s   %s\n", color, Reset, l))
		}
	}

	b.WriteString(fmt.Sprintf("%s└─────────────────────────────────────────────────────────────%s\n", color, Reset))
	return b.String()
}

// PlaySession outputs trajectory events sequentially to the terminal.
func (p *Player) PlaySession(session *trace.Session, delay time.Duration) {
	fmt.Fprintf(p.Out, "\n%s%s▶ [AgentVCR Player] Replaying Trajectory: %s%s\n", Bold, Cyan, session.ID, Reset)
	fmt.Fprintf(p.Out, "   Agent: %s | Events: %d | Status: %s\n\n", session.AgentName, len(session.Events), session.Status)

	for _, ev := range session.Events {
		card := p.RenderEventCard(ev, len(session.Events))
		fmt.Fprint(p.Out, card)
		if delay > 0 {
			time.Sleep(delay)
		}
	}

	fmt.Fprintf(p.Out, "%s%s✔ [AgentVCR Player] Playback completed.%s\n\n", Bold, Green, Reset)
}

func renderProgressBar(current, total, width int) string {
	if total <= 0 {
		return ""
	}
	ratio := float64(current) / float64(total)
	if ratio > 1.0 {
		ratio = 1.0
	}
	filled := int(ratio * float64(width))
	empty := width - filled

	return fmt.Sprintf("[%s%s]", strings.Repeat("=", filled), strings.Repeat("-", empty))
}
