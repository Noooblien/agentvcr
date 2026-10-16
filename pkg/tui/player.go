package tui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// Player renders animated or buffered step-by-step playback of an AgentVCR trajectory.
type Player struct {
	Out   io.Writer
	Theme Theme
}

// NewPlayer creates a new terminal trajectory player writing to the provided io.Writer.
func NewPlayer(out io.Writer) *Player {
	return &Player{
		Out:   out,
		Theme: DefaultTheme(),
	}
}

// SetTheme configures the color palette for rendering.
func (p *Player) SetTheme(theme Theme) {
	p.Theme = theme
}

// SetThemeByName sets theme by lookup in global registry.
func (p *Player) SetThemeByName(name string) {
	p.Theme = GetTheme(name)
}

// RenderEventCard formats a single trajectory event as a styled terminal card with current theme.
func (p *Player) RenderEventCard(ev trace.TraceEvent, totalSteps int) string {
	var b strings.Builder

	icon := "🔹"
	color := p.Theme.Border
	typeLabel := string(ev.Type)

	switch ev.Type {
	case trace.EventLLMRequest:
		icon = "🧠"
		color = p.Theme.Accent
		typeLabel = "LLM INVOCATION REQUEST"
	case trace.EventLLMResponse:
		icon = "💬"
		color = p.Theme.Success
		typeLabel = "LLM COMPLETION RESPONSE"
	case trace.EventToolCall:
		icon = "🛠️ "
		color = p.Theme.Warning
		typeLabel = "TOOL DISPATCH"
	case trace.EventToolResult:
		icon = "📦"
		color = p.Theme.Info
		typeLabel = "TOOL RESULT CAPTURE"
	case trace.EventError:
		icon = "🚨"
		color = p.Theme.Error
		typeLabel = "RUNTIME EXCEPTION"
	}

	progress := renderProgressBar(ev.Index, totalSteps, 16)

	b.WriteString(fmt.Sprintf("%s┌── %s %s[%s] %s(Step %d/%d) %s%s\n",
		color, icon, Bold, typeLabel, p.Theme.DimText, ev.Index, totalSteps, progress, Reset))
	b.WriteString(fmt.Sprintf("%s│%s  Timestamp: %s%s | Duration: %d ms\n",
		color, p.Theme.Text, ev.Timestamp.Format("15:04:05.000"), p.Theme.DimText, ev.DurationMs))

	payloadBytes, _ := json.MarshalIndent(ev.Payload, "", "  ")
	lines := strings.Split(string(payloadBytes), "\n")
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			b.WriteString(fmt.Sprintf("%s│%s   %s\n", color, p.Theme.Text, l))
		}
	}

	b.WriteString(fmt.Sprintf("%s└─────────────────────────────────────────────────────────────%s\n", color, Reset))
	return b.String()
}

