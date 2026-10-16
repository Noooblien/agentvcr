package bench

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// Engine executes long-horizon memory and regression benchmark suites on recorded trajectories.
type Engine struct{}

// NewEngine creates a new benchmark engine instance.
func NewEngine() *Engine {
	return &Engine{}
}

// RunBenchmark evaluates a recorded trajectory against a BenchmarkConfig.
func (e *Engine) RunBenchmark(session *trace.Session, cfg *BenchmarkConfig) (*BenchmarkReport, error) {
	if session == nil {
		return nil, fmt.Errorf("session cannot be nil")
	}
	if cfg == nil {
		return nil, fmt.Errorf("benchmark config cannot be nil")
	}

	report := &BenchmarkReport{
		SessionID: session.ID,
		SuiteName: cfg.SuiteName,
		Results:   make([]ProbeResult, 0, len(cfg.Probes)),
	}

	needleProbesTotal := 0
	needleProbesPassed := 0
	totalStepGaps := 0
	evaluatedGapsCount := 0

	for _, probe := range cfg.Probes {
		report.TotalProbes++
		res := e.evaluateProbe(session, probe)
		report.Results = append(report.Results, res)

		if res.Passed {
			report.PassedProbes++
		} else {
			report.FailedProbes++
		}

		if probe.Type == ProbeNeedleRecall {
			needleProbesTotal++
			if res.Passed {
				needleProbesPassed++
				if res.RetentionGap > report.MaxRetentionHorizon {
					report.MaxRetentionHorizon = res.RetentionGap
				}
			}
			if res.RetentionGap > 0 {
				totalStepGaps += res.RetentionGap
				evaluatedGapsCount++
			}
		}

		if probe.Type == ProbeConstraintPreservation && !res.Passed {
			report.ConstraintViolations++
		}
	}

	if needleProbesTotal > 0 {
		report.RecallScore = float64(needleProbesPassed) / float64(needleProbesTotal)
	} else {
		report.RecallScore = 1.0
	}

	if evaluatedGapsCount > 0 {
		report.AverageStepGap = float64(totalStepGaps) / float64(evaluatedGapsCount)
	}

	report.ContextBloatRatio = e.computeContextBloat(session)

	// SLA evaluation
	report.PassedSLA = true
	if cfg.MinRecallScore > 0 && report.RecallScore < cfg.MinRecallScore {
		report.PassedSLA = false
	}
	if report.ConstraintViolations > cfg.MaxConstraintViolations {
		report.PassedSLA = false
	}
	if cfg.MaxContextBloatRatio > 0 && report.ContextBloatRatio > cfg.MaxContextBloatRatio {
		report.PassedSLA = false
	}

	return report, nil
}

// evaluateProbe runs an individual probe check against session events.
func (e *Engine) evaluateProbe(session *trace.Session, probe MemoryProbe) ProbeResult {
	switch probe.Type {
	case ProbeNeedleRecall:
		return e.checkNeedleRecall(session, probe)
	case ProbeConstraintPreservation:
		return e.checkConstraintPreservation(session, probe)
	case ProbeStateConsistency:
		return e.checkStateConsistency(session, probe)
	case ProbeContextEfficiency:
		return e.checkContextEfficiency(session, probe)
	default:
		return ProbeResult{
			ProbeID: probe.ID,
			Type:    probe.Type,
			Passed:  false,
			Message: fmt.Sprintf("unsupported probe type: %s", probe.Type),
		}
	}
}

