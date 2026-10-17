package debugger

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// ConditionEvaluator evaluates breakpoint conditions and watchpoint invariants.
type ConditionEvaluator struct{}

// NewConditionEvaluator instantiates a new expression evaluator.
func NewConditionEvaluator() *ConditionEvaluator {
	return &ConditionEvaluator{}
}

// MatchesBreakpoint determines if a breakpoint triggers on the current event/state.
func (e *ConditionEvaluator) MatchesBreakpoint(
	bp *Breakpoint,
	event *trace.TraceEvent,
	step int,
	cumulativeTokens int,
	memory map[string]interface{},
) bool {
	if !bp.Enabled {
		return false
	}

	switch bp.Type {
	case BreakStep:
		return step == bp.StepNumber

	case BreakToolCall:
		if event.Type != trace.EventToolCall {
			return false
		}
		if bp.Target == "" {
			return true
		}
		toolName := extractToolName(event)
		return strings.EqualFold(toolName, bp.Target)

	case BreakTokenThreshold:
		return cumulativeTokens >= bp.TokenLimit

	case BreakError:
		if event.Type == trace.EventError {
			return true
		}
		if val, exists := event.Payload["error"]; exists && val != nil && val != "" {
			return true
		}
		if val, exists := event.Payload["is_error"]; exists {
			if b, ok := val.(bool); ok && b {
				return true
			}
		}
		return false

	case BreakRegex:
		if bp.Target == "" {
			return false
		}
		re, err := regexp.Compile(bp.Target)
		if err != nil {
			return false
		}
		payloadBytes, _ := json.Marshal(event.Payload)
		return re.Match(payloadBytes)

	case BreakCondition:
		return e.EvaluateExpression(bp.ConditionExpr, event, step, cumulativeTokens, memory)

	default:
		return false
	}
}

// CheckWatchpoint validates a state invariant against active working memory.
func (e *ConditionEvaluator) CheckWatchpoint(wp *Watchpoint, memory map[string]interface{}) (bool, interface{}) {
	if !wp.Enabled {
		return false, nil
	}

	currentVal, exists := memory[wp.VariableKey]
	if !exists {
		// If checking for not_empty, absence is a violation
		if wp.Operator == "not_empty" {
			return true, nil
		}
		return false, nil
	}

	violated := false
	switch wp.Operator {
	case "==":
		violated = (fmt.Sprintf("%v", currentVal) == fmt.Sprintf("%v", wp.ExpectedValue))
	case "!=":
		violated = (fmt.Sprintf("%v", currentVal) != fmt.Sprintf("%v", wp.ExpectedValue))
	case "<":
		violated = compareNumeric(currentVal, wp.ExpectedValue) < 0
	case ">":
		violated = compareNumeric(currentVal, wp.ExpectedValue) > 0
	case "<=":
		violated = compareNumeric(currentVal, wp.ExpectedValue) <= 0
	case ">=":
		violated = compareNumeric(currentVal, wp.ExpectedValue) >= 0
	case "contains":
		violated = strings.Contains(fmt.Sprintf("%v", currentVal), fmt.Sprintf("%v", wp.ExpectedValue))
	case "not_empty":
		violated = fmt.Sprintf("%v", currentVal) == ""
	}

	return violated, currentVal
}

// EvaluateExpression evaluates basic boolean predicates like:
// "tool == 'bash' && arg contains 'rm'" or "step > 5" or "tokens > 1000" or "memory['key'] == 'val'".
func (e *ConditionEvaluator) EvaluateExpression(
	expr string,
	event *trace.TraceEvent,
	step int,
	cumulativeTokens int,
	memory map[string]interface{},
) bool {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return false
	}

	// Split by logical AND (&&)
	subConditions := strings.Split(expr, "&&")
	for _, sub := range subConditions {
		clause := strings.TrimSpace(sub)
		if !e.evaluateClause(clause, event, step, cumulativeTokens, memory) {
			return false
		}
	}
	return true
}

