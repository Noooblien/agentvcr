package graph

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// GraphNode defines a single operational node in the graph.
type GraphNode struct {
	Name string
	Func NodeFunc
}

// GraphDirectEdge connects one node directly to another.
type GraphDirectEdge struct {
	From string
	To   string
}

// GraphConditionalEdge evaluates state dynamically to route to one of multiple target nodes.
type GraphConditionalEdge struct {
	From     string
	Router   RouterFunc
	Branches map[string]string // branchKey -> targetNode
}

// StateGraph is the builder for constructing stateful agent workflows.
type StateGraph struct {
	mu               sync.RWMutex
	channels         map[string]*StateChannel
	nodes            map[string]*GraphNode
	edges            map[string][]string // from -> []to
	conditionalEdges map[string]*GraphConditionalEdge
	entryPoint       string
	finishPoints     map[string]bool
}

// NewStateGraph initializes a new blank state graph builder.
func NewStateGraph() *StateGraph {
	return &StateGraph{
		channels:         make(map[string]*StateChannel),
		nodes:            make(map[string]*GraphNode),
		edges:            make(map[string][]string),
		conditionalEdges: make(map[string]*GraphConditionalEdge),
		finishPoints:     make(map[string]bool),
	}
}

// AddChannel registers a named state variable with schema and reduction logic.
func (g *StateGraph) AddChannel(name string, defaultVal interface{}, reducer ReducerFunc) *StateGraph {
	g.mu.Lock()
	defer g.mu.Unlock()
	if reducer == nil {
		reducer = ReducerOverwrite
	}
	g.channels[name] = &StateChannel{
		Name:         name,
		DefaultValue: defaultVal,
		Reducer:      reducer,
	}
	return g
}

// AddNode registers an executable worker step into the graph.
func (g *StateGraph) AddNode(name string, fn NodeFunc) *StateGraph {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.nodes[name] = &GraphNode{
		Name: name,
		Func: fn,
	}
	return g
}

// AddEdge adds a static direct transition from one node to another.
func (g *StateGraph) AddEdge(from, to string) *StateGraph {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.edges[from] = append(g.edges[from], to)
	return g
}

// AddConditionalEdge adds dynamic branching based on runtime state evaluation.
func (g *StateGraph) AddConditionalEdge(from string, router RouterFunc, branches map[string]string) *StateGraph {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.conditionalEdges[from] = &GraphConditionalEdge{
		From:     from,
		Router:   router,
		Branches: branches,
	}
	return g
}

// SetEntryPoint defines the initial node to execute when starting from NodeStart.
func (g *StateGraph) SetEntryPoint(node string) *StateGraph {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.entryPoint = node
	g.edges[NodeStart] = []string{node}
	return g
}

// SetFinishPoint marks a node as directly concluding at NodeEnd.
func (g *StateGraph) SetFinishPoint(node string) *StateGraph {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.finishPoints[node] = true
	g.edges[node] = append(g.edges[node], NodeEnd)
	return g
}

// CheckpointStore manages immutable step snapshots for time-travel replay.
type CheckpointStore struct {
	mu          sync.RWMutex
	checkpoints map[string]*Checkpoint   // checkpointID -> Checkpoint
	threads     map[string][]*Checkpoint // threadID -> chronological Checkpoints
}

// NewCheckpointStore creates a memory-backed thread-safe checkpoint repository.
func NewCheckpointStore() *CheckpointStore {
	return &CheckpointStore{
		checkpoints: make(map[string]*Checkpoint),
		threads:     make(map[string][]*Checkpoint),
	}
}

func (s *CheckpointStore) Save(cp *Checkpoint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checkpoints[cp.CheckpointID] = cp
	s.threads[cp.ThreadID] = append(s.threads[cp.ThreadID], cp)
}

func (s *CheckpointStore) Get(checkpointID string) (*Checkpoint, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp, ok := s.checkpoints[checkpointID]
	return cp, ok
}

func (s *CheckpointStore) List(threadID string) []*Checkpoint {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cps := s.threads[threadID]
	res := make([]*Checkpoint, len(cps))
	copy(res, cps)
	return res
}

// CompiledGraph is the validated, executable state machine instance.
type CompiledGraph struct {
	channels         map[string]*StateChannel
	nodes            map[string]*GraphNode
	edges            map[string][]string
	conditionalEdges map[string]*GraphConditionalEdge
	entryPoint       string
	finishPoints     map[string]bool
	config           GraphConfig
	store            *CheckpointStore
}

