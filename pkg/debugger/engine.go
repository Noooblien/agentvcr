package debugger

import (
	"fmt"
	"sync"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// DebuggerEngine orchestrates live and replay time-travel debugging.
type DebuggerEngine struct {
	mu               sync.RWMutex
	session          *trace.Session
	pc               int // Current event index in session.Events
	state            DebuggerState
	lastStopReason   StopReason
	lastStopMessage  string
	breakpoints      map[string]*Breakpoint
	watchpoints      map[string]*Watchpoint
	evaluator        *ConditionEvaluator
	workingMemory    map[string]interface{}
	cumulativeTokens int
	callStack        []StackFrame
	historySnapshots []HistorySnapshot
	eventListeners   []func(DebugEvent)
	dapAdapter       *DAPAdapter
	bpCounter        int
	wpCounter        int
}

// NewDebuggerEngine initializes a debugger for a trace session.
func NewDebuggerEngine(session *trace.Session) *DebuggerEngine {
	if session == nil {
		session = &trace.Session{
			ID:        "debug-empty-session",
			AgentName: "orchestrator",
			CreatedAt: time.Now(),
		}
	}

	initialMemory := make(map[string]interface{})
	if session.Metadata != nil {
		for k, v := range session.Metadata {
			initialMemory[k] = v
		}
	}

	initialStack := []StackFrame{
		{
			Level:         0,
			AgentID:       session.AgentName,
			AgentRole:     "Orchestrator",
			CurrentAction: "initialize",
			StepIndex:     0,
			Timestamp:     time.Now(),
			Variables:     initialMemory,
		},
	}

	return &DebuggerEngine{
		session:          session,
		pc:               0,
		state:            StatePaused,
		lastStopReason:   ReasonNone,
		breakpoints:      make(map[string]*Breakpoint),
		watchpoints:      make(map[string]*Watchpoint),
		evaluator:        NewConditionEvaluator(),
		workingMemory:    initialMemory,
		cumulativeTokens: 0,
		callStack:        initialStack,
		historySnapshots: make([]HistorySnapshot, 0),
		eventListeners:   make([]func(DebugEvent), 0),
		dapAdapter:       NewDAPAdapter(),
	}
}

// AddBreakpoint registers a new breakpoint condition.
func (e *DebuggerEngine) AddBreakpoint(bp Breakpoint) string {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.bpCounter++
	if bp.ID == "" {
		bp.ID = fmt.Sprintf("bp-%d", e.bpCounter)
	}
	bp.Enabled = true
	bpCopy := bp
	e.breakpoints[bp.ID] = &bpCopy
	return bp.ID
}

// RemoveBreakpoint removes a breakpoint by ID.
func (e *DebuggerEngine) RemoveBreakpoint(id string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, exists := e.breakpoints[id]; exists {
		delete(e.breakpoints, id)
		return true
	}
	return false
}

// ToggleBreakpoint enables or disables a breakpoint.
func (e *DebuggerEngine) ToggleBreakpoint(id string) (bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	bp, exists := e.breakpoints[id]
	if !exists {
		return false, fmt.Errorf("breakpoint not found: %s", id)
	}
	bp.Enabled = !bp.Enabled
	return bp.Enabled, nil
}

// GetBreakpoints lists all registered breakpoints.
func (e *DebuggerEngine) GetBreakpoints() []Breakpoint {
	e.mu.RLock()
	defer e.mu.RUnlock()

	list := make([]Breakpoint, 0, len(e.breakpoints))
	for _, bp := range e.breakpoints {
		list = append(list, *bp)
	}
	return list
}

// AddWatchpoint registers a memory variable invariant watchpoint.
func (e *DebuggerEngine) AddWatchpoint(wp Watchpoint) string {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.wpCounter++
	if wp.ID == "" {
		wp.ID = fmt.Sprintf("wp-%d", e.wpCounter)
	}
	wp.Enabled = true
	wpCopy := wp
	e.watchpoints[wp.ID] = &wpCopy
	return wp.ID
}

// RemoveWatchpoint deletes a watchpoint by ID.
func (e *DebuggerEngine) RemoveWatchpoint(id string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, exists := e.watchpoints[id]; exists {
		delete(e.watchpoints, id)
		return true
	}
	return false
}

// GetWatchpoints returns all registered watchpoints.
func (e *DebuggerEngine) GetWatchpoints() []Watchpoint {
	e.mu.RLock()
	defer e.mu.RUnlock()

	list := make([]Watchpoint, 0, len(e.watchpoints))
	for _, wp := range e.watchpoints {
		list = append(list, *wp)
	}
	return list
}

// Subscribe registers an event listener callback.
func (e *DebuggerEngine) Subscribe(fn func(DebugEvent)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.eventListeners = append(e.eventListeners, fn)
}

func (e *DebuggerEngine) emitEvent(event DebugEvent) {
	for _, fn := range e.eventListeners {
		fn(event)
	}
}

// StepInto executes the current event at PC and advances the counter by 1.
func (e *DebuggerEngine) StepInto() (*DebugEvent, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.pc >= len(e.session.Events) {
		e.state = StateTerminated
		e.lastStopReason = ReasonEndOfTrace
		e.lastStopMessage = "Reached end of execution trace"
		evt := DebugEvent{
			Type:       "terminated",
			Timestamp:  time.Now(),
			StepIndex:  e.pc,
			StopReason: ReasonEndOfTrace,
			Message:    e.lastStopMessage,
		}
		e.emitEvent(evt)
		return &evt, nil
	}

	currentEvent := e.session.Events[e.pc]

	// 1. Save history snapshot for time-travel step back
	snapshot := HistorySnapshot{
		StepIndex:        e.pc,
		Event:            currentEvent,
		StackFrames:      cloneStack(e.callStack),
		WorkingMemory:    cloneMemory(e.workingMemory),
		CumulativeTokens: e.cumulativeTokens,
		Timestamp:        time.Now(),
	}
	e.historySnapshots = append(e.historySnapshots, snapshot)

	// 2. Process event tokens
	if tokens := extractTokens(&currentEvent); tokens > 0 {
		e.cumulativeTokens += tokens
	}

	// 3. Process Call Stack mutations
	switch currentEvent.Type {
	case trace.EventToolCall:
		toolName := extractToolName(&currentEvent)
		newFrame := StackFrame{
			Level:         len(e.callStack),
			AgentID:       e.session.AgentName,
			AgentRole:     "ToolExecutor",
			CurrentAction: fmt.Sprintf("call:%s", toolName),
			StepIndex:     e.pc,
			Timestamp:     currentEvent.Timestamp,
			Variables:     cloneMemory(currentEvent.Payload),
		}
		e.callStack = append(e.callStack, newFrame)

	case trace.EventToolResult:
		if len(e.callStack) > 1 {
			e.callStack = e.callStack[:len(e.callStack)-1]
		}
	}

	// 4. Update memory from payload mutations if present
	if stateChanges, ok := currentEvent.Payload["state_mutation"].(map[string]interface{}); ok {
		for k, v := range stateChanges {
			e.workingMemory[k] = v
		}
	}

	// Advance PC
	executedStep := e.pc
	e.pc++

	// 5. Check Breakpoints
	for _, bp := range e.breakpoints {
		if e.evaluator.MatchesBreakpoint(bp, &currentEvent, executedStep, e.cumulativeTokens, e.workingMemory) {
			bp.HitCount++
			bp.LastHitStep = executedStep

			if bp.HitLimit == 0 || bp.HitCount >= bp.HitLimit {
				e.state = StatePaused
				e.lastStopReason = ReasonBreakpointHit
				e.lastStopMessage = fmt.Sprintf("Breakpoint %s triggered on step %d", bp.ID, executedStep)
				evt := DebugEvent{
					Type:         "breakpoint_hit",
					Timestamp:    time.Now(),
					StepIndex:    executedStep,
					StopReason:   ReasonBreakpointHit,
					Message:      e.lastStopMessage,
					BreakpointID: bp.ID,
					ActiveFrame:  e.getActiveFrame(),
					Payload:      currentEvent.Payload,
				}
				e.emitEvent(evt)
				return &evt, nil
			}
		}
	}

	// 6. Check Watchpoints
	for _, wp := range e.watchpoints {
		violated, currentVal := e.evaluator.CheckWatchpoint(wp, e.workingMemory)
		wp.CurrentValue = currentVal
		if violated {
			wp.Violated = true
			wp.ViolationCount++
			e.state = StatePaused
			e.lastStopReason = ReasonWatchpointViolated
			e.lastStopMessage = fmt.Sprintf("Watchpoint %s violated: %s %s %v (got %v)",
				wp.ID, wp.VariableKey, wp.Operator, wp.ExpectedValue, currentVal)
			evt := DebugEvent{
				Type:         "watchpoint_violated",
				Timestamp:    time.Now(),
				StepIndex:    executedStep,
				StopReason:   ReasonWatchpointViolated,
				Message:      e.lastStopMessage,
				WatchpointID: wp.ID,
				ActiveFrame:  e.getActiveFrame(),
			}
			e.emitEvent(evt)
			return &evt, nil
		}
	}

	// Default pause on step completion
	e.state = StatePaused
	e.lastStopReason = ReasonStepCompleted
	e.lastStopMessage = fmt.Sprintf("Step %d completed successfully", executedStep)

	evt := DebugEvent{
		Type:        "step_completed",
		Timestamp:   time.Now(),
		StepIndex:   executedStep,
		StopReason:  ReasonStepCompleted,
		Message:     e.lastStopMessage,
		ActiveFrame: e.getActiveFrame(),
		Payload:     currentEvent.Payload,
	}
	e.emitEvent(evt)
	return &evt, nil
}

// StepOver advances through tool calls to the next agent turn or result.
func (e *DebuggerEngine) StepOver() (*DebugEvent, error) {
	e.mu.RLock()
	if e.pc >= len(e.session.Events) {
		e.mu.RUnlock()
		return e.StepInto()
	}
	currentEvent := e.session.Events[e.pc]
	e.mu.RUnlock()

	if currentEvent.Type != trace.EventToolCall {
		return e.StepInto()
	}

	initialStackLen := len(e.callStack)
	for {
		evt, err := e.StepInto()
		if err != nil {
			return evt, err
		}
		// If hit breakpoint or watchpoint or finished
		if evt.StopReason == ReasonBreakpointHit || evt.StopReason == ReasonWatchpointViolated || evt.StopReason == ReasonEndOfTrace {
			return evt, nil
		}
		e.mu.RLock()
		stackLen := len(e.callStack)
		pc := e.pc
		eventsLen := len(e.session.Events)
		e.mu.RUnlock()

		if stackLen <= initialStackLen || pc >= eventsLen {
			return evt, nil
		}
	}
}

// StepBack time-travels backwards by 1 step, restoring previous memory and state.
func (e *DebuggerEngine) StepBack() (*DebugEvent, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(e.historySnapshots) == 0 {
		return nil, fmt.Errorf("cannot step back: beginning of trace or no snapshots available")
	}

	// Pop the last snapshot
	lastIdx := len(e.historySnapshots) - 1
	snapshot := e.historySnapshots[lastIdx]
	e.historySnapshots = e.historySnapshots[:lastIdx]

	// Restore state
	e.pc = snapshot.StepIndex
	e.callStack = cloneStack(snapshot.StackFrames)
	e.workingMemory = cloneMemory(snapshot.WorkingMemory)
	e.cumulativeTokens = snapshot.CumulativeTokens
	e.state = StatePaused
	e.lastStopReason = ReasonStepBackCompleted
	e.lastStopMessage = fmt.Sprintf("Time-traveled back to step %d", e.pc)

	evt := DebugEvent{
		Type:        "step_back_completed",
		Timestamp:   time.Now(),
		StepIndex:   e.pc,
		StopReason:  ReasonStepBackCompleted,
		Message:     e.lastStopMessage,
		ActiveFrame: e.getActiveFrame(),
	}
	e.emitEvent(evt)
	return &evt, nil
}

// Continue resumes execution until a breakpoint triggers, watchpoint is violated, or trace finishes.
func (e *DebuggerEngine) Continue() (*DebugEvent, error) {
	for {
		evt, err := e.StepInto()
		if err != nil {
			return nil, err
		}
		if evt.StopReason == ReasonBreakpointHit ||
			evt.StopReason == ReasonWatchpointViolated ||
			evt.StopReason == ReasonEndOfTrace ||
			evt.StopReason == ReasonException {
			return evt, nil
		}
	}
}

// Restart rewinds the debugger to step 0 with clean state.
func (e *DebuggerEngine) Restart() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.pc = 0
	e.state = StatePaused
	e.lastStopReason = ReasonNone
	e.lastStopMessage = "Debugger restarted to step 0"
	e.cumulativeTokens = 0
	e.historySnapshots = make([]HistorySnapshot, 0)
	e.workingMemory = make(map[string]interface{})
	if e.session.Metadata != nil {
		for k, v := range e.session.Metadata {
			e.workingMemory[k] = v
		}
	}
	e.callStack = []StackFrame{
		{
			Level:         0,
			AgentID:       e.session.AgentName,
			AgentRole:     "Orchestrator",
			CurrentAction: "initialize",
			StepIndex:     0,
			Timestamp:     time.Now(),
			Variables:     cloneMemory(e.workingMemory),
		},
	}

	for _, bp := range e.breakpoints {
		bp.HitCount = 0
		bp.LastHitStep = 0
	}
	for _, wp := range e.watchpoints {
		wp.Violated = false
		wp.ViolationCount = 0
		wp.CurrentValue = nil
	}
	return nil
}

