package sim

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// FallbackHandlerFunc defines custom fallback logic when no mock rule matches.
type FallbackHandlerFunc func(toolName string, args map[string]interface{}) (interface{}, error)

// Simulator executes offline agent tool invocations against declarative mock rules and sandboxed state.
type Simulator struct {
	rules           []MockRule
	state           *SandboxState
	fallbackHandler FallbackHandlerFunc
}

// NewSimulator creates a new offline tool simulator with given rules.
func NewSimulator(rules []MockRule) *Simulator {
	return &Simulator{
		rules: rules,
		state: NewSandboxState(),
	}
}

// SetFallbackHandler registers a custom fallback handler for unmatched tool calls.
func (s *Simulator) SetFallbackHandler(fn FallbackHandlerFunc) {
	s.fallbackHandler = fn
}

// GetState returns the internal sandbox state store.
func (s *Simulator) GetState() *SandboxState {
	return s.state
}

// ExecuteTool evaluates a tool invocation against the registered mock rules and sandbox providers.
func (s *Simulator) ExecuteTool(ctx context.Context, toolName string, args map[string]interface{}) (*SimulatedResult, error) {
	// 1. Try matching registered mock rules
	for _, rule := range s.rules {
		if s.matchRule(&rule, toolName, args) {
			return s.executeRule(&rule, toolName, args)
		}
	}

	// 2. Check built-in mock providers (memory_kv, bash_sandbox, mock_search)
	if res, handled := s.executeBuiltin(toolName, args); handled {
		return res, nil
	}

	// 3. Check custom fallback handler
	if s.fallbackHandler != nil {
		resp, err := s.fallbackHandler(toolName, args)
		status := "success"
		if err != nil {
			status = "error"
			return &SimulatedResult{
				ToolName:    toolName,
				Status:      status,
				Response:    map[string]interface{}{"error": err.Error()},
				LatencyMs:   10,
				IsSynthetic: true,
			}, nil
		}
		return &SimulatedResult{
			ToolName:    toolName,
			Status:      status,
			Response:    resp,
			LatencyMs:   15,
			IsSynthetic: true,
		}, nil
	}

	// 4. Default synthetic stub
	return &SimulatedResult{
		ToolName: toolName,
		Status:   "success",
		Response: map[string]interface{}{
			"status":  "simulated_default",
			"tool":    toolName,
			"message": "offline mock tool execution stub",
		},
		LatencyMs:   5,
		IsSynthetic: true,
	}, nil
}

// matchRule checks if a tool invocation satisfies a MockRule's predicates.
func (s *Simulator) matchRule(rule *MockRule, toolName string, args map[string]interface{}) bool {
	// Match tool name (exact, regex, or wildcard)
	if rule.ToolName != "*" && rule.ToolName != "" {
		if rule.ToolName != toolName {
			matched, err := regexp.MatchString(rule.ToolName, toolName)
			if err != nil || !matched {
				return false
			}
		}
	}

	// Match arguments predicate
	if len(rule.ArgMatch) > 0 {
		if args == nil {
			return false
		}
		for key, expectedVal := range rule.ArgMatch {
			actualVal, exists := args[key]
			if !exists {
				return false
			}
			if !compareArgValues(expectedVal, actualVal) {
				return false
			}
		}
	}

	return true
}

func compareArgValues(expected, actual interface{}) bool {
	if expected == actual {
		return true
	}
	expectedStr := fmt.Sprintf("%v", expected)
	actualStr := fmt.Sprintf("%v", actual)
	if expectedStr == actualStr {
		return true
	}
	// Support substring match for text prompts/queries
	if strings.Contains(strings.ToLower(actualStr), strings.ToLower(expectedStr)) {
		return true
	}
	return false
}

