package lineage

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// ExtractLineage inspects an agent session trace and builds a microsecond-precision lineage graph.
func ExtractLineage(session *trace.Session) (*LineageGraph, error) {
	if session == nil {
		return nil, fmt.Errorf("session cannot be nil")
	}

	graph := &LineageGraph{
		SessionID:      session.ID,
		TotalSteps:     len(session.Events),
		Variables:      make(map[string]*VariableLineage),
		Timeline:       make([]StateMutation, 0),
		SandboxedState: make(map[string]interface{}),
	}

	baseTime := session.CreatedAt
	if baseTime.IsZero() && len(session.Events) > 0 {
		baseTime = session.Events[0].Timestamp
	}
	if baseTime.IsZero() {
		baseTime = time.Now()
	}

	var mutationSeq int64
	var lastOffset int64

	for _, event := range session.Events {
		eventTime := event.Timestamp
		if eventTime.IsZero() {
			eventTime = baseTime.Add(time.Duration(event.Index*10) * time.Millisecond)
		}

		offsetUs := eventTime.Sub(baseTime).Microseconds()
		if offsetUs < 0 {
			offsetUs = 0
		}
		if offsetUs <= lastOffset && len(graph.Timeline) > 0 {
			offsetUs = lastOffset + 1
		}
		lastOffset = offsetUs

		mutations := parseEventMutations(event, offsetUs, &mutationSeq, session.AgentName, graph.SandboxedState)
		for _, mut := range mutations {
			graph.applyMutation(mut)
		}
	}

	graph.TotalMutations = len(graph.Timeline)
	graph.UniqueKeys = len(graph.Variables)

	return graph, nil
}

