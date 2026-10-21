package counterfactual

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

const (
	// Estimated standard pricing per 1,000 tokens for cost modeling ($0.005 / 1k average blended).
	CostPer1kTokens = 0.005
)

// Simulator executes counterfactual interventions on recorded agent trajectories.
type Simulator struct{}

// NewSimulator creates a counterfactual trajectory simulator.
func NewSimulator() *Simulator {
	return &Simulator{}
}

// Simulate executes the what-if mutation on the baseline session and computes full divergence metrics.
func (s *Simulator) Simulate(baseline *trace.Session, mut CounterfactualMutation) (*trace.Session, *BranchComparison, error) {
	if baseline == nil {
		return nil, nil, fmt.Errorf("baseline session cannot be nil")
	}

	branchID := fmt.Sprintf("%s_whatif_%s", baseline.ID, mut.ID)
	now := time.Now().UTC()

	branch := &trace.Session{
		ID:         branchID,
		AgentName:  baseline.AgentName,
		CreatedAt:  now,
		UpdatedAt:  now,
		ParentID:   baseline.ID,
		ForkAtStep: mut.BranchStep,
		Metadata: map[string]string{
			"counterfactual_id":   mut.ID,
			"counterfactual_type": string(mut.Type),
			"mutation_step":       fmt.Sprintf("%d", mut.BranchStep),
		},
		Status: "completed",
		Events: make([]trace.TraceEvent, 0),
	}

	// 1. Copy events prior to branch step
	targetIdx := mut.BranchStep - 1
	if targetIdx < 0 {
		targetIdx = 0
	}
	if targetIdx > len(baseline.Events) {
		targetIdx = len(baseline.Events)
	}

	for i := 0; i < targetIdx; i++ {
		branch.Events = append(branch.Events, baseline.Events[i])
	}

	// 2. Synthesize or inject the counterfactual mutation at branch step
	var branchEvents []trace.TraceEvent
	switch mut.Type {
	case MutateToolSubstitution:
		branchEvents = s.synthesizeToolSubstitution(baseline, mut)
	case MutateArgumentOverride:
		branchEvents = s.synthesizeArgumentOverride(baseline, mut)
	case MutateResponseMock:
		branchEvents = s.synthesizeResponseMock(baseline, mut)
	case MutateSteeringPrompt:
		branchEvents = s.synthesizeSteeringPrompt(baseline, mut)
	default:
		return nil, nil, fmt.Errorf("unsupported mutation type: %s", mut.Type)
	}

	for _, ev := range branchEvents {
		ev.Index = len(branch.Events) + 1
		ev.Checksum = computeEventChecksum(ev)
		branch.Events = append(branch.Events, ev)
	}

	// 3. Compute telemetry, tokens, latency, and outcome comparison
	comp := s.compareTrajectories(baseline, branch, mut)
	return branch, comp, nil
}

func (s *Simulator) synthesizeToolSubstitution(baseline *trace.Session, mut CounterfactualMutation) []trace.TraceEvent {
	var events []trace.TraceEvent
	now := time.Now().UTC()

	// Intercepted call to replacement tool
	callPayload := map[string]interface{}{
		"tool":      mut.ReplacementTool,
		"arguments": mut.ArgumentOverrides,
		"substituted_from": mut.TargetTool,
		"tokens":    45,
	}
	events = append(events, trace.TraceEvent{
		ID:         fmt.Sprintf("evt-cf-subcall-%d", now.UnixNano()),
		Timestamp:  now,
		Type:       trace.EventToolCall,
		DurationMs: 12,
		Payload:    callPayload,
	})

	// Response from replacement tool (e.g. cache hit)
	respPayload := mut.MockResponse
	if respPayload == nil {
		respPayload = map[string]interface{}{
			"status": "success",
			"source": "counterfactual_replacement",
			"data":   "cached_result_instant",
		}
	}
	respPayload["tokens"] = 60

	events = append(events, trace.TraceEvent{
		ID:         fmt.Sprintf("evt-cf-subresp-%d", now.UnixNano()),
		Timestamp:  now.Add(12 * time.Millisecond),
		Type:       trace.EventToolResult,
		DurationMs: 8,
		Payload:    respPayload,
	})

	// Final LLM synthesis
	events = append(events, trace.TraceEvent{
		ID:         fmt.Sprintf("evt-cf-llm-%d", now.UnixNano()),
		Timestamp:  now.Add(25 * time.Millisecond),
		Type:       trace.EventLLMResponse,
		DurationMs: 85,
		Payload: map[string]interface{}{
			"content": "Execution succeeded using cached fallback tool.",
			"tokens":  120,
		},
	})

	return events
}