// executeRule executes stateful operations and formats the result.
func (s *Simulator) executeRule(rule *MockRule, toolName string, args map[string]interface{}) (*SimulatedResult, error) {
	latency := rule.LatencyMs
	if latency <= 0 {
		latency = 20
	}

	// Stateful operation handling
	if rule.StatefulOp != "" && rule.StateKey != "" {
		switch strings.ToLower(rule.StatefulOp) {
		case "set":
			val := rule.Response
			if v, ok := args["value"]; ok {
				val = v
			}
			s.state.Set(rule.StateKey, val)
		case "get":
			if val, ok := s.state.Get(rule.StateKey); ok {
				return &SimulatedResult{
					RuleID:      rule.ID,
					ToolName:    toolName,
					Status:      "success",
					Response:    val,
					LatencyMs:   latency,
					IsSynthetic: true,
				}, nil
			}
		case "delete":
			s.state.Delete(rule.StateKey)
		case "append":
			val := rule.Response
			if v, ok := args["value"]; ok {
				val = v
			}
			s.state.Append(rule.StateKey, val)
		}
	}

	if rule.Status == "error" {
		errMsg := rule.ErrorMessage
		if errMsg == "" {
			errMsg = "simulated tool fault error"
		}
		return &SimulatedResult{
			RuleID:      rule.ID,
			ToolName:    toolName,
			Status:      "error",
			Response:    map[string]interface{}{"error": errMsg},
			LatencyMs:   latency,
			IsSynthetic: true,
		}, nil
	}

	if rule.Status == "timeout" {
		return &SimulatedResult{
			RuleID:      rule.ID,
			ToolName:    toolName,
			Status:      "timeout",
			Response:    map[string]interface{}{"error": "simulated tool execution timeout"},
			LatencyMs:   latency,
			IsSynthetic: true,
		}, nil
	}

	return &SimulatedResult{
		RuleID:      rule.ID,
		ToolName:    toolName,
		Status:      "success",
		Response:    rule.Response,
		LatencyMs:   latency,
		IsSynthetic: true,
	}, nil
}

// executeBuiltin provides built-in emulation for standard tool suites.
func (s *Simulator) executeBuiltin(toolName string, args map[string]interface{}) (*SimulatedResult, bool) {
	switch strings.ToLower(toolName) {
	case "memory_kv", "kv_store", "state_store":
		action, _ := args["action"].(string)
		key, _ := args["key"].(string)
		if key == "" {
			key = "default"
		}
		switch strings.ToLower(action) {
		case "set":
			s.state.Set(key, args["value"])
			return &SimulatedResult{
				ToolName:    toolName,
				Status:      "success",
				Response:    map[string]interface{}{"status": "ok", "key": key},
				LatencyMs:   10,
				IsSynthetic: true,
			}, true
		case "get":
			val, found := s.state.Get(key)
			return &SimulatedResult{
				ToolName:    toolName,
				Status:      "success",
				Response:    map[string]interface{}{"key": key, "value": val, "found": found},
				LatencyMs:   10,
				IsSynthetic: true,
			}, true
		case "delete":
			s.state.Delete(key)
			return &SimulatedResult{
				ToolName:    toolName,
				Status:      "success",
				Response:    map[string]interface{}{"status": "deleted", "key": key},
				LatencyMs:   10,
				IsSynthetic: true,
			}, true
		}

	case "bash_sandbox", "bash", "exec", "terminal":
		cmd, _ := args["command"].(string)
		if cmd == "" {
			cmd, _ = args["cmd"].(string)
		}
		output := "simulated command execution"
		trimmed := strings.TrimSpace(cmd)
		if strings.HasPrefix(trimmed, "echo ") {
			output = strings.TrimPrefix(trimmed, "echo ")
		} else if trimmed == "pwd" {
			output = "/workspace/sandbox"
		} else if trimmed == "whoami" {
			output = "sandbox_user"
		} else if trimmed == "date" {
			output = time.Now().UTC().Format(time.RFC3339)
		}
		return &SimulatedResult{
			ToolName: toolName,
			Status:   "success",
			Response: map[string]interface{}{
				"stdout":    output,
				"exit_code": 0,
			},
			LatencyMs:   35,
			IsSynthetic: true,
		}, true

	case "mock_search", "web_search", "search":
		query, _ := args["query"].(string)
		if query == "" {
			query, _ = args["q"].(string)
		}
		return &SimulatedResult{
			ToolName: toolName,
			Status:   "success",
			Response: map[string]interface{}{
				"results": []map[string]string{
					{
						"title":   fmt.Sprintf("Search Result for '%s'", query),
						"snippet": "Synthetic web search snippet for offline testing.",
						"url":     "https://agentvcr.local/mock-doc",
					},
				},
			},
			LatencyMs:   80,
			IsSynthetic: true,
		}, true
	}

	return nil, false
}

