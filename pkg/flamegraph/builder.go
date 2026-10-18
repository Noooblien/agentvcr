package flamegraph

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// PricingRates defines per-million token pricing for cost estimation.
type PricingRates struct {
	PromptPerMillion     float64
	CompletionPerMillion float64
}

// DefaultPricing returns standard rates (approximating Sonnet 3.5 / GPT-4o: $3/M in, $15/M out).
var DefaultPricing = map[string]PricingRates{
	"default":             {PromptPerMillion: 3.0, CompletionPerMillion: 15.0},
	"claude-3-5-sonnet":   {PromptPerMillion: 3.0, CompletionPerMillion: 15.0},
	"gpt-4o":              {PromptPerMillion: 2.5, CompletionPerMillion: 10.0},
	"claude-3-haiku":      {PromptPerMillion: 0.25, CompletionPerMillion: 1.25},
	"gpt-4o-mini":         {PromptPerMillion: 0.15, CompletionPerMillion: 0.60},
}

// BuildProfile analyzes a recorded trace session and compiles a complete FlameProfile.
func BuildProfile(session *trace.Session) (*FlameProfile, error) {
	if session == nil {
		return nil, fmt.Errorf("session cannot be nil")
	}

	profile := &FlameProfile{
		SessionID:         session.ID,
		AgentName:         session.AgentName,
		CreatedAt:         session.CreatedAt,
		CategoryBreakdown: make(map[SpanCategory]CategoryStat),
	}

	root := &Span{
		ID:       fmt.Sprintf("session_%s", session.ID),
		Name:     fmt.Sprintf("Session: %s (%s)", session.AgentName, session.ID),
		Category: CategorySession,
		Metadata: make(map[string]interface{}),
		Children: make([]*Span, 0),
	}

	if len(session.Events) == 0 {
		profile.Root = root
		return profile, nil
	}

	baseTime := session.Events[0].Timestamp

	// Group events into turns / steps
	turns := groupEventsByTurn(session.Events)
	profile.TotalTurns = len(turns)

	var sessionDurationMs int64
	var sessionPromptTokens int
	var sessionCompletionTokens int
	var sessionCostUSD float64
	var toolCount int

	for turnIdx, turnEvents := range turns {
		turnSpan := buildTurnSpan(turnIdx+1, turnEvents, baseTime, &toolCount)
		root.Children = append(root.Children, turnSpan)

		sessionPromptTokens += turnSpan.PromptTokens
		sessionCompletionTokens += turnSpan.CompletionTokens
		sessionCostUSD += turnSpan.CostUSD
	}

	// Calculate overall duration from earliest to latest event or sum of turns
	lastEvent := session.Events[len(session.Events)-1]
	sessionDurationMs = lastEvent.Timestamp.Sub(baseTime).Milliseconds() + lastEvent.DurationMs
	if sessionDurationMs <= 0 {
		var sumTurns int64
		for _, c := range root.Children {
			sumTurns += c.DurationMs
		}
		sessionDurationMs = sumTurns
	}

	root.DurationMs = sessionDurationMs
	root.PromptTokens = sessionPromptTokens
	root.CompletionTokens = sessionCompletionTokens
	root.TotalTokens = sessionPromptTokens + sessionCompletionTokens
	root.CostUSD = sessionCostUSD

	profile.TotalDurationMs = sessionDurationMs
	profile.TotalPromptTokens = sessionPromptTokens
	profile.TotalCompletionTokens = sessionCompletionTokens
	profile.TotalTokens = root.TotalTokens
	profile.TotalCostUSD = sessionCostUSD
	profile.TotalToolsCalled = toolCount
	profile.Root = root

	// Calculate self metrics and categories recursively
	computeSelfMetrics(root)
	aggregateCategories(root, profile.CategoryBreakdown, sessionDurationMs, profile.TotalTokens)

	// Identify Critical Path and Hotspots
	identifyCriticalPath(root)
	profile.CriticalPath = extractCriticalPathSpans(root)
	profile.Hotspots = detectHotspots(root, sessionDurationMs, profile.TotalTokens)

	return profile, nil
}

