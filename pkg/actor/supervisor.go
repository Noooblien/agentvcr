package actor

import (
	"sync"
	"time"
)

// Supervisor manages a tree or pool of child actors and applies recovery strategies upon failures.
type Supervisor struct {
	id         string
	strategy   SupervisionStrategy
	policy     RestartPolicy
	system     *ActorSystem
	mu         sync.RWMutex
	children   []*ActorCell
	childMap   map[string]*ActorCell
	restarts   map[string][]time.Time
	escalated  bool
}

// NewSupervisor creates a supervisor with specified strategy and restart policy.
func NewSupervisor(id string, strategy SupervisionStrategy, policy RestartPolicy, sys *ActorSystem) *Supervisor {
	return &Supervisor{
		id:        id,
		strategy:  strategy,
		policy:    policy,
		system:    sys,
		children:  make([]*ActorCell, 0),
		childMap:  make(map[string]*ActorCell),
		restarts:  make(map[string][]time.Time),
	}
}

// RegisterChild adds an actor cell under this supervisor's management.
func (s *Supervisor) RegisterChild(cell *ActorCell) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.children = append(s.children, cell)
	s.childMap[cell.id] = cell
	s.restarts[cell.id] = make([]time.Time, 0)
}

// HandleFailure is triggered by a child actor cell when a panic or error occurs.
func (s *Supervisor) HandleFailure(failedCell *ActorCell, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	cutoff := now.Add(-s.policy.TimeWindow)

	// Prune old restarts
	var recent []time.Time
	for _, t := range s.restarts[failedCell.id] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	recent = append(recent, now)
	s.restarts[failedCell.id] = recent

	// Check if restart budget exceeded
	if len(recent) > s.policy.MaxRestarts {
		s.escalated = true
		s.system.recordEvent("ESCALATE_FAILURE", failedCell.id, s.id, map[string]interface{}{
			"reason":        "max_restarts_exceeded",
			"restarts_made": len(recent),
			"error":         err.Error(),
		})
		failedCell.Stop()
		return
	}

	// Apply configured supervision strategy
	switch s.strategy {
	case StrategyOneForOne:
		s.restartChildLocked(failedCell, err)

	case StrategyOneForAll:
		s.system.recordEvent("SUPERVISOR_ONE_FOR_ALL", s.id, failedCell.id, map[string]interface{}{
			"trigger_actor": failedCell.id,
			"children_cnt":  len(s.children),
		})
		for _, child := range s.children {
			s.restartChildLocked(child, err)
		}

	case StrategyRestForOne:
		found := false
		for _, child := range s.children {
			if child.id == failedCell.id {
				found = true
			}
			if found {
				s.restartChildLocked(child, err)
			}
		}
	}
}

func (s *Supervisor) restartChildLocked(cell *ActorCell, triggerErr error) {
	cell.mu.Lock()
	cell.status = StatusRestarting
	cell.restartsCount++
	cell.context.SetState(nil) // Clean slate state
	cell.mu.Unlock()

	time.Sleep(s.policy.Backoff)

	cell.mu.Lock()
	cell.status = StatusReady
	cell.lastHeartbeat = time.Now().UTC()
	cell.mu.Unlock()

	s.system.recordEvent("RESTART", cell.id, s.id, map[string]interface{}{
		"strategy":      string(s.strategy),
		"restarts_made": cell.restartsCount,
		"trigger_error": triggerErr.Error(),
	})
}

// Children returns a snapshot of all managed child actors.
func (s *Supervisor) Children() []*ActorCell {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := make([]*ActorCell, len(s.children))
	copy(res, s.children)
	return res
}

// Strategy returns the active supervision strategy.
func (s *Supervisor) Strategy() SupervisionStrategy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.strategy
}

// Escalated returns true if the supervisor breached its restart threshold.
func (s *Supervisor) Escalated() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.escalated
}