// SimulateSession replays a recorded trajectory session offline, replacing live tool calls with mock synthesizers.
func (s *Simulator) SimulateSession(baseSession *trace.Session, newSessionID string) (*trace.Session, *SimulationReport, error) {
	if baseSession == nil {
		return nil, nil, fmt.Errorf("baseSession is nil")
	}

	simSession := &trace.Session{
		ID:         newSessionID,
		AgentName:  baseSession.AgentName + "-simulated",
		CreatedAt:  time.Now(),
		ParentID:   baseSession.ID,
		ForkAtStep: 0,
		Metadata: map[string]string{
			"simulator":       "agentvcr_synthetic",
			"base_session_id": baseSession.ID,
			"simulated_at":    time.Now().Format(time.RFC3339),
		},
		Status: "completed",
	}

	report := &SimulationReport{
		SessionID:     newSessionID,
		BaseSessionID: baseSession.ID,
	}

	prevChecksum := "0000000000000000000000000000000000000000000000000000000000000000"
	simTime := simSession.CreatedAt
	stepIndex := 0

	for i := 0; i < len(baseSession.Events); i++ {
		ev := baseSession.Events[i]

		// Skip live tool results from the base session; we synthesize new ones for each tool call
		if ev.Type == trace.EventToolResult {
			continue
		}

		stepIndex++
		simTime = simTime.Add(time.Duration(ev.DurationMs) * time.Millisecond)

		// Create copied event
		payloadBytes, _ := json.Marshal(ev.Payload)
		chk := trace.ComputeChecksum(prevChecksum, stepIndex, ev.Type, payloadBytes)
		prevChecksum = chk

		simEvent := trace.TraceEvent{
			ID:         fmt.Sprintf("sim_%d_%d", simTime.UnixNano(), stepIndex),
			Index:      stepIndex,
			Timestamp:  simTime,
			Type:       ev.Type,
			DurationMs: ev.DurationMs,
			Payload:    ev.Payload,
			Checksum:   chk,
		}
		simSession.Events = append(simSession.Events, simEvent)

		// If this is a tool call, run the simulation and generate a corresponding EventToolResult
		if ev.Type == trace.EventToolCall {
			toolName, args := ExtractToolInfo(ev.Payload)
			report.ToolCallsSimulated++

			simRes, err := s.ExecuteTool(context.Background(), toolName, args)
			if err != nil {
				simRes = &SimulatedResult{
					ToolName:    toolName,
					Status:      "error",
					Response:    map[string]interface{}{"error": err.Error()},
					LatencyMs:   10,
					IsSynthetic: true,
				}
			}

			if simRes.RuleID != "" {
				report.RulesMatched++
			} else {
				report.FallbacksUsed++
			}

			if simRes.Status == "error" || simRes.Status == "timeout" {
				report.ErrorsInjected++
			}
			report.SimulatedLatencyMs += simRes.LatencyMs

			// Generate synthetic tool result event
			stepIndex++
			simTime = simTime.Add(time.Duration(simRes.LatencyMs) * time.Millisecond)

			resultPayload := map[string]interface{}{
				"tool_name":    toolName,
				"status":       simRes.Status,
				"result":       simRes.Response,
				"is_synthetic": true,
			}
			if simRes.RuleID != "" {
				resultPayload["rule_id"] = simRes.RuleID
			}

			resBytes, _ := json.Marshal(resultPayload)
			resChk := trace.ComputeChecksum(prevChecksum, stepIndex, trace.EventToolResult, resBytes)
			prevChecksum = resChk

			resEvent := trace.TraceEvent{
				ID:         fmt.Sprintf("sim_res_%d_%d", simTime.UnixNano(), stepIndex),
				Index:      stepIndex,
				Timestamp:  simTime,
				Type:       trace.EventToolResult,
				DurationMs: simRes.LatencyMs,
				Payload:    resultPayload,
				Checksum:   resChk,
			}
			simSession.Events = append(simSession.Events, resEvent)
		}
	}

	simSession.UpdatedAt = simTime
	report.TotalEvents = len(simSession.Events)

	return simSession, report, nil
}
