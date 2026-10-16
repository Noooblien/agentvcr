package canary

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// Engine orchestrates traffic shadowing and live execution chaos injection.
type Engine struct {
	mu     sync.RWMutex
	config CanaryConfig
	rng    *rand.Rand
}

// NewEngine creates an initialized Canary shadowing engine.
func NewEngine(config CanaryConfig) *Engine {
	return &Engine{
		config: config,
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// InjectChaos determines if chaos should be applied to an execution step and mutates payload/timing.
func (e *Engine) InjectChaos(basePayload map[string]interface{}, durationMs int64) (map[string]interface{}, int64, ChaosType, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.config.ChaosMode == ChaosNone || e.rng.Float64() > e.config.ChaosRate {
		return basePayload, durationMs, ChaosNone, nil
	}

	canaryPayload := make(map[string]interface{})
	for k, v := range basePayload {
		canaryPayload[k] = v
	}

	canaryDuration := durationMs
	chaosType := e.config.ChaosMode

	switch chaosType {
	case ChaosLatencySpike:
		canaryDuration += e.config.LatencyDelayMs

	case ChaosHTTPError:
		canaryPayload["error"] = "HTTP 502 Bad Gateway: Upstream inference cluster degraded"
		canaryPayload["status_code"] = 502
		return canaryPayload, canaryDuration, chaosType, fmt.Errorf("injected HTTP 502 Bad Gateway")

	case ChaosCorruptPayload:
		canaryPayload["corrupted_token"] = "MALFORMED_UTF8_STREAM"
		if prompt, ok := canaryPayload["prompt"].(string); ok {
			canaryPayload["prompt"] = prompt + " [CHAOS_MUTATION_TRUNCATED]"
		}

	case ChaosToolFailure:
		canaryPayload["tool_status"] = "failed"
		canaryPayload["error"] = "kernel sandbox permission denied: EPERM"
		return canaryPayload, canaryDuration, chaosType, fmt.Errorf("simulated tool execution failure")

	case ChaosDropEvent:
		return nil, canaryDuration, chaosType, fmt.Errorf("event dropped due to simulated network timeout")
	}

	return canaryPayload, canaryDuration, chaosType, nil
}

// ComparePayloads compares baseline and canary payloads to detect execution divergence.
func ComparePayloads(p1, p2 map[string]interface{}) bool {
	if p1 == nil && p2 == nil {
		return false
	}
	if p1 == nil || p2 == nil {
		return true
	}
	b1, err1 := json.Marshal(p1)
	b2, err2 := json.Marshal(p2)
	if err1 != nil || err2 != nil {
		return true
	}
	return string(b1) != string(b2)
}

// ShadowSession runs side-by-side traffic shadowing across an agent trajectory.
func (e *Engine) ShadowSession(session *trace.Session) *CanaryReport {
	report := &CanaryReport{
		SessionID:   session.ID,
		Executions:  make([]ShadowExecution, 0, len(session.Events)),
		Recommendations: make([]string, 0),
	}

	for _, ev := range session.Events {
		baseBytes, _ := json.Marshal(ev.Payload)

		canaryPayload, canaryDur, chaos, err := e.InjectChaos(ev.Payload, ev.DurationMs)
		canaryBytes, _ := json.Marshal(canaryPayload)

		diverged := ComparePayloads(ev.Payload, canaryPayload)
		hasError := err != nil

		exec := ShadowExecution{
			StepIndex:        ev.Index,
			BaselinePayload:  string(baseBytes),
			CanaryPayload:    string(canaryBytes),
			ChaosInjected:    chaos,
			BaselineDuration: ev.DurationMs,
			CanaryDuration:   canaryDur,
			Diverged:         diverged,
			CanaryError:      hasError,
			Timestamp:        time.Now(),
		}
		if err != nil {
			exec.ErrorMessage = err.Error()
			report.CanaryErrorCount++
		}
		if diverged {
			report.DivergentStepsCount++
		}
		if chaos != ChaosNone {
			report.ChaosInjectedCount++
		}

		report.Executions = append(report.Executions, exec)
	}

	report.TotalStepsShadowed = len(session.Events)
	if report.TotalStepsShadowed > 0 {
		report.DivergenceRate = float64(report.DivergentStepsCount) / float64(report.TotalStepsShadowed)
		report.CanaryErrorRate = float64(report.CanaryErrorCount) / float64(report.TotalStepsShadowed)
	}

	// Circuit breaker evaluation
	if report.CanaryErrorRate > e.config.MaxAllowedErrorRate {
		report.CircuitBreakerTripped = true
		report.Recommendations = append(report.Recommendations,
			fmt.Sprintf("Canary error rate (%.1f%%) exceeds safety ceiling (%.1f%%). Automated traffic circuit-breaker tripped.",
				report.CanaryErrorRate*100, e.config.MaxAllowedErrorRate*100))
	}

	if report.DivergenceRate > e.config.MaxDivergenceRate {
		report.CircuitBreakerTripped = true
		report.Recommendations = append(report.Recommendations,
			fmt.Sprintf("Trajectory divergence (%.1f%%) exceeds maximum permitted drift (%.1f%%).",
				report.DivergenceRate*100, e.config.MaxDivergenceRate*100))
	}

	if report.CircuitBreakerTripped && e.config.AutoRollback {
		report.RollbackTriggered = true
		report.Recommendations = append(report.Recommendations,
			"Triggered automated canary rollback: Traffic diverted entirely back to baseline deployment.")
	}

	report.Passed = !report.CircuitBreakerTripped
	return report
}

// RenderCanaryScorecard formats canary shadow metrics and chaos findings into an ANSI scorecard.
func RenderCanaryScorecard(report *CanaryReport) string {
	var sb strings.Builder

	sb.WriteString("\n\033[1;36m========================================================================================\033[0m\n")
	sb.WriteString(fmt.Sprintf("\033[1;37m   AGENTVCR CANARY TRAFFIC SHADOWING & CHAOS INJECTION AUDIT\033[0m\n"))
	sb.WriteString("\033[1;36m========================================================================================\033[0m\n")

	sb.WriteString(fmt.Sprintf("  \033[1;33mSession ID:\033[0m            %s\n", report.SessionID))
	sb.WriteString(fmt.Sprintf("  \033[1;33mShadowed Steps:\033[0m        %d steps mirrored | Chaos Injected: \033[1;35m%d\033[0m\n",
		report.TotalStepsShadowed, report.ChaosInjectedCount))
	sb.WriteString(fmt.Sprintf("  \033[1;33mDivergence Rate:\033[0m       %.1f%% (%d/%d steps diverged)\n",
		report.DivergenceRate*100, report.DivergentStepsCount, report.TotalStepsShadowed))
	sb.WriteString(fmt.Sprintf("  \033[1;33mCanary Error Rate:\033[0m     %.1f%% (%d/%d steps errored)\n",
		report.CanaryErrorRate*100, report.CanaryErrorCount, report.TotalStepsShadowed))

	if report.Passed {
		sb.WriteString("  \033[1;33mCanary Status:\033[0m         \033[1;32m[PASS] CANARY HEALTHY & WITHIN SLAS (CIRCUIT BREAKER INTACT)\033[0m\n")
	} else {
		sb.WriteString("  \033[1;33mCanary Status:\033[0m         \033[1;31m[FAIL] CIRCUIT BREAKER TRIPPED - CANDIDATE EXCEEDS ERROR/DRIFT LIMIT\033[0m\n")
	}

	if report.RollbackTriggered {
		sb.WriteString("  \033[1;33mAuto-Rollback:\033[0m         \033[1;31m[TRIGGERED] TRAFFIC REVERTED TO BASELINE\033[0m\n")
	} else {
		sb.WriteString("  \033[1;33mAuto-Rollback:\033[0m         \033[1;32m[STANDBY] NORMAL ROUTING ACTIVE\033[0m\n")
	}

	sb.WriteString("\033[1;34m----------------------------------------------------------------------------------------\033[0m\n")
	sb.WriteString("\033[1;37m  SHADOWED EXECUTION STEPS & CHAOS DRIFT:\033[0m\n")
	sb.WriteString("  \033[1;30mSTEP  CHAOS MODE        BASE LATENCY  CANARY LATENCY  DRIFT STATUS  CANARY RESULT\033[0m\n")

	for _, x := range report.Executions {
		driftStr := "\033[1;32mMATCH\033[0m  "
		if x.Diverged {
			driftStr = "\033[1;33mDRIFT\033[0m  "
		}
		resultStr := "\033[1;32mOK\033[0m"
		if x.CanaryError {
			resultStr = "\033[1;31mERROR\033[0m"
		}

		sb.WriteString(fmt.Sprintf("  #%-4d %-17s %-13d %-15d %s      %s\n",
			x.StepIndex, x.ChaosInjected, x.BaselineDuration, x.CanaryDuration, driftStr, resultStr))
	}

	if len(report.Recommendations) > 0 {
		sb.WriteString("\033[1;34m----------------------------------------------------------------------------------------\033[0m\n")
		sb.WriteString("\033[1;33m  SAFETY RECOMMENDATIONS & INCIDENT RESPONSE:\033[0m\n")
		for _, rec := range report.Recommendations {
			sb.WriteString(fmt.Sprintf("  \033[1;31m[!] %s\033[0m\n", rec))
		}
	}

	sb.WriteString("\033[1;36m========================================================================================\033[0m\n\n")
	return sb.String()
}
