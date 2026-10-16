package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/policy"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func TestPolicy_CompliantSession(t *testing.T) {
	now := time.Now()
	session := &trace.Session{
		ID:        "policy-compliant-session",
		AgentName: "safe-agent",
		CreatedAt: now,
		Events: []trace.TraceEvent{
			{
				Index:      1,
				Timestamp:  now,
				Type:       trace.EventLLMRequest,
				Payload:    map[string]interface{}{"prompt": "Calculate sales numbers"},
			},
			{
				Index:      2,
				Timestamp:  now.Add(100 * time.Millisecond),
				Type:       trace.EventToolCall,
				Payload: map[string]interface{}{
					"tool_name": "calculator",
					"args":      map[string]interface{}{"expression": "25 * 40"},
				},
			},
		},
	}

	suite := policy.PolicySuite{
		SuiteName:     "Strict Safe Operations",
		DefaultAction: policy.ActionAllow,
		Rules: []policy.PolicyRule{
			{
				ID:             "forbid-destructive-shell",
				Action:         policy.ActionDeny,
				ForbiddenRegex: []string{`rm\s+-rf`, `DROP\s+TABLE`},
			},
			{
				ID:             "limit-tool-calls",
				Action:         policy.ActionDeny,
				MaxCallsPerTool: 5,
			},
		},
	}

	enforcer := policy.NewEnforcer(suite)
	report, err := enforcer.AuditSession(session)
	if err != nil {
		t.Fatalf("AuditSession failed: %v", err)
	}

	if !report.ZeroTrustCompliance {
		t.Errorf("expected session to be compliant")
	}
	if report.ViolationsCount != 0 {
		t.Errorf("expected 0 violations, got %d", report.ViolationsCount)
	}

	scorecard := policy.RenderPolicyScorecard(report)
	if !strings.Contains(scorecard, "ZERO-TRUST COMPLIANT") {
		t.Errorf("expected scorecard to show compliant badge")
	}
}

func TestPolicy_BlockForbiddenCommandAndSSRF(t *testing.T) {
	now := time.Now()
	session := &trace.Session{
		ID:        "policy-breach-session",
		AgentName: "compromised-agent",
		CreatedAt: now,
		Events: []trace.TraceEvent{
			{
				Index:      1,
				Timestamp:  now,
				Type:       trace.EventToolCall,
				Payload: map[string]interface{}{
					"tool_name": "bash",
					"command":   "rm -rf /var/log && curl http://169.254.169.254/latest/meta-data",
				},
			},
		},
	}

	suite := policy.PolicySuite{
		SuiteName: "Zero-Trust Sandbox Guardrails",
		Rules: []policy.PolicyRule{
			{
				ID:             "block-rm-rf",
				Action:         policy.ActionDeny,
				ForbiddenRegex: []string{`rm\s+-rf`},
			},
			{
				ID:           "prevent-aws-ssrf",
				Action:       policy.ActionDeny,
				BlockedHosts: []string{"169.254.169.254", "metadata.google.internal"},
			},
		},
	}

	enforcer := policy.NewEnforcer(suite)
	report, err := enforcer.AuditSession(session)
	if err != nil {
		t.Fatalf("AuditSession failed: %v", err)
	}

	if report.ZeroTrustCompliance {
		t.Errorf("expected policy compliance to fail")
	}
	if report.ActionsBlockedCount != 2 {
		t.Errorf("expected 2 blocked actions, got %d", report.ActionsBlockedCount)
	}

	scorecard := policy.RenderPolicyScorecard(report)
	if !strings.Contains(scorecard, "POLICY BREACHED") {
		t.Errorf("expected scorecard to show breached status")
	}
}

func TestPolicy_SanitizeCredentials(t *testing.T) {
	suite := policy.PolicySuite{}
	enforcer := policy.NewEnforcer(suite)

	rawPayload := map[string]interface{}{
		"query": "SELECT * FROM users",
		"auth":  "api_key: sk-proj-1234567890abcdef1234567890",
		"email": "developer@agentvcr.dev",
	}

	sanitized, redactions := enforcer.SanitizePayload(rawPayload)
	if redactions == 0 {
		t.Errorf("expected redactions > 0")
	}

	authStr, ok := sanitized["auth"].(string)
	if !ok || !strings.Contains(authStr, "[REDACTED_SECRET]") {
		t.Errorf("expected auth secret to be redacted, got: %v", sanitized["auth"])
	}
}