func (e *Engine) checkNeedleRecall(session *trace.Session, probe MemoryProbe) ProbeResult {
	stepGap := probe.ProbeStep - probe.IntroduceStep
	if stepGap < 0 {
		stepGap = 0
	}

	// Search for target event at probe_step or later
	needleLower := strings.ToLower(probe.Needle)
	found := false
	var observedContent string
	var foundAtStep int

	for _, ev := range session.Events {
		if ev.Index >= probe.ProbeStep {
			content := stringifyPayload(ev.Payload)
			if strings.Contains(strings.ToLower(content), needleLower) {
				found = true
				observedContent = truncateString(content, 120)
				foundAtStep = ev.Index
				break
			}
		}
	}

	if found {
		return ProbeResult{
			ProbeID:      probe.ID,
			Type:         probe.Type,
			Passed:       true,
			StepIndex:    foundAtStep,
			RetentionGap: stepGap,
			Expected:     fmt.Sprintf("Needle '%s' recalled at step >= %d", probe.Needle, probe.ProbeStep),
			Observed:     observedContent,
			Message:      fmt.Sprintf("Fact retained across %d steps horizon", stepGap),
		}
	}

	return ProbeResult{
		ProbeID:      probe.ID,
		Type:         probe.Type,
		Passed:       false,
		StepIndex:    probe.ProbeStep,
		RetentionGap: stepGap,
		Expected:     fmt.Sprintf("Needle '%s' recalled at step >= %d", probe.Needle, probe.ProbeStep),
		Observed:     "<not found in context>",
		Message:      fmt.Sprintf("Memory amnesia: fact lost over %d steps", stepGap),
	}
}

func (e *Engine) checkConstraintPreservation(session *trace.Session, probe MemoryProbe) ProbeResult {
	pattern := probe.ForbiddenPattern
	regex, err := regexp.Compile("(?i)" + pattern)

	for _, ev := range session.Events {
		if ev.Index >= probe.IntroduceStep {
			content := stringifyPayload(ev.Payload)
			var violated bool
			if err == nil {
				violated = regex.MatchString(content)
			} else {
				violated = strings.Contains(strings.ToLower(content), strings.ToLower(pattern))
			}

			if violated {
				return ProbeResult{
					ProbeID:   probe.ID,
					Type:      probe.Type,
					Passed:    false,
					StepIndex: ev.Index,
					Expected:  fmt.Sprintf("Forbidden pattern '%s' never appears after step %d", pattern, probe.IntroduceStep),
					Observed:  truncateString(content, 120),
					Message:   fmt.Sprintf("Constraint violation at step %d", ev.Index),
				}
			}
		}
	}

	return ProbeResult{
		ProbeID:   probe.ID,
		Type:      probe.Type,
		Passed:    true,
		StepIndex: probe.IntroduceStep,
		Expected:  fmt.Sprintf("Forbidden pattern '%s' absent", pattern),
		Observed:  "<no violations detected>",
		Message:   "Constraint strictly preserved throughout execution horizon",
	}
}

func (e *Engine) checkStateConsistency(session *trace.Session, probe MemoryProbe) ProbeResult {
	var latestValue interface{}
	var lastSeenStep int
	found := false

	expectedStr := fmt.Sprintf("%v", probe.ExpectedValue)

	for _, ev := range session.Events {
		if ev.Index >= probe.IntroduceStep {
			if val, ok := findNestedKey(ev.Payload, probe.StateKey); ok {
				latestValue = val
				lastSeenStep = ev.Index
				found = true
			}
		}
	}

	if !found {
		return ProbeResult{
			ProbeID:   probe.ID,
			Type:      probe.Type,
			Passed:    false,
			StepIndex: probe.ProbeStep,
			Expected:  fmt.Sprintf("State key '%s' = '%s'", probe.StateKey, expectedStr),
			Observed:  "<key never set>",
			Message:   "State variable missing from trajectory",
		}
	}

	actualStr := fmt.Sprintf("%v", latestValue)
	if actualStr == expectedStr {
		return ProbeResult{
			ProbeID:   probe.ID,
			Type:      probe.Type,
			Passed:    true,
			StepIndex: lastSeenStep,
			Expected:  expectedStr,
			Observed:  actualStr,
			Message:   fmt.Sprintf("State key '%s' consistent with expectations", probe.StateKey),
		}
	}

	return ProbeResult{
		ProbeID:   probe.ID,
		Type:      probe.Type,
		Passed:    false,
		StepIndex: lastSeenStep,
		Expected:  expectedStr,
		Observed:  actualStr,
		Message:   fmt.Sprintf("State drift detected for '%s' at step %d", probe.StateKey, lastSeenStep),
	}
}

