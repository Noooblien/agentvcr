package flamegraph

import (
	"fmt"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// GenerateDemoSession creates a realistic multi-turn agent execution trace for testing and demos.
func GenerateDemoSession(scenario string) *trace.Session {
	now := time.Now().UTC().Add(-10 * time.Minute)

	switch scenario {
	case "research":
		return createResearchScenario(now)
	case "runaway":
		return createRunawayScenario(now)
	default: // "swe"
		return createSWEScenario(now)
	}
}

func createSWEScenario(start time.Time) *trace.Session {
	t := start
	events := make([]trace.TraceEvent, 0)
	idx := 0

	addEvent := func(eventType trace.EventType, durMs int64, payload map[string]interface{}) {
		events = append(events, trace.TraceEvent{
			ID:         fmt.Sprintf("ev_%d", idx),
			Index:      idx,
			Timestamp:  t,
			Type:       eventType,
			DurationMs: durMs,
			Payload:    payload,
			Checksum:   fmt.Sprintf("sha256_%d", idx),
		})
		idx++
		t = t.Add(time.Duration(durMs) * time.Millisecond)
	}

	// Turn 1: Inspect Repository
	addEvent(trace.EventLLMRequest, 30, map[string]interface{}{
		"model":         "claude-3-5-sonnet",
		"prompt_tokens": 1250,
	})
	addEvent(trace.EventLLMResponse, 1850, map[string]interface{}{
		"model":             "claude-3-5-sonnet",
		"completion_tokens": 280,
	})
	addEvent(trace.EventToolCall, 20, map[string]interface{}{
		"tool":    "search_code",
		"call_id": "call_1",
		"query":   "func TestReplay",
	})
	addEvent(trace.EventToolResult, 420, map[string]interface{}{
		"call_id": "call_1",
		"results": 3,
	})

	// Turn 2: Run Full Integration Test Suite (Known Latency Sink)
	addEvent(trace.EventLLMRequest, 40, map[string]interface{}{
		"model":         "claude-3-5-sonnet",
		"prompt_tokens": 2100,
	})
	addEvent(trace.EventLLMResponse, 1620, map[string]interface{}{
		"model":             "claude-3-5-sonnet",
		"completion_tokens": 210,
	})
	addEvent(trace.EventToolCall, 15, map[string]interface{}{
		"tool":    "bash_exec",
		"call_id": "call_2",
		"cmd":     "go test -v ./...",
	})
	// Heavy tool execution: 6450ms!
	addEvent(trace.EventToolResult, 6450, map[string]interface{}{
		"call_id": "call_2",
		"exit":    1,
		"error":   true,
	})
	addEvent(trace.EventStateChange, 25, map[string]interface{}{
		"action": "snapshot_workspace",
	})

	// Turn 3: Apply Targeted Patch
	addEvent(trace.EventLLMRequest, 50, map[string]interface{}{
		"model":         "claude-3-5-sonnet",
		"prompt_tokens": 3400,
	})
	addEvent(trace.EventLLMResponse, 2400, map[string]interface{}{
		"model":             "claude-3-5-sonnet",
		"completion_tokens": 580,
	})
	addEvent(trace.EventToolCall, 15, map[string]interface{}{
		"tool":    "replace_file_content",
		"call_id": "call_3",
		"file":    "pkg/replay/engine.go",
	})
	addEvent(trace.EventToolResult, 180, map[string]interface{}{
		"call_id": "call_3",
		"status":  "ok",
	})

	// Turn 4: Verify Fix
	addEvent(trace.EventLLMRequest, 50, map[string]interface{}{
		"model":         "claude-3-5-sonnet",
		"prompt_tokens": 4200,
	})
	addEvent(trace.EventLLMResponse, 1400, map[string]interface{}{
		"model":             "claude-3-5-sonnet",
		"completion_tokens": 140,
	})
	addEvent(trace.EventToolCall, 15, map[string]interface{}{
		"tool":    "bash_exec",
		"call_id": "call_4",
		"cmd":     "go test -run TestReplay ./...",
	})
	addEvent(trace.EventToolResult, 890, map[string]interface{}{
		"call_id": "call_4",
		"exit":    0,
	})

	return &trace.Session{
		ID:        "trace_swe_agent_01",
		AgentName: "SWE-Bench Autonomous Debugger",
		CreatedAt: start,
		UpdatedAt: t,
		Events:    events,
		Status:    "completed",
	}
}

func createResearchScenario(start time.Time) *trace.Session {
	t := start
	events := make([]trace.TraceEvent, 0)
	idx := 0

	addEvent := func(eventType trace.EventType, durMs int64, payload map[string]interface{}) {
		events = append(events, trace.TraceEvent{
			ID:         fmt.Sprintf("ev_%d", idx),
			Index:      idx,
			Timestamp:  t,
			Type:       eventType,
			DurationMs: durMs,
			Payload:    payload,
			Checksum:   fmt.Sprintf("sha256_%d", idx),
		})
		idx++
		t = t.Add(time.Duration(durMs) * time.Millisecond)
	}

	// Turn 1: Massive context ingestion (Token Sink)
	addEvent(trace.EventLLMRequest, 80, map[string]interface{}{
		"model":         "gpt-4o",
		"prompt_tokens": 18500, // Massive prompt context!
	})
	addEvent(trace.EventLLMResponse, 3200, map[string]interface{}{
		"model":             "gpt-4o",
		"completion_tokens": 1200,
	})
	addEvent(trace.EventToolCall, 20, map[string]interface{}{
		"tool":    "web_search",
		"call_id": "call_web_1",
		"query":   "quantum key distribution mcp specs",
	})
	addEvent(trace.EventToolResult, 1150, map[string]interface{}{
		"call_id": "call_web_1",
		"sources": 5,
	})

	// Turn 2: Synthesis
	addEvent(trace.EventLLMRequest, 90, map[string]interface{}{
		"model":         "gpt-4o",
		"prompt_tokens": 21500,
	})
	addEvent(trace.EventLLMResponse, 2800, map[string]interface{}{
		"model":             "gpt-4o",
		"completion_tokens": 950,
	})

	return &trace.Session{
		ID:        "trace_deep_research_01",
		AgentName: "Autonomous Market & Tech Researcher",
		CreatedAt: start,
		UpdatedAt: t,
		Events:    events,
		Status:    "completed",
	}
}

func createRunawayScenario(start time.Time) *trace.Session {
	t := start
	events := make([]trace.TraceEvent, 0)
	idx := 0

	addEvent := func(eventType trace.EventType, durMs int64, payload map[string]interface{}) {
		events = append(events, trace.TraceEvent{
			ID:         fmt.Sprintf("ev_%d", idx),
			Index:      idx,
			Timestamp:  t,
			Type:       eventType,
			DurationMs: durMs,
			Payload:    payload,
			Checksum:   fmt.Sprintf("sha256_%d", idx),
		})
		idx++
		t = t.Add(time.Duration(durMs) * time.Millisecond)
	}

	for turn := 1; turn <= 3; turn++ {
		addEvent(trace.EventLLMRequest, 30, map[string]interface{}{
			"model":         "claude-3-haiku",
			"prompt_tokens": 1000 * turn,
		})
		addEvent(trace.EventLLMResponse, 800, map[string]interface{}{
			"model":             "claude-3-haiku",
			"completion_tokens": 120,
		})
		addEvent(trace.EventToolCall, 15, map[string]interface{}{
			"tool":    "failing_api",
			"call_id": fmt.Sprintf("call_fail_%d", turn),
		})
		addEvent(trace.EventToolResult, 2100, map[string]interface{}{
			"call_id": fmt.Sprintf("call_fail_%d", turn),
			"error":   true,
		})
		addEvent(trace.EventError, 50, map[string]interface{}{
			"error": "HTTP 504 Gateway Timeout",
		})
	}

	return &trace.Session{
		ID:        "trace_runaway_loop_01",
		AgentName: "Faulted Flaky Worker",
		CreatedAt: start,
		UpdatedAt: t,
		Events:    events,
		Status:    "failed",
	}
}

// RunDemoSuite executes the multi-scenario profiling demo and outputs formatted results.
func RunDemoSuite(format ExportFormat, metric string) (string, error) {
	session := GenerateDemoSession("swe")
	profile, err := BuildProfile(session)
	if err != nil {
		return "", err
	}

	switch format {
	case FormatSpeedscope:
		bytes, err := ExportSpeedscope(profile)
		return string(bytes), err
	case FormatChrome:
		bytes, err := ExportChromeTrace(profile)
		return string(bytes), err
	case FormatMermaid:
		return RenderMermaidGantt(profile), nil
	case FormatMarkdown:
		return RenderMarkdownReport(profile), nil
	default:
		return RenderTerminalFlame(profile, metric), nil
	}
}