// parseEventMutations extracts all state mutations from an individual TraceEvent.
func parseEventMutations(event trace.TraceEvent, baseOffsetUs int64, seq *int64, actor string, currentState map[string]interface{}) []StateMutation {
	var results []StateMutation
	payload := event.Payload
	if payload == nil {
		return results
	}

	actorName := actor
	if a, ok := payload["actor"].(string); ok && a != "" {
		actorName = a
	}

	// 1. Direct State Change Event
	if event.Type == trace.EventStateChange {
		// Single key/val direct specification
		if key, ok := payload["key"].(string); ok && key != "" {
			actionStr := "set"
			if act, ok := payload["action"].(string); ok && act != "" {
				actionStr = strings.ToLower(act)
			}
			mType := MutationSet
			switch actionStr {
			case "update", "modify":
				mType = MutationUpdate
			case "delete", "remove", "drop":
				mType = MutationDelete
			case "read", "get":
				mType = MutationRead
			}

			oldVal := payload["old_value"]
			if oldVal == nil && mType != MutationSet {
				oldVal = currentState[key]
			}
			newVal := payload["value"]

			*seq++
			results = append(results, StateMutation{
				MutationID:        fmt.Sprintf("mut-%05d", *seq),
				StepIndex:         event.Index,
				Timestamp:         event.Timestamp,
				MicrosecondOffset: baseOffsetUs + *seq%10,
				Key:               key,
				Type:              mType,
				OldValue:          oldVal,
				NewValue:          newVal,
				Source:            "state_change",
				EventID:           event.ID,
				Actor:             actorName,
				Dependencies:      extractDependencies(payload),
			})
		}

		// Multiple keys in "state", "diff", or "variables" map
		for _, mapKey := range []string{"state", "diff", "variables", "state_diff"} {
			if kvMap, ok := payload[mapKey].(map[string]interface{}); ok {
				for k, v := range kvMap {
					mType := MutationUpdate
					oldVal := currentState[k]
					if oldVal == nil {
						mType = MutationSet
					}
					*seq++
					results = append(results, StateMutation{
						MutationID:        fmt.Sprintf("mut-%05d", *seq),
						StepIndex:         event.Index,
						Timestamp:         event.Timestamp,
						MicrosecondOffset: baseOffsetUs + *seq%10,
						Key:               k,
						Type:              mType,
						OldValue:          oldVal,
						NewValue:          v,
						Source:            fmt.Sprintf("state_change:%s", mapKey),
						EventID:           event.ID,
						Actor:             actorName,
						Dependencies:      extractDependencies(payload),
					})
				}
			}
		}
	}

	// 2. Tool Calls mutating or reading state
	if event.Type == trace.EventToolCall {
		toolName := ""
		if tn, ok := payload["tool_name"].(string); ok {
			toolName = tn
		} else if tn, ok := payload["tool"].(string); ok {
			toolName = tn
		}

		args, _ := payload["args"].(map[string]interface{})
		if args != nil {
			isWriteTool := isStateWriteTool(toolName)
			isReadTool := isStateReadTool(toolName)

			if isWriteTool {
				targetKey := extractKeyFromArgs(args)
				targetVal := extractValFromArgs(args)
				if targetKey != "" {
					mType := MutationUpdate
					oldVal := currentState[targetKey]
					if oldVal == nil {
						mType = MutationSet
					}
					*seq++
					results = append(results, StateMutation{
						MutationID:        fmt.Sprintf("mut-%05d", *seq),
						StepIndex:         event.Index,
						Timestamp:         event.Timestamp,
						MicrosecondOffset: baseOffsetUs + *seq%10,
						Key:               targetKey,
						Type:              mType,
						OldValue:          oldVal,
						NewValue:          targetVal,
						Source:            fmt.Sprintf("tool:%s", toolName),
						EventID:           event.ID,
						Actor:             actorName,
						Dependencies:      extractDependencies(args),
					})
				}
			} else if isReadTool {
				targetKey := extractKeyFromArgs(args)
				if targetKey != "" {
					*seq++
					results = append(results, StateMutation{
						MutationID:        fmt.Sprintf("mut-%05d", *seq),
						StepIndex:         event.Index,
						Timestamp:         event.Timestamp,
						MicrosecondOffset: baseOffsetUs + *seq%10,
						Key:               targetKey,
						Type:              MutationRead,
						OldValue:          currentState[targetKey],
						NewValue:          currentState[targetKey],
						Source:            fmt.Sprintf("tool:%s", toolName),
						EventID:           event.ID,
						Actor:             actorName,
						Dependencies:      nil,
					})
				}
			}
		}
	}

	// 3. LLM Response Context & Memory Updates
	if event.Type == trace.EventLLMResponse {
		for _, memKey := range []string{"memory", "context_updates", "scratchpad_vars"} {
			if memMap, ok := payload[memKey].(map[string]interface{}); ok {
				for k, v := range memMap {
					mType := MutationUpdate
					oldVal := currentState[k]
					if oldVal == nil {
						mType = MutationSet
					}
					*seq++
					results = append(results, StateMutation{
						MutationID:        fmt.Sprintf("mut-%05d", *seq),
						StepIndex:         event.Index,
						Timestamp:         event.Timestamp,
						MicrosecondOffset: baseOffsetUs + *seq%10,
						Key:               k,
						Type:              mType,
						OldValue:          oldVal,
						NewValue:          v,
						Source:            fmt.Sprintf("llm:%s", memKey),
						EventID:           event.ID,
						Actor:             actorName,
					})
				}
			}
		}
	}

	return results
}

