package swarm

import (
	"fmt"
	"strings"
)

// CycleDetector analyzes the runtime handoff chain of a swarm to prevent infinite loops and ping-pong deadlocks.
type CycleDetector struct {
	MaxHops          int
	MaxCycleRepeats  int
	VisitedEdges     map[string]int // "AgentA->AgentB" count
	VisitedNodes     map[string]int // "AgentA" count
}

// NewCycleDetector initializes a CycleDetector with safe operational limits.
func NewCycleDetector(maxHops int, maxCycleRepeats int) *CycleDetector {
	if maxHops <= 0 {
		maxHops = 15
	}
	if maxCycleRepeats <= 0 {
		maxCycleRepeats = 2
	}
	return &CycleDetector{
		MaxHops:         maxHops,
		MaxCycleRepeats: maxCycleRepeats,
		VisitedEdges:    make(map[string]int),
		VisitedNodes:    make(map[string]int),
	}
}

// RecordHandoff evaluates whether adding this handoff transition constitutes an infinite cycle or exceeds hop budget.
func (cd *CycleDetector) RecordHandoff(history []HandoffContract, source string, target string) (bool, string) {
	edge := fmt.Sprintf("%s->%s", source, target)
	cd.VisitedEdges[edge]++
	cd.VisitedNodes[target]++

	// 1. Max Hops Budget Check
	if len(history)+1 > cd.MaxHops {
		return true, fmt.Sprintf("Max handoff hop budget exceeded (%d > %d)", len(history)+1, cd.MaxHops)
	}

	// 2. Direct Ping-Pong Detection (A -> B -> A -> B)
	if len(history) >= 2 {
		last := history[len(history)-1]
		secondLast := history[len(history)-2]

		// Pattern: A -> B followed by B -> A
		if secondLast.SourceAgent == source && secondLast.TargetAgent == target &&
			last.SourceAgent == target && last.TargetAgent == source {
			if cd.VisitedEdges[edge] >= cd.MaxCycleRepeats {
				return true, fmt.Sprintf("Ping-pong deadlock cycle detected between [%s] and [%s] (%d repeats)",
					source, target, cd.VisitedEdges[edge])
			}
		}
	}

	// 3. Directed Cyclic Walk Detection (e.g. A -> B -> C -> A -> B -> C)
	chain := make([]string, 0, len(history)+2)
	if len(history) > 0 {
		chain = append(chain, history[0].SourceAgent)
		for _, h := range history {
			chain = append(chain, h.TargetAgent)
		}
	} else {
		chain = append(chain, source)
	}
	chain = append(chain, target)

	if hasRepeatedCyclePattern(chain, cd.MaxCycleRepeats) {
		return true, fmt.Sprintf("Multi-node circular handoff loop detected in chain [%s]", strings.Join(chain, " -> "))
	}

	return false, ""
}

// hasRepeatedCyclePattern detects cyclic sub-slices repeated >= maxRepeats times at the end of the chain.
func hasRepeatedCyclePattern(chain []string, maxRepeats int) bool {
	n := len(chain)
	if n < 4 {
		return false
	}

	// Check candidate cycle lengths from 2 up to n/2
	for cycleLen := 2; cycleLen <= n/2; cycleLen++ {
		patternCount := 1
		baseSlice := chain[n-cycleLen : n]

		for offset := cycleLen * 2; offset <= n; offset += cycleLen {
			candidateSlice := chain[n-offset : n-offset+cycleLen]
			if slicesEqual(baseSlice, candidateSlice) {
				patternCount++
				if patternCount >= maxRepeats {
					return true
				}
			} else {
				break
			}
		}
	}
	return false
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
