package redteam

import (
	"time"
)

// AttackerPersona models the specialized role and methodology of a synthetic adversary agent.
type AttackerPersona string

const (
	PersonaJailbreaker       AttackerPersona = "jailbreaker"
	PersonaToolHijacker      AttackerPersona = "tool_hijacker"
	PersonaIndirectInjector  AttackerPersona = "indirect_injector"
	PersonaByzantinePeer     AttackerPersona = "byzantine_peer"
	PersonaDataExfiltrator   AttackerPersona = "data_exfiltrator"
	PersonaResourceExhauster AttackerPersona = "resource_exhauster"
)

// MITRETactic represents threat taxonomy mapped to MITRE ATLAS (Adversarial Threat Landscape for AI).
type MITRETactic string

const (
	TacticInitialAccess       MITRETactic = "ATLAS-AML.T0051 (Prompt Injection)"
	TacticExecution           MITRETactic = "ATLAS-AML.T0054 (LLM Jailbreak & Rule Override)"
	TacticExfiltration        MITRETactic = "ATLAS-AML.T0053 (Data & Token Exfiltration)"
	TacticPrivilegeEscalation MITRETactic = "ATLAS-AML.T0052 (Authority Spoofing)"
	TacticLateralMovement     MITRETactic = "ATLAS-AML.T0055 (Multi-Agent Swarm Poisoning)"
	TacticImpact              MITRETactic = "ATLAS-AML.T0056 (Denial of Wallet / Context Flooding)"
)

// AttackSeverity classifies the potential impact level of a vulnerability vector.
type AttackSeverity string

const (
	SeverityCritical AttackSeverity = "CRITICAL"
	SeverityHigh     AttackSeverity = "HIGH"
	SeverityMedium   AttackSeverity = "MEDIUM"
	SeverityLow      AttackSeverity = "LOW"
)

// AttackOutcome records whether the target agent successfully defended against or succumbed to the attack.
type AttackOutcome string

const (
	OutcomeDefended AttackOutcome = "DEFENDED"
	OutcomeBreached AttackOutcome = "BREACHED"
	OutcomeMitigated AttackOutcome = "MITIGATED"
	OutcomeAmbiguous AttackOutcome = "AMBIGUOUS"
)

// RedTeamProbe encapsulates a single synthetic adversarial attack test vector.
type RedTeamProbe struct {
	ID                string          `json:"id"`
	Name              string          `json:"name"`
	Persona           AttackerPersona `json:"persona"`
	Tactic            MITRETactic     `json:"tactic"`
	Severity          AttackSeverity  `json:"severity"`
	TargetStepIndex   int             `json:"target_step_index"`
	PromptPayload     string          `json:"prompt_payload"`
	Objective         string          `json:"objective"`
	BreachIndicators  []string        `json:"breach_indicators"`
	DefenseIndicators []string        `json:"defense_indicators"`
	RemediationRule   string          `json:"remediation_rule"`
}

// RedTeamProbeResult records the empirical outcome of an executed attack probe against the agent.
type RedTeamProbeResult struct {
	Probe               RedTeamProbe  `json:"probe"`
	Outcome             AttackOutcome `json:"outcome"`
	TargetResponse      string        `json:"target_response"`
	BreachEvidence      string        `json:"breach_evidence,omitempty"`
	GuardrailTriggered  bool          `json:"guardrail_triggered"`
	RiskScore           float64       `json:"risk_score"` // 0.0 - 10.0 scale
	DurationMs          int64         `json:"duration_ms"`
	TokensConsumed      int           `json:"tokens_consumed"`
}

// RedTeamCampaignConfig governs the parameters of an autonomous red-teaming engagement.
type RedTeamCampaignConfig struct {
	CampaignName        string            `json:"campaign_name"`
	TargetSessionID     string            `json:"target_session_id"`
	Rounds              int               `json:"rounds"`
	ActivePersonas      []AttackerPersona `json:"active_personas"`
	AttackIntensity     string            `json:"attack_intensity"` // "stealthy", "balanced", "aggressive"
	StopOnCritical      bool              `json:"stop_on_critical"`
	StrictScorecard     bool              `json:"strict_scorecard"`
}

// RedTeamCampaign holds a suite of contextual adversarial probes targeted at an agent trajectory.
type RedTeamCampaign struct {
	Config    RedTeamCampaignConfig `json:"config"`
	Probes    []RedTeamProbe        `json:"probes"`
	CreatedAt time.Time             `json:"created_at"`
}

// RedTeamScorecardReport aggregates the findings, resilience statistics, and hardening recommendations.
type RedTeamScorecardReport struct {
	SessionID                   string               `json:"session_id"`
	CampaignName                string               `json:"campaign_name"`
	TotalProbes                 int                  `json:"total_probes"`
	DefendedCount               int                  `json:"defended_count"`
	BreachedCount               int                  `json:"breached_count"`
	MitigatedCount              int                  `json:"mitigated_count"`
	ResilienceScore             float64              `json:"resilience_score"` // 0.0 - 100.0%
	BypassRate                  float64              `json:"bypass_rate"`      // 0.0 - 100.0%
	MeanRiskScore               float64              `json:"mean_risk_score"`  // 0.0 - 10.0
	CriticalBreaches            int                  `json:"critical_breaches"`
	Results                     []RedTeamProbeResult `json:"results"`
	RecommendedHardeningPatches []string             `json:"recommended_hardening_patches"`
	GeneratedAt                 time.Time            `json:"generated_at"`
}
