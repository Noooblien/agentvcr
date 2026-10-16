package policy

// PolicyAction defines the enforcement action taken upon rule evaluation.
type PolicyAction string

const (
	ActionAllow     PolicyAction = "allow"
	ActionDeny      PolicyAction = "deny"
	ActionSanitize  PolicyAction = "sanitize"  // Redact secrets, PII, or forbidden tokens
	ActionQuarantine PolicyAction = "quarantine" // Freeze agent trajectory and alert
)

// PolicyRule defines an individual zero-trust runtime policy constraint.
type PolicyRule struct {
	ID             string       `json:"id"`
	Description    string       `json:"description"`
	Action         PolicyAction `json:"action"` // "allow", "deny", "sanitize", "quarantine"
	ToolPattern    string       `json:"tool_pattern,omitempty"`    // Regex/wildcard tool name match
	MaxTokenBudget int          `json:"max_token_budget,omitempty"` // Ceiling on token usage
	MaxCallsPerTool int         `json:"max_calls_per_tool,omitempty"`
	ForbiddenRegex []string     `json:"forbidden_regex,omitempty"` // Patterns forbidden in prompts/args (e.g., shell injection, rm -rf)
	BlockedHosts   []string     `json:"blocked_hosts,omitempty"`   // SSRF / exfiltration target hosts
	RedactPIIPatterns bool      `json:"redact_pii,omitempty"`      // Automatically mask API keys, credit cards, emails
}

// PolicySuite represents a bundle of zero-trust governance rules.
type PolicySuite struct {
	SuiteName      string       `json:"suite_name"`
	DefaultAction  PolicyAction `json:"default_action"` // "allow" or "deny"
	Rules          []PolicyRule `json:"rules"`
}

// PolicyViolation details an intercepted malicious or policy-breaching event.
type PolicyViolation struct {
	StepIndex   int          `json:"step_index"`
	EventID     string       `json:"event_id,omitempty"`
	RuleID      string       `json:"rule_id"`
	ActionTaken PolicyAction `json:"action_taken"`
	Severity    string       `json:"severity"` // "critical", "high", "medium"
	Reason      string       `json:"reason"`
	BlockedData string       `json:"blocked_data,omitempty"`
}

// GovernanceAuditReport provides an executive audit of runtime policy enforcement on a session.
type GovernanceAuditReport struct {
	SessionID             string            `json:"session_id"`
	TotalEventsEvaluated  int               `json:"total_events_evaluated"`
	ViolationsCount       int               `json:"violations_count"`
	ActionsBlockedCount   int               `json:"actions_blocked_count"`
	SanitizationsCount    int               `json:"sanitizations_count"`
	ZeroTrustCompliance   bool              `json:"zero_trust_compliance"`
	ComplianceScore       float64           `json:"compliance_score"` // 0.0 to 1.0
	Violations            []PolicyViolation `json:"violations"`
}
