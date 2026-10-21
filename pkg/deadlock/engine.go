package deadlock

import (
	"fmt"
	"math"
	"time"
)

// Engine performs Wait-For Graph (WFG) analysis and distributed deadlock/livelock detection.
type Engine struct {
	config DeadlockConfig
}

// DefaultDeadlockConfig returns production-tuned defaults.
func DefaultDeadlockConfig() DeadlockConfig {
	return DeadlockConfig{
		StallThreshold:     5 * time.Second,
		LivelockTurnRepeat: 3,
		MaxSwarmAgents:     64,
	}
}

// NewEngine initializes a deadlock analysis engine.
func NewEngine(cfg DeadlockConfig) *Engine {
	return &Engine{config: cfg}
}

// DetectCycles analyzes the directed dependency graph using depth-first cycle traversal.
func (e *Engine) DetectCycles(nodes []AgentNode, edges []DependencyEdge) []DeadlockCycle {
	adj := make(map[string][]string)
	edgeMap := make(map[string]DependencyEdge)
	nodeMap := make(map[string]AgentNode)

	for _, n := range nodes {
		nodeMap[n.AgentID] = n
	}

	for _, edge := range edges {
		adj[edge.FromAgent] = append(adj[edge.FromAgent], edge.ToAgent)
		key := fmt.Sprintf("%s->%s", edge.FromAgent, edge.ToAgent)
		edgeMap[key] = edge
	}

	visited := make(map[string]bool)
	recStack := make(map[string]bool)
	path := make([]string, 0)
	var foundCycles [][]string

	var dfs func(u string)
	dfs = func(u string) {
		visited[u] = true
		recStack[u] = true
		path = append(path, u)

		for _, v := range adj[u] {
			if !visited[v] {
				dfs(v)
			} else if recStack[v] {
				// Cycle detected: extract subpath from v to u, plus v
				idx := -1
				for i, node := range path {
					if node == v {
						idx = i
						break
					}
				}
				if idx != -1 {
					cycle := make([]string, len(path)-idx+1)
					copy(cycle, path[idx:])
					cycle[len(cycle)-1] = v
					foundCycles = append(foundCycles, cycle)
				}
			}
		}

		path = path[:len(path)-1]
		recStack[u] = false
	}

	for _, n := range nodes {
		if !visited[n.AgentID] {
			dfs(n.AgentID)
		}
	}

	// De-duplicate cycles
	uniqueCycles := deduplicateCycles(foundCycles)
	var results []DeadlockCycle

	for idx, cyclePath := range uniqueCycles {
		var involvedAgents []string
		agentSet := make(map[string]bool)
		for _, a := range cyclePath[:len(cyclePath)-1] {
			if !agentSet[a] {
				agentSet[a] = true
				involvedAgents = append(involvedAgents, a)
			}
		}

		var maxDuration time.Duration
		var resources []string
		hasResourceLock := false

		for i := 0; i < len(cyclePath)-1; i++ {
			from := cyclePath[i]
			to := cyclePath[i+1]
			edgeKey := fmt.Sprintf("%s->%s", from, to)
			if edge, ok := edgeMap[edgeKey]; ok {
				if edge.Duration > maxDuration {
					maxDuration = edge.Duration
				}
				if edge.Resource != "" {
					resources = append(resources, edge.Resource)
					hasResourceLock = true
				}
			}
		}

		deadlockType := TypeCircularWait
		strategy := StrategyBreakCycleTimeout
		action := fmt.Sprintf("Inject timeout exception into edge %s->%s to break dependency wait", cyclePath[0], cyclePath[1])

		if hasResourceLock {
			deadlockType = TypeResourceContention
			strategy = StrategyPreemptOldest
			action = fmt.Sprintf("Preempt exclusive resource lock on '%s' held by %s", resources[0], involvedAgents[0])
		}

		severity := "HIGH"
		if len(involvedAgents) >= 2 && maxDuration >= e.config.StallThreshold {
			severity = "CRITICAL"
		}

		results = append(results, DeadlockCycle{
			CycleID:            fmt.Sprintf("cycle_dlk_%03d", idx+1),
			Type:               deadlockType,
			InvolvedAgents:     involvedAgents,
			InvolvedResources:  resources,
			CyclePath:          cyclePath,
			StallDuration:      maxDuration,
			ProposedResolution: strategy,
			ResolutionAction:   action,
			Severity:           severity,
		})
	}

	return results
}