// Compile validates graph connectivity and prepares an immutable executable engine.
func (g *StateGraph) Compile(config GraphConfig) (*CompiledGraph, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if g.entryPoint == "" {
		return nil, fmt.Errorf("state graph missing entry point (call SetEntryPoint)")
	}
	if _, exists := g.nodes[g.entryPoint]; !exists {
		return nil, fmt.Errorf("entry point node '%s' does not exist in graph", g.entryPoint)
	}

	// Validate target nodes in edges
	for from, targets := range g.edges {
		if from != NodeStart && from != NodeEnd {
			if _, exists := g.nodes[from]; !exists {
				return nil, fmt.Errorf("edge source '%s' not registered", from)
			}
		}
		for _, to := range targets {
			if to != NodeStart && to != NodeEnd {
				if _, exists := g.nodes[to]; !exists {
					return nil, fmt.Errorf("edge target '%s' from '%s' not registered", to, from)
				}
			}
		}
	}

	// Validate conditional edges
	for from, ce := range g.conditionalEdges {
		if _, exists := g.nodes[from]; !exists {
			return nil, fmt.Errorf("conditional edge source '%s' not registered", from)
		}
		for branchKey, target := range ce.Branches {
			if target != NodeEnd {
				if _, exists := g.nodes[target]; !exists {
					return nil, fmt.Errorf("conditional branch '%s' -> target '%s' not registered", branchKey, target)
				}
			}
		}
	}

	if config.MaxRecursionLimit <= 0 {
		config.MaxRecursionLimit = 25
	}

	cg := &CompiledGraph{
		channels:         g.channels,
		nodes:            g.nodes,
		edges:            g.edges,
		conditionalEdges: g.conditionalEdges,
		entryPoint:       g.entryPoint,
		finishPoints:     g.finishPoints,
		config:           config,
		store:            NewCheckpointStore(),
	}

	return cg, nil
}

// Invoke initiates execution of the graph with initial state variables.
func (cg *CompiledGraph) Invoke(threadID string, initialState map[string]interface{}) (*ExecutionTrace, error) {
	if threadID == "" {
		threadID = fmt.Sprintf("thread_%d", time.Now().UnixNano())
	}

	currentState := cg.initializeState(initialState)
	trace := &ExecutionTrace{
		ThreadID:   threadID,
		Status:     StatusRunning,
		FinalState: currentState,
	}

	start := time.Now()
	err := cg.runLoop(trace, cg.entryPoint, currentState, 1, "", false)
	trace.TotalDurationMs = time.Now().Sub(start).Milliseconds()

	return trace, err
}

// Resume rewinds to a historical checkpoint and re-executes with optional state injections.
func (cg *CompiledGraph) Resume(threadID string, checkpointID string, stateOverrides map[string]interface{}) (*ExecutionTrace, error) {
	cp, found := cg.store.Get(checkpointID)
	if !found {
		return nil, fmt.Errorf("checkpoint '%s' not found in store", checkpointID)
	}

	resumeState := cp.CloneState()
	for k, v := range stateOverrides {
		reducer := ReducerOverwrite
		if ch, ok := cg.channels[k]; ok && ch.Reducer != nil {
			reducer = ch.Reducer
		}
		newVal, err := reducer(resumeState[k], v)
		if err != nil {
			return nil, fmt.Errorf("failed to apply state override for channel '%s': %w", k, err)
		}
		resumeState[k] = newVal
	}

	targetNode := cp.NodeName
	if len(cp.NextNodes) > 0 && cp.NextNodes[0] != NodeEnd {
		targetNode = cp.NextNodes[0]
	}

	newThreadID := fmt.Sprintf("%s_fork_cp%d", threadID, time.Now().Unix())
	trace := &ExecutionTrace{
		ThreadID:   newThreadID,
		Status:     StatusRunning,
		FinalState: resumeState,
	}

	start := time.Now()
	err := cg.runLoop(trace, targetNode, resumeState, cp.Step+1, cp.CheckpointID, true)
	trace.TotalDurationMs = time.Now().Sub(start).Milliseconds()

	return trace, err
}

