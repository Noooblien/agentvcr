package debugger

import (
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// DebuggerState represents the execution lifecycle of the debugger.
type DebuggerState string

const (
	StateUninitialized DebuggerState = "UNINITIALIZED"
	StateRunning       DebuggerState = "RUNNING"
	StatePaused        DebuggerState = "PAUSED"
	StateTerminated    DebuggerState = "TERMINATED"
	StateFaulted       DebuggerState = "FAULTED"
)

// BreakpointType defines the mechanism that triggers a breakpoint pause.
type BreakpointType string

const (
	BreakStep           BreakpointType = "STEP"
	BreakToolCall       BreakpointType = "TOOL_CALL"
	BreakTokenThreshold BreakpointType = "TOKEN_THRESHOLD"
	BreakError          BreakpointType = "ERROR"
	BreakCondition      BreakpointType = "CONDITION"
	BreakRegex          BreakpointType = "REGEX"
)

// StopReason describes why the debugger paused execution.
type StopReason string

const (
	ReasonNone               StopReason = "NONE"
	ReasonBreakpointHit      StopReason = "BREAKPOINT_HIT"
	ReasonWatchpointViolated StopReason = "WATCHPOINT_VIOLATED"
	ReasonStepCompleted      StopReason = "STEP_COMPLETED"
	ReasonStepBackCompleted  StopReason = "STEP_BACK_COMPLETED"
	ReasonException          StopReason = "EXCEPTION"
	ReasonEndOfTrace         StopReason = "END_OF_TRACE"
	ReasonManualPause        StopReason = "MANUAL_PAUSE"
)

// Breakpoint defines a pause condition in the agent's execution trajectory.
type Breakpoint struct {
	ID            string         `json:"id"`
	Type          BreakpointType `json:"type"`
	Target        string         `json:"target,omitempty"`         // E.g. tool name, regex pattern
	StepNumber    int            `json:"step_number,omitempty"`    // For BreakStep
	TokenLimit    int            `json:"token_limit,omitempty"`    // For BreakTokenThreshold
	ConditionExpr string         `json:"condition_expr,omitempty"` // E.g. "arg contains 'rm' && memory['retries'] > 2"
	HitCount      int            `json:"hit_count"`
	HitLimit      int            `json:"hit_limit,omitempty"`      // 0 for unlimited, or break only after N hits
	Enabled       bool           `json:"enabled"`
	LastHitStep   int            `json:"last_hit_step,omitempty"`
}

// Watchpoint monitors an invariant or memory variable across all steps.
type Watchpoint struct {
	ID             string      `json:"id"`
	VariableKey    string      `json:"variable_key"`
	Operator       string      `json:"operator"` // ==, !=, <, >, <=, >=, contains, not_empty
	ExpectedValue  interface{} `json:"expected_value"`
	CurrentValue   interface{} `json:"current_value,omitempty"`
	Violated       bool        `json:"violated"`
	ViolationCount int         `json:"violation_count"`
	Enabled        bool        `json:"enabled"`
}

// StackFrame represents one invocation frame in an agent swarm or tool call hierarchy.
type StackFrame struct {
	Level          int                    `json:"level"`
	AgentID        string                 `json:"agent_id"`
	AgentRole      string                 `json:"agent_role"`
	CurrentAction  string                 `json:"current_action"`
	StepIndex      int                    `json:"step_index"`
	Timestamp      time.Time              `json:"timestamp"`
	Variables      map[string]interface{} `json:"variables"`
	SourceLocation string                 `json:"source_location,omitempty"`
}

// HistorySnapshot preserves the full state of a single step to allow instant time-travel rewind.
type HistorySnapshot struct {
	StepIndex     int                    `json:"step_index"`
	Event         trace.TraceEvent       `json:"event"`
	StackFrames   []StackFrame           `json:"stack_frames"`
	WorkingMemory map[string]interface{} `json:"working_memory"`
	CumulativeTokens int                 `json:"cumulative_tokens"`
	Timestamp     time.Time              `json:"timestamp"`
}

// DebugEvent carries telemetry when state changes or a breakpoint is triggered.
type DebugEvent struct {
	Type          string                 `json:"type"` // e.g. "breakpoint_hit", "watchpoint_violated", "paused"
	Timestamp     time.Time              `json:"timestamp"`
	StepIndex     int                    `json:"step_index"`
	StopReason    StopReason             `json:"stop_reason"`
	Message       string                 `json:"message"`
	BreakpointID  string                 `json:"breakpoint_id,omitempty"`
	WatchpointID  string                 `json:"watchpoint_id,omitempty"`
	ActiveFrame   *StackFrame            `json:"active_frame,omitempty"`
	Payload       map[string]interface{} `json:"payload,omitempty"`
}

// DebugSessionSummary encapsulates active debugger status for inspection and scorecard rendering.
type DebugSessionSummary struct {
	SessionID            string                  `json:"session_id"`
	AgentName            string                  `json:"agent_name"`
	PC                   int                     `json:"pc"`
	TotalSteps           int                     `json:"total_steps"`
	State                DebuggerState           `json:"state"`
	LastStopReason       StopReason              `json:"last_stop_reason"`
	CurrentEvent         *trace.TraceEvent       `json:"current_event,omitempty"`
	CallStack            []StackFrame            `json:"call_stack"`
	WorkingMemory        map[string]interface{}  `json:"working_memory"`
	CumulativeTokens     int                     `json:"cumulative_tokens"`
	ActiveBreakpoints    []Breakpoint            `json:"active_breakpoints"`
	ActiveWatchpoints    []Watchpoint            `json:"active_watchpoints"`
	CanStepBack          bool                    `json:"can_step_back"`
	CanStepForward       bool                    `json:"can_step_forward"`
}