func (e *ConditionEvaluator) evaluateClause(
	clause string,
	event *trace.TraceEvent,
	step int,
	cumulativeTokens int,
	memory map[string]interface{},
) bool {
	clause = strings.TrimSpace(clause)
	if clause == "" {
		return true
	}

	// Check for 'contains'
	if strings.Contains(clause, " contains ") {
		parts := strings.SplitN(clause, " contains ", 2)
		lhs := strings.TrimSpace(parts[0])
		rhs := cleanQuotes(strings.TrimSpace(parts[1]))
		lhsVal := e.resolveValue(lhs, event, step, cumulativeTokens, memory)
		return strings.Contains(fmt.Sprintf("%v", lhsVal), rhs)
	}

	// Operators: ==, !=, <=, >=, <, >
	operators := []string{"==", "!=", "<=", ">=", "<", ">"}
	for _, op := range operators {
		if strings.Contains(clause, op) {
			parts := strings.SplitN(clause, op, 2)
			lhs := strings.TrimSpace(parts[0])
			rhs := cleanQuotes(strings.TrimSpace(parts[1]))

			lhsVal := e.resolveValue(lhs, event, step, cumulativeTokens, memory)
			switch op {
			case "==":
				return fmt.Sprintf("%v", lhsVal) == rhs
			case "!=":
				return fmt.Sprintf("%v", lhsVal) != rhs
			case "<":
				return compareNumeric(lhsVal, rhs) < 0
			case ">":
				return compareNumeric(lhsVal, rhs) > 0
			case "<=":
				return compareNumeric(lhsVal, rhs) <= 0
			case ">=":
				return compareNumeric(lhsVal, rhs) >= 0
			}
		}
	}

	// Truthy check (e.g. "error")
	val := e.resolveValue(clause, event, step, cumulativeTokens, memory)
	if val == nil {
		return false
	}
	if b, ok := val.(bool); ok {
		return b
	}
	return fmt.Sprintf("%v", val) != "" && fmt.Sprintf("%v", val) != "0"
}

func (e *ConditionEvaluator) resolveValue(
	token string,
	event *trace.TraceEvent,
	step int,
	cumulativeTokens int,
	memory map[string]interface{},
) interface{} {
	token = strings.TrimSpace(token)
	switch strings.ToLower(token) {
	case "step":
		return step
	case "tokens":
		return cumulativeTokens
	case "tool":
		return extractToolName(event)
	case "type":
		return string(event.Type)
	case "error":
		if event.Type == trace.EventError {
			return true
		}
		if v, exists := event.Payload["error"]; exists && v != nil {
			return v
		}
		return nil
	}

	// Check arg / args / payload
	if strings.HasPrefix(token, "arg") || strings.HasPrefix(token, "payload.") {
		if strings.Contains(token, ".") {
			parts := strings.SplitN(token, ".", 2)
			key := parts[1]
			if v, ok := event.Payload[key]; ok {
				return v
			}
		} else {
			// Entire payload serialized
			bytes, _ := json.Marshal(event.Payload)
			return string(bytes)
		}
	}

	// Check memory['key'] or memory["key"]
	if strings.HasPrefix(token, "memory[") && strings.HasSuffix(token, "]") {
		key := token[7 : len(token)-1]
		key = cleanQuotes(key)
		if v, ok := memory[key]; ok {
			return v
		}
		return nil
	}

	// Literal or direct memory key
	if v, ok := memory[token]; ok {
		return v
	}

	return token
}

func extractToolName(event *trace.TraceEvent) string {
	if event == nil || event.Payload == nil {
		return ""
	}
	keys := []string{"tool", "name", "tool_name", "function", "command"}
	for _, k := range keys {
		if v, ok := event.Payload[k]; ok && v != nil {
			return fmt.Sprintf("%v", v)
		}
	}
	return ""
}

func cleanQuotes(s string) string {
	s = strings.TrimSpace(s)
	if (strings.HasPrefix(s, "'") && strings.HasSuffix(s, "'")) ||
		(strings.HasPrefix(s, "\"") && strings.HasSuffix(s, "\"")) {
		return s[1 : len(s)-1]
	}
	return s
}

func compareNumeric(a, b interface{}) int {
	fa := toFloat(a)
	fb := toFloat(b)
	if fa < fb {
		return -1
	} else if fa > fb {
		return 1
	}
	return 0
}

func toFloat(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case string:
		f, _ := strconv.ParseFloat(val, 64)
		return f
	default:
		s := fmt.Sprintf("%v", v)
		f, _ := strconv.ParseFloat(s, 64)
		return f
	}
}