// groupEventsByTurn segments a flat sequence of trace events into distinct conversational turns.
func groupEventsByTurn(events []trace.TraceEvent) [][]trace.TraceEvent {
	var turns [][]trace.TraceEvent
	var currentTurn []trace.TraceEvent

	for _, ev := range events {
		// New turn typically begins with an LLM request
		if ev.Type == trace.EventLLMRequest && len(currentTurn) > 0 {
			turns = append(turns, currentTurn)
			currentTurn = nil
		}
		currentTurn = append(currentTurn, ev)
	}

	if len(currentTurn) > 0 {
		turns = append(turns, currentTurn)
	}

	return turns
}

// buildTurnSpan constructs a turn-level span and its nested child operations.
func buildTurnSpan(turnNum int, events []trace.TraceEvent, baseTime time.Time, toolCounter *int) *Span {
	turnStart := events[0].Timestamp
	turnOffset := turnStart.Sub(baseTime).Milliseconds()
	if turnOffset < 0 {
		turnOffset = 0
	}

	var turnDuration int64
	lastEv := events[len(events)-1]
	turnDuration = lastEv.Timestamp.Sub(turnStart).Milliseconds() + lastEv.DurationMs
	if turnDuration <= 0 {
		turnDuration = 50 // Minimum nominal duration
	}

	turnSpan := &Span{
		ID:            fmt.Sprintf("turn_%d", turnNum),
		Name:          fmt.Sprintf("Turn %d", turnNum),
		Category:      CategoryTurn,
		StepIndex:     turnNum,
		StartOffsetMs: turnOffset,
		DurationMs:    turnDuration,
		Metadata:      make(map[string]interface{}),
		Children:      make([]*Span, 0),
	}

	// Match tool calls with corresponding tool results
	pendingTools := make(map[string]*Span)

	for _, ev := range events {
		evOffset := ev.Timestamp.Sub(baseTime).Milliseconds()

		switch ev.Type {
		case trace.EventLLMRequest:
			promptTokens := extractTokens(ev.Payload, "prompt_tokens")
			if promptTokens == 0 {
				promptTokens = estimateTokensFromPayload(ev.Payload)
			}
			promptSpan := &Span{
				ID:            fmt.Sprintf("prompt_%d_%d", turnNum, ev.Index),
				Name:          "Prompt Assembly & Context Ingestion",
				Category:      CategoryPrompt,
				StepIndex:     ev.Index,
				StartOffsetMs: evOffset,
				DurationMs:    ev.DurationMs,
				PromptTokens:  promptTokens,
				TotalTokens:   promptTokens,
				Metadata:      map[string]interface{}{"event_id": ev.ID},
			}
			if promptSpan.DurationMs <= 0 {
				promptSpan.DurationMs = 20
			}
			turnSpan.Children = append(turnSpan.Children, promptSpan)
			turnSpan.PromptTokens += promptTokens

		case trace.EventLLMResponse:
			compTokens := extractTokens(ev.Payload, "completion_tokens")
			if compTokens == 0 {
				compTokens = estimateTokensFromPayload(ev.Payload)
			}
			model := extractString(ev.Payload, "model")
			if model == "" {
				model = "claude-3-5-sonnet"
			}
			infDuration := ev.DurationMs
			if infDuration <= 0 {
				infDuration = 450
			}

			rates := getPricing(model)
			cost := (float64(turnSpan.PromptTokens)*rates.PromptPerMillion + float64(compTokens)*rates.CompletionPerMillion) / 1_000_000.0

			infSpan := &Span{
				ID:               fmt.Sprintf("inference_%d_%d", turnNum, ev.Index),
				Name:             fmt.Sprintf("LLM Inference (%s)", model),
				Category:         CategoryInference,
				StepIndex:        ev.Index,
				StartOffsetMs:    evOffset,
				DurationMs:       infDuration,
				CompletionTokens: compTokens,
				TotalTokens:      compTokens,
				CostUSD:          cost,
				Metadata: map[string]interface{}{
					"model":    model,
					"event_id": ev.ID,
				},
			}
			turnSpan.Children = append(turnSpan.Children, infSpan)
			turnSpan.CompletionTokens += compTokens
			turnSpan.CostUSD += cost

		case trace.EventToolCall:
			*toolCounter++
			toolName := extractString(ev.Payload, "tool")
			if toolName == "" {
				toolName = extractString(ev.Payload, "name")
			}
			if toolName == "" {
				toolName = "generic_tool"
			}
			callID := extractString(ev.Payload, "call_id")
			if callID == "" {
				callID = fmt.Sprintf("call_%d_%d", turnNum, ev.Index)
			}

			toolSpan := &Span{
				ID:            fmt.Sprintf("tool_%s_%d", toolName, ev.Index),
				Name:          fmt.Sprintf("Tool: %s", toolName),
				Category:      CategoryTool,
				StepIndex:     ev.Index,
				StartOffsetMs: evOffset,
				DurationMs:    ev.DurationMs,
				Metadata: map[string]interface{}{
					"tool_name": toolName,
					"call_id":   callID,
				},
			}
			if toolSpan.DurationMs <= 0 {
				toolSpan.DurationMs = 120
			}
			pendingTools[callID] = toolSpan
			turnSpan.Children = append(turnSpan.Children, toolSpan)

		case trace.EventToolResult:
			callID := extractString(ev.Payload, "call_id")
			if target, ok := pendingTools[callID]; ok {
				if ev.DurationMs > 0 {
					target.DurationMs = ev.DurationMs
				} else {
					target.DurationMs = ev.Timestamp.Sub(turnStart).Milliseconds() - (target.StartOffsetMs - turnOffset)
					if target.DurationMs <= 0 {
						target.DurationMs = 150
					}
				}
				if isErr, ok := ev.Payload["error"].(bool); ok && isErr {
					target.Metadata["has_error"] = true
				}
			}

		case trace.EventStateChange:
			stateSpan := &Span{
				ID:            fmt.Sprintf("state_%d_%d", turnNum, ev.Index),
				Name:          "State Snapshot & CoW Delta",
				Category:      CategoryIO,
				StepIndex:     ev.Index,
				StartOffsetMs: evOffset,
				DurationMs:    ev.DurationMs,
				Metadata:      map[string]interface{}{"event_id": ev.ID},
			}
			if stateSpan.DurationMs <= 0 {
				stateSpan.DurationMs = 15
			}
			turnSpan.Children = append(turnSpan.Children, stateSpan)

		case trace.EventError:
			errSpan := &Span{
				ID:            fmt.Sprintf("error_%d_%d", turnNum, ev.Index),
				Name:          "Trajectory Error & Recovery",
				Category:      CategoryOrchestration,
				StepIndex:     ev.Index,
				StartOffsetMs: evOffset,
				DurationMs:    ev.DurationMs,
				Metadata:      map[string]interface{}{"error": extractString(ev.Payload, "error")},
			}
			if errSpan.DurationMs <= 0 {
				errSpan.DurationMs = 30
			}
			turnSpan.Children = append(turnSpan.Children, errSpan)
		}
	}

	// Compute turn duration as sum of children if measured duration was too small
	var childrenSum int64
	for _, c := range turnSpan.Children {
		childrenSum += c.DurationMs
	}
	if childrenSum > turnSpan.DurationMs {
		turnSpan.DurationMs = childrenSum
	}
	turnSpan.TotalTokens = turnSpan.PromptTokens + turnSpan.CompletionTokens

	return turnSpan
}

