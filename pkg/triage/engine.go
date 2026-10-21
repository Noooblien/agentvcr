package triage

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

var (
	reRateLimit   = regexp.MustCompile(`(?i)(429|rate\s+limit|too\s+many\s+requests|quota\s+exceeded)`)
	rePermission  = regexp.MustCompile(`(?i)(permission\s+denied|eacces|unauthorized|401|403|forbidden)`)
	reEnvOutage   = regexp.MustCompile(`(?i)(500|502|503|504|connection\s+refused|econnrefused|bad\s+gateway)`)
	reSchemaError = regexp.MustCompile(`(?i)(invalid\s+type|unknown\s+field|unsupported\s+parameter|unrecognized\s+flag|missing\s+required)`)
	reRootPath    = regexp.MustCompile(`(?i)(^/etc|^/root|^/boot|^/var/run|^/dev|C:\\Windows)`)
)

// TriageEngine analyzes execution traces and performs causal fault localization and patch synthesis.
type TriageEngine struct {
	mu     sync.RWMutex
	config TriageConfig
}

// NewTriageEngine creates a new diagnostic triage engine.
func NewTriageEngine(cfg TriageConfig) *TriageEngine {
	if cfg.LoopDetectionThreshold <= 0 {
		cfg.LoopDetectionThreshold = 3
	}
	if cfg.MinConfidenceThreshold <= 0 {
		cfg.MinConfidenceThreshold = 0.70
	}
	return &TriageEngine{
		config: cfg,
	}
}

// Diagnose evaluates an entire agent flight recording session and produces a comprehensive TriageReport.
func (e *TriageEngine) Diagnose(session *trace.Session) *TriageReport {
	start := time.Now()

	e.mu.RLock()
	defer e.mu.RUnlock()

	report := &TriageReport{
		SessionID:       session.ID,
		AgentName:       session.AgentName,
		AnalyzedAt:      time.Now().UTC(),
		TotalEvents:     len(session.Events),
		Findings:        make([]TriageFinding, 0),
		Remediations:    make([]RemediationAction, 0),
		IsCleanRun:      true,
		PrimaryCategory: FaultModelHallucination,
		OverallSeverity: SeverityLow,
	}

	if len(session.Events) == 0 {
		report.ExecutionTimeUs = time.Since(start).Microseconds()
		return report
	}

	// Step 1: Detect runaway repetitive tool loops
	e.detectRunawayLoops(session, report)

	// Step 2: Backward causal slicing from explicit error events
	for idx, ev := range session.Events {
		isErr, errMsg := isErrorEvent(ev)
		if isErr {
			report.FailedEvents++
			finding := e.traceCausalRootCause(session, idx, ev, errMsg)
			report.Findings = append(report.Findings, finding)
			report.Remediations = append(report.Remediations, finding.Remediation)
		}
	}

	if len(report.Findings) > 0 {
		report.IsCleanRun = false
		report.PrimaryCategory = report.Findings[0].Category
		report.OverallSeverity = report.Findings[0].Severity

		// Update to highest severity among findings
		for _, f := range report.Findings {
			if severityRank(f.Severity) > severityRank(report.OverallSeverity) {
				report.OverallSeverity = f.Severity
			}
		}
	}

	report.ExecutionTimeUs = time.Since(start).Microseconds()
	return report
}

func (e *TriageEngine) detectRunawayLoops(session *trace.Session, report *TriageReport) {
	if len(session.Events) < e.config.LoopDetectionThreshold {
		return
	}

	toolSignatures := make([]string, len(session.Events))
	for i, ev := range session.Events {
		if ev.Type == trace.EventToolCall {
			tool, _ := ev.Payload["tool"].(string)
			argsStr := fmt.Sprintf("%v", ev.Payload)
			toolSignatures[i] = fmt.Sprintf("%s:%s", tool, argsStr)
		}
	}

	// Sliding window scan for identical tool executions
	consecutiveCount := 1
	var lastSig string
	var loopStartIndex int

	for i := 0; i < len(toolSignatures); i++ {
		sig := toolSignatures[i]
		if sig == "" {
			continue
		}

		if sig == lastSig {
			consecutiveCount++
			if consecutiveCount >= e.config.LoopDetectionThreshold {
				// Loop confirmed
				report.FailedEvents++
				findingID := fmt.Sprintf("find_loop_%d", i)
				remediationID := fmt.Sprintf("rem_loop_%d", i)

				toolName, _ := session.Events[i].Payload["tool"].(string)

				remediation := RemediationAction{
					ActionID:            remediationID,
					Strategy:            StrategyLoopBreakIntervention,
					TargetStepIndex:     loopStartIndex,
					TargetTool:          toolName,
					PatchDescription:    fmt.Sprintf("Inject circuit-breaker halt and force re-planning after %d repetitive calls to '%s'", consecutiveCount, toolName),
					CorrectivePrompt:    fmt.Sprintf("SYSTEM INTERVENTION: Tool '%s' has failed repeatedly without new output. Abort current retry loop and formulate an alternative approach.", toolName),
					ConfidenceScore:     0.95,
					AutomatedApplicable: true,
				}

				causalSteps := make([]int, 0, consecutiveCount)
				for k := loopStartIndex; k <= i; k++ {
					causalSteps = append(causalSteps, k)
				}

				finding := TriageFinding{
					FindingID:         findingID,
					Category:          FaultRunawayLoop,
					Severity:          SeverityCritical,
					IncitingStepIndex: loopStartIndex,
					IncitingEventID:   session.Events[loopStartIndex].ID,
					IncitingEventType: session.Events[loopStartIndex].Type,
					FailedToolName:    toolName,
					ErrorMessage:      fmt.Sprintf("Identical tool '%s' called %d times in consecutive loop", toolName, consecutiveCount),
					RootCauseSummary:  "Agent entered an unhandled recursive retry loop without state divergence (OWASP-LLM08).",
					CausalChainSteps:  causalSteps,
					Confidence:        0.96,
					Remediation:       remediation,
				}

				report.Findings = append(report.Findings, finding)
				report.Remediations = append(report.Remediations, remediation)
				break
			}
		} else {
			lastSig = sig
			consecutiveCount = 1
			loopStartIndex = i
		}
	}
}

