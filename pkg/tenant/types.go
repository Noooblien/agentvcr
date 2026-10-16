package tenant

import (
	"time"
)

// Role defines the authorization privilege level within a tenant namespace.
type Role string

const (
	RoleAdmin    Role = "admin"     // Full read/write/fork/delete/attest
	RoleOperator Role = "operator"  // Read/write/fork/play
	RoleAuditor  Role = "auditor"   // Read-only inspection & compliance verification
	RoleAgent    Role = "agent"     // Append-only trajectory recording
)

// Permission defines an individual granular operation.
type Permission string

const (
	PermTraceRead     Permission = "trace:read"
	PermTraceWrite    Permission = "trace:write"
	PermTraceFork     Permission = "trace:fork"
	PermTraceDelete   Permission = "trace:delete"
	PermTraceExport   Permission = "trace:export"
	PermAttestSign    Permission = "attest:sign"
	PermAttestVerify  Permission = "attest:verify"
	PermPolicyAudit   Permission = "policy:audit"
	PermThrottleSet   Permission = "throttle:set"
)

// Subject represents an authenticated entity (user, agent, or service).
type Subject struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Role      Role      `json:"role"`
	APIKeyHash string   `json:"api_key_hash,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// NamespacePolicy defines cryptographic namespace barriers and RBAC rules for a tenant.
type NamespacePolicy struct {
	TenantID          string                  `json:"tenant_id"`
	Organization      string                  `json:"organization"`
	EnforceEncryption bool                    `json:"enforce_encryption"`
	MaxRetentionDays  int                     `json:"max_retention_days"`
	RolePermissions   map[Role][]Permission   `json:"role_permissions"`
	BlockedToolList   []string                `json:"blocked_tool_list,omitempty"`
	RestrictedPrefixes []string               `json:"restricted_prefixes,omitempty"`
}

// AccessRequest represents a pending operation subject to authorization.
type AccessRequest struct {
	SubjectID  string     `json:"subject_id"`
	TenantID   string     `json:"tenant_id"`
	Role       Role       `json:"role"`
	Permission Permission `json:"permission"`
	Resource   string     `json:"resource"` // e.g. "session:trace_123"
}

// AccessDecision records the outcome of an access control evaluation.
type AccessDecision struct {
	Allowed    bool      `json:"allowed"`
	Reason     string    `json:"reason"`
	EvaluatedAt time.Time `json:"evaluated_at"`
	TenantID   string    `json:"tenant_id"`
	SubjectID  string    `json:"subject_id"`
	Permission Permission `json:"permission"`
}

// TenantAuditReport provides an inspection report on namespace boundary enforcement and authorization events.
type TenantAuditReport struct {
	TenantID             string           `json:"tenant_id"`
	Organization         string           `json:"organization"`
	TotalAccessRequests  int              `json:"total_access_requests"`
	AllowedRequests      int              `json:"allowed_requests"`
	DeniedRequests       int              `json:"denied_requests"`
	CrossTenantViolations int             `json:"cross_tenant_violations"`
	NamespaceIntegrityOK bool             `json:"namespace_integrity_ok"`
	Violations           []string         `json:"violations,omitempty"`
	Decisions            []AccessDecision `json:"decisions"`
}
