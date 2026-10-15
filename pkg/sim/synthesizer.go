package sim

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// Synthesizer extracts declarative mock tool rules from recorded agent traces.
type Synthesizer struct{}

// NewSynthesizer returns an instance of Synthesizer.
func NewSynthesizer() *Synthesizer {
	return &Synthesizer{}
}

// SynthesizeFromSession inspects all events in a session and generates mock tool rules.
func (s *Synthesizer) SynthesizeFromSession(session *trace.Session) ([]MockRule, error) {
	if session == nil || len(session.Events) == 0 {
		return nil, fmt.Errorf("session contains no events to synthesize")
	}

	var rules []MockRule
	events := session.Events

	for i := 0; i < len(events); i++ {
		ev := events[i]
		if ev.Type != trace.EventToolCall {
			continue
		}

		toolName, args := ExtractToolInfo(ev.Payload)
		if toolName == "" {
			continue
		}

		// Look ahead for corresponding tool result
		var resultPayload interface{}
		status := "success"
		var errorMsg string
		latency := ev.DurationMs

		for j := i + 1; j < len(events); j++ {
			nextEv := events[j]
			if nextEv.Type == trace.EventToolResult {
				res, st, errStr := ExtractToolResult(nextEv.Payload)
				resultPayload = res
				status = st
				errorMsg = errStr
				if nextEv.DurationMs > 0 {
					latency = nextEv.DurationMs
				}
				break
			}
			// If another tool call occurs first, break lookahead
			if nextEv.Type == trace.EventToolCall {
				break
			}
		}

		ruleID := fmt.Sprintf("mock_%s_step%d", sanitizeToolName(toolName), ev.Index)
		rule := MockRule{
			ID:           ruleID,
			ToolName:     toolName,
			ArgMatch:     args,
			Response:     resultPayload,
			Status:       status,
			ErrorMessage: errorMsg,
			LatencyMs:    latency,
		}

		rules = append(rules, rule)
	}

	return rules, nil
}

// ExportRulesJSON serializes rules into formatted JSON.
func (s *Synthesizer) ExportRulesJSON(rules []MockRule) ([]byte, error) {
	return json.MarshalIndent(rules, "", "  ")
}

// ImportRulesJSON deserializes rules from JSON bytes.
func (s *Synthesizer) ImportRulesJSON(data []byte) ([]MockRule, error) {
	var rules []MockRule
	if err := json.Unmarshal(data, &rules); err != nil {
		return nil, fmt.Errorf("failed to parse mock rules: %w", err)
	}
	return rules, nil
}

// ExtractToolInfo extracts tool name and parameters from diverse agent payloads.
func ExtractToolInfo(payload map[string]interface{}) (string, map[string]interface{}) {
	if payload == nil {
		return "", nil
	}

	toolName := ""
	for _, key := range []string{"tool_name", "tool", "name", "method", "function"} {
		if v, ok := payload[key]; ok {
			if s, ok := v.(string); ok && s != "" {
				toolName = s
				break
			}
		}
	}

	var args map[string]interface{}
	for _, key := range []string{"params", "arguments", "args", "input"} {
		if v, ok := payload[key]; ok {
			if m, ok := v.(map[string]interface{}); ok {
				args = m
				break
			} else if s, ok := v.(string); ok {
				// Might be JSON encoded string
				var parsed map[string]interface{}
				if err := json.Unmarshal([]byte(s), &parsed); err == nil {
					args = parsed
					break
				}
			}
		}
	}

	return toolName, args
}

// ExtractToolResult extracts response content, status, and error string from tool result payloads.
func ExtractToolResult(payload map[string]interface{}) (interface{}, string, string) {
	if payload == nil {
		return nil, "success", ""
	}

	// Check for errors
	for _, key := range []string{"error", "err"} {
		if v, ok := payload[key]; ok && v != nil {
			errStr := fmt.Sprintf("%v", v)
			return nil, "error", errStr
		}
	}

	for _, key := range []string{"result", "output", "response", "data", "content"} {
		if v, ok := payload[key]; ok {
			return v, "success", ""
		}
	}

	return payload, "success", ""
}

func sanitizeToolName(name string) string {
	clean := strings.ReplaceAll(name, "/", "_")
	clean = strings.ReplaceAll(clean, ".", "_")
	clean = strings.ReplaceAll(clean, ":", "_")
	return clean
}
