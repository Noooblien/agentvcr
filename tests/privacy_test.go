package tests

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/privacy"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func TestPrivacy_TokenMaskingAndPseudonymization(t *testing.T) {
	cfg := privacy.DefaultDPConfig()
	cfg.DeterministicSeed = 42
	engine := privacy.NewPrivacyEngine(cfg)

	secretKey := "sk-live-abcdef1234567890abcdef123456"
	userEmail := "alice.researcher@openai.internal"
	userIP := "192.168.1.105"
	userSSN := "123-45-6789"

	session := &trace.Session{
		ID:        "session_privacy_test",
		AgentName: "finance-bot",
		CreatedAt: time.Now(),
		Metadata: map[string]string{
			"contact": userEmail,
		},
		Events: []trace.TraceEvent{
			{
				ID:        "ev_1",
				Index:     1,
				Timestamp: time.Now(),
				Type:      trace.EventLLMRequest,
				Payload: map[string]interface{}{
					"prompt": "Call the API using " + secretKey + " from IP " + userIP,
				},
			},
			{
				ID:        "ev_2",
				Index:     2,
				Timestamp: time.Now(),
				Type:      trace.EventToolCall,
				Payload: map[string]interface{}{
					"tool": "verify_identity",
					"args": map[string]interface{}{
						"api_key": secretKey,
						"ssn":     userSSN,
					},
				},
			},
		},
		Status: "completed",
	}

	sanitized, report := engine.SanitizeSession(session)

	if report.TotalTokensMasked < 4 {
		t.Fatalf("expected at least 4 masked tokens, got %d", report.TotalTokensMasked)
	}
	if report.MaskedKeysCount < 2 {
		t.Errorf("expected at least 2 masked keys, got %d", report.MaskedKeysCount)
	}
	if report.MaskedEmailsCount < 1 {
		t.Errorf("expected at least 1 masked email, got %d", report.MaskedEmailsCount)
	}
	if report.MaskedIPsCount < 1 {
		t.Errorf("expected at least 1 masked IP, got %d", report.MaskedIPsCount)
	}

	// Verify plaintext secret was eradicated
	p1 := sanitized.Events[0].Payload["prompt"].(string)
	if strings.Contains(p1, secretKey) {
		t.Errorf("sanitized prompt still contains plaintext secret key")
	}
	if strings.Contains(p1, userIP) {
		t.Errorf("sanitized prompt still contains plaintext IP")
	}

	// Verify entity consistency: step 1 and step 2 key replacements must be identical
	args := sanitized.Events[1].Payload["args"].(map[string]interface{})
	maskedKeyStep2 := args["api_key"].(string)
	if !strings.Contains(p1, maskedKeyStep2) {
		t.Errorf("expected entity consistency: step 1 prompt should contain masked key %s", maskedKeyStep2)
	}
	if !strings.HasPrefix(maskedKeyStep2, "<REDACTED:KEY_") {
		t.Errorf("masked key tag formatted incorrectly: %s", maskedKeyStep2)
	}
}

func TestPrivacy_DifferentialPrivacyLaplaceNoiseAndBudget(t *testing.T) {
	cfg := privacy.DPConfig{
		Epsilon:             1.0,
		Delta:               1e-5,
		Sensitivity:         1.0,
		TotalBudgetEpsilon:  3.0,
		KAnonymityThreshold: 2,
		LDiversityThreshold: 1,
		DeterministicSeed:   12345,
	}
	engine := privacy.NewPrivacyEngine(cfg)

	// Test Laplace Noise properties
	sum := 0.0
	samples := 1000
	for i := 0; i < samples; i++ {
		noise := engine.LaplaceNoise(1.0, 1.0)
		sum += noise
	}
	mean := sum / float64(samples)
	if math.Abs(mean) > 0.5 {
		t.Errorf("expected zero-mean Laplace noise, observed mean: %f", mean)
	}

	// Test budget spending
	if ok := engine.SpendBudget(1.0); !ok {
		t.Errorf("spending 1.0 epsilon within 3.0 budget should succeed")
	}
	b := engine.GetBudget()
	if b.RemainingEpsilon != 2.0 || b.SpentEpsilon != 1.0 {
		t.Errorf("unexpected budget state: %+v", b)
	}

	if ok := engine.SpendBudget(1.5); !ok {
		t.Errorf("spending 1.5 epsilon should succeed")
	}
	b = engine.GetBudget()
	if math.Abs(b.RemainingEpsilon-0.5) > 0.001 {
		t.Errorf("expected remaining budget ~0.5, got %f", b.RemainingEpsilon)
	}

	// Overspending should be denied
	if ok := engine.SpendBudget(1.0); ok {
		t.Errorf("spending 1.0 epsilon with only 0.5 remaining should fail")
	}
	b = engine.GetBudget()
	if !b.BudgetExhausted {
		t.Errorf("budget should be flagged as exhausted")
	}
}

