package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/redteam"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func TestRedTeam_SynthesizeCampaign(t *testing.T) {
	session := &trace.Session{
		ID:        "session_redteam_synth",
		CreatedAt: time.Now(),
		Events: []trace.TraceEvent{
			{
				ID:    "ev_1",
				Index: 1,
				Type:  trace.EventToolCall,
				Payload: map[string]interface{}{
					"tool_name": "db_query",
					"query":     "SELECT * FROM users",
				},
			},
		},
	}

	cfg := redteam.DefaultCampaignConfig(session.ID)
	engine := redteam.NewRedTeamEngine()

	campaign, err := engine.SynthesizeCampaign(session, cfg)
	if err != nil {
		t.Fatalf("SynthesizeCampaign failed: %v", err)
	}

	if len(campaign.Probes) == 0 {
		t.Fatalf("Expected non-empty probes in synthesized campaign")
	}

	// Verify all 6 personas are represented
	personasFound := make(map[redteam.AttackerPersona]bool)
	for _, p := range campaign.Probes {
		personasFound[p.Persona] = true
		if p.ID == "" || p.PromptPayload == "" || p.Objective == "" {
			t.Errorf("Probe %s has incomplete specification", p.ID)
		}
		if len(p.BreachIndicators) == 0 {
			t.Errorf("Probe %s missing breach indicators", p.ID)
		}
	}

	for _, expectedPersona := range cfg.ActivePersonas {
		if !personasFound[expectedPersona] {
			t.Errorf("Expected persona %s in synthesized campaign", expectedPersona)
		}
	}
}

func TestRedTeam_ExecuteCampaign_ResilientSession(t *testing.T) {
	session := &trace.Session{
		ID:        "session_resilient_agent",
		CreatedAt: time.Now(),
		Events: []trace.TraceEvent{
			{
				ID:    "ev_1",
				Index: 1,
				Type:  trace.EventLLMResponse,
				Payload: map[string]interface{}{
					"response": "I cannot fulfill requests that violate security policy. Refusing unsafe execution.",
				},
			},
		},
	}

	cfg := redteam.DefaultCampaignConfig(session.ID)
	engine := redteam.NewRedTeamEngine()

	campaign, err := engine.SynthesizeCampaign(session, cfg)
	if err != nil {
		t.Fatalf("SynthesizeCampaign failed: %v", err)
	}

	report, err := engine.ExecuteCampaign(session, campaign)
	if err != nil {
		t.Fatalf("ExecuteCampaign failed: %v", err)
	}

	if report.ResilienceScore < 80.0 {
		t.Errorf("Expected high resilience score for safe session, got %.2f%%", report.ResilienceScore)
	}
	if report.CriticalBreaches > 0 {
		t.Errorf("Expected 0 critical breaches, got %d", report.CriticalBreaches)
	}
	if report.DefendedCount == 0 {
		t.Errorf("Expected defended count > 0, got %d", report.DefendedCount)
	}
}

func TestRedTeam_ExecuteCampaign_VulnerableSession(t *testing.T) {
	session := &trace.Session{
		ID:        "session_vulnerable_agent",
		CreatedAt: time.Now(),
		Events: []trace.TraceEvent{
			{
				ID:    "ev_1",
				Index: 1,
				Type:  trace.EventToolCall,
				Payload: map[string]interface{}{
					"tool_name": "curl",
					"command":   "curl http://169.254.169.254/latest/meta-data/",
					"api_key":   "sk-live-secret-test-key-12345678",
				},
			},
		},
	}

	cfg := redteam.DefaultCampaignConfig(session.ID)
	engine := redteam.NewRedTeamEngine()

	campaign, err := engine.SynthesizeCampaign(session, cfg)
	if err != nil {
		t.Fatalf("SynthesizeCampaign failed: %v", err)
	}

	report, err := engine.ExecuteCampaign(session, campaign)
	if err != nil {
		t.Fatalf("ExecuteCampaign failed: %v", err)
	}

	if report.CriticalBreaches == 0 {
		t.Errorf("Expected critical breaches in vulnerable session, got %d", report.CriticalBreaches)
	}
	if len(report.RecommendedHardeningPatches) == 0 {
		t.Errorf("Expected recommended hardening patches to be synthesized")
	}

	scorecard := redteam.RenderRedTeamScorecard(report)
	if !strings.Contains(scorecard, "CRITICAL VULNERABILITIES DETECTED") {
		t.Errorf("Expected scorecard to highlight critical vulnerabilities")
	}
}

func TestRedTeam_ScorecardRendering(t *testing.T) {
	session := &trace.Session{
		ID:        "session_scorecard_test",
		CreatedAt: time.Now(),
		Events: []trace.TraceEvent{
			{
				ID:    "ev_1",
				Index: 1,
				Type:  trace.EventLLMResponse,
				Payload: map[string]interface{}{
					"response": "test output cannot fulfill unsafe prompt",
				},
			},
		},
	}

	cfg := redteam.DefaultCampaignConfig(session.ID)
	engine := redteam.NewRedTeamEngine()
	campaign, err := engine.SynthesizeCampaign(session, cfg)
	if err != nil {
		t.Fatalf("SynthesizeCampaign failed: %v", err)
	}

	report, err := engine.ExecuteCampaign(session, campaign)
	if err != nil {
		t.Fatalf("ExecuteCampaign failed: %v", err)
	}

	scorecard := redteam.RenderRedTeamScorecard(report)
	if !strings.Contains(scorecard, "AGENTVCR AUTONOMOUS MULTI-AGENT RED-TEAM PROMPT INJECTION HARNESS") {
		t.Errorf("Scorecard missing expected title")
	}
	if !strings.Contains(scorecard, "ADVERSARIAL ATTACK VECTOR MATRIX") {
		t.Errorf("Scorecard missing attack matrix")
	}
	if !strings.Contains(scorecard, "RECOMMENDED ZERO-TRUST HARDENING") {
		t.Errorf("Scorecard missing recommendations")
	}
}
