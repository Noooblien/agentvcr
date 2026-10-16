package policy

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

var defaultPIIPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(api[_-]?key|secret|token|bearer|auth|password)[\s:=]+(['"]?[a-zA-Z0-9_\-\.]{16,}['"]?)`),
	regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`),
	regexp.MustCompile(`\b(?:\d{4}[-\s]?){3}\d{4}\b`), // Credit card format
}

// Enforcer conducts real-time and post-hoc runtime zero-trust policy evaluation.
type Enforcer struct {
	suite PolicySuite
}

// NewEnforcer instantiates a policy enforcer with the specified policy suite.
func NewEnforcer(suite PolicySuite) *Enforcer {
	if suite.DefaultAction == "" {
		suite.DefaultAction = ActionAllow
	}
	return &Enforcer{
		suite: suite,
	}
}

// AuditSession analyzes a full recorded trajectory session against the governance policy suite.
func (e *Enforcer) AuditSession(session *trace.Session) (*GovernanceAuditReport, error) {
	if session == nil {
		return nil, fmt.Errorf("session cannot be nil")
	}

	report := &GovernanceAuditReport{
		SessionID:  session.ID,
		Violations: make([]PolicyViolation, 0),
	}

	toolCallCounts := make(map[string]int)

	for _, ev := range session.Events {
		report.TotalEventsEvaluated++

		// Track tool counts
		if ev.Type == trace.EventToolCall {
			toolName := extractToolName(ev.Payload)
			toolCallCounts[toolName]++
		}

		violations := e.evaluateEvent(ev, toolCallCounts)
		for _, v := range violations {
			report.Violations = append(report.Violations, v)
			report.ViolationsCount++
			if v.ActionTaken == ActionDeny || v.ActionTaken == ActionQuarantine {
				report.ActionsBlockedCount++
			} else if v.ActionTaken == ActionSanitize {
				report.SanitizationsCount++
			}
		}
	}

	if report.TotalEventsEvaluated > 0 {
		cleanEvents := report.TotalEventsEvaluated - report.ViolationsCount
		if cleanEvents < 0 {
			cleanEvents = 0
		}
		report.ComplianceScore = float64(cleanEvents) / float64(report.TotalEventsEvaluated)
	} else {
		report.ComplianceScore = 1.0
	}

	report.ZeroTrustCompliance = (report.ActionsBlockedCount == 0)

	return report, nil
}

// SanitizePayload redacts sensitive PII, credentials, and API secrets from a payload.
func (e *Enforcer) SanitizePayload(payload map[string]interface{}) (map[string]interface{}, int) {
	if payload == nil {
		return nil, 0
	}

	sanitized := make(map[string]interface{})
	redactions := 0

	for k, v := range payload {
		switch val := v.(type) {
		case string:
			cleanStr := val
			for _, pattern := range defaultPIIPatterns {
				if pattern.MatchString(cleanStr) {
					cleanStr = pattern.ReplaceAllString(cleanStr, "[REDACTED_SECRET]")
					redactions++
				}
			}
			sanitized[k] = cleanStr
		case map[string]interface{}:
			subSanitized, count := e.SanitizePayload(val)
			sanitized[k] = subSanitized
			redactions += count
		default:
			sanitized[k] = val
		}
	}

	return sanitized, redactions
}

func (e *Enforcer) evaluateEvent(ev trace.TraceEvent, toolCounts map[string]int) []PolicyViolation {
	var violations []PolicyViolation
	payloadStr := stringifyPayload(ev.Payload)
	toolName := extractToolName(ev.Payload)

	for _, rule := range e.suite.Rules {
		// 1. Tool pattern check
		if rule.ToolPattern != "" && ev.Type == trace.EventToolCall {
			re, err := regexp.Compile(rule.ToolPattern)
			if err == nil && re.MatchString(toolName) {
				if rule.Action == ActionDeny {
					violations = append(violations, PolicyViolation{
						StepIndex:   ev.Index,
						EventID:     ev.ID,
						RuleID:      rule.ID,
						ActionTaken: ActionDeny,
						Severity:    "critical",
						Reason:      fmt.Sprintf("Tool '%s' matched forbidden tool pattern '%s'", toolName, rule.ToolPattern),
					})
				}
			}
		}

		// 2. Max calls per tool
		if rule.MaxCallsPerTool > 0 && ev.Type == trace.EventToolCall {
			if toolCounts[toolName] > rule.MaxCallsPerTool {
				violations = append(violations, PolicyViolation{
					StepIndex:   ev.Index,
					EventID:     ev.ID,
					RuleID:      rule.ID,
					ActionTaken: ActionDeny,
					Severity:    "high",
					Reason:      fmt.Sprintf("Tool '%s' exceeded max call threshold (%d > %d)", toolName, toolCounts[toolName], rule.MaxCallsPerTool),
				})
			}
		}

		// 3. Forbidden regex patterns (e.g. rm -rf, drop table, eval)
		for _, forbidden := range rule.ForbiddenRegex {
			re, err := regexp.Compile("(?i)" + forbidden)
			if err == nil && re.MatchString(payloadStr) {
				violations = append(violations, PolicyViolation{
					StepIndex:   ev.Index,
					EventID:     ev.ID,
					RuleID:      rule.ID,
					ActionTaken: rule.Action,
					Severity:    "critical",
					Reason:      fmt.Sprintf("Payload triggered forbidden pattern '%s'", forbidden),
					BlockedData: truncateString(payloadStr, 80),
				})
			}
		}

		// 4. Blocked hosts (SSRF prevention)
		for _, host := range rule.BlockedHosts {
			if strings.Contains(strings.ToLower(payloadStr), strings.ToLower(host)) {
				violations = append(violations, PolicyViolation{
					StepIndex:   ev.Index,
					EventID:     ev.ID,
					RuleID:      rule.ID,
					ActionTaken: ActionDeny,
					Severity:    "critical",
					Reason:      fmt.Sprintf("Egress target host '%s' is blocked by security policy", host),
					BlockedData: host,
				})
			}
		}

		// 5. Automatic PII / Secret leak check
		if rule.RedactPIIPatterns {
			for _, pattern := range defaultPIIPatterns {
				if pattern.MatchString(payloadStr) {
					violations = append(violations, PolicyViolation{
						StepIndex:   ev.Index,
						EventID:     ev.ID,
						RuleID:      rule.ID,
						ActionTaken: ActionSanitize,
						Severity:    "medium",
						Reason:      "Unmasked API credential or PII detected in event payload",
					})
					break
				}
			}
		}
	}

	return violations
}

// RenderPolicyScorecard formats the governance report into an executive terminal summary.
func RenderPolicyScorecard(report *GovernanceAuditReport) string {
	var b strings.Builder

	complianceBadge := "✅ ZERO-TRUST COMPLIANT"
	if !report.ZeroTrustCompliance {
		complianceBadge = fmt.Sprintf("❌ POLICY BREACHED (%d Actions Blocked)", report.ActionsBlockedCount)
	}

	b.WriteString("\n🔒 [AgentVCR] Zero-Trust Runtime Policy Governance & Sandboxing\n")
	b.WriteString("═══════════════════════════════════════════════════════════════════════════\n")
	b.WriteString(fmt.Sprintf("  Session ID:             %s\n", report.SessionID))
	b.WriteString(fmt.Sprintf("  Governance Status:      %s\n", complianceBadge))
	b.WriteString(fmt.Sprintf("  Compliance Score:       %.1f%%\n", report.ComplianceScore*100.0))
	b.WriteString(fmt.Sprintf("  Evaluated Events:       %d\n", report.TotalEventsEvaluated))
	b.WriteString(fmt.Sprintf("  Violations Detected:    %d\n", report.ViolationsCount))
	b.WriteString(fmt.Sprintf("  Actions Blocked/Denied: %d\n", report.ActionsBlockedCount))
	b.WriteString(fmt.Sprintf("  Secrets/PII Sanitized:  %d\n", report.SanitizationsCount))
	b.WriteString("───────────────────────────────────────────────────────────────────────────\n")

	if len(report.Violations) == 0 {
		b.WriteString("  No policy violations detected. Trajectory operated strictly within sandbox.\n")
	} else {
		b.WriteString("  Policy Violations Logged:\n")
		for i, v := range report.Violations {
			icon := "🚫"
			if v.ActionTaken == ActionSanitize {
				icon = "🧹"
			}
			b.WriteString(fmt.Sprintf("    %s [%d] Step %d: Rule '%s' [%s] -> %s\n", icon, i+1, v.StepIndex, v.RuleID, strings.ToUpper(string(v.ActionTaken)), v.Reason))
			if v.BlockedData != "" {
				b.WriteString(fmt.Sprintf("        Trigger Content: %s\n", v.BlockedData))
			}
		}
	}
	b.WriteString("═══════════════════════════════════════════════════════════════════════════\n")

	return b.String()
}

func extractToolName(payload map[string]interface{}) string {
	if payload == nil {
		return ""
	}
	for _, k := range []string{"tool_name", "tool", "name", "method"} {
		if v, ok := payload[k].(string); ok {
			return v
		}
	}
	return ""
}

func stringifyPayload(payload map[string]interface{}) string {
	if payload == nil {
		return ""
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Sprintf("%v", payload)
	}
	return string(b)
}

func truncateString(s string, maxLen int) string {
	clean := strings.ReplaceAll(s, "\n", " ")
	clean = strings.ReplaceAll(clean, "\r", " ")
	if len(clean) > maxLen {
		return clean[:maxLen] + "..."
	}
	return clean
}
