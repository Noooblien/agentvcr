package eval

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Noooblien/agentvcr/pkg/metrics"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

// RuleType specifies the assertion category.
type RuleType string

const (
	RuleToolCallCount RuleType = "tool_call_count"
	RuleMaxTokens     RuleType = "max_tokens"
	RuleMaxCost       RuleType = "max_cost_usd"
	RuleMaxLatency    RuleType = "max_latency"
	RuleNoErrors      RuleType = "no_errors"
	RulePayloadMatch  RuleType = "payload_match"
)

// AssertionRule defines a verification rule to run against an agent trajectory.
type AssertionRule struct {
	Type        RuleType `json:"type"`
	ToolName    string   `json:"tool_name,omitempty"`
	MinCount    int      `json:"min_count,omitempty"`
	MaxCount    int      `json:"max_count,omitempty"`
	MaxTokens   int      `json:"max_tokens,omitempty"`
	MaxCostUSD  float64  `json:"max_cost_usd,omitempty"`
	MaxDuration string   `json:"max_duration,omitempty"` // e.g. "5s", "500ms"
	Substring   string   `json:"substring,omitempty"`
}

// AssertionResult captures the pass/fail state of a single evaluated rule.
type AssertionResult struct {
	Passed   bool     `json:"passed"`
	RuleType RuleType `json:"rule_type"`
	Details  string   `json:"details"`
}

// SuiteReport contains aggregated evaluation metrics and individual test results.
type SuiteReport struct {
	TotalRules int               `json:"total_rules"`
	Passed     int               `json:"passed"`
	Failed     int               `json:"failed"`
	AllPassed  bool              `json:"all_passed"`
	Results    []AssertionResult `json:"results"`
}

// Evaluate runs a set of assertions against an AgentVCR trace session.
func Evaluate(session *trace.Session, rules []AssertionRule) (*SuiteReport, error) {
	if session == nil {
		return nil, fmt.Errorf("session cannot be nil")
	}

	report := &SuiteReport{
		TotalRules: len(rules),
		Results:    make([]AssertionResult, 0, len(rules)),
	}

	summary := metrics.ComputeSessionMetrics(session)

	for _, rule := range rules {
		result := evaluateRule(session, summary, rule)
		if result.Passed {
			report.Passed++
		} else {
			report.Failed++
		}
		report.Results = append(report.Results, result)
	}

	report.AllPassed = (report.Failed == 0)
	return report, nil
}

func evaluateRule(session *trace.Session, summary metrics.SessionMetrics, rule AssertionRule) AssertionResult {
	switch rule.Type {
	case RuleToolCallCount:
		count := 0
		for _, ev := range session.Events {
			if ev.Type == trace.EventToolCall {
				toolName := extractToolName(ev.Payload)
				if rule.ToolName == "" || strings.EqualFold(toolName, rule.ToolName) {
					count++
				}
			}
		}

		passed := true
		var msg string
		if rule.MinCount > 0 && count < rule.MinCount {
			passed = false
			msg = fmt.Sprintf("Tool %q called %d times, expected at least %d", rule.ToolName, count, rule.MinCount)
		} else if rule.MaxCount > 0 && count > rule.MaxCount {
			passed = false
			msg = fmt.Sprintf("Tool %q called %d times, exceeded max limit %d", rule.ToolName, count, rule.MaxCount)
		} else {
			msg = fmt.Sprintf("Tool %q call count (%d) within expected bounds [%d, %d]", rule.ToolName, count, rule.MinCount, rule.MaxCount)
		}
		return AssertionResult{Passed: passed, RuleType: rule.Type, Details: msg}

	case RuleMaxTokens:
		passed := summary.TotalTokens <= rule.MaxTokens
		msg := fmt.Sprintf("Total tokens consumed: %d (max budget: %d)", summary.TotalTokens, rule.MaxTokens)
		if !passed {
			msg = fmt.Sprintf("Token budget exceeded: consumed %d > budget %d", summary.TotalTokens, rule.MaxTokens)
		}
		return AssertionResult{Passed: passed, RuleType: rule.Type, Details: msg}

	case RuleMaxCost:
		passed := summary.EstimatedCostUSD <= rule.MaxCostUSD
		msg := fmt.Sprintf("Total estimated cost: $%.5f (max budget: $%.5f)", summary.EstimatedCostUSD, rule.MaxCostUSD)
		if !passed {
			msg = fmt.Sprintf("Cost budget exceeded: $%.5f > $%.5f", summary.EstimatedCostUSD, rule.MaxCostUSD)
		}
		return AssertionResult{Passed: passed, RuleType: rule.Type, Details: msg}

	case RuleMaxLatency:
		maxDur, err := time.ParseDuration(rule.MaxDuration)
		if err != nil {
			return AssertionResult{Passed: false, RuleType: rule.Type, Details: fmt.Sprintf("invalid duration %q: %v", rule.MaxDuration, err)}
		}
		totalDuration := time.Duration(summary.TotalDurationMs) * time.Millisecond
		passed := totalDuration <= maxDur
		msg := fmt.Sprintf("Total execution latency: %s (limit: %s)", totalDuration, maxDur)
		if !passed {
			msg = fmt.Sprintf("Latency threshold exceeded: %s > %s", totalDuration, maxDur)
		}
		return AssertionResult{Passed: passed, RuleType: rule.Type, Details: msg}

	case RuleNoErrors:
		errorCount := 0
		var errorDetails []string
		for _, ev := range session.Events {
			if ev.Type == trace.EventError {
				errorCount++
				errorDetails = append(errorDetails, fmt.Sprintf("Event #%d: error event", ev.Index))
				continue
			}
			if ev.Type == trace.EventToolResult {
				if payloadBytes, err := json.Marshal(ev.Payload); err == nil {
					payloadStr := strings.ToLower(string(payloadBytes))
					if strings.Contains(payloadStr, "\"error\"") {
						errorCount++
						errorDetails = append(errorDetails, fmt.Sprintf("Event #%d: %s", ev.Index, string(payloadBytes)))
					}
				}
			}
		}
		passed := errorCount == 0
		msg := "No execution errors detected in trajectory"
		if !passed {
			msg = fmt.Sprintf("Found %d error(s): %s", errorCount, strings.Join(errorDetails, "; "))
		}
		return AssertionResult{Passed: passed, RuleType: rule.Type, Details: msg}

	case RulePayloadMatch:
		found := false
		for _, ev := range session.Events {
			if payloadBytes, err := json.Marshal(ev.Payload); err == nil {
				if strings.Contains(string(payloadBytes), rule.Substring) {
					found = true
					break
				}
			}
		}
		passed := found
		msg := fmt.Sprintf("Substring %q verified in trajectory events", rule.Substring)
		if !passed {
			msg = fmt.Sprintf("Substring %q was NOT observed in trajectory events", rule.Substring)
		}
		return AssertionResult{Passed: passed, RuleType: rule.Type, Details: msg}

	default:
		return AssertionResult{
			Passed:   false,
			RuleType: rule.Type,
			Details:  fmt.Sprintf("unknown rule type: %s", rule.Type),
		}
	}
}

func extractToolName(payload map[string]interface{}) string {
	if name, ok := payload["tool_name"].(string); ok && name != "" {
		return name
	}
	if name, ok := payload["tool"].(string); ok && name != "" {
		return name
	}
	if method, ok := payload["method"].(string); ok && method != "" {
		return method
	}
	if params, ok := payload["params"].(map[string]interface{}); ok {
		if name, ok := params["name"].(string); ok && name != "" {
			return name
		}
	}
	return ""
}