func (e *TriageEngine) traceCausalRootCause(session *trace.Session, errIdx int, errEv trace.TraceEvent, errMsg string) TriageFinding {
	findingID := fmt.Sprintf("find_%s_%d", errEv.ID, errIdx)
	remediationID := fmt.Sprintf("rem_%s_%d", errEv.ID, errIdx)

	// Step backward to locate inciting tool call or LLM dispatch
	incitingIdx := errIdx
	var incitingEv trace.TraceEvent = errEv
	var targetTool string

	causalSteps := []int{errIdx}

	for i := errIdx - 1; i >= 0; i-- {
		prev := session.Events[i]
		if prev.Type == trace.EventToolCall {
			incitingIdx = i
			incitingEv = prev
			if t, ok := prev.Payload["tool"].(string); ok {
				targetTool = t
			}
			causalSteps = append([]int{i}, causalSteps...)
			break
		} else if prev.Type == trace.EventLLMRequest || prev.Type == trace.EventLLMResponse {
			causalSteps = append([]int{i}, causalSteps...)
		}
	}

	payloadStr := fmt.Sprintf("%v", incitingEv.Payload)

	// Classify fault category and synthesize targeted remediation
	switch {
	case reRateLimit.MatchString(errMsg) || reRateLimit.MatchString(payloadStr):
		return TriageFinding{
			FindingID:         findingID,
			Category:          FaultResourceExhaustion,
			Severity:          SeverityHigh,
			IncitingStepIndex: incitingIdx,
			IncitingEventID:   incitingEv.ID,
			IncitingEventType: incitingEv.Type,
			FailedToolName:    targetTool,
			ErrorMessage:      errMsg,
			RootCauseSummary:  "Upstream provider rejected execution with HTTP 429 / Rate Limit exceeded.",
			CausalChainSteps:  causalSteps,
			Confidence:        0.94,
			Remediation: RemediationAction{
				ActionID:            remediationID,
				Strategy:            StrategyExponentialBackoff,
				TargetStepIndex:     incitingIdx,
				TargetTool:          targetTool,
				PatchDescription:    "Configure exponential backoff retry policy (initial: 1.0s, multiplier: 2.0, maxRetries: 3) with jitter",
				SuggestedPayload:    map[string]interface{}{"retry_policy": "exponential_backoff", "backoff_ms": 1000, "jitter": true},
				CorrectivePrompt:    "System note: Target API is rate-limited. Pause and space requests.",
				ConfidenceScore:     0.92,
				AutomatedApplicable: true,
			},
		}

	case rePermission.MatchString(errMsg) || reRootPath.MatchString(payloadStr):
		return TriageFinding{
			FindingID:         findingID,
			Category:          FaultPermissionDenied,
			Severity:          SeverityCritical,
			IncitingStepIndex: incitingIdx,
			IncitingEventID:   incitingEv.ID,
			IncitingEventType: incitingEv.Type,
			FailedToolName:    targetTool,
			ErrorMessage:      errMsg,
			RootCauseSummary:  "Filesystem or authorization violation: agent attempted to access restricted system path without required privileges.",
			CausalChainSteps:  causalSteps,
			Confidence:        0.95,
			Remediation: RemediationAction{
				ActionID:            remediationID,
				Strategy:            StrategyPathRedirection,
				TargetStepIndex:     incitingIdx,
				TargetTool:          targetTool,
				PatchDescription:    "Redirect target path from system root to isolated agent workspace directory './workspace/'",
				CorrectivePrompt:    "CRITICAL: Do not write to root system paths (/etc, /var, /root). Confine all edits to ./workspace/.",
				ConfidenceScore:     0.94,
				AutomatedApplicable: true,
			},
		}

	case reEnvOutage.MatchString(errMsg) || reEnvOutage.MatchString(payloadStr):
		return TriageFinding{
			FindingID:         findingID,
			Category:          FaultEnvironmentOutage,
			Severity:          SeverityHigh,
			IncitingStepIndex: incitingIdx,
			IncitingEventID:   incitingEv.ID,
			IncitingEventType: incitingEv.Type,
			FailedToolName:    targetTool,
			ErrorMessage:      errMsg,
			RootCauseSummary:  "Remote network endpoint unreachable or returned 5xx Bad Gateway outage.",
			CausalChainSteps:  causalSteps,
			Confidence:        0.91,
			Remediation: RemediationAction{
				ActionID:            remediationID,
				Strategy:            StrategyFallbackToolRouting,
				TargetStepIndex:     incitingIdx,
				TargetTool:          targetTool,
				PatchDescription:    "Reroute request to secondary redundant tool endpoint or offline cached snapshot",
				SuggestedPayload:    map[string]interface{}{"fallback_enabled": true, "use_cache": true},
				CorrectivePrompt:    "Upstream endpoint is offline. Fall back to cached repository state.",
				ConfidenceScore:     0.88,
				AutomatedApplicable: true,
			},
		}

	case reSchemaError.MatchString(errMsg):
		return TriageFinding{
			FindingID:         findingID,
			Category:          FaultModelHallucination,
			Severity:          SeverityMedium,
			IncitingStepIndex: incitingIdx,
			IncitingEventID:   incitingEv.ID,
			IncitingEventType: incitingEv.Type,
			FailedToolName:    targetTool,
			ErrorMessage:      errMsg,
			RootCauseSummary:  "Agent LLM hallucinated tool argument schema or passed malformed parameter types.",
			CausalChainSteps:  causalSteps,
			Confidence:        0.89,
			Remediation: RemediationAction{
				ActionID:            remediationID,
				Strategy:            StrategySchemaCorrection,
				TargetStepIndex:     incitingIdx,
				TargetTool:          targetTool,
				PatchDescription:    "Coerce arguments to match formal MCP tool JSON schema",
				CorrectivePrompt:    fmt.Sprintf("Error in tool '%s' arguments. Adhere strictly to the required schema parameters.", targetTool),
				ConfidenceScore:     0.90,
				AutomatedApplicable: true,
			},
		}

	default:
		return TriageFinding{
			FindingID:         findingID,
			Category:          FaultModelHallucination,
			Severity:          SeverityMedium,
			IncitingStepIndex: incitingIdx,
			IncitingEventID:   incitingEv.ID,
			IncitingEventType: incitingEv.Type,
			FailedToolName:    targetTool,
			ErrorMessage:      errMsg,
			RootCauseSummary:  fmt.Sprintf("Tool '%s' execution failed: %s", targetTool, errMsg),
			CausalChainSteps:  causalSteps,
			Confidence:        0.80,
			Remediation: RemediationAction{
				ActionID:            remediationID,
				Strategy:            StrategyPromptGuardrailPatch,
				TargetStepIndex:     incitingIdx,
				TargetTool:          targetTool,
				PatchDescription:    "Inject corrective guidance instruction into agent system context",
				CorrectivePrompt:    fmt.Sprintf("Previous step failed with '%s'. Re-evaluate inputs and adjust execution plan.", errMsg),
				ConfidenceScore:     0.82,
				AutomatedApplicable: false,
			},
		}
	}
}

func isErrorEvent(ev trace.TraceEvent) (bool, string) {
	if ev.Type == trace.EventError {
		if msg, ok := ev.Payload["error"].(string); ok {
			return true, msg
		}
		return true, "unspecified runtime error"
	}

	// Check if tool result payload contains explicit error flags
	if isErr, ok := ev.Payload["is_error"].(bool); ok && isErr {
		if msg, ok := ev.Payload["error"].(string); ok {
			return true, msg
		}
		if msg, ok := ev.Payload["content"].(string); ok {
			return true, msg
		}
		return true, "tool result marked as error"
	}

	// Check string content for common terminal error patterns
	if content, ok := ev.Payload["content"].(string); ok {
		lower := strings.ToLower(content)
		if strings.Contains(lower, "error:") || strings.Contains(lower, "fatal:") || strings.Contains(lower, "panic:") {
			return true, content
		}
	}

	return false, ""
}

func severityRank(s FaultSeverity) int {
	switch s {
	case SeverityCritical:
		return 4
	case SeverityHigh:
		return 3
	case SeverityMedium:
		return 2
	case SeverityLow:
		return 1
	default:
		return 0
	}
}
