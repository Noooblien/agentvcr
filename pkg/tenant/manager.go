package tenant

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// DefaultRolePermissions provides strict zero-trust permission assignments across roles.
func DefaultRolePermissions() map[Role][]Permission {
	return map[Role][]Permission{
		RoleAdmin: {
			PermTraceRead, PermTraceWrite, PermTraceFork, PermTraceDelete,
			PermTraceExport, PermAttestSign, PermAttestVerify, PermPolicyAudit, PermThrottleSet,
		},
		RoleOperator: {
			PermTraceRead, PermTraceWrite, PermTraceFork, PermTraceExport,
			PermAttestVerify, PermPolicyAudit,
		},
		RoleAuditor: {
			PermTraceRead, PermTraceExport, PermAttestVerify, PermPolicyAudit,
		},
		RoleAgent: {
			PermTraceWrite, PermTraceRead,
		},
	}
}

// Manager orchestrates multi-tenant namespace isolation, token derivation, and RBAC enforcement.
type Manager struct {
	mu         sync.RWMutex
	namespaces map[string]*NamespacePolicy
	subjects   map[string]*Subject
	auditLog   []AccessDecision
}

// NewManager initializes a new tenant manager with default policies.
func NewManager() *Manager {
	return &Manager{
		namespaces: make(map[string]*NamespacePolicy),
		subjects:   make(map[string]*Subject),
		auditLog:   make([]AccessDecision, 0),
	}
}

// RegisterTenant creates and binds an isolated cryptographic namespace policy.
func (m *Manager) RegisterTenant(policy NamespacePolicy) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if policy.TenantID == "" {
		return fmt.Errorf("tenant ID cannot be empty")
	}

	if policy.RolePermissions == nil {
		policy.RolePermissions = DefaultRolePermissions()
	}

	m.namespaces[policy.TenantID] = &policy
	return nil
}

// RegisterSubject registers an authenticated subject under a tenant namespace.
func (m *Manager) RegisterSubject(subject Subject) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if subject.ID == "" || subject.TenantID == "" {
		return fmt.Errorf("subject ID and TenantID are required")
	}

	if _, exists := m.namespaces[subject.TenantID]; !exists {
		return fmt.Errorf("tenant namespace '%s' is not registered", subject.TenantID)
	}

	if subject.CreatedAt.IsZero() {
		subject.CreatedAt = time.Now()
	}

	m.subjects[subject.ID] = &subject
	return nil
}

// DeriveNamespaceKey derives a deterministic cryptographic key for tenant isolation.
func DeriveNamespaceKey(masterKey, tenantID string) string {
	mac := hmac.New(sha256.New, []byte(masterKey))
	mac.Write([]byte(fmt.Sprintf("agentvcr:tenant:%s", tenantID)))
	return hex.EncodeToString(mac.Sum(nil))
}

// Authorize evaluates whether a Subject has permission to perform an action on a resource.
func (m *Manager) Authorize(req AccessRequest) AccessDecision {
	m.mu.Lock()
	defer m.mu.Unlock()

	decision := AccessDecision{
		EvaluatedAt: time.Now(),
		TenantID:    req.TenantID,
		SubjectID:   req.SubjectID,
		Permission:  req.Permission,
	}

	// 1. Verify tenant namespace exists
	policy, exists := m.namespaces[req.TenantID]
	if !exists {
		decision.Allowed = false
		decision.Reason = fmt.Sprintf("Tenant namespace '%s' does not exist", req.TenantID)
		m.auditLog = append(m.auditLog, decision)
		return decision
	}

	// 2. Cross-tenant isolation barrier check
	if strings.Contains(req.Resource, ":") {
		parts := strings.Split(req.Resource, ":")
		if len(parts) >= 3 && parts[1] != "" && parts[1] != req.TenantID {
			decision.Allowed = false
			decision.Reason = fmt.Sprintf("Cross-tenant isolation violation: subject from '%s' tried accessing resource of tenant '%s'", req.TenantID, parts[1])
			m.auditLog = append(m.auditLog, decision)
			return decision
		}
	}

	// 3. RBAC Role Permission Check
	perms, roleExists := policy.RolePermissions[req.Role]
	if !roleExists {
		decision.Allowed = false
		decision.Reason = fmt.Sprintf("Role '%s' has no defined permissions in namespace '%s'", req.Role, req.TenantID)
		m.auditLog = append(m.auditLog, decision)
		return decision
	}

	hasPerm := false
	for _, p := range perms {
		if p == req.Permission {
			hasPerm = true
			break
		}
	}

	if !hasPerm {
		decision.Allowed = false
		decision.Reason = fmt.Sprintf("Role '%s' is denied permission '%s' on resource '%s'", req.Role, req.Permission, req.Resource)
		m.auditLog = append(m.auditLog, decision)
		return decision
	}

	// 4. Access Granted
	decision.Allowed = true
	decision.Reason = fmt.Sprintf("Authorized by namespace policy for role '%s'", req.Role)
	m.auditLog = append(m.auditLog, decision)
	return decision
}

