package a2a

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// Interceptor taps A2A envelopes into the VCR trace recorder.
type Interceptor struct {
	session *trace.Session
	seq     int
}

// NewInterceptor initializes a flight recorder interceptor for A2A communication.
func NewInterceptor(session *trace.Session) *Interceptor {
	if session == nil {
		session = &trace.Session{
			ID:        fmt.Sprintf("trace-a2a-%d", time.Now().Unix()),
			AgentName: "a2a-swarm-bus",
			CreatedAt: time.Now(),
			Metadata:  make(map[string]string),
		}
	}
	return &Interceptor{
		session: session,
		seq:     len(session.Events),
	}
}

// RecordEnvelope intercepts an incoming/outgoing A2A message and creates an atomic trace event.
func (i *Interceptor) RecordEnvelope(env MessageEnvelope) trace.TraceEvent {
	i.seq++

	payload := map[string]interface{}{
		"a2a_id":          env.ID,
		"conversation_id": env.ConversationID,
		"message_type":    string(env.Type),
		"sender_id":       env.SenderID,
		"recipient_id":    env.RecipientID,
		"reply_to_id":     env.ReplyToID,
		"hops":            env.Hops,
		"payload":         env.Payload,
		"checksum":        env.Checksum,
		"signature":       env.Signature,
	}

	payloadBytes, _ := json.Marshal(payload)
	prevChecksum := ""
	if len(i.session.Events) > 0 {
		prevChecksum = i.session.Events[len(i.session.Events)-1].Checksum
	}

	checksum := trace.ComputeChecksum(prevChecksum, i.seq, trace.EventStateChange, payloadBytes)

	event := trace.TraceEvent{
		ID:         fmt.Sprintf("evt-a2a-%d", i.seq),
		Index:      i.seq,
		Timestamp:  env.Timestamp,
		Type:       trace.EventStateChange,
		DurationMs: 1,
		Payload:    payload,
		Checksum:   checksum,
	}

	i.session.Events = append(i.session.Events, event)
	return event
}

// Session returns the underlying recorded trace session.
func (i *Interceptor) Session() *trace.Session {
	return i.session
}

// ANSI colors
const (
	ColorReset   = "\033[0m"
	ColorBold    = "\033[1m"
	ColorDim     = "\033[2m"
	ColorRed     = "\033[31m"
	ColorGreen   = "\033[32m"
	ColorYellow  = "\033[33m"
	ColorBlue    = "\033[34m"
	ColorMagenta = "\033[35m"
	ColorCyan    = "\033[36m"
	ColorWhite   = "\033[37m"
)