func isStateWriteTool(name string) bool {
	lower := strings.ToLower(name)
	keywords := []string{"set", "write", "save", "put", "store", "remember", "update", "modify", "record", "assign"}
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func isStateReadTool(name string) bool {
	lower := strings.ToLower(name)
	keywords := []string{"get", "read", "fetch", "load", "retrieve", "recall", "search_memory"}
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func extractKeyFromArgs(args map[string]interface{}) string {
	for _, k := range []string{"key", "name", "variable", "var", "target", "path", "prop"} {
		if val, ok := args[k].(string); ok && val != "" {
			return val
		}
	}
	return ""
}

func extractValFromArgs(args map[string]interface{}) interface{} {
	for _, k := range []string{"value", "val", "data", "content", "payload"} {
		if val, ok := args[k]; ok {
			return val
		}
	}
	return args
}

func extractDependencies(payload map[string]interface{}) []string {
	var deps []string
	if d, ok := payload["dependencies"].([]interface{}); ok {
		for _, dep := range d {
			if s, ok := dep.(string); ok {
				deps = append(deps, s)
			}
		}
	} else if d, ok := payload["dependencies"].([]string); ok {
		deps = append(deps, d...)
	}
	return deps
}

func (g *LineageGraph) applyMutation(m StateMutation) {
	g.Timeline = append(g.Timeline, m)

	v, exists := g.Variables[m.Key]
	if !exists {
		v = &VariableLineage{
			Key:            m.Key,
			InitialStep:    m.StepIndex,
			CreatedAt:      m.Timestamp,
			LastModifiedAt: m.Timestamp,
			History:        make([]StateMutation, 0),
			Dependencies:   make([]string, 0),
		}
		g.Variables[m.Key] = v
	}

	v.History = append(v.History, m)
	v.TotalMutations++
	v.LastModifiedAt = m.Timestamp

	for _, dep := range m.Dependencies {
		hasDep := false
		for _, existing := range v.Dependencies {
			if existing == dep {
				hasDep = true
				break
			}
		}
		if !hasDep {
			v.Dependencies = append(v.Dependencies, dep)
		}
	}

	switch m.Type {
	case MutationSet, MutationUpdate:
		v.WriteCount++
		v.CurrentValue = m.NewValue
		g.SandboxedState[m.Key] = m.NewValue
	case MutationDelete:
		v.WriteCount++
		v.CurrentValue = nil
		delete(g.SandboxedState, m.Key)
	case MutationRead:
		v.ReadCount++
	}
}

// StateAt reconstructs the deterministic ephemeral sandbox state at step n.
func (g *LineageGraph) StateAt(step int) map[string]interface{} {
	snapshot := make(map[string]interface{})
	for _, m := range g.Timeline {
		if m.StepIndex > step {
			break
		}
		switch m.Type {
		case MutationSet, MutationUpdate:
			snapshot[m.Key] = m.NewValue
		case MutationDelete:
			delete(snapshot, m.Key)
		}
	}
	return snapshot
}

// StateAtMicrosecond reconstructs point-in-time state by relative microsecond offset.
func (g *LineageGraph) StateAtMicrosecond(us int64) map[string]interface{} {
	snapshot := make(map[string]interface{})
	for _, m := range g.Timeline {
		if m.MicrosecondOffset > us {
			break
		}
		switch m.Type {
		case MutationSet, MutationUpdate:
			snapshot[m.Key] = m.NewValue
		case MutationDelete:
			delete(snapshot, m.Key)
		}
	}
	return snapshot
}

// GetVariableHistory retrieves the ancestral provenance of a single state variable.
func (g *LineageGraph) GetVariableHistory(key string) (*VariableLineage, error) {
	v, exists := g.Variables[key]
	if !exists {
		return nil, fmt.Errorf("variable '%s' not found in lineage graph", key)
	}
	return v, nil
}

// AnalyzeDivergenceRisks scans the mutation timeline for state anomalies and non-deterministic hazards.
func (g *LineageGraph) AnalyzeDivergenceRisks() []string {
	var risks []string
	seenKeys := make(map[string]bool)

	// Check uninitialized reads or premature deletes
	for _, m := range g.Timeline {
		if m.Type == MutationRead && !seenKeys[m.Key] {
			risks = append(risks, fmt.Sprintf("Uninitialized state read on '%s' at step %d (+%dµs)", m.Key, m.StepIndex, m.MicrosecondOffset))
		}
		if m.Type == MutationDelete && !seenKeys[m.Key] {
			risks = append(risks, fmt.Sprintf("Premature state deletion on '%s' at step %d (+%dµs) before declaration", m.Key, m.StepIndex, m.MicrosecondOffset))
		}
		if m.Type == MutationSet || m.Type == MutationUpdate {
			seenKeys[m.Key] = true
		}
	}

	// Check rapid state thrashing (rapid writes to same key within < 50µs)
	for i := 1; i < len(g.Timeline); i++ {
		prev := g.Timeline[i-1]
		curr := g.Timeline[i]
		if prev.Key == curr.Key && (curr.Type == MutationUpdate || curr.Type == MutationSet) && (prev.Type == MutationUpdate || prev.Type == MutationSet) {
			diff := curr.MicrosecondOffset - prev.MicrosecondOffset
			if diff >= 0 && diff < 50 {
				risks = append(risks, fmt.Sprintf("Rapid state thrashing on '%s' (%dµs interval between step %d and %d)", curr.Key, diff, prev.StepIndex, curr.StepIndex))
			}
		}
	}

	// Check for high mutation count indicating instability
	for k, v := range g.Variables {
		if v.WriteCount > 10 {
			risks = append(risks, fmt.Sprintf("High mutation frequency on '%s' (%d writes across %d steps)", k, v.WriteCount, g.TotalSteps))
		}
	}

	return risks
}

// GenerateLineageReport builds a full audit report for CLI or programmatic inspection.
func GenerateLineageReport(session *trace.Session, filterKey string, targetStep int) (*LineageReport, error) {
	graph, err := ExtractLineage(session)
	if err != nil {
		return nil, err
	}

	report := &LineageReport{
		SessionID:       session.ID,
		GeneratedAt:     time.Now(),
		Graph:           graph,
		FilterKey:       filterKey,
		TargetStep:      targetStep,
		DivergenceRisks: graph.AnalyzeDivergenceRisks(),
	}

	if targetStep >= 0 {
		report.SnapshotAtStep = graph.StateAt(targetStep)
	} else {
		report.SnapshotAtStep = graph.SandboxedState
	}

	report.EphemeralSandboxValid = len(report.DivergenceRisks) == 0

	return report, nil
}

// RenderLineageScorecard generates a clean ANSI terminal visualization of the lineage graph.
func RenderLineageScorecard(report *LineageReport) string {
	var sb strings.Builder

	sb.WriteString("\n\033[1;36m=================================================================================\033[0m\n")
	sb.WriteString(fmt.Sprintf("\033[1;37m   AGENTVCR MICROSECOND STATE LINEAGE GRAPH & EPHEMERAL SANDBOX\033[0m\n"))
	sb.WriteString("\033[1;36m=================================================================================\033[0m\n")

	sb.WriteString(fmt.Sprintf("  \033[1;33mSession ID:\033[0m          %s\n", report.SessionID))
	sb.WriteString(fmt.Sprintf("  \033[1;33mTotal Trajectory:\033[0m   %d steps\n", report.Graph.TotalSteps))
	sb.WriteString(fmt.Sprintf("  \033[1;33mState Mutations:\033[0m    %d operations\n", report.Graph.TotalMutations))
	sb.WriteString(fmt.Sprintf("  \033[1;33mUnique Variables:\033[0m   %d tracked keys\n", report.Graph.UniqueKeys))

	if report.EphemeralSandboxValid {
		sb.WriteString("  \033[1;33mSandbox Status:\033[0m     \033[1;32m[PASS] DETERMINISTICALLY REPRODUCIBLE (0 RISKS)\033[0m\n")
	} else {
		sb.WriteString(fmt.Sprintf("  \033[1;33mSandbox Status:\033[0m     \033[1;31m[WARN] %d DIVERGENCE HAZARDS DETECTED\033[0m\n", len(report.DivergenceRisks)))
	}
	sb.WriteString("\033[1;34m---------------------------------------------------------------------------------\033[0m\n")

	// Render Variable Provenance Summary
	sb.WriteString("\033[1;37m  STATE VARIABLE PROVENANCE TABLE:\033[0m\n")
	sb.WriteString("  \033[1;30mKEY                  FIRST-STEP  WRITES  READS   LATEST VALUE\033[0m\n")

	var sortedKeys []string
	for k := range report.Graph.Variables {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)

	for _, k := range sortedKeys {
		v := report.Graph.Variables[k]
		valBytes, _ := json.Marshal(v.CurrentValue)
		valStr := string(valBytes)
		if len(valStr) > 30 {
			valStr = valStr[:27] + "..."
		}
		sb.WriteString(fmt.Sprintf("  %-20s Step %-5d %-7d %-7d %s\n", k, v.InitialStep, v.WriteCount, v.ReadCount, valStr))
	}

	sb.WriteString("\033[1;34m---------------------------------------------------------------------------------\033[0m\n")

	// Render Key Inspection or Timeline
	if report.FilterKey != "" {
		v, exists := report.Graph.Variables[report.FilterKey]
		if !exists {
			sb.WriteString(fmt.Sprintf("  \033[1;31mKey '%s' has no mutations recorded.\033[0m\n", report.FilterKey))
		} else {
			sb.WriteString(fmt.Sprintf("\033[1;37m  LINEAGE HISTORY FOR VARIABLE: \033[1;33m%s\033[0m\n", report.FilterKey))
			for i, m := range v.History {
				prefix := "├──"
				if i == len(v.History)-1 {
					prefix = "└──"
				}
				sb.WriteString(fmt.Sprintf("  %s [+%6dµs] Step %-2d [%-6s] %v -> %v (src: %s)\n",
					prefix, m.MicrosecondOffset, m.StepIndex, m.Type, m.OldValue, m.NewValue, m.Source))
			}
		}
		sb.WriteString("\033[1;34m---------------------------------------------------------------------------------\033[0m\n")
	} else {
		// Render Microsecond Mutation Timeline (first 10 items)
		sb.WriteString("\033[1;37m  MICROSECOND MUTATION TIMELINE TRACE:\033[0m\n")
		limit := len(report.Graph.Timeline)
		if limit > 8 {
			limit = 8
		}
		for i := 0; i < limit; i++ {
			m := report.Graph.Timeline[i]
			sb.WriteString(fmt.Sprintf("  [+%-7dµs] Step %-2d | %-6s | \033[1;32m%-15s\033[0m = %v (src: %s)\n",
				m.MicrosecondOffset, m.StepIndex, m.Type, m.Key, m.NewValue, m.Source))
		}
		if len(report.Graph.Timeline) > limit {
			sb.WriteString(fmt.Sprintf("  ... [%d additional microsecond mutations hidden]\n", len(report.Graph.Timeline)-limit))
		}
		sb.WriteString("\033[1;34m---------------------------------------------------------------------------------\033[0m\n")
	}

	// Render Point-in-time Snapshot
	stepLabel := "FINAL EPHEMERAL SANDBOX STATE"
	if report.TargetStep >= 0 {
		stepLabel = fmt.Sprintf("TIME-TRAVEL SANDBOX STATE AT STEP %d", report.TargetStep)
	}
	sb.WriteString(fmt.Sprintf("\033[1;37m  %s:\033[0m\n", stepLabel))
	if len(report.SnapshotAtStep) == 0 {
		sb.WriteString("  [Empty State Sandbox]\n")
	} else {
		var snapKeys []string
		for sk := range report.SnapshotAtStep {
			snapKeys = append(snapKeys, sk)
		}
		sort.Strings(snapKeys)
		for _, sk := range snapKeys {
			vBytes, _ := json.Marshal(report.SnapshotAtStep[sk])
			sb.WriteString(fmt.Sprintf("  \033[1;36m%-20s\033[0m : %s\n", sk, string(vBytes)))
		}
	}

	// Render Divergence Hazards if any
	if len(report.DivergenceRisks) > 0 {
		sb.WriteString("\033[1;34m---------------------------------------------------------------------------------\033[0m\n")
		sb.WriteString("\033[1;31m  DETECTED STATE DIVERGENCE HAZARDS:\033[0m\n")
		for _, risk := range report.DivergenceRisks {
			sb.WriteString(fmt.Sprintf("  \033[1;31m[!] %s\033[0m\n", risk))
		}
	}

	sb.WriteString("\033[1;36m=================================================================================\033[0m\n\n")

	return sb.String()
}
