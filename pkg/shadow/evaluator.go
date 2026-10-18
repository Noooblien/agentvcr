package shadow

import (
	"fmt"
	"math"
	"strings"
	"unicode"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// ComputeSemanticSimilarity calculates the token-level Jaccard & N-gram overlap similarity
// between two output strings, returning a normalized score in [0.0, 1.0].
func ComputeSemanticSimilarity(strA, strB string) float64 {
	tokensA := tokenize(strA)
	tokensB := tokenize(strB)

	if len(tokensA) == 0 && len(tokensB) == 0 {
		return 1.0
	}
	if len(tokensA) == 0 || len(tokensB) == 0 {
		return 0.0
	}

	setA := make(map[string]int)
	for _, t := range tokensA {
		setA[t]++
	}

	setB := make(map[string]int)
	for _, t := range tokensB {
		setB[t]++
	}

	intersection := 0
	union := 0

	allKeys := make(map[string]bool)
	for k := range setA {
		allKeys[k] = true
	}
	for k := range setB {
		allKeys[k] = true
	}

	for k := range allKeys {
		countA := setA[k]
		countB := setB[k]

		intersection += int(math.Min(float64(countA), float64(countB)))
		union += int(math.Max(float64(countA), float64(countB)))
	}

	if union == 0 {
		return 1.0
	}
	return float64(intersection) / float64(union)
}

// CompareEvents performs step-by-step telemetry, payload, and drift analysis across two events.
func CompareEvents(stepIdx int, evtBase, evtCand trace.TraceEvent) StepComparison {
	basePayloadStr := fmt.Sprintf("%v", evtBase.Payload)
	candPayloadStr := fmt.Sprintf("%v", evtCand.Payload)

	baseDuration := evtBase.DurationMs
	if baseDuration == 0 {
		baseDuration = 100
	}
	candDuration := evtCand.DurationMs
	if candDuration == 0 {
		candDuration = 100
	}
	latencyDelta := candDuration - baseDuration

	baseTokens := extractTokens(evtBase.Payload)
	candTokens := extractTokens(evtCand.Payload)
	tokenDelta := candTokens - baseTokens

	similarity := ComputeSemanticSimilarity(basePayloadStr, candPayloadStr)
	driftScore := math.Max(0.0, math.Min(1.0, 1.0-similarity))

	// Tool divergence check
	toolDiverged := false
	if evtBase.Type == trace.EventToolCall || evtCand.Type == trace.EventToolCall {
		baseTool, _ := evtBase.Payload["tool"].(string)
		candTool, _ := evtCand.Payload["tool"].(string)
		if baseTool != candTool {
			toolDiverged = true
			driftScore = math.Max(driftScore, 0.70) // Penalize tool divergence
		}
	}

	baseErr := evtBase.Type == trace.EventError || containsError(evtBase.Payload)
	candErr := evtCand.Type == trace.EventError || containsError(evtCand.Payload)
	if candErr && !baseErr {
		driftScore = 1.0 // Severe drift due to unhandled candidate error
	}

	severity := SeverityNone
	rationale := "Payloads aligned within tolerance"
	switch {
	case driftScore >= 0.75:
		severity = SeverityCritical
		rationale = "Critical semantic divergence or candidate failure"
	case driftScore >= 0.50:
		severity = SeverityHigh
		rationale = "High semantic drift or tool variation"
	case driftScore >= 0.25:
		severity = SeverityModerate
		rationale = "Moderate payload divergence"
	case driftScore >= 0.10:
		severity = SeverityLow
		rationale = "Minor phrasing variations"
	}

	return StepComparison{
		StepIndex:          stepIdx,
		EventType:          evtBase.Type,
		BaselinePayload:    basePayloadStr,
		CandidatePayload:   candPayloadStr,
		BaselineLatencyMs:  baseDuration,
		CandidateLatencyMs: candDuration,
		LatencyDeltaMs:     latencyDelta,
		BaselineTokens:     baseTokens,
		CandidateTokens:    candTokens,
		TokenDelta:         tokenDelta,
		SemanticSimilarity: similarity,
		DriftScore:         driftScore,
		DriftSeverity:      severity,
		ToolCallDiverged:   toolDiverged,
		BaselineError:      baseErr,
		CandidateError:     candErr,
		DriftRationale:     rationale,
		Timestamp:          evtBase.Timestamp,
	}
}

func tokenize(s string) []string {
	words := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	return words
}

func extractTokens(payload map[string]interface{}) int {
	if payload == nil {
		return 0
	}
	if tok, ok := payload["tokens"].(int); ok {
		return tok
	}
	if tok, ok := payload["tokens"].(float64); ok {
		return int(tok)
	}
	if tok, ok := payload["token_usage"].(int); ok {
		return tok
	}
	// Fallback heuristic: 4 chars per token
	contentStr := fmt.Sprintf("%v", payload)
	return len(contentStr) / 4
}

func containsError(payload map[string]interface{}) bool {
	if payload == nil {
		return false
	}
	if _, ok := payload["error"]; ok {
		return true
	}
	if status, ok := payload["status"].(string); ok && strings.ToLower(status) == "failed" {
		return true
	}
	return false
}