// computeSelfMetrics sets SelfDurationMs and SelfTokens for each node.
func computeSelfMetrics(span *Span) {
	var childrenDuration int64
	var childrenTokens int

	for _, child := range span.Children {
		computeSelfMetrics(child)
		childrenDuration += child.DurationMs
		childrenTokens += child.TotalTokens
	}

	span.SelfDurationMs = span.DurationMs - childrenDuration
	if span.SelfDurationMs < 0 {
		span.SelfDurationMs = 0
	}

	span.SelfTokens = span.TotalTokens - childrenTokens
	if span.SelfTokens < 0 {
		span.SelfTokens = 0
	}
}

// aggregateCategories calculates totals per category.
func aggregateCategories(span *Span, breakdown map[SpanCategory]CategoryStat, totalDuration int64, totalTokens int) {
	// Only count leaf or meaningful component categories, avoiding double counting root/turn wrappers
	if span.Category != CategorySession && span.Category != CategoryTurn {
		stat := breakdown[span.Category]
		stat.Category = span.Category
		stat.DurationMs += span.DurationMs
		stat.Tokens += span.TotalTokens
		stat.Count++
		breakdown[span.Category] = stat
	}

	for _, child := range span.Children {
		aggregateCategories(child, breakdown, totalDuration, totalTokens)
	}

	if span.Category == CategorySession {
		for cat, stat := range breakdown {
			if totalDuration > 0 {
				stat.DurationPct = (float64(stat.DurationMs) / float64(totalDuration)) * 100.0
			}
			if totalTokens > 0 {
				stat.TokensPct = (float64(stat.Tokens) / float64(totalTokens)) * 100.0
			}
			breakdown[cat] = stat
		}
	}
}

