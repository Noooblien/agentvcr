package diff

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/Noooblien/agentvcr/pkg/metrics"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

// SessionDiff represents the structural and performance delta between two agent traces.
type SessionDiff struct {
	BaseSessionID     string                 `json:"base_session_id"`
	TargetSessionID   string                 `json:"target_session_id"`
	DivergenceStep    int                    `json:"divergence_step"` // -1 if identical
	BaseEventCount    int                    `json:"base_event_count"`
	TargetEventCount  int                    `json:"target_event_count"`
	TokenDelta        int                    `json:"token_delta"`
	CostDeltaUSD      float64                `json:"cost_delta_usd"`
	DurationDeltaMs   int64                  `json:"duration_delta_ms"`
	DivergentDetails  *StepDivergence        `json:"divergent_details,omitempty"`
}

// StepDivergence captures exact payload or type differences at the point of divergence.
type StepDivergence struct {
	StepIndex  int             `json:"step_index"`
	BaseType   trace.EventType `json:"base_type"`
	TargetType trace.EventType `json:"target_type"`
	Reason     string          `json:"reason"`
}

// CompareSessions computes differences between a base session and target session.
func CompareSessions(base, target *trace.Session) SessionDiff {
	baseMetrics := metrics.ComputeSessionMetrics(base)
	targetMetrics := metrics.ComputeSessionMetrics(target)

	diff := SessionDiff{
		BaseSessionID:    base.ID,
		TargetSessionID:  target.ID,
		DivergenceStep:   -1,
		BaseEventCount:   len(base.Events),
		TargetEventCount: len(target.Events),
		TokenDelta:       targetMetrics.TotalTokens - baseMetrics.TotalTokens,
		CostDeltaUSD:     targetMetrics.EstimatedCostUSD - baseMetrics.EstimatedCostUSD,
		DurationDeltaMs:  targetMetrics.TotalDurationMs - baseMetrics.TotalDurationMs,
	}

	minSteps := len(base.Events)
	if len(target.Events) < minSteps {
		minSteps = len(target.Events)
	}

	for i := 0; i < minSteps; i++ {
		bEvt := base.Events[i]
		tEvt := target.Events[i]

		if bEvt.Type != tEvt.Type {
			diff.DivergenceStep = i
			diff.DivergentDetails = &StepDivergence{
				StepIndex:  i,
				BaseType:   bEvt.Type,
				TargetType: tEvt.Type,
				Reason:     fmt.Sprintf("Event type mismatch: '%s' vs '%s'", bEvt.Type, tEvt.Type),
			}
			return diff
		}

		if !payloadsEqual(bEvt.Payload, tEvt.Payload) {
			diff.DivergenceStep = i
			diff.DivergentDetails = &StepDivergence{
				StepIndex:  i,
				BaseType:   bEvt.Type,
				TargetType: tEvt.Type,
				Reason:     "Payload content deviated",
			}
			return diff
		}
	}

	if len(base.Events) != len(target.Events) {
		diff.DivergenceStep = minSteps
		diff.DivergentDetails = &StepDivergence{
			StepIndex: minSteps,
			Reason:    fmt.Sprintf("Trajectory length differed: %d steps vs %d steps", len(base.Events), len(target.Events)),
		}
	}

	return diff
}

func payloadsEqual(p1, p2 map[string]interface{}) bool {
	if len(p1) == 0 && len(p2) == 0 {
		return true
	}
	b1, err1 := json.Marshal(p1)
	b2, err2 := json.Marshal(p2)
	if err1 != nil || err2 != nil {
		return reflect.DeepEqual(p1, p2)
	}
	return string(b1) == string(b2)
}
