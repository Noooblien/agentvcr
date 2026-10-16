package taint

import (
	"fmt"
	"strings"
	"time"
)

// TaintTag identifies a specific sensitivity or hazard classification of data.
type TaintTag string

const (
	TaintUntrustedWeb       TaintTag = "UNTRUSTED_WEB"       // Fetched from public internet / scraped HTML
	TaintUserSupplied       TaintTag = "USER_SUPPLIED"       // Raw prompt from untrusted end user
	TaintSecretKey          TaintTag = "SECRET_KEY"          // API keys, tokens, private keys
	TaintUserPII            TaintTag = "USER_PII"            // Passwords, SSN, email, credit cards
	TaintSystemInstruction  TaintTag = "SYSTEM_INSTRUCTION"  // Immutable base prompt instructions
	TaintIndirectInjection  TaintTag = "INDIRECT_INJECTION"  // Suspected jailbreak / prompt injection content
)

// SourceType denotes the origination point of tainted data.
type SourceType string

const (
	SourceWebScrape     SourceType = "web_scrape"
	SourceUserMessage   SourceType = "user_message"
	SourceToolOutput    SourceType = "tool_output"
	SourceEnvironment   SourceType = "env_var"
	SourceDatabaseRead  SourceType = "database_read"
	SourceMemoryStore   SourceType = "agent_memory"
)

// SinkType represents a security-sensitive execution destination.
type SinkType string

const (
	SinkShellExecution  SinkType = "shell_exec"     // os/exec, bash tool
	SinkNetworkEgress   SinkType = "network_egress" // HTTP POST, webhook, curl
	SinkFileSystemWrite SinkType = "filesystem_write" // Write to disk
	SinkAgentDelegation SinkType = "agent_delegation" // Passing prompt to subordinate agent
	SinkMemoryCommit    SinkType = "memory_commit"    // Saving into long-term agent memory
	SinkSystemPrompt    SinkType = "system_prompt"    // Modifying system instructions
)

// FlowStep records a single hop in the data provenance chain.
type FlowStep struct {
	StepID      string    `json:"step_id"`
	Operation   string    `json:"operation"` // "source", "concat", "json_extract", "declassify"
	Details     string    `json:"details"`
	TaintsAdded []TaintTag `json:"taints_added,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
}

// TaintedValue tracks a variable alongside its cryptographic or provenance taint tags.
type TaintedValue struct {
	VarName         string      `json:"var_name"`
	RawValue        interface{} `json:"raw_value"`
	Taints          map[TaintTag]bool `json:"taints"`
	ProvenanceChain []FlowStep  `json:"provenance_chain"`
	CreatedAt       time.Time   `json:"created_at"`
	LastModifiedAt  time.Time   `json:"last_modified_at"`
}

// HasTaint checks if the value carries a specific taint mark.
func (tv *TaintedValue) HasTaint(tag TaintTag) bool {
	if tv == nil || tv.Taints == nil {
		return false
	}
	return tv.Taints[tag]
}

// TagsList returns all active taint tags as a sorted slice.
func (tv *TaintedValue) TagsList() []TaintTag {
	if tv == nil || tv.Taints == nil {
		return nil
	}
	var list []TaintTag
	for t, active := range tv.Taints {
		if active {
			list = append(list, t)
		}
	}
	return list
}

// TaintViolation describes an illicit flow intercepted at a sensitive sink.
type TaintViolation struct {
	ViolationID    string     `json:"violation_id"`
	Sink           SinkType   `json:"sink"`
	TargetResource string     `json:"target_resource"`
	TaintedVar     string     `json:"tainted_var"`
	ViolatingTags  []TaintTag `json:"violating_tags"`
	Reason         string     `json:"reason"`
	InterceptedAt  time.Time  `json:"intercepted_at"`
}

// DeclassifierFunc transforms a value and strips specified taints upon verified validation.
type DeclassifierFunc func(val interface{}) (cleaned interface{}, ok bool)

// DeclassifierConfig defines a registered sanitization gate.
type DeclassifierConfig struct {
	ID             string
	Description    string
	RemovesTaints  []TaintTag
	Transform      DeclassifierFunc
}

// Policy defines forbidden taint tags for each sink.
type Policy struct {
	ForbiddenSinkTaints map[SinkType][]TaintTag
}

// DefaultPolicy provides industry-standard Non-Interference Information Flow Control.
func DefaultPolicy() *Policy {
	return &Policy{
		ForbiddenSinkTaints: map[SinkType][]TaintTag{
			SinkShellExecution: {
				TaintUntrustedWeb,
				TaintUserSupplied,
				TaintIndirectInjection,
			},
			SinkNetworkEgress: {
				TaintSecretKey,
				TaintUserPII,
			},
			SinkFileSystemWrite: {
				TaintUntrustedWeb,
				TaintIndirectInjection,
			},
			SinkSystemPrompt: {
				TaintUntrustedWeb,
				TaintUserSupplied,
				TaintIndirectInjection,
			},
			SinkMemoryCommit: {
				TaintIndirectInjection,
			},
		},
	}
}

// TaintReport summarizes information flow security tracking across a session.
type TaintReport struct {
	TotalFlowsTracked   int              `json:"total_flows_tracked"`
	ActiveTaintedVars   int              `json:"active_tainted_vars"`
	ViolationsCount     int              `json:"violations_count"`
	Violations          []TaintViolation `json:"violations"`
	NonInterferenceHeld bool             `json:"non_interference_held"`
}

// FormatViolations generates a clean human-readable audit string.
func (r *TaintReport) FormatViolations() string {
	if len(r.Violations) == 0 {
		return "Zero information-flow violations detected. Non-interference invariant preserved."
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("CRITICAL: %d Information Flow Control Violations Intercepted:\n", len(r.Violations)))
	for i, v := range r.Violations {
		sb.WriteString(fmt.Sprintf("  [%d] Sink: %s -> Var: %s (Violating Taints: %v) Reason: %s\n",
			i+1, v.Sink, v.TaintedVar, v.ViolatingTags, v.Reason))
	}
	return sb.String()
}