// identifyCriticalPath finds the critical latency path through sequential turns and operations.
func identifyCriticalPath(root *Span) {
	root.IsCriticalPath = true

	for _, turn := range root.Children {
		turn.IsCriticalPath = true

		// Within a turn, find the dominant operation (usually inference or longest tool)
		var maxChild *Span
		var maxDur int64
		for _, op := range turn.Children {
			if op.DurationMs > maxDur {
				maxDur = op.DurationMs
				maxChild = op
			}
		}
		if maxChild != nil {
			maxChild.IsCriticalPath = true
		}
	}
}

// extractCriticalPathSpans returns an ordered list of spans flagged on the critical path.
func extractCriticalPathSpans(root *Span) []*Span {
	var path []*Span
	var walk func(s *Span)
	walk = func(s *Span) {
		if s.IsCriticalPath {
			path = append(path, s)
		}
		for _, c := range s.Children {
			walk(c)
		}
	}
	walk(root)
	return path
}

// detectHotspots identifies operations that exceed 15% of total time or 25% of tokens.
func detectHotspots(root *Span, totalDuration int64, totalTokens int) []Hotspot {
	var hotspots []Hotspot

	var walk func(s *Span)
	walk = func(s *Span) {
		if s.Category != CategorySession && s.Category != CategoryTurn {
			if totalDuration > 0 {
				pctDur := (float64(s.DurationMs) / float64(totalDuration)) * 100.0
				if pctDur >= 15.0 {
					hotspots = append(hotspots, Hotspot{
						SpanID:      s.ID,
						Name:        s.Name,
						Category:    s.Category,
						Metric:      "latency",
						Value:       float64(s.DurationMs),
						Percentage:  pctDur,
						Description: fmt.Sprintf("Consumes %.1f%% of trajectory wall-clock time (%d ms)", pctDur, s.DurationMs),
					})
				}
			}

			if totalTokens > 0 && s.TotalTokens > 0 {
				pctTok := (float64(s.TotalTokens) / float64(totalTokens)) * 100.0
				if pctTok >= 25.0 {
					hotspots = append(hotspots, Hotspot{
						SpanID:      s.ID,
						Name:        s.Name,
						Category:    s.Category,
						Metric:      "tokens",
						Value:       float64(s.TotalTokens),
						Percentage:  pctTok,
						Description: fmt.Sprintf("Consumes %.1f%% of trajectory token budget (%d tokens)", pctTok, s.TotalTokens),
					})
				}
			}
		}

		for _, c := range s.Children {
			walk(c)
		}
	}

	walk(root)

	// Sort hotspots descending by percentage
	sort.Slice(hotspots, func(i, j int) bool {
		return hotspots[i].Percentage > hotspots[j].Percentage
	})

	return hotspots
}

func extractTokens(payload map[string]interface{}, key string) int {
	if payload == nil {
		return 0
	}
	if v, ok := payload[key]; ok {
		if f, ok := v.(float64); ok {
			return int(f)
		}
		if i, ok := v.(int); ok {
			return i
		}
	}
	if usage, ok := payload["usage"].(map[string]interface{}); ok {
		if v, ok := usage[key]; ok {
			if f, ok := v.(float64); ok {
				return int(f)
			}
			if i, ok := v.(int); ok {
				return i
			}
		}
	}
	return 0
}

func extractString(payload map[string]interface{}, key string) string {
	if payload == nil {
		return ""
	}
	if v, ok := payload[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func estimateTokensFromPayload(payload map[string]interface{}) int {
	if payload == nil {
		return 0
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return 0
	}
	tok := len(raw) / 4
	if tok == 0 && len(raw) > 0 {
		return 1
	}
	return tok
}

func getPricing(model string) PricingRates {
	model = strings.ToLower(model)
	for k, rates := range DefaultPricing {
		if strings.Contains(model, k) {
			return rates
		}
	}
	return DefaultPricing["default"]
}