func (cg *CompiledGraph) runLoop(trace *ExecutionTrace, startNode string, state map[string]interface{}, startStep int, parentCP string, skipFirstInterrupt bool) error {
	currentNode := startNode
	step := startStep
	visitedNodes := make(map[string]int)
	cycleCounter := 0

	for {
		if currentNode == NodeEnd || currentNode == "" {
			trace.Status = StatusCompleted
			trace.FinalState = DeepCloneState(state)
			trace.CycleCount = cycleCounter
			return nil
		}

		if step > cg.config.MaxRecursionLimit {
			trace.Status = StatusRecursionLimitExceeded
			trace.Error = fmt.Sprintf("Recursion limit of %d steps exceeded", cg.config.MaxRecursionLimit)
			trace.FinalState = DeepCloneState(state)
			trace.CycleCount = cycleCounter
			return fmt.Errorf("%s", trace.Error)
		}

		// Detect loop cycle
		if count := visitedNodes[currentNode]; count > 0 {
			cycleCounter++
		}
		visitedNodes[currentNode]++

		// Check Human-in-the-Loop: Interrupt Before
		if !skipFirstInterrupt && cg.shouldInterrupt(currentNode, cg.config.InterruptBefore) {
			interrupt := &HumanInterrupt{
				InterruptID: fmt.Sprintf("int_before_%s_step%d", currentNode, step),
				Type:        "before",
				NodeName:    currentNode,
				State:       DeepCloneState(state),
				Timestamp:   time.Now().UTC(),
			}
			trace.Interrupt = interrupt
			trace.Status = StatusInterrupted
			trace.FinalState = DeepCloneState(state)
			trace.CycleCount = cycleCounter
			return nil
		}
		skipFirstInterrupt = false

		node, exists := cg.nodes[currentNode]
		if !exists {
			trace.Status = StatusFailed
			trace.Error = fmt.Sprintf("Node '%s' not found during execution", currentNode)
			trace.FinalState = DeepCloneState(state)
			return fmt.Errorf("%s", trace.Error)
		}

		trace.ExecutionPath = append(trace.ExecutionPath, currentNode)

		// Execute Node
		ctx := &GraphContext{
			ThreadID:     trace.ThreadID,
			Step:         step,
			ActiveNode:   currentNode,
			CheckpointID: fmt.Sprintf("cp_%s_step%d_%d", trace.ThreadID, step, time.Now().UnixNano()),
		}

		nodeStart := time.Now()
		updates, err := node.Func(ctx, DeepCloneState(state))
		duration := time.Now().Sub(nodeStart).Milliseconds()

		if err != nil {
			trace.Status = StatusFailed
			trace.Error = fmt.Sprintf("Node '%s' failed: %v", currentNode, err)
			trace.FinalState = DeepCloneState(state)
			return err
		}

		// Apply state channel updates with reducers
		if err := cg.applyStateUpdates(state, updates); err != nil {
			trace.Status = StatusFailed
			trace.Error = fmt.Sprintf("Failed to reduce state in node '%s': %v", currentNode, err)
			trace.FinalState = DeepCloneState(state)
			return err
		}

		// Determine Next Nodes
		nextNodes, err := cg.determineNextNodes(ctx, currentNode, state)
		if err != nil {
			trace.Status = StatusFailed
			trace.Error = fmt.Sprintf("Routing error from node '%s': %v", currentNode, err)
			trace.FinalState = DeepCloneState(state)
			return err
		}

		// Create Checkpoint Snapshot
		cp := &Checkpoint{
			CheckpointID:       ctx.CheckpointID,
			ThreadID:           trace.ThreadID,
			Step:               step,
			NodeName:           currentNode,
			NextNodes:          nextNodes,
			State:              DeepCloneState(state),
			ParentCheckpointID: parentCP,
			Timestamp:          time.Now().UTC(),
			DurationMs:         duration,
		}
		cg.store.Save(cp)
		trace.Checkpoints = append(trace.Checkpoints, cp)
		trace.ActiveCheckpoint = cp
		parentCP = cp.CheckpointID

		// Check Human-in-the-Loop: Interrupt After
		if cg.shouldInterrupt(currentNode, cg.config.InterruptAfter) {
			interrupt := &HumanInterrupt{
				InterruptID: fmt.Sprintf("int_after_%s_step%d", currentNode, step),
				Type:        "after",
				NodeName:    currentNode,
				State:       DeepCloneState(state),
				Timestamp:   time.Now().UTC(),
			}
			trace.Interrupt = interrupt
			trace.Status = StatusInterrupted
			trace.FinalState = DeepCloneState(state)
			trace.CycleCount = cycleCounter
			return nil
		}

		if len(nextNodes) == 0 {
			currentNode = NodeEnd
		} else {
			currentNode = nextNodes[0]
		}
		step++
		trace.TotalSteps = len(trace.ExecutionPath)
	}
}

