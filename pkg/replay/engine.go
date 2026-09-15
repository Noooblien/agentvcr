package replay

import (
	"fmt"
	"time"

	"github.com/Noooblien/agentvcr/pkg/storage"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

// Engine manages playing back and branching past agent trajectories.
type Engine struct {
	store *storage.Store
}

// NewEngine creates a new replay and branching engine.
func NewEngine(store *storage.Store) *Engine {
	return &Engine{store: store}
}

// ForkSession duplicates an existing session up to forkStep, creating an active branch for mutation.
func (e *Engine) ForkSession(parentID string, forkStep int, newSessionID string) (*trace.Session, error) {
	parent, err := e.store.LoadSession(parentID)
	if err != nil {
		return nil, fmt.Errorf("failed to load parent session: %w", err)
	}

	if forkStep > len(parent.Events) {
		return nil, fmt.Errorf("fork step %d exceeds session event count (%d)", forkStep, len(parent.Events))
	}

	// Truncate events up to the chosen fork point
	forkedEvents := make([]trace.TraceEvent, forkStep)
	copy(forkedEvents, parent.Events[:forkStep])

	branch := &trace.Session{
		ID:         newSessionID,
		AgentName:  parent.AgentName,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		ParentID:   parent.ID,
		ForkAtStep: forkStep,
		Status:     "forked",
		Events:     forkedEvents,
		Metadata: map[string]string{
			"forked_from": parent.ID,
			"fork_step":   fmt.Sprintf("%d", forkStep),
		},
	}

	if err := e.store.SaveSession(branch); err != nil {
		return nil, fmt.Errorf("failed to save forked branch session: %w", err)
	}

	return branch, nil
}

// PrintSummary outputs a step-by-step audit of a recorded trajectory.
func (e *Engine) PrintSummary(session *trace.Session) {
	fmt.Printf("====================================================\n")
	fmt.Printf("  TRACE SESSION: %s\n", session.ID)
	fmt.Printf("  Agent:         %s\n", session.AgentName)
	fmt.Printf("  Recorded At:   %s\n", session.CreatedAt.Format(time.RFC3339))
	fmt.Printf("  Total Steps:   %d\n", len(session.Events))
	fmt.Printf("  Status:        %s\n", session.Status)
	if session.ParentID != "" {
		fmt.Printf("  Branch Of:     %s @ step %d\n", session.ParentID, session.ForkAtStep)
	}
	fmt.Printf("====================================================\n\n")

	for _, evt := range session.Events {
		fmt.Printf("[%02d] %s (%s)\n", evt.Index, evt.Type, evt.Timestamp.Format("15:04:05.000"))
		if evt.Type == trace.EventLLMRequest {
			if path, ok := evt.Payload["path"].(string); ok {
				fmt.Printf("     Endpoint: %s\n", path)
			}
		} else if evt.Type == trace.EventLLMResponse {
			if status, ok := evt.Payload["status_code"]; ok {
				fmt.Printf("     HTTP Status: %v\n", status)
			}
		}
	}
	fmt.Println()
}
