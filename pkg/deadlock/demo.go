package deadlock

import (
	"time"
)

// RunDeadlockDemo executes 4 realistic enterprise multi-agent swarm scenarios.
func RunDeadlockDemo() *DeadlockSummary {
	cfg := DefaultDeadlockConfig()
	engine := NewEngine(cfg)

	// Scenario 1: Clean Acyclic CI/CD Swarm (Healthy)
	nodes1 := []AgentNode{
		{AgentID: "agent-architect", AgentName: "Architect", State: "RUNNING", TurnIndex: 3},
		{AgentID: "agent-coder", AgentName: "Coder", State: "RUNNING", TurnIndex: 4},
		{AgentID: "agent-tester", AgentName: "Tester", State: "RUNNING", TurnIndex: 2},
	}
	edges1 := []DependencyEdge{
		{FromAgent: "agent-coder", ToAgent: "agent-architect", Reason: "Waiting for API spec approval", Duration: 2 * time.Second},
		{FromAgent: "agent-tester", ToAgent: "agent-coder", Reason: "Waiting for compiled test binary", Duration: 1 * time.Second},
	}
	s1 := engine.AssessSwarm("swarm-clean-cicd", nodes1, edges1)

	// Scenario 2: 3-Agent Circular Message RPC Cycle (Deadlocked)
	nodes2 := []AgentNode{
		{AgentID: "agent-lead", AgentName: "Lead", State: "WAITING", TurnIndex: 12},
		{AgentID: "agent-secops", AgentName: "SecOps", State: "WAITING", TurnIndex: 8},
		{AgentID: "agent-compliance", AgentName: "Compliance", State: "WAITING", TurnIndex: 5},
	}
	edges2 := []DependencyEdge{
		{FromAgent: "agent-lead", ToAgent: "agent-secops", Reason: "Waiting for security clearance signoff", Duration: 42 * time.Second},
		{FromAgent: "agent-secops", ToAgent: "agent-compliance", Reason: "Waiting for GDPR data mapping", Duration: 35 * time.Second},
		{FromAgent: "agent-compliance", ToAgent: "agent-lead", Reason: "Waiting for data retention architecture", Duration: 28 * time.Second},
	}
	s2 := engine.AssessSwarm("swarm-circular-rpc", nodes2, edges2)

	// Scenario 3: Resource Lock Collision (Deadlocked)
	nodes3 := []AgentNode{
		{AgentID: "agent-backend", AgentName: "Backend", State: "BLOCKED", TurnIndex: 7},
		{AgentID: "agent-frontend", AgentName: "Frontend", State: "BLOCKED", TurnIndex: 9},
	}
	edges3 := []DependencyEdge{
		{FromAgent: "agent-backend", ToAgent: "agent-frontend", Resource: "file:///workspace/types/api.ts", Reason: "Waiting for shared interface lock", Duration: 15 * time.Second},
		{FromAgent: "agent-frontend", ToAgent: "agent-backend", Resource: "file:///workspace/db/schema.sql", Reason: "Waiting for migration lock", Duration: 18 * time.Second},
	}
	s3 := engine.AssessSwarm("swarm-lock-collision", nodes3, edges3)

	// Scenario 4: Orphaned Subagent Handoff (Deadlocked)
	nodes4 := []AgentNode{
		{AgentID: "agent-orchestrator", AgentName: "Orchestrator", State: "WAITING", TurnIndex: 14},
		{AgentID: "agent-worker-crashed", AgentName: "Worker", State: "TERMINATED", TurnIndex: 1},
	}
	edges4 := []DependencyEdge{
		{FromAgent: "agent-orchestrator", ToAgent: "agent-worker-crashed", Reason: "Waiting for subagent completion RPC", Duration: 65 * time.Second},
	}
	s4 := engine.AssessSwarm("swarm-orphaned-wait", nodes4, edges4)

	assessments := []*DeadlockAssessment{s1, s2, s3, s4}
	deadlocks := 0
	livelocks := 0
	resolved := 0
	var totalDFS float64

	for _, a := range assessments {
		if a.HasDeadlock {
			deadlocks++
		}
		if a.HasLivelock {
			livelocks++
		}
		resolved += len(a.Cycles)
		totalDFS += a.DeadlockFreedomScore
	}

	return &DeadlockSummary{
		TotalSwarmsAudited: len(assessments),
		DeadlocksDetected:  deadlocks,
		LivelocksDetected:  livelocks,
		CyclesResolved:     resolved,
		AverageDFS:         totalDFS / float64(len(assessments)),
		Assessments:        assessments,
	}
}