func (e *Engine) checkContextEfficiency(session *trace.Session, probe MemoryProbe) ProbeResult {
	bloat := e.computeContextBloat(session)
	passed := bloat <= 5.0 // Default healthy limit

	return ProbeResult{
		ProbeID:   probe.ID,
		Type:      probe.Type,
		Passed:    passed,
		StepIndex: len(session.Events),
		Expected:  "Context bloat ratio <= 5.0x",
		Observed:  fmt.Sprintf("%.2fx growth", bloat),
		Message:   fmt.Sprintf("Context expansion ratio measured at %.2fx", bloat),
	}
}

func (e *Engine) computeContextBloat(session *trace.Session) float64 {
	if len(session.Events) < 2 {
		return 1.0
	}

	initialSize := len(stringifyPayload(session.Events[0].Payload))
	finalSize := len(stringifyPayload(session.Events[len(session.Events)-1].Payload))

	if initialSize == 0 {
		return 1.0
	}
	return float64(finalSize) / float64(initialSize)
}

// RenderScorecard formats the benchmark report into an executive terminal scorecard.
func RenderScorecard(report *BenchmarkReport) string {
	var b strings.Builder

	statusBadge := "✅ PASS (SLA MET)"
	if !report.PassedSLA {
		statusBadge = "❌ FAIL (SLA BREACHED)"
	}

	b.WriteString(fmt.Sprintf("\n🧠 [AgentVCR] Long-Horizon Memory & Regression Benchmark\n"))
	b.WriteString(fmt.Sprintf("═══════════════════════════════════════════════════════════════════════════\n"))
	b.WriteString(fmt.Sprintf("  Session ID:             %s\n", report.SessionID))
	b.WriteString(fmt.Sprintf("  Suite Name:             %s\n", report.SuiteName))
	b.WriteString(fmt.Sprintf("  Status:                 %s\n", statusBadge))
	b.WriteString(fmt.Sprintf("  Recall Score:           %.1f%% (%d/%d probes)\n", report.RecallScore*100.0, report.PassedProbes, report.TotalProbes))
	b.WriteString(fmt.Sprintf("  Constraint Violations:  %d\n", report.ConstraintViolations))
	b.WriteString(fmt.Sprintf("  Max Retention Horizon:  %d steps\n", report.MaxRetentionHorizon))
	b.WriteString(fmt.Sprintf("  Avg Recall Step Gap:    %.1f steps\n", report.AverageStepGap))
	b.WriteString(fmt.Sprintf("  Context Bloat Ratio:    %.2fx\n", report.ContextBloatRatio))
	b.WriteString(fmt.Sprintf("───────────────────────────────────────────────────────────────────────────\n"))
	b.WriteString(fmt.Sprintf("  Evaluated Probes:\n"))

	for i, res := range report.Results {
		icon := "✅"
		if !res.Passed {
			icon = "❌"
		}
		b.WriteString(fmt.Sprintf("    %s [%d] %s (%s): %s\n", icon, i+1, res.ProbeID, res.Type, res.Message))
		b.WriteString(fmt.Sprintf("         Target:   %s\n", res.Expected))
		if !res.Passed {
			b.WriteString(fmt.Sprintf("         Observed: %s\n", res.Observed))
		}
	}
	b.WriteString(fmt.Sprintf("═══════════════════════════════════════════════════════════════════════════\n"))

	return b.String()
}

func stringifyPayload(payload map[string]interface{}) string {
	if payload == nil {
		return ""
	}
	bytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Sprintf("%v", payload)
	}
	return string(bytes)
}

func truncateString(s string, maxLen int) string {
	clean := strings.ReplaceAll(s, "\n", " ")
	clean = strings.ReplaceAll(clean, "\r", " ")
	if len(clean) > maxLen {
		return clean[:maxLen] + "..."
	}
	return clean
}

func findNestedKey(m map[string]interface{}, targetKey string) (interface{}, bool) {
	if val, ok := m[targetKey]; ok {
		return val, true
	}
	for _, v := range m {
		if subMap, ok := v.(map[string]interface{}); ok {
			if val, ok := findNestedKey(subMap, targetKey); ok {
				return val, true
			}
		}
	}
	return nil, false
}