func (cg *CompiledGraph) determineNextNodes(ctx *GraphContext, currentNode string, state map[string]interface{}) ([]string, error) {
	// Check conditional edge first
	if ce, exists := cg.conditionalEdges[currentNode]; exists {
		routeKey, err := ce.Router(ctx, state)
		if err != nil {
			return nil, fmt.Errorf("router func failed: %w", err)
		}
		target, ok := ce.Branches[routeKey]
		if !ok {
			return nil, fmt.Errorf("router key '%s' not found in branch targets for node '%s'", routeKey, currentNode)
		}
		return []string{target}, nil
	}

	// Check direct static edges
	if targets, exists := cg.edges[currentNode]; exists && len(targets) > 0 {
		return targets, nil
	}

	// If marked as finish point
	if cg.finishPoints[currentNode] {
		return []string{NodeEnd}, nil
	}

	return []string{NodeEnd}, nil
}

func (cg *CompiledGraph) shouldInterrupt(node string, list []string) bool {
	for _, target := range list {
		if target == node {
			return true
		}
	}
	return false
}

func (cg *CompiledGraph) initializeState(initial map[string]interface{}) map[string]interface{} {
	state := make(map[string]interface{})
	for name, ch := range cg.channels {
		state[name] = ch.DefaultValue
	}
	for k, v := range initial {
		state[k] = v
	}
	return state
}

func (cg *CompiledGraph) applyStateUpdates(state, updates map[string]interface{}) error {
	for k, v := range updates {
		reducer := ReducerOverwrite
		if ch, exists := cg.channels[k]; exists && ch.Reducer != nil {
			reducer = ch.Reducer
		}
		reduced, err := reducer(state[k], v)
		if err != nil {
			return err
		}
		state[k] = reduced
	}
	return nil
}

// GetCheckpoint retrieves a historical checkpoint by its ID.
func (cg *CompiledGraph) GetCheckpoint(checkpointID string) (*Checkpoint, bool) {
	return cg.store.Get(checkpointID)
}

// ListCheckpoints returns all checkpoints for a thread.
func (cg *CompiledGraph) ListCheckpoints(threadID string) []*Checkpoint {
	return cg.store.List(threadID)
}

// ExportMermaid renders a complete Mermaid state flowchart visualizing graph structure and active trace path.
func (cg *CompiledGraph) ExportMermaid(trace *ExecutionTrace) string {
	var b strings.Builder
	b.WriteString("flowchart TD\n")
	b.WriteString("    %% Nodes\n")
	b.WriteString("    START([Start]):::startEnd\n")

	for name := range cg.nodes {
		b.WriteString(fmt.Sprintf("    %s[\"%s\"]\n", name, name))
	}
	b.WriteString("    END([End]):::startEnd\n\n")

	b.WriteString("    %% Edges\n")
	if targets, ok := cg.edges[NodeStart]; ok {
		for _, to := range targets {
			b.WriteString(fmt.Sprintf("    START --> %s\n", to))
		}
	}

	for from, targets := range cg.edges {
		if from == NodeStart {
			continue
		}
		for _, to := range targets {
			if to == NodeEnd {
				b.WriteString(fmt.Sprintf("    %s --> END\n", from))
			} else {
				b.WriteString(fmt.Sprintf("    %s --> %s\n", from, to))
			}
		}
	}

	for from, ce := range cg.conditionalEdges {
		for branchKey, target := range ce.Branches {
			if target == NodeEnd {
				b.WriteString(fmt.Sprintf("    %s -. \"%s\" .-> END\n", from, branchKey))
			} else {
				b.WriteString(fmt.Sprintf("    %s -. \"%s\" .-> %s\n", from, branchKey, target))
			}
		}
	}

	if trace != nil && len(trace.ExecutionPath) > 0 {
		b.WriteString("\n    %% Traversed Execution Path\n")
		pathSet := make(map[string]bool)
		for _, p := range trace.ExecutionPath {
			pathSet[p] = true
		}
		var pathNodes []string
		for p := range pathSet {
			pathNodes = append(pathNodes, p)
		}
		b.WriteString(fmt.Sprintf("    classDef activePath fill:#4CAF50,stroke:#388E3C,stroke-width:3px,color:#fff;\n"))
		b.WriteString(fmt.Sprintf("    class %s activePath;\n", strings.Join(pathNodes, ",")))
	}

	b.WriteString("    classDef startEnd fill:#2196F3,stroke:#1976D2,stroke-width:2px,color:#fff;\n")

	return b.String()
}