// PlaySession outputs trajectory events sequentially to the terminal.
func (p *Player) PlaySession(session *trace.Session, delay time.Duration) {
	fmt.Fprintf(p.Out, "\n%s%s▶ [AgentVCR Player] Replaying Trajectory: %s%s (Theme: %s)\n",
		Bold, p.Theme.Header, session.ID, Reset, p.Theme.Name)
	fmt.Fprintf(p.Out, "   Agent: %s | Events: %d | Status: %s\n\n", session.AgentName, len(session.Events), session.Status)

	for _, ev := range session.Events {
		card := p.RenderEventCard(ev, len(session.Events))
		fmt.Fprint(p.Out, card)
		if delay > 0 {
			time.Sleep(delay)
		}
	}

	fmt.Fprintf(p.Out, "%s%s✔ [AgentVCR Player] Playback completed.%s\n\n", Bold, p.Theme.Success, Reset)
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

// InteractivePlayer coordinates step-by-step interactive vi-keybinding playback and branching.
type InteractivePlayer struct {
	Session           *trace.Session
	CurrentIndex      int
	Player            *Player
	In                io.Reader
	Out               io.Writer
	ForkCount         int
	LastForkedSession *trace.Session
	ShowHelp          bool
	themeIndex        int
	availableThemes   []string
}

// NewInteractivePlayer initializes an interactive vi debugger for a session.
func NewInteractivePlayer(session *trace.Session, player *Player, in io.Reader, out io.Writer) *InteractivePlayer {
	if player == nil {
		player = NewPlayer(out)
	}
	themes := []string{"default", "monokai", "dracula", "gruvbox", "nord"}
	return &InteractivePlayer{
		Session:         session,
		CurrentIndex:    0,
		Player:          player,
		In:              in,
		Out:             out,
		availableThemes: themes,
	}
}

// HandleKey processes a single vi or terminal key command.
func (ip *InteractivePlayer) HandleKey(rawKey string) (action string, shouldQuit bool) {
	key := strings.TrimSpace(rawKey)

	switch key {
	case "j", "n", "down":
		if ip.CurrentIndex < len(ip.Session.Events)-1 {
			ip.CurrentIndex++
			return fmt.Sprintf("step_forward:%d", ip.CurrentIndex), false
		}
		return "at_end", false

	case "k", "p", "up":
		if ip.CurrentIndex > 0 {
			ip.CurrentIndex--
			return fmt.Sprintf("step_backward:%d", ip.CurrentIndex), false
		}
		return "at_start", false

	case "g":
		ip.CurrentIndex = 0
		return "jump_start", false

	case "G":
		if len(ip.Session.Events) > 0 {
			ip.CurrentIndex = len(ip.Session.Events) - 1
		}
		return "jump_end", false

	case "f":
		// Fork trajectory at current step
		forked := ip.ForkAtCurrentStep()
		ip.LastForkedSession = forked
		ip.ForkCount++
		return fmt.Sprintf("forked:%s", forked.ID), false

	case "t":
		// Cycle theme
		ip.themeIndex = (ip.themeIndex + 1) % len(ip.availableThemes)
		nextTheme := ip.availableThemes[ip.themeIndex]
		ip.Player.SetThemeByName(nextTheme)
		return fmt.Sprintf("theme:%s", nextTheme), false

	case "?", "h":
		ip.ShowHelp = !ip.ShowHelp
		return "toggle_help", false

	case "q", "exit", "quit":
		return "quit", true

	default:
		return "unknown_command", false
	}
}

// ForkAtCurrentStep clones the session up to the current event index into a new execution fork.
func (ip *InteractivePlayer) ForkAtCurrentStep() *trace.Session {
	if ip.Session == nil || len(ip.Session.Events) == 0 {
		return nil
	}

	limit := ip.CurrentIndex + 1
	if limit > len(ip.Session.Events) {
		limit = len(ip.Session.Events)
	}

	forkedEvents := make([]trace.TraceEvent, limit)
	copy(forkedEvents, ip.Session.Events[:limit])

	forkedID := fmt.Sprintf("%s-fork-step%d-%d", ip.Session.ID, limit, time.Now().UnixNano()%10000)
	return &trace.Session{
		ID:        forkedID,
		AgentName: ip.Session.AgentName,
		Status:    "forked",
		CreatedAt: ip.Session.CreatedAt,
		UpdatedAt: time.Now(),
		ParentID:   ip.Session.ID,
		ForkAtStep: limit,
		Events:     forkedEvents,
		Metadata: map[string]string{
			"parent_session_id": ip.Session.ID,
			"fork_step_index":   fmt.Sprintf("%d", limit),
		},
	}
}

// RenderHeader generates the interactive status bar.
func (ip *InteractivePlayer) RenderHeader() string {
	t := ip.Player.Theme
	total := len(ip.Session.Events)
	cur := ip.CurrentIndex + 1
	if total == 0 {
		cur = 0
	}
	return fmt.Sprintf("%s%s[AgentVCR Interactive Debugger] %sSession: %s | Step: %d/%d | Theme: %s%s\n",
		Bold, t.Header, Reset, ip.Session.ID, cur, total, t.Name, Reset)
}

// RenderHelp returns vi keybinding instructions.
func (ip *InteractivePlayer) RenderHelp() string {
	t := ip.Player.Theme
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s┌── Keyboard Shortcuts (vi-mode) ─────────────────────────────%s\n", t.Border, Reset))
	b.WriteString(fmt.Sprintf("%s│%s  [j/n/↓] Next step      | [k/p/↑] Previous step\n", t.Border, Reset))
	b.WriteString(fmt.Sprintf("%s│%s  [g]     Jump to start  | [G]     Jump to end\n", t.Border, Reset))
	b.WriteString(fmt.Sprintf("%s│%s  [f]     Fork at step   | [t]     Cycle color theme\n", t.Border, Reset))
	b.WriteString(fmt.Sprintf("%s│%s  [?/h]   Toggle help    | [q]     Quit debugger\n", t.Border, Reset))
	b.WriteString(fmt.Sprintf("%s└─────────────────────────────────────────────────────────────%s\n", t.Border, Reset))
	return b.String()
}

// RenderCurrentStep renders header, current event card, help (if toggled), and interactive prompt.
func (ip *InteractivePlayer) RenderCurrentStep() string {
	var b strings.Builder
	b.WriteString(ip.RenderHeader())

	if ip.ShowHelp {
		b.WriteString(ip.RenderHelp())
	}

	if len(ip.Session.Events) > 0 && ip.CurrentIndex < len(ip.Session.Events) {
		ev := ip.Session.Events[ip.CurrentIndex]
		card := ip.Player.RenderEventCard(ev, len(ip.Session.Events))
		b.WriteString(card)
	} else {
		b.WriteString(fmt.Sprintf("%s(No events recorded in session)%s\n", ip.Player.Theme.DimText, Reset))
	}

	b.WriteString(fmt.Sprintf("%s[vi: j/k=step, f=fork, t=theme, ?=help, q=quit] > %s",
		ip.Player.Theme.Prompt, Reset))
	return b.String()
}

// RunInteractive executes the interactive command loop until 'q' or EOF.
func (ip *InteractivePlayer) RunInteractive() error {
	scanner := bufio.NewScanner(ip.In)

	fmt.Fprint(ip.Out, ip.RenderCurrentStep())
	for scanner.Scan() {
		line := scanner.Text()
		action, shouldQuit := ip.HandleKey(line)
		if shouldQuit {
			fmt.Fprintf(ip.Out, "\n%s%sQuitting debugger.%s\n", Bold, ip.Player.Theme.Success, Reset)
			return nil
		}
		if action == "unknown_command" && line != "" {
			fmt.Fprintf(ip.Out, "%sUnknown key '%s'. Press ? for help.%s\n", ip.Player.Theme.Warning, line, Reset)
		}
		fmt.Fprint(ip.Out, ip.RenderCurrentStep())
	}

	return scanner.Err()
}