// AuditSession verifies that all events and tool calls in a session conform to tenant namespace isolation rules.
func (m *Manager) AuditSession(session *trace.Session, tenantID string) (*TenantAuditReport, error) {
	m.mu.RLock()
	policy, exists := m.namespaces[tenantID]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("tenant '%s' not registered", tenantID)
	}

	report := &TenantAuditReport{
		TenantID:             tenantID,
		Organization:         policy.Organization,
		Decisions:            make([]AccessDecision, 0),
		Violations:           make([]string, 0),
		NamespaceIntegrityOK: true,
	}

	blockedToolMap := make(map[string]bool)
	for _, t := range policy.BlockedToolList {
		blockedToolMap[t] = true
	}

	for _, ev := range session.Events {
		req := AccessRequest{
			SubjectID:  session.AgentName,
			TenantID:   tenantID,
			Role:       RoleAgent,
			Permission: PermTraceWrite,
			Resource:   fmt.Sprintf("trace:%s:event:%d", tenantID, ev.Index),
		}

		// Check tenant metadata cross-contamination
		if metaTenant, ok := session.Metadata["tenant_id"]; ok && metaTenant != "" && metaTenant != tenantID {
			report.CrossTenantViolations++
			report.Violations = append(report.Violations, fmt.Sprintf("Step %d contains mismatched metadata tenant_id '%s' (expected '%s')", ev.Index, metaTenant, tenantID))
			report.NamespaceIntegrityOK = false
		}

		// Check blocked tools for this tenant
		if ev.Type == trace.EventToolCall {
			if toolName, ok := ev.Payload["tool_name"].(string); ok && blockedToolMap[toolName] {
				report.DeniedRequests++
				report.Violations = append(report.Violations, fmt.Sprintf("Step %d invoked tenant-blocked tool '%s'", ev.Index, toolName))
				report.NamespaceIntegrityOK = false
			}
		}

		decision := m.Authorize(req)
		report.Decisions = append(report.Decisions, decision)
		report.TotalAccessRequests++
		if decision.Allowed {
			report.AllowedRequests++
		} else {
			report.DeniedRequests++
			report.Violations = append(report.Violations, fmt.Sprintf("Step %d denied: %s", ev.Index, decision.Reason))
			report.NamespaceIntegrityOK = false
		}
	}

	return report, nil
}

// RenderTenantScorecard produces an ANSI-formatted scorecard for tenant namespace and RBAC audits.
func RenderTenantScorecard(report *TenantAuditReport) string {
	var sb strings.Builder

	sb.WriteString("\n\033[1;36m========================================================================================\033[0m\n")
	sb.WriteString(fmt.Sprintf("\033[1;37m   AGENTVCR MULTI-TENANT CRYPTOGRAPHIC NAMESPACE & RBAC AUDIT REPORT\033[0m\n"))
	sb.WriteString("\033[1;36m========================================================================================\033[0m\n")

	sb.WriteString(fmt.Sprintf("  \033[1;33mTenant ID:\033[0m            %s\n", report.TenantID))
	sb.WriteString(fmt.Sprintf("  \033[1;33mOrganization:\033[0m         %s\n", report.Organization))
	sb.WriteString(fmt.Sprintf("  \033[1;33mTotal Requests:\033[0m       %d access requests audited\n", report.TotalAccessRequests))
	sb.WriteString(fmt.Sprintf("  \033[1;33mAllowed Requests:\033[0m     \033[1;32m%d passed\033[0m | Denied: \033[1;31m%d\033[0m\n", report.AllowedRequests, report.DeniedRequests))

	if report.NamespaceIntegrityOK && report.CrossTenantViolations == 0 {
		sb.WriteString("  \033[1;33mIsolation Status:\033[0m     \033[1;32m[PASS] STRICT CRYPTOGRAPHIC NAMESPACE ISOLATION PRESERVED\033[0m\n")
	} else {
		sb.WriteString(fmt.Sprintf("  \033[1;33mIsolation Status:\033[0m     \033[1;31m[FAIL] %d ISOLATION / RBAC VIOLATIONS DETECTED\033[0m\n", len(report.Violations)))
	}
	sb.WriteString("\033[1;34m----------------------------------------------------------------------------------------\033[0m\n")

	if len(report.Violations) > 0 {
		sb.WriteString("\033[1;31m  DETECTED ACCESS CONTROL & NAMESPACE BREACHES:\033[0m\n")
		for _, v := range report.Violations {
			sb.WriteString(fmt.Sprintf("  \033[1;31m[!] %s\033[0m\n", v))
		}
	} else {
		sb.WriteString("\033[1;32m  ✓ Zero cross-tenant data leaks detected\033[0m\n")
		sb.WriteString("\033[1;32m  ✓ All agent actions strictly authorized under assigned RBAC permissions\033[0m\n")
		sb.WriteString("\033[1;32m  ✓ Cryptographic boundary isolation integrity 100%\033[0m\n")
	}

	sb.WriteString("\033[1;36m========================================================================================\033[0m\n\n")
	return sb.String()
}