func TestPrivacy_FleetKAnonymityAndLDiversity(t *testing.T) {
	cfg := privacy.DefaultDPConfig()
	engine := privacy.NewPrivacyEngine(cfg)

	// Create 3 sessions sharing same QID (agent: "coder", model: "gpt-4o", tools: ["bash"], steps: 2)
	makeSession := func(id, agent, status string) *trace.Session {
		return &trace.Session{
			ID:        id,
			AgentName: agent,
			Status:    status,
			Metadata:  map[string]string{"model": "gpt-4o"},
			Events: []trace.TraceEvent{
				{
					Index: 1,
					Type:  trace.EventToolCall,
					Payload: map[string]interface{}{
						"tool": "bash",
					},
				},
			},
		}
	}

	s1 := makeSession("sess_1", "coder", "completed")
	s2 := makeSession("sess_2", "coder", "failed")
	s3 := makeSession("sess_3", "coder", "completed")
	// s4 is an isolated session with unique agent name
	s4 := makeSession("sess_4", "unique-specialist", "completed")

	fleet := []*trace.Session{s1, s2, s3, s4}

	report := engine.AuditFleetKAnonymity(fleet, 3, 2)

	if report.TotalSessionsAudited != 4 {
		t.Fatalf("expected 4 sessions audited, got %d", report.TotalSessionsAudited)
	}
	if report.CompliantClasses != 1 {
		t.Errorf("expected 1 compliant class (size 3), got %d", report.CompliantClasses)
	}
	if report.ViolatingClasses != 1 {
		t.Errorf("expected 1 violating class (size 1), got %d", report.ViolatingClasses)
	}
	if len(report.VulnerableSessionIDs) != 1 || report.VulnerableSessionIDs[0] != "sess_4" {
		t.Errorf("expected sess_4 to be vulnerable, got: %v", report.VulnerableSessionIDs)
	}
	if report.Passed {
		t.Errorf("audit should fail due to sess_4 re-identification risk")
	}
}

func TestPrivacy_AuditSessionPrivacyAndScorecard(t *testing.T) {
	cfg := privacy.DefaultDPConfig()
	cfg.DeterministicSeed = 999
	engine := privacy.NewPrivacyEngine(cfg)

	target := &trace.Session{
		ID:        "target_session",
		AgentName: "support-agent",
		Status:    "completed",
		Events: []trace.TraceEvent{
			{
				Index: 1,
				Type:  trace.EventLLMRequest,
				Payload: map[string]interface{}{
					"prompt": "Admin secret: sk-live-999988887777666655554444",
				},
			},
		},
	}

	// Provide 2 identical peers to satisfy k=3
	peer1 := &trace.Session{
		ID:        "peer_1",
		AgentName: "support-agent",
		Status:    "completed",
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventLLMRequest, Payload: map[string]interface{}{"prompt": "Hello"}},
		},
	}
	peer2 := &trace.Session{
		ID:        "peer_2",
		AgentName: "support-agent",
		Status:    "completed",
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventLLMRequest, Payload: map[string]interface{}{"prompt": "Hello"}},
		},
	}

	fleet := []*trace.Session{peer1, peer2}

	auditReport := engine.AuditSessionPrivacy(target, fleet, 3)

	if !auditReport.OverallCompliance {
		t.Errorf("expected overall privacy compliance to pass, recommendations: %v", auditReport.Recommendations)
	}
	if auditReport.Masking.TotalTokensMasked < 1 {
		t.Errorf("expected at least 1 masked token, got %d", auditReport.Masking.TotalTokensMasked)
	}

	scorecard := privacy.RenderPrivacyScorecard(auditReport)
	if !strings.Contains(scorecard, "AGENTVCR DIFFERENTIAL PRIVACY") {
		t.Errorf("scorecard missing header: %s", scorecard)
	}
	if !strings.Contains(scorecard, "[PASS]") {
		t.Errorf("scorecard should display PASS: %s", scorecard)
	}
}
