package graph

import (
	"encoding/json"
	"fmt"
	"time"
)

// Standard graph special node constants.
const (
	NodeStart = "__start__"
	NodeEnd   = "__end__"
)

// GraphStatus indicates the lifecycle state of a graph execution thread.
type GraphStatus string

const (
	StatusRunning                GraphStatus = "running"
	StatusCompleted              GraphStatus = "completed"
	StatusInterrupted            GraphStatus = "interrupted"
	StatusRecursionLimitExceeded GraphStatus = "recursion_limit_exceeded"
	StatusFailed                 GraphStatus = "failed"
)

// ReducerFunc resolves state channel updates when new data is emitted by a node.
type ReducerFunc func(current, update interface{}) (interface{}, error)

// Standard built-in state channel reducers.
func ReducerOverwrite(current, update interface{}) (interface{}, error) {
	return update, nil
}

func ReducerAppendList(current, update interface{}) (interface{}, error) {
	if current == nil {
		current = []interface{}{}
	}
	var list []interface{}
	switch v := current.(type) {
	case []interface{}:
		list = append([]interface{}{}, v...)
	case []string:
		for _, s := range v {
			list = append(list, s)
		}
	default:
		list = []interface{}{current}
	}

	switch v := update.(type) {
	case []interface{}:
		list = append(list, v...)
	case []string:
		for _, s := range v {
			list = append(list, s)
		}
	default:
		list = append(list, update)
	}
	return list, nil
}

func ReducerMergeMap(current, update interface{}) (interface{}, error) {
	res := make(map[string]interface{})
	if cMap, ok := current.(map[string]interface{}); ok {
		for k, v := range cMap {
			res[k] = v
		}
	}
	if uMap, ok := update.(map[string]interface{}); ok {
		for k, v := range uMap {
			res[k] = v
		}
	}
	return res, nil
}

// StateChannel defines the schema and reducer for a named state variable.
type StateChannel struct {
	Name         string      `json:"name"`
	DefaultValue interface{} `json:"defaultValue,omitempty"`
	Reducer      ReducerFunc `json:"-"`
}

// GraphContext provides execution metadata and helper utilities to node functions.
type GraphContext struct {
	ThreadID     string                 `json:"threadId"`
	Step         int                    `json:"step"`
	ActiveNode   string                 `json:"activeNode"`
	CheckpointID string                 `json:"checkpointId"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// NodeFunc is the logic executed by a graph worker node.
// It receives immutable state snapshot and emits state updates.
type NodeFunc func(ctx *GraphContext, state map[string]interface{}) (map[string]interface{}, error)

// RouterFunc evaluates state and determines the next target node identifier.
type RouterFunc func(ctx *GraphContext, state map[string]interface{}) (string, error)

// Checkpoint represents an immutable snapshot of graph state at a specific step.
type Checkpoint struct {
	CheckpointID       string                 `json:"checkpointId"`
	ThreadID           string                 `json:"threadId"`
	Step               int                    `json:"step"`
	NodeName           string                 `json:"nodeName"`
	NextNodes          []string               `json:"nextNodes"`
	State              map[string]interface{} `json:"state"`
	ParentCheckpointID string                 `json:"parentCheckpointId,omitempty"`
	Timestamp          time.Time              `json:"timestamp"`
	DurationMs         int64                  `json:"durationMs"`
	Error              string                 `json:"error,omitempty"`
}

func deepCloneValue(val interface{}) interface{} {
	if val == nil {
		return nil
	}
	switch v := val.(type) {
	case map[string]interface{}:
		m := make(map[string]interface{}, len(v))
		for k, valItem := range v {
			m[k] = deepCloneValue(valItem)
		}
		return m
	case []interface{}:
		s := make([]interface{}, len(v))
		for i, valItem := range v {
			s[i] = deepCloneValue(valItem)
		}
		return s
	case []string:
		s := make([]string, len(v))
		copy(s, v)
		return s
	default:
		return v
	}
}

// Clone creates a deep copy of the checkpoint state map.
func (c *Checkpoint) CloneState() map[string]interface{} {
	return DeepCloneState(c.State)
}

// HumanInterrupt represents a pause gate requiring human approval or state mutation.
type HumanInterrupt struct {
	InterruptID    string                 `json:"interruptId"`
	Type           string                 `json:"type"` // "before" or "after"
	NodeName       string                 `json:"nodeName"`
	State          map[string]interface{} `json:"state"`
	Resumed        bool                   `json:"resumed"`
	InjectedValues map[string]interface{} `json:"injectedValues,omitempty"`
	Timestamp      time.Time              `json:"timestamp"`
}

// GraphConfig specifies execution safety boundaries and interrupt gates.
type GraphConfig struct {
	MaxRecursionLimit int      `json:"maxRecursionLimit"` // Default: 25
	InterruptBefore   []string `json:"interruptBefore,omitempty"`
	InterruptAfter    []string `json:"interruptAfter,omitempty"`
}

// DefaultGraphConfig provides standard production defaults.
func DefaultGraphConfig() GraphConfig {
	return GraphConfig{
		MaxRecursionLimit: 25,
	}
}

// ExecutionTrace captures the full deterministic time-travel trajectory of a graph execution.
type ExecutionTrace struct {
	ThreadID         string                 `json:"threadId"`
	Status           GraphStatus            `json:"status"`
	TotalSteps       int                    `json:"totalSteps"`
	ExecutionPath    []string               `json:"executionPath"`
	Checkpoints      []*Checkpoint          `json:"checkpoints"`
	ActiveCheckpoint *Checkpoint            `json:"activeCheckpoint,omitempty"`
	Interrupt        *HumanInterrupt        `json:"interrupt,omitempty"`
	FinalState       map[string]interface{} `json:"finalState"`
	TotalDurationMs  int64                  `json:"totalDurationMs"`
	Error            string                 `json:"error,omitempty"`
	CycleCount       int                    `json:"cycleCount"`
}

// DeepCloneState helper to safely clone arbitrary map state.
func DeepCloneState(in map[string]interface{}) map[string]interface{} {
	if in == nil {
		return make(map[string]interface{})
	}
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = deepCloneValue(v)
	}
	return out
}

func FormatStateJSON(state map[string]interface{}) string {
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", state)
	}
	return string(b)
}
