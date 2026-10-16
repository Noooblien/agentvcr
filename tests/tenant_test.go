package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/tenant"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func TestTenant_NamespaceRegistrationAndKeyDerivation(t *testing.T) {
	mgr := tenant.NewManager()

	policy := tenant.NamespacePolicy{
		TenantID:          "tenant-alpha",
		Organization:      "Acme Corp",
		EnforceEncryption: true,
		MaxRetentionDays:  90,
	}

	if err := mgr.RegisterTenant(policy); err != nil {
		t.Fatalf("RegisterTenant failed: %v", err)
	}

	key1 := tenant.DeriveNamespaceKey("master-secret-key-1", "tenant-alpha")
	key2 := tenant.DeriveNamespaceKey("master-secret-key-1", "tenant-alpha")
	keyBeta := tenant.DeriveNamespaceKey("master-secret-key-1", "tenant-beta")

	if key1 != key2 {
		t.Errorf("Derived namespace keys should be deterministic")
	}
	if key1 == keyBeta {
		t.Errorf("Different tenant IDs must yield cryptographically isolated keys")
	}
}

func TestTenant_RBACAuthorization(t *testing.T) {
	mgr := tenant.NewManager()
	policy := tenant.NamespacePolicy{
		TenantID:     "tenant-fin",
		Organization: "FinTech Global",
	}
	_ = mgr.RegisterTenant(policy)

	// Admin should be allowed to delete traces
	reqAdmin := tenant.AccessRequest{
		SubjectID:  "user-admin",
		TenantID:   "tenant-fin",
		Role:       tenant.RoleAdmin,
		Permission: tenant.PermTraceDelete,
		Resource:   "trace:tenant-fin:session:123",
	}
	decAdmin := mgr.Authorize(reqAdmin)
	if !decAdmin.Allowed {
		t.Errorf("Expected admin to be allowed PermTraceDelete: %s", decAdmin.Reason)
	}

	// Auditor should NOT be allowed to delete traces
	reqAuditor := tenant.AccessRequest{
		SubjectID:  "user-auditor",
		TenantID:   "tenant-fin",
		Role:       tenant.RoleAuditor,
		Permission: tenant.PermTraceDelete,
		Resource:   "trace:tenant-fin:session:123",
	}
	decAuditor := mgr.Authorize(reqAuditor)
	if decAuditor.Allowed {
		t.Errorf("Expected auditor to be denied PermTraceDelete")
	}

	// Cross-tenant access must be rejected
	reqCross := tenant.AccessRequest{
		SubjectID:  "user-agent",
		TenantID:   "tenant-fin",
		Role:       tenant.RoleAdmin,
		Permission: tenant.PermTraceRead,
		Resource:   "trace:tenant-other:session:456",
	}
	decCross := mgr.Authorize(reqCross)
	if decCross.Allowed {
		t.Errorf("Expected cross-tenant access to be blocked")
	}
	if !strings.Contains(decCross.Reason, "Cross-tenant isolation violation") {
		t.Errorf("Expected cross-tenant violation reason, got: %s", decCross.Reason)
	}
}

func TestTenant_AuditSessionCompliance(t *testing.T) {
	mgr := tenant.NewManager()
	policy := tenant.NamespacePolicy{
		TenantID:        "tenant-sec",
		Organization:    "SecureOrg",
		BlockedToolList: []string{"rm_rf", "exfiltrate_data"},
	}
	_ = mgr.RegisterTenant(policy)

	now := time.Now()
	// Compliant session
	sessOK := &trace.Session{
		ID:        "sess-compliant",
		AgentName: "safe-bot",
		Metadata:  map[string]string{"tenant_id": "tenant-sec"},
		Events: []trace.TraceEvent{
			{Index: 1, Timestamp: now, Type: trace.EventToolCall, Payload: map[string]interface{}{"tool_name": "list_files"}},
		},
	}

	repOK, err := mgr.AuditSession(sessOK, "tenant-sec")
	if err != nil {
		t.Fatalf("AuditSession failed: %v", err)
	}
	if !repOK.NamespaceIntegrityOK || repOK.DeniedRequests > 0 {
		t.Errorf("Expected compliant session to pass tenant audit")
	}

	// Breached session (calls blocked tool)
	sessBreach := &trace.Session{
		ID:        "sess-breach",
		AgentName: "bad-bot",
		Metadata:  map[string]string{"tenant_id": "tenant-sec"},
		Events: []trace.TraceEvent{
			{Index: 1, Timestamp: now, Type: trace.EventToolCall, Payload: map[string]interface{}{"tool_name": "exfiltrate_data"}},
		},
	}

	repBreach, err := mgr.AuditSession(sessBreach, "tenant-sec")
	if err != nil {
		t.Fatalf("AuditSession failed: %v", err)
	}
	if repBreach.NamespaceIntegrityOK {
		t.Errorf("Expected breached session to fail namespace integrity")
	}
	if len(repBreach.Violations) == 0 {
		t.Errorf("Expected recorded violations for blocked tool call")
	}

	card := tenant.RenderTenantScorecard(repBreach)
	if !strings.Contains(card, "AGENTVCR MULTI-TENANT CRYPTOGRAPHIC NAMESPACE") {
		t.Errorf("Expected scorecard header, got:\n%s", card)
	}
	if !strings.Contains(card, "[FAIL]") {
		t.Errorf("Expected FAIL status in scorecard, got:\n%s", card)
	}
}
