package flamegraph

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ANSI color codes for terminal rendering.
const (
	colorReset     = "\033[0m"
	colorBold      = "\033[1m"
	colorDim       = "\033[2m"
	colorRed       = "\033[31m"
	colorGreen     = "\033[32m"
	colorYellow    = "\033[33m"
	colorBlue      = "\033[34m"
	colorMagenta   = "\033[35m"
	colorCyan      = "\033[36m"
	colorWhite     = "\033[37m"
	colorBgRed     = "\033[41m"
	colorBgYellow  = "\033[43m"
	colorBgBlue    = "\033[44m"
)

// RenderTerminalFlame renders a full ANSI hierarchical execution flamegraph into a string.
func RenderTerminalFlame(profile *FlameProfile, metric string) string {
	if profile == nil || profile.Root == nil {
		return "No flame profile available."
	}

	var sb strings.Builder

	sb.WriteString("\n" + colorBold + colorCyan + "==========================================================================================" + colorReset + "\n")
	sb.WriteString(colorBold + "  AGENTVCR EXECUTION FLAMEGRAPH & CRITICAL PATH PROFILER" + colorReset + "\n")
	sb.WriteString(colorDim + fmt.Sprintf("  Session: %s  |  Agent: %s  |  Turns: %d  |  Tools: %d",
		profile.SessionID, profile.AgentName, profile.TotalTurns, profile.TotalToolsCalled) + colorReset + "\n")
	sb.WriteString(colorBold + colorCyan + "==========================================================================================" + colorReset + "\n\n")

	// Overview summary box
	sb.WriteString(fmt.Sprintf("  "+colorBold+"Wall-Clock Time:"+colorReset+" %s  |  "+
		colorBold+"Total Tokens:"+colorReset+" %d (%d in / %d out)  |  "+
		colorBold+"Est. Cost:"+colorReset+" $%.4f\n\n",
		formatDuration(profile.TotalDurationMs),
		profile.TotalTokens,
		profile.TotalPromptTokens,
		profile.TotalCompletionTokens,
		profile.TotalCostUSD,
	))

	// Render Category Breakdown
	sb.WriteString(colorBold + "--- Resource Breakdown by Operation Category ---" + colorReset + "\n")
	sb.WriteString(fmt.Sprintf("  %-22s %-12s %-10s %-12s %-10s %s\n",
		"Category", "Duration", "Wall %", "Tokens", "Tok %", "Ops"))
	sb.WriteString("  -----------------------------------------------------------------------\n")

	categories := []SpanCategory{
		CategoryInference,
		CategoryTool,
		CategoryPrompt,
		CategoryIO,
		CategoryOrchestration,
	}

	for _, cat := range categories {
		stat, ok := profile.CategoryBreakdown[cat]
		if !ok || stat.Count == 0 {
			continue
		}
		sb.WriteString(fmt.Sprintf("  %-22s %-12s %6.1f%%    %-12d %6.1f%%    %d\n",
			catName(cat),
			formatDuration(stat.DurationMs),
			stat.DurationPct,
			stat.Tokens,
			stat.TokensPct,
			stat.Count,
		))
	}
	sb.WriteString("\n")

	// Render Flame Tree
	sb.WriteString(colorBold + "--- Hierarchical Execution Flame Tree ---" + colorReset + "\n")
	if strings.ToLower(metric) == "tokens" {
		sb.WriteString(colorDim + "  (Rendering prioritized by token distribution)" + colorReset + "\n")
	} else {
		sb.WriteString(colorDim + "  (Rendering prioritized by wall-clock latency & critical path)" + colorReset + "\n")
	}

	renderTreeNode(&sb, profile.Root, "", true, profile.TotalDurationMs, profile.TotalTokens, metric)
	sb.WriteString("\n")

	// Render Critical Path Summary
	sb.WriteString(colorBold + colorYellow + "--- Critical Latency Path Chain ---" + colorReset + "\n")
	if len(profile.CriticalPath) == 0 {
		sb.WriteString("  None identified.\n")
	} else {
		for i, cp := range profile.CriticalPath {
			arrow := "└──>"
			if i == 0 {
				arrow = " ● "
			}
			durPct := 0.0
			if profile.TotalDurationMs > 0 {
				durPct = (float64(cp.DurationMs) / float64(profile.TotalDurationMs)) * 100.0
			}
			sb.WriteString(fmt.Sprintf("  %s %s [%s, %4.1f%% wall-clock, offset: +%dms]\n",
				arrow, cp.Name, formatDuration(cp.DurationMs), durPct, cp.StartOffsetMs))
		}
	}
	sb.WriteString("\n")

	// Render Hotspots
	if len(profile.Hotspots) > 0 {
		sb.WriteString(colorBold + colorRed + "--- Identified Bottlenecks & Execution Hotspots ---" + colorReset + "\n")
		for idx, h := range profile.Hotspots {
			sb.WriteString(fmt.Sprintf("  [%d] %s%s%s (%s: %.1f%%)\n      ↳ %s\n",
				idx+1, colorBold, h.Name, colorReset, h.Metric, h.Percentage, h.Description))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func renderTreeNode(sb *strings.Builder, s *Span, prefix string, isLast bool, totalDur int64, totalTok int, metric string) {
	if s == nil {
		return
	}

	connector := "├── "
	if isLast {
		connector = "└── "
	}

	durPct := 0.0
	if totalDur > 0 {
		durPct = (float64(s.DurationMs) / float64(totalDur)) * 100.0
	}

	tokPct := 0.0
	if totalTok > 0 && s.TotalTokens > 0 {
		tokPct = (float64(s.TotalTokens) / float64(totalTok)) * 100.0
	}

	displayPct := durPct
	if strings.ToLower(metric) == "tokens" {
		displayPct = tokPct
	}

	critBadge := ""
	if s.IsCriticalPath && s.Category != CategorySession {
		critBadge = " " + colorBold + colorRed + "[CRITICAL PATH]" + colorReset
	}

	// Bar visualization (up to 12 chars)
	barLen := int((displayPct / 100.0) * 12.0)
	if barLen == 0 && displayPct > 1.0 {
		barLen = 1
	}
	bar := strings.Repeat("█", barLen) + strings.Repeat("░", 12-barLen)

	catColor := colorWhite
	switch s.Category {
	case CategoryInference:
		catColor = colorMagenta
	case CategoryTool:
		catColor = colorGreen
	case CategoryPrompt:
		catColor = colorBlue
	case CategoryIO:
		catColor = colorCyan
	case CategoryOrchestration:
		catColor = colorYellow
	}

	var metricStr string
	if strings.ToLower(metric) == "tokens" {
		metricStr = fmt.Sprintf("%5.1f%% tok | %d tok | %s", tokPct, s.TotalTokens, formatDuration(s.DurationMs))
	} else {
		metricStr = fmt.Sprintf("%5.1f%% wall | %s | %d tok", durPct, formatDuration(s.DurationMs), s.TotalTokens)
	}

	line := fmt.Sprintf("%s%s%s[%s]%s %s %s(%s)%s%s\n",
		prefix,
		connector,
		catColor,
		s.Name,
		colorReset,
		colorDim+bar+colorReset,
		colorBold,
		metricStr,
		colorReset,
		critBadge,
	)
	sb.WriteString(line)

	childPrefix := prefix
	if isLast {
		childPrefix += "    "
	} else {
		childPrefix += "│   "
	}

	for i, child := range s.Children {
		lastChild := i == len(s.Children)-1
		renderTreeNode(sb, child, childPrefix, lastChild, totalDur, totalTok, metric)
	}
}

// ExportSpeedscope formats the profile according to speedscope v0.1.2 JSON specification.
func ExportSpeedscope(profile *FlameProfile) ([]byte, error) {
	type Frame struct {
		Name string `json:"name"`
		File string `json:"file,omitempty"`
	}

	type SpeedscopeEvent struct {
		Type  string `json:"type"` // "O" for open, "C" for close
		Frame int    `json:"frame"`
		At    int64  `json:"at"`
	}

	type SpeedscopeProfile struct {
		Type       string            `json:"type"`
		Name       string            `json:"name"`
		Unit       string            `json:"unit"`
		StartValue int64             `json:"startValue"`
		EndValue   int64             `json:"endValue"`
		Events     []SpeedscopeEvent `json:"events"`
	}

	type SpeedscopeRoot struct {
		Schema             string              `json:"$schema"`
		ActiveProfileIndex int                 `json:"activeProfileIndex"`
		Exporter           string              `json:"exporter"`
		Name               string              `json:"name"`
		Shared             map[string][]Frame  `json:"shared"`
		Profiles           []SpeedscopeProfile `json:"profiles"`
	}

	frames := make([]Frame, 0)
	frameIndex := make(map[string]int)

	getFrame := func(name string) int {
		if idx, ok := frameIndex[name]; ok {
			return idx
		}
		idx := len(frames)
		frames = append(frames, Frame{Name: name})
		frameIndex[name] = idx
		return idx
	}

	events := make([]SpeedscopeEvent, 0)

	var walkEvents func(s *Span)
	walkEvents = func(s *Span) {
		fID := getFrame(s.Name)
		events = append(events, SpeedscopeEvent{
			Type:  "O",
			Frame: fID,
			At:    s.StartOffsetMs,
		})

		for _, c := range s.Children {
			walkEvents(c)
		}

		events = append(events, SpeedscopeEvent{
			Type:  "C",
			Frame: fID,
			At:    s.StartOffsetMs + s.DurationMs,
		})
	}

	if profile.Root != nil {
		walkEvents(profile.Root)
	}

	rootDoc := SpeedscopeRoot{
		Schema:             "https://www.speedscope.app/file-format-schema.json",
		ActiveProfileIndex: 0,
		Exporter:           "AgentVCR Execution Flamegraph v1.0",
		Name:               fmt.Sprintf("%s (%s)", profile.AgentName, profile.SessionID),
		Shared:             map[string][]Frame{"frames": frames},
		Profiles: []SpeedscopeProfile{
			{
				Type:       "evented",
				Name:       "Execution Wall-Clock Time",
				Unit:       "milliseconds",
				StartValue: 0,
				EndValue:   profile.TotalDurationMs,
				Events:     events,
			},
		},
	}

	return json.MarshalIndent(rootDoc, "", "  ")
}

// ExportChromeTrace converts the execution profile to standard Chrome Tracing / Perfetto format.
func ExportChromeTrace(profile *FlameProfile) ([]byte, error) {
	type ChromeTraceEvent struct {
		Name string                 `json:"name"`
		Cat  string                 `json:"cat"`
		Ph   string                 `json:"ph"` // "X" for Complete Event
		Ts   int64                  `json:"ts"` // Microseconds
		Dur  int64                  `json:"dur"`
		Pid  int                    `json:"pid"`
		Tid  int                    `json:"tid"`
		Args map[string]interface{} `json:"args,omitempty"`
	}

	events := make([]ChromeTraceEvent, 0)

	var walk func(s *Span)
	walk = func(s *Span) {
		args := make(map[string]interface{})
		for k, v := range s.Metadata {
			args[k] = v
		}
		args["tokens"] = s.TotalTokens
		args["critical_path"] = s.IsCriticalPath

		events = append(events, ChromeTraceEvent{
			Name: s.Name,
			Cat:  string(s.Category),
			Ph:   "X",
			Ts:   s.StartOffsetMs * 1000,
			Dur:  s.DurationMs * 1000,
			Pid:  1,
			Tid:  s.StepIndex,
			Args: args,
		})

		for _, c := range s.Children {
			walk(c)
		}
	}

	if profile.Root != nil {
		walk(profile.Root)
	}

	return json.MarshalIndent(events, "", "  ")
}

// RenderMermaidGantt outputs a Mermaid Gantt diagram capturing timeline spans.
func RenderMermaidGantt(profile *FlameProfile) string {
	if profile == nil || profile.Root == nil {
		return "```mermaid\ngantt\n    title No Profile\n```"
	}

	var sb strings.Builder
	sb.WriteString("```mermaid\ngantt\n")
	sb.WriteString(fmt.Sprintf("    title Trajectory Execution Timeline: %s (%s)\n", profile.AgentName, profile.SessionID))
	sb.WriteString("    dateFormat X\n")
	sb.WriteString("    axisFormat %s ms\n\n")

	for _, turn := range profile.Root.Children {
		sb.WriteString(fmt.Sprintf("    section Turn %d\n", turn.StepIndex))
		for _, op := range turn.Children {
			cleanName := strings.ReplaceAll(op.Name, ":", "-")
			crit := ""
			if op.IsCriticalPath {
				crit = "crit, "
			}
			start := op.StartOffsetMs
			end := start + op.DurationMs
			sb.WriteString(fmt.Sprintf("    %s :%sdone, %d, %d\n", cleanName, crit, start, end))
		}
	}

	sb.WriteString("```\n")
	return sb.String()
}

// RenderMarkdownReport generates a GitHub Flavored Markdown summary report.
func RenderMarkdownReport(profile *FlameProfile) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# AgentVCR Execution Flamegraph Report: `%s`\n\n", profile.SessionID))
	sb.WriteString(fmt.Sprintf("- **Agent Name:** `%s`\n", profile.AgentName))
	sb.WriteString(fmt.Sprintf("- **Total Wall-Clock Time:** `%s` (%d ms)\n", formatDuration(profile.TotalDurationMs), profile.TotalDurationMs))
	sb.WriteString(fmt.Sprintf("- **Total Token Consumption:** `%d tokens` (Prompt: %d, Completion: %d)\n",
		profile.TotalTokens, profile.TotalPromptTokens, profile.TotalCompletionTokens))
	sb.WriteString(fmt.Sprintf("- **Estimated Trajectory Cost:** `$%.4f`\n", profile.TotalCostUSD))
	sb.WriteString(fmt.Sprintf("- **Conversational Turns:** `%d`\n", profile.TotalTurns))
	sb.WriteString(fmt.Sprintf("- **Tools Intercepted:** `%d`\n\n", profile.TotalToolsCalled))

	sb.WriteString("## Category Resource Allocation\n\n")
	sb.WriteString("| Operation Category | Wall-Clock Duration | Duration % | Token Consumption | Token % | Total Operations |\n")
	sb.WriteString("| :--- | :--- | :--- | :--- | :--- | :--- |\n")

	categories := []SpanCategory{
		CategoryInference,
		CategoryTool,
		CategoryPrompt,
		CategoryIO,
		CategoryOrchestration,
	}

	for _, cat := range categories {
		stat, ok := profile.CategoryBreakdown[cat]
		if !ok || stat.Count == 0 {
			continue
		}
		sb.WriteString(fmt.Sprintf("| **%s** | `%s` | `%.1f%%` | `%d` | `%.1f%%` | `%d` |\n",
			catName(cat), formatDuration(stat.DurationMs), stat.DurationPct, stat.Tokens, stat.TokensPct, stat.Count))
	}
	sb.WriteString("\n")

	if len(profile.Hotspots) > 0 {
		sb.WriteString("## Execution Bottlenecks & Hotspots\n\n")
		sb.WriteString("| Priority | Hotspot Target | Category | Metric | Consumption | Description |\n")
		sb.WriteString("| :---: | :--- | :--- | :---: | :---: | :--- |\n")
		for idx, h := range profile.Hotspots {
			sb.WriteString(fmt.Sprintf("| #%d | **%s** | `%s` | `%s` | `%.1f%%` | %s |\n",
				idx+1, h.Name, h.Category, h.Metric, h.Percentage, h.Description))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Execution Gantt Timeline\n\n")
	sb.WriteString(RenderMermaidGantt(profile))

	return sb.String()
}

func formatDuration(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	sec := float64(ms) / 1000.0
	if sec < 60.0 {
		return fmt.Sprintf("%.2fs", sec)
	}
	min := int(sec / 60.0)
	remSec := sec - float64(min*60)
	return fmt.Sprintf("%dm%.1fs", min, remSec)
}

func catName(c SpanCategory) string {
	switch c {
	case CategoryInference:
		return "LLM Inference"
	case CategoryTool:
		return "Tool Execution"
	case CategoryPrompt:
		return "Prompt Context"
	case CategoryIO:
		return "State Snapshot & CoW"
	case CategoryOrchestration:
		return "Orchestration & Error"
	default:
		return string(c)
	}
}