// RenderA2AScorecard renders a terminal scorecard visualizing inter-agent message bus activity.
func RenderA2AScorecard(report A2AAuditReport) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("%s%s========================================================================================%s\n", ColorBold, ColorCyan, ColorReset))
	sb.WriteString(fmt.Sprintf("%s%s  AGENTVCR // A2A AGENT PROTOCOL & SWARM MESSAGE BUS SCORECARD%s\n", ColorBold, ColorWhite, ColorReset))
	sb.WriteString(fmt.Sprintf("%s%s========================================================================================%s\n\n", ColorBold, ColorCyan, ColorReset))

	slaColor := ColorGreen
	if report.SLAPercent < 99.0 {
		slaColor = ColorYellow
	}
	if report.SLAPercent < 90.0 {
		slaColor = ColorRed
	}

	sb.WriteString(fmt.Sprintf("  %sDelivery SLA:%s        %s%s%.1f%%%s\n", ColorBold, ColorReset, ColorBold, slaColor, report.SLAPercent, ColorReset))
	sb.WriteString(fmt.Sprintf("  %sRegistered Agents:%s   %s%d%s\n", ColorBold, ColorReset, ColorCyan, report.RegisteredAgents, ColorReset))
	sb.WriteString(fmt.Sprintf("  %sTotal Wire Frames:%s   %d (Delivered: %s%d%s, Failed: %d)\n",
		ColorBold, ColorReset, report.TotalMessages, ColorGreen, report.DeliveredMessages, ColorReset, report.FailedDelegations))
	sb.WriteString(fmt.Sprintf("  %sActive Delegations:%s  %d\n", ColorBold, ColorReset, report.ActiveDelegations))
	sb.WriteString(fmt.Sprintf("  %sAverage Latency:%s     %.2f ms\n\n", ColorBold, ColorReset, report.AvgLatencyMs))

	// Registered Agent Directory
	sb.WriteString(fmt.Sprintf("  %s%sREGISTERED AGENT DIRECTORY:%s\n", ColorBold, ColorWhite, ColorReset))
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("  %-20s %-22s %-16s %s\n", "AGENT ID", "NAME", "ROLE", "CAPABILITIES"))
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────\n")
	for _, card := range report.AgentDirectory {
		capNames := make([]string, len(card.Capabilities))
		for i, c := range card.Capabilities {
			capNames[i] = c.Name
		}
		sb.WriteString(fmt.Sprintf("  %-20s %-22s %-16s %s\n",
			card.AgentID, card.Name, card.Role, strings.Join(capNames, ", ")))
	}
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────\n\n")

	// Inter-Agent Message Log
	sb.WriteString(fmt.Sprintf("  %s%sINTER-AGENT MESSAGE BUS JOURNAL:%s\n", ColorBold, ColorWhite, ColorReset))
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("  %-16s %-14s %-16s %-16s %-6s %s\n", "MSG ID", "TYPE", "SENDER", "RECIPIENT", "HOPS", "CHECKSUM"))
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────\n")
	for _, msg := range report.MessageLog {
		ck := msg.Checksum
		if len(ck) > 12 {
			ck = ck[:12] + "..."
		}
		typeColor := ColorYellow
		switch msg.Type {
		case TypeDelegate:
			typeColor = ColorCyan
		case TypeResult:
			typeColor = ColorGreen
		case TypeCancel, TypeInterrupt:
			typeColor = ColorRed
		case TypeHandshake:
			typeColor = ColorMagenta
		}
		sb.WriteString(fmt.Sprintf("  %-16s %s%-14s%s %-16s %-16s %-6d %s\n",
			msg.ID, typeColor, msg.Type, ColorReset, msg.SenderID, msg.RecipientID, msg.Hops, ck))
	}
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────\n\n")

	sb.WriteString(fmt.Sprintf("%s%s========================================================================================%s\n", ColorBold, ColorCyan, ColorReset))
	return sb.String()
}

// RenderA2AMermaid creates a Mermaid sequence diagram of the inter-agent communications.
func RenderA2AMermaid(report A2AAuditReport) string {
	var sb strings.Builder

	sb.WriteString("```mermaid\n")
	sb.WriteString("sequenceDiagram\n")
	sb.WriteString("    autonumber\n")

	for _, card := range report.AgentDirectory {
		cleanName := strings.ReplaceAll(card.Name, " ", "_")
		sb.WriteString(fmt.Sprintf("    participant %s as %s (%s)\n", card.AgentID, cleanName, card.Role))
	}
	sb.WriteString("\n")

	for _, msg := range report.MessageLog {
		arrow := "->>"
		if msg.Type == TypeResult {
			arrow = "-->>"
		}
		label := string(msg.Type)
		if goal, ok := msg.Payload["goal"].(string); ok && goal != "" {
			if len(goal) > 30 {
				goal = goal[:27] + "..."
			}
			label = fmt.Sprintf("%s: %s", msg.Type, goal)
		} else if status, ok := msg.Payload["status"].(string); ok && status != "" {
			label = fmt.Sprintf("Result [%s]", status)
		}
		sb.WriteString(fmt.Sprintf("    %s%s%s: %s\n", msg.SenderID, arrow, msg.RecipientID, label))
	}

	sb.WriteString("```\n")
	return sb.String()
}
