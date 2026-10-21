package deadlock

import (
	"testing"
	"time"
)

func TestDeadlock_AcyclicSwarmHealthy(t *testing.T) {
	cfg := DefaultDeadlockConfig()
	engine := NewEngine(cfg)

	nodes := []AgentNode{
		{AgentID: "agent-a", AgentName: "A", State: "RUNNING"},
		{AgentID: "agent-b", AgentName: "B", State: "RUNNING"},
		{AgentID: "agent-c", AgentName: "C", State: "RUNNING"},
	}
	edges := []DependencyEdge{
		{FromAgent: "agent-a", ToAgent: "agent-b", Duration: 1 * time.Second},
		{FromAgent: "agent-b", ToAgent: "agent-c", Duration: 1 * time.Second},
	}

	assessment := engine.AssessSwarm("test-acyclic", nodes, edges)
	if assessment.HasDeadlock {
		t.Fatalf("expected acyclic swarm to have no deadlock")
	}
	if assessment.DeadlockFreedomScore != 100.0 {
		t.Fatalf("expected DFS 100.0, got %.1f", assessment.DeadlockFreedomScore)
	}
	if assessment.Status != "HEALTHY" {
		t.Fatalf("expected status HEALTHY, got %s", assessment.Status)
	}
}

func TestDeadlock_TwoAgentDirectCycle(t *testing.T) {
	cfg := DefaultDeadlockConfig()
	engine := NewEngine(cfg)

	nodes := []AgentNode{
		{AgentID: "agent-1", AgentName: "1", State: "WAITING"},
		{AgentID: "agent-2", AgentName: "2", State: "WAITING"},
	}
	edges := []DependencyEdge{
		{FromAgent: "agent-1", ToAgent: "agent-2", Duration: 10 * time.Second},
		{FromAgent: "agent-2", ToAgent: "agent-1", Duration: 10 * time.Second},
	}

	assessment := engine.AssessSwarm("test-2-cycle", nodes, edges)
	if !assessment.HasDeadlock {
		t.Fatalf("expected deadlock in 2-agent cycle")
	}
	if len(assessment.Cycles) != 1 {
		t.Fatalf("expected 1 cycle, got %d", len(assessment.Cycles))
	}
	cycle := assessment.Cycles[0]
	if cycle.Type != TypeCircularWait {
		t.Fatalf("expected TypeCircularWait, got %s", cycle.Type)
	}
	if cycle.ProposedResolution != StrategyBreakCycleTimeout {
		t.Fatalf("expected StrategyBreakCycleTimeout, got %s", cycle.ProposedResolution)
	}
}

func TestDeadlock_ThreeAgentCircularWait(t *testing.T) {
	cfg := DefaultDeadlockConfig()
	engine := NewEngine(cfg)

	nodes := []AgentNode{
		{AgentID: "agent-a", State: "WAITING"},
		{AgentID: "agent-b", State: "WAITING"},
		{AgentID: "agent-c", State: "WAITING"},
	}
	edges := []DependencyEdge{
		{FromAgent: "agent-a", ToAgent: "agent-b", Duration: 15 * time.Second},
		{FromAgent: "agent-b", ToAgent: "agent-c", Duration: 15 * time.Second},
		{FromAgent: "agent-c", ToAgent: "agent-a", Duration: 15 * time.Second},
	}

	assessment := engine.AssessSwarm("test-3-cycle", nodes, edges)
	if !assessment.HasDeadlock {
		t.Fatalf("expected deadlock in 3-agent cycle")
	}
	if len(assessment.Cycles) != 1 {
		t.Fatalf("expected 1 cycle, got %d", len(assessment.Cycles))
	}
	if len(assessment.Cycles[0].InvolvedAgents) != 3 {
		t.Fatalf("expected 3 involved agents, got %d", len(assessment.Cycles[0].InvolvedAgents))
	}
}

func TestDeadlock_ResourceLockContention(t *testing.T) {
	cfg := DefaultDeadlockConfig()
	engine := NewEngine(cfg)

	nodes := []AgentNode{
		{AgentID: "agent-x", State: "BLOCKED"},
		{AgentID: "agent-y", State: "BLOCKED"},
	}
	edges := []DependencyEdge{
		{FromAgent: "agent-x", ToAgent: "agent-y", Resource: "file:///lock/db", Duration: 12 * time.Second},
		{FromAgent: "agent-y", ToAgent: "agent-x", Resource: "file:///lock/api", Duration: 14 * time.Second},
	}

	assessment := engine.AssessSwarm("test-lock-collision", nodes, edges)
	if !assessment.HasDeadlock {
		t.Fatalf("expected deadlock from resource lock contention")
	}
	cycle := assessment.Cycles[0]
	if cycle.Type != TypeResourceContention {
		t.Fatalf("expected TypeResourceContention, got %s", cycle.Type)
	}
	if cycle.ProposedResolution != StrategyPreemptOldest {
		t.Fatalf("expected StrategyPreemptOldest, got %s", cycle.ProposedResolution)
	}
}

func TestDeadlock_OrphanedHandoff(t *testing.T) {
	cfg := DefaultDeadlockConfig()
	engine := NewEngine(cfg)

	nodes := []AgentNode{
		{AgentID: "agent-parent", State: "WAITING"},
		{AgentID: "agent-child", State: "TERMINATED"},
	}
	edges := []DependencyEdge{
		{FromAgent: "agent-parent", ToAgent: "agent-child", Duration: 40 * time.Second},
	}

	assessment := engine.AssessSwarm("test-orphan", nodes, edges)
	if len(assessment.Cycles) != 1 {
		t.Fatalf("expected 1 orphaned handoff cycle, got %d", len(assessment.Cycles))
	}
	if assessment.Cycles[0].Type != TypeOrphanedHandoff {
		t.Fatalf("expected TypeOrphanedHandoff, got %s", assessment.Cycles[0].Type)
	}
}

func TestDeadlock_DFSMetricCalculation(t *testing.T) {
	engine := NewEngine(DefaultDeadlockConfig())

	// No cycles -> 100.0
	scoreClean := engine.CalculateDeadlockFreedomScore(nil, 4)
	if scoreClean != 100.0 {
		t.Fatalf("expected 100.0, got %.1f", scoreClean)
	}

	// 1 circular wait -> 60.0
	cycles := []DeadlockCycle{
		{Type: TypeCircularWait},
	}
	scoreOne := engine.CalculateDeadlockFreedomScore(cycles, 3)
	if scoreOne != 60.0 {
		t.Fatalf("expected 60.0, got %.1f", scoreOne)
	}
}