func (s *Simulator) synthesizeArgumentOverride(baseline *trace.Session, mut CounterfactualMutation) []trace.TraceEvent {
	var events []trace.TraceEvent
	now := time.Now().UTC()

	callPayload := map[string]interface{}{
		"tool":      mut.TargetTool,
		"arguments": mut.ArgumentOverrides,
		"corrected": true,
		"tokens":    55,
	}
	events = append(events, trace.TraceEvent{
		ID:         fmt.Sprintf("evt-cf-argcall-%d", now.UnixNano()),
		Timestamp:  now,
		Type:       trace.EventToolCall,
		DurationMs: 15,
		Payload:    callPayload,
	})

	respPayload := mut.MockResponse
	if respPayload == nil {
		respPayload = map[string]interface{}{
			"status":  "success",
			"rows":    []interface{}{map[string]interface{}{"id": 1, "value": "valid_data"}},
			"message": "Query executed cleanly with corrected arguments.",
		}
	}
	respPayload["tokens"] = 90

	events = append(events, trace.TraceEvent{
		ID:         fmt.Sprintf("evt-cf-argresp-%d", now.UnixNano()),
		Timestamp:  now.Add(15 * time.Millisecond),
		Type:       trace.EventToolResult,
		DurationMs: 10,
		Payload:    respPayload,
	})

	events = append(events, trace.TraceEvent{
		ID:         fmt.Sprintf("evt-cf-argllm-%d", now.UnixNano()),
		Timestamp:  now.Add(30 * time.Millisecond),
		Type:       trace.EventLLMResponse,
		DurationMs: 95,
		Payload: map[string]interface{}{
			"content": "Data retrieved successfully with schema-corrected parameters.",
			"tokens":  140,
		},
	})

	return events
}

func (s *Simulator) synthesizeResponseMock(baseline *trace.Session, mut CounterfactualMutation) []trace.TraceEvent {
	var events []trace.TraceEvent
	now := time.Now().UTC()

	events = append(events, trace.TraceEvent{
		ID:         fmt.Sprintf("evt-cf-mockcall-%d", now.UnixNano()),
		Timestamp:  now,
		Type:       trace.EventToolCall,
		DurationMs: 10,
		Payload: map[string]interface{}{
			"tool":      mut.TargetTool,
			"arguments": mut.ArgumentOverrides,
			"tokens":    40,
		},
	})

	mockPayload := mut.MockResponse
	if mockPayload == nil {
		mockPayload = map[string]interface{}{"status": "mock_success", "items_count": 3}
	}
	mockPayload["tokens"] = 75

	events = append(events, trace.TraceEvent{
		ID:         fmt.Sprintf("evt-cf-mockresp-%d", now.UnixNano()),
		Timestamp:  now.Add(10 * time.Millisecond),
		Type:       trace.EventToolResult,
		DurationMs: 5,
		Payload:    mockPayload,
	})

	events = append(events, trace.TraceEvent{
		ID:         fmt.Sprintf("evt-cf-mockllm-%d", now.UnixNano()),
		Timestamp:  now.Add(20 * time.Millisecond),
		Type:       trace.EventLLMResponse,
		DurationMs: 70,
		Payload: map[string]interface{}{
			"content": "Task completed successfully using simulated response data.",
			"tokens":  110,
		},
	})

	return events
}