// DetectOrphanedHandoffs identifies edges where the target agent has crashed, exited, or does not exist.
func (e *Engine) DetectOrphanedHandoffs(nodes []AgentNode, edges []DependencyEdge) []DeadlockCycle {
	activeNodes := make(map[string]AgentNode)
	for _, n := range nodes {
		activeNodes[n.AgentID] = n
	}

	var orphans []DeadlockCycle
	idx := 1
	for _, edge := range edges {
		targetNode, exists := activeNodes[edge.ToAgent]
		isDead := !exists || targetNode.State == "TERMINATED" || targetNode.State == "CRASHED"
		if isDead {
			orphans = append(orphans, DeadlockCycle{
				CycleID:            fmt.Sprintf("cycle_orph_%03d", idx),
				Type:               TypeOrphanedHandoff,
				InvolvedAgents:     []string{edge.FromAgent, edge.ToAgent},
				CyclePath:          []string{edge.FromAgent, edge.ToAgent},
				StallDuration:      edge.Duration,
				ProposedResolution: StrategySynthesizeUnblock,
				ResolutionAction:   fmt.Sprintf("Synthesize synthetic abort/fallback reply for dead target '%s'", edge.ToAgent),
				Severity:           "CRITICAL",
			})
			idx++
		}
	}
	return orphans
}

// CalculateDeadlockFreedomScore calculates the 0.0 to 100.0 health metric.
func (e *Engine) CalculateDeadlockFreedomScore(cycles []DeadlockCycle, totalAgents int) float64 {
	if len(cycles) == 0 {
		return 100.0
	}

	score := 100.0
	for _, c := range cycles {
		switch c.Type {
		case TypeCircularWait:
			score -= 40.0
		case TypeResourceContention:
			score -= 35.0
		case TypeOrphanedHandoff:
			score -= 30.0
		case TypeLivelockSpin:
			score -= 25.0
		}
	}

	if score < 0.0 {
		score = 0.0
	}
	return math.Round(score*10) / 10
}

// AssessSwarm inspects the full agent swarm and computes complete deadlock diagnostics.
func (e *Engine) AssessSwarm(swarmID string, nodes []AgentNode, edges []DependencyEdge) *DeadlockAssessment {
	cycles := e.DetectCycles(nodes, edges)
	orphans := e.DetectOrphanedHandoffs(nodes, edges)

	allCycles := append(cycles, orphans...)
	dfs := e.CalculateDeadlockFreedomScore(allCycles, len(nodes))

	hasDeadlock := false
	hasLivelock := false

	for _, c := range allCycles {
		if c.Type == TypeLivelockSpin {
			hasLivelock = true
		} else {
			hasDeadlock = true
		}
	}

	status := "HEALTHY"
	if hasDeadlock || dfs < 60.0 {
		status = "DEADLOCKED"
	} else if hasLivelock || dfs < 90.0 {
		status = "DEGRADED"
	}

	return &DeadlockAssessment{
		SwarmID:              swarmID,
		TotalAgents:          len(nodes),
		TotalEdges:           len(edges),
		HasDeadlock:          hasDeadlock,
		HasLivelock:          hasLivelock,
		Cycles:               allCycles,
		DeadlockFreedomScore: dfs,
		Status:               status,
		EvaluatedAt:          time.Now(),
	}
}

// deduplicateCycles removes canonical rotations of the same directed cycle.
func deduplicateCycles(cycles [][]string) [][]string {
	seen := make(map[string]bool)
	var unique [][]string

	for _, cycle := range cycles {
		if len(cycle) <= 1 {
			continue
		}
		// Cycle vertices excluding closing vertex
		nodes := cycle[:len(cycle)-1]
		canonical := canonicalCycleKey(nodes)
		if !seen[canonical] {
			seen[canonical] = true
			unique = append(unique, cycle)
		}
	}
	return unique
}

func canonicalCycleKey(nodes []string) string {
	if len(nodes) == 0 {
		return ""
	}
	minIdx := 0
	for i := 1; i < len(nodes); i++ {
		if nodes[i] < nodes[minIdx] {
			minIdx = i
		}
	}
	var rotated []string
	for i := 0; i < len(nodes); i++ {
		idx := (minIdx + i) % len(nodes)
		rotated = append(rotated, nodes[idx])
	}
	sortKey := ""
	for _, n := range rotated {
		sortKey += n + "->"
	}
	return sortKey
}