// MutateMemory allows live hot-patching of working memory variables.
func (e *DebuggerEngine) MutateMemory(key string, val interface{}) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.workingMemory[key] = val
	if len(e.callStack) > 0 {
		e.callStack[len(e.callStack)-1].Variables[key] = val
	}
}

// Summary generates an audit snapshot for inspect operations and scorecards.
func (e *DebuggerEngine) Summary() DebugSessionSummary {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var curEvent *trace.TraceEvent
	if e.pc < len(e.session.Events) {
		ev := e.session.Events[e.pc]
		curEvent = &ev
	}

	return DebugSessionSummary{
		SessionID:         e.session.ID,
		AgentName:         e.session.AgentName,
		PC:                e.pc,
		TotalSteps:        len(e.session.Events),
		State:             e.state,
		LastStopReason:    e.lastStopReason,
		CurrentEvent:      curEvent,
		CallStack:         cloneStack(e.callStack),
		WorkingMemory:     cloneMemory(e.workingMemory),
		CumulativeTokens:  e.cumulativeTokens,
		ActiveBreakpoints: e.GetBreakpoints(),
		ActiveWatchpoints: e.GetWatchpoints(),
		CanStepBack:       len(e.historySnapshots) > 0,
		CanStepForward:    e.pc < len(e.session.Events),
	}
}

func (e *DebuggerEngine) getActiveFrame() *StackFrame {
	if len(e.callStack) == 0 {
		return nil
	}
	f := e.callStack[len(e.callStack)-1]
	return &f
}

func extractTokens(event *trace.TraceEvent) int {
	if event == nil || event.Payload == nil {
		return 0
	}
	keys := []string{"tokens", "total_tokens", "token_count"}
	for _, k := range keys {
		if v, ok := event.Payload[k]; ok {
			if f, ok := v.(float64); ok {
				return int(f)
			}
			if i, ok := v.(int); ok {
				return i
			}
		}
	}
	return 0
}

func cloneStack(stack []StackFrame) []StackFrame {
	out := make([]StackFrame, len(stack))
	for i, f := range stack {
		out[i] = StackFrame{
			Level:          f.Level,
			AgentID:        f.AgentID,
			AgentRole:      f.AgentRole,
			CurrentAction:  f.CurrentAction,
			StepIndex:      f.StepIndex,
			Timestamp:      f.Timestamp,
			Variables:      cloneMemory(f.Variables),
			SourceLocation: f.SourceLocation,
		}
	}
	return out
}

func cloneMemory(mem map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(mem))
	for k, v := range mem {
		out[k] = v
	}
	return out
}