func (s *Simulator) synthesizeSteeringPrompt(baseline *trace.Session, mut CounterfactualMutation) []trace.TraceEvent {
	var events []trace.TraceEvent
	now := time.Now().UTC()

	// Injected steering prompt
	events = append(events, trace.TraceEvent{
		ID:         fmt.Sprintf("evt-cf-steer-%d", now.UnixNano()),
		Timestamp:  now,
		Type:       trace.EventStateChange,
		DurationMs: 2,
		Payload: map[string]interface{}{
			"action":         "inject_steering_prompt",
			"steering_text":  mut.SteeringPrompt,
			"human_in_loop": true,
			"tokens":         45,
		},
	})

	// Directed optimal tool call
	events = append(events, trace.TraceEvent{
		ID:         fmt.Sprintf("evt-cf-dircall-%d", now.UnixNano()),
		Timestamp:  now.Add(5 * time.Millisecond),
		Type:       trace.EventToolCall,
		DurationMs: 20,
		Payload: map[string]interface{}{
			"tool":      mut.TargetTool,
			"arguments": mut.ArgumentOverrides,
			"tokens":    50,
		},
	})

	events = append(events, trace.TraceEvent{
		ID:         fmt.Sprintf("evt-cf-dirresp-%d", now.UnixNano()),
		Timestamp:  now.Add(25 * time.Millisecond),
		Type:       trace.EventToolResult,
		DurationMs: 12,
		Payload: map[string]interface{}{
			"status": "success",
			"result": "target_retrieved",
			"tokens": 85,
		},
	})

	events = append(events, trace.TraceEvent{
		ID:         fmt.Sprintf("evt-cf-dirllm-%d", now.UnixNano()),
		Timestamp:  now.Add(40 * time.Millisecond),
		Type:       trace.EventLLMResponse,
		DurationMs: 65,
		Payload: map[string]interface{}{
			"content": "Target goal resolved concisely under human steering guidance.",
			"tokens":  95,
		},
	})

	return events
}

func (s *Simulator) compareTrajectories(baseline, branch *trace.Session, mut CounterfactualMutation) *BranchComparison {
	baseTools := extractToolSequence(baseline)
	branchTools := extractToolSequence(branch)

	editDist := computeLevenshtein(baseTools, branchTools)
	baseTokens, baseLat := calculateTokensAndLatency(baseline)
	branchTokens, branchLat := calculateTokensAndLatency(branch)

	tokenDelta := branchTokens - baseTokens
	tokenSavingsPct := 0.0
	if baseTokens > 0 {
		tokenSavingsPct = float64(-tokenDelta) / float64(baseTokens) * 100.0
		if tokenSavingsPct < 0 {
			tokenSavingsPct = 0.0
		}
	}

	costBase := (float64(baseTokens) / 1000.0) * CostPer1kTokens
	costBranch := (float64(branchTokens) / 1000.0) * CostPer1kTokens
	costDelta := costBranch - costBase

	baselineHadFailure := hasErrors(baseline)
	branchHasFailure := hasErrors(branch)

	outcome := OutcomeAlternativeEquiv
	resolvedFailure := false
	summaryNote := "Trajectory branched with equivalent outcome."

	if baselineHadFailure && !branchHasFailure {
		outcome = OutcomeResolvedSuccess
		resolvedFailure = true
		summaryNote = fmt.Sprintf("What-If intervention at step %d successfully resolved baseline failure, saving %d tokens.", mut.BranchStep, -tokenDelta)
	} else if !baselineHadFailure && branchHasFailure {
		outcome = OutcomeRegressionFailure
		summaryNote = fmt.Sprintf("Counterfactual intervention introduced error at step %d.", mut.BranchStep)
	} else if branchTokens > baseTokens*2 {
		outcome = OutcomeRunawayDivergence
		summaryNote = fmt.Sprintf("Branch diverged into runaway execution (+%d tokens).", tokenDelta)
	} else if len(branchTools) < len(baseTools) && !branchHasFailure {
		outcome = OutcomeResolvedSuccess
		summaryNote = fmt.Sprintf("Pruned redundant steps from %d to %d (saved %d tokens).", len(baseTools), len(branchTools), -tokenDelta)
	}

	divPct := 0.0
	totalStepsMax := len(baseline.Events)
	if len(branch.Events) > totalStepsMax {
		totalStepsMax = len(branch.Events)
	}
	if totalStepsMax > 0 {
		divPct = float64(editDist) / float64(totalStepsMax) * 100.0
	}

	return &BranchComparison{
		SessionID:         baseline.ID,
		BranchID:          branch.ID,
		Mutation:          mut,
		BaselineSteps:     len(baseline.Events),
		BranchSteps:       len(branch.Events),
		StepDelta:         len(branch.Events) - len(baseline.Events),
		PathEditDistance:  editDist,
		PathDivergencePct: divPct,
		BaselineTokens:    baseTokens,
		BranchTokens:      branchTokens,
		TokenDelta:        tokenDelta,
		TokenSavingsPct:   tokenSavingsPct,
		BaselineLatencyMs: baseLat,
		BranchLatencyMs:   branchLat,
		LatencyDeltaMs:    branchLat - baseLat,
		CostBaselineUSD:   costBase,
		CostBranchUSD:     costBranch,
		CostDeltaUSD:      costDelta,
		Outcome:           outcome,
		ResolvedFailure:   resolvedFailure,
		SummaryNote:       summaryNote,
		CreatedAt:         time.Now().UTC(),
	}
}

func extractToolSequence(s *trace.Session) []string {
	var tools []string
	for _, ev := range s.Events {
		if ev.Type == trace.EventToolCall {
			if tool, ok := ev.Payload["tool"].(string); ok && tool != "" {
				tools = append(tools, tool)
			}
		}
	}
	return tools
}

func calculateTokensAndLatency(s *trace.Session) (int, int64) {
	totalTokens := 0
	var totalLat int64 = 0

	for _, ev := range s.Events {
		totalLat += ev.DurationMs
		if tokVal, exists := ev.Payload["tokens"]; exists {
			switch v := tokVal.(type) {
			case int:
				totalTokens += v
			case float64:
				totalTokens += int(v)
			}
		} else {
			totalTokens += 40 // Default step baseline
		}
	}
	return totalTokens, totalLat
}

func hasErrors(s *trace.Session) bool {
	for _, ev := range s.Events {
		if ev.Type == trace.EventError {
			return true
		}
		if status, ok := ev.Payload["status"].(string); ok && (status == "error" || status == "failed") {
			return true
		}
		if code, ok := ev.Payload["status_code"].(int); ok && code >= 400 {
			return true
		}
	}
	return false
}

func computeLevenshtein(a, b []string) int {
	la := len(a)
	lb := len(b)
	d := make([][]int, la+1)
	for i := range d {
		d[i] = make([]int, lb+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}

	for i := 1; i <= la; i++ {
		for j := 1; j <= lb; j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			d[i][j] = min(
				d[i-1][j]+1,      // deletion
				d[i][j-1]+1,      // insertion
				d[i-1][j-1]+cost, // substitution
			)
		}
	}
	return d[la][lb]
}

func min(vals ...int) int {
	m := vals[0]
	for _, v := range vals[1:] {
		if v < m {
			m = v
		}
	}
	return m
}

func computeEventChecksum(ev trace.TraceEvent) string {
	b, _ := json.Marshal(ev.Payload)
	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%s:%d:%s", ev.ID, ev.Index, string(b))))
	return hex.EncodeToString(h.Sum(nil))[:16]
}
