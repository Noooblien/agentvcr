package diff

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"

	"github.com/Noooblien/agentvcr/pkg/metrics"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

// AnalyzeSemanticDiff performs sequence alignment and multi-dimensional semantic comparison
// between a baseline trajectory and a candidate/forked trajectory.
func AnalyzeSemanticDiff(base, target *trace.Session, opts DiffOptions) *SemanticDiffReport {
	if opts.Tolerance <= 0 {
		opts.Tolerance = 0.80
	}

	baseMetrics := metrics.ComputeSessionMetrics(base)
	targetMetrics := metrics.ComputeSessionMetrics(target)

	report := &SemanticDiffReport{
		BaseSessionID:     base.ID,
		TargetSessionID:   target.ID,
		BaseAgentName:     base.AgentName,
		TargetAgentName:   target.AgentName,
		TotalBaseEvents:   len(base.Events),
		TotalTargetEvents: len(target.Events),
		TotalTokenDelta:   targetMetrics.TotalTokens - baseMetrics.TotalTokens,
		CostDeltaUSD:      targetMetrics.EstimatedCostUSD - baseMetrics.EstimatedCostUSD,
		DurationDeltaMs:   targetMetrics.TotalDurationMs - baseMetrics.TotalDurationMs,
	}

	// 1. Perform Global Sequence Alignment (Needleman-Wunsch / Dynamic Time Warping variant)
	aligned := alignTrajectories(base.Events, target.Events)

	var totalSimilarity float64
	var baseTools, targetTools []string

	for _, pair := range aligned {
		step := evaluateAlignedStep(pair.base, pair.target, pair.baseIdx, pair.targetIdx, opts)
		report.AlignedSteps = append(report.AlignedSteps, step)

		totalSimilarity += step.SimilarityScore

		switch step.Classification {
		case DiffMatchExact:
			report.ExactMatches++
		case DiffMatchSemantic:
			report.SemanticMatches++
		case DiffParamDrift:
			report.ParamDrifts++
		case DiffControlFlowDrift:
			report.ControlFlowDrifts++
		case DiffStepInserted:
			report.Insertions++
		case DiffStepDeleted:
			report.Deletions++
		case DiffOutcomeRegression, DiffErrorInduced:
			report.Regressions++
		}

		if step.BaseToolName != "" {
			baseTools = append(baseTools, step.BaseToolName)
		}
		if step.TargetToolName != "" {
			targetTools = append(targetTools, step.TargetToolName)
		}
	}

	// 2. Compute Stability Score & Tool Sequence Similarity
	maxSteps := len(aligned)
	if maxSteps == 0 {
		maxSteps = 1
	}

	rawStability := (totalSimilarity / float64(maxSteps)) * 100.0

	// Deduct penalties only for severe regressions, unexpected errors, and control-flow breaks
	penalty := float64(report.Regressions)*25.0 + float64(report.ControlFlowDrifts)*5.0
	finalStability := math.Max(0.0, math.Min(100.0, rawStability-penalty))
	report.StabilityScore = math.Round(finalStability*10) / 10

	report.ToolSequenceSimilarity = math.Round(computeSequenceSimilarity(baseTools, targetTools)*1000) / 10

	// 3. Evaluate Verdict
	tolerancePct := opts.Tolerance * 100.0
	if report.Regressions > 0 {
		report.Verdict = VerdictFail
		report.VerdictReason = fmt.Sprintf("Critical regression detected: %d execution failures or error-induced steps observed.", report.Regressions)
	} else if opts.Strict && (report.ControlFlowDrifts > 0 || report.StabilityScore < 90.0) {
		report.Verdict = VerdictFail
		report.VerdictReason = fmt.Sprintf("Strict policy breach: %d control-flow divergences detected (stability: %.1f%%).", report.ControlFlowDrifts, report.StabilityScore)
	} else if report.StabilityScore < tolerancePct {
		report.Verdict = VerdictFail
		report.VerdictReason = fmt.Sprintf("Stability score (%.1f%%) fell below required threshold (%.1f%%).", report.StabilityScore, tolerancePct)
	} else if report.ParamDrifts > 0 || report.Insertions > 0 || report.Deletions > 0 || report.ControlFlowDrifts > 0 {
		report.Verdict = VerdictWarn
		report.VerdictReason = fmt.Sprintf("Trajectories aligned with minor semantic/parameter drift within tolerance (%.1f%% stability).", report.StabilityScore)
	} else {
		report.Verdict = VerdictPass
		report.VerdictReason = "Zero regressions: candidate trajectory matches baseline execution semantics."
	}

	return report
}

type alignedPair struct {
	base      *trace.TraceEvent
	target    *trace.TraceEvent
	baseIdx   int
	targetIdx int
}

func alignTrajectories(baseEvents, targetEvents []trace.TraceEvent) []alignedPair {
	n := len(baseEvents)
	m := len(targetEvents)

	if n == 0 && m == 0 {
		return nil
	}

	// DP Matrix for Needleman-Wunsch Alignment
	dp := make([][]float64, n+1)
	for i := range dp {
		dp[i] = make([]float64, m+1)
	}

	gapPenalty := -0.5
	for i := 0; i <= n; i++ {
		dp[i][0] = float64(i) * gapPenalty
	}
	for j := 0; j <= m; j++ {
		dp[0][j] = float64(j) * gapPenalty
	}

	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			sim := scoreStepSimilarity(baseEvents[i-1], targetEvents[j-1])
			matchScore := dp[i-1][j-1] + sim
			deleteScore := dp[i-1][j] + gapPenalty
			insertScore := dp[i][j-1] + gapPenalty

			dp[i][j] = math.Max(matchScore, math.Max(deleteScore, insertScore))
		}
	}

	// Traceback to reconstruct optimal alignment
	var result []alignedPair
	i, j := n, m
	for i > 0 || j > 0 {
		if i > 0 && j > 0 && dp[i][j] == dp[i-1][j-1]+scoreStepSimilarity(baseEvents[i-1], targetEvents[j-1]) {
			bEvt := baseEvents[i-1]
			tEvt := targetEvents[j-1]
			result = append(result, alignedPair{base: &bEvt, target: &tEvt, baseIdx: i - 1, targetIdx: j - 1})
			i--
			j--
		} else if i > 0 && (j == 0 || dp[i][j] == dp[i-1][j]+gapPenalty) {
			bEvt := baseEvents[i-1]
			result = append(result, alignedPair{base: &bEvt, target: nil, baseIdx: i - 1, targetIdx: -1})
			i--
		} else {
			tEvt := targetEvents[j-1]
			result = append(result, alignedPair{base: nil, target: &tEvt, baseIdx: -1, targetIdx: j - 1})
			j--
		}
	}

	// Reverse to restore forward chronological order
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}

	return result
}

func scoreStepSimilarity(b, t trace.TraceEvent) float64 {
	if b.Type == t.Type {
		if b.Type == trace.EventToolCall {
			bTool := extractToolName(b.Payload)
			tTool := extractToolName(t.Payload)
			if bTool == tTool && bTool != "" {
				return 1.5 // Strong match for same tool call
			}
			return 0.3
		}
		return 1.0
	}
	return -0.8
}

func evaluateAlignedStep(base, target *trace.TraceEvent, bIdx, tIdx int, opts DiffOptions) StepDiff {
	diff := StepDiff{
		StepIndexBase:   bIdx,
		StepIndexTarget: tIdx,
	}

	// 1. Deletion (Present in Base, missing in Target)
	if target == nil {
		diff.BaseType = base.Type
		diff.BaseToolName = extractToolName(base.Payload)
		diff.BaseSummary = extractSummary(base.Payload)
		diff.Classification = DiffStepDeleted
		diff.SimilarityScore = 0.0
		diff.LatencyDeltaMs = -base.DurationMs
		return diff
	}

	// 2. Insertion (Missing in Base, added in Target)
	if base == nil {
		diff.TargetType = target.Type
		diff.TargetToolName = extractToolName(target.Payload)
		diff.TargetSummary = extractSummary(target.Payload)
		diff.Classification = DiffStepInserted
		diff.SimilarityScore = 0.0
		diff.LatencyDeltaMs = target.DurationMs
		return diff
	}

	// 3. Both steps present
	diff.BaseType = base.Type
	diff.TargetType = target.Type
	diff.BaseToolName = extractToolName(base.Payload)
	diff.TargetToolName = extractToolName(target.Payload)
	diff.BaseSummary = extractSummary(base.Payload)
	diff.TargetSummary = extractSummary(target.Payload)
	diff.LatencyDeltaMs = target.DurationMs - base.DurationMs

	// Check for outcome regression (e.g. error in target while base succeeded)
	if target.Type == trace.EventError || isErrorPayload(target.Payload) {
		if base.Type != trace.EventError && !isErrorPayload(base.Payload) {
			diff.Classification = DiffOutcomeRegression
			diff.SimilarityScore = 0.1
			diff.ParamDiffs = append(diff.ParamDiffs, fmt.Sprintf("Target failed with error: %s", diff.TargetSummary))
			return diff
		}
		diff.Classification = DiffErrorInduced
		diff.SimilarityScore = 0.5
		return diff
	}

	// Control flow mismatch
	if base.Type != target.Type {
		diff.Classification = DiffControlFlowDrift
		diff.SimilarityScore = 0.2
		diff.ParamDiffs = append(diff.ParamDiffs, fmt.Sprintf("Event type changed: '%s' -> '%s'", base.Type, target.Type))
		return diff
	}

	// Tool call comparison
	if base.Type == trace.EventToolCall {
		if diff.BaseToolName != diff.TargetToolName {
			diff.Classification = DiffControlFlowDrift
			diff.SimilarityScore = 0.3
			diff.ParamDiffs = append(diff.ParamDiffs, fmt.Sprintf("Tool swapped: '%s' -> '%s'", diff.BaseToolName, diff.TargetToolName))
			return diff
		}

		// Compare parameters
		pDiffs, paramScore := comparePayloadParameters(base.Payload, target.Payload)
		diff.ParamDiffs = pDiffs

		if len(pDiffs) > 0 {
			diff.Classification = DiffParamDrift
			diff.SimilarityScore = 0.75 + 0.25*paramScore
		} else {
			diff.Classification = DiffMatchExact
			diff.SimilarityScore = 1.0
		}
		return diff
	}

	// Tool result comparison
	if base.Type == trace.EventToolResult && target.Type == trace.EventToolResult {
		if payloadsEqual(base.Payload, target.Payload) {
			diff.Classification = DiffMatchExact
			diff.SimilarityScore = 1.0
			return diff
		}
		diff.Classification = DiffMatchSemantic
		diff.SimilarityScore = 0.90
		return diff
	}

	// LLM text / response comparison
	if base.Type == trace.EventLLMRequest || base.Type == trace.EventLLMResponse {
		if payloadsEqual(base.Payload, target.Payload) {
			diff.Classification = DiffMatchExact
			diff.SimilarityScore = 1.0
			return diff
		}

		textSim := computeTextJaccardSimilarity(diff.BaseSummary, diff.TargetSummary)
		if textSim >= 0.70 || (opts.IgnoreCosmeticParaphrase && textSim >= 0.25) {
			diff.Classification = DiffMatchSemantic
			diff.SimilarityScore = math.Max(0.85, textSim)
		} else {
			diff.Classification = DiffParamDrift
			diff.SimilarityScore = math.Max(0.60, textSim)
			diff.ParamDiffs = append(diff.ParamDiffs, fmt.Sprintf("Semantic drift: prompt/response diverged (similarity: %.0f%%)", textSim*100))
		}
		return diff
	}

	// General payload check
	if payloadsEqual(base.Payload, target.Payload) {
		diff.Classification = DiffMatchExact
		diff.SimilarityScore = 1.0
	} else {
		diff.Classification = DiffParamDrift
		diff.SimilarityScore = 0.7
	}

	return diff
}

func extractToolName(payload map[string]interface{}) string {
	if payload == nil {
		return ""
	}
	keys := []string{"name", "tool", "tool_name", "cmd", "function"}
	for _, k := range keys {
		if v, ok := payload[k].(string); ok && v != "" {
			return v
		}
	}
	if fn, ok := payload["function"].(map[string]interface{}); ok {
		if v, ok := fn["name"].(string); ok {
			return v
		}
	}
	return ""
}

func extractSummary(payload map[string]interface{}) string {
	if payload == nil {
		return ""
	}
	keys := []string{"cmd", "query", "path", "prompt", "p", "msg", "message", "content", "error", "reason", "output"}
	for _, k := range keys {
		if v, ok := payload[k].(string); ok && v != "" {
			return cleanSummaryText(v)
		}
	}
	if body, ok := payload["body"].(map[string]interface{}); ok {
		if choices, ok := body["choices"].([]interface{}); ok && len(choices) > 0 {
			if choice, ok := choices[0].(map[string]interface{}); ok {
				if msg, ok := choice["message"].(map[string]interface{}); ok {
					if content, ok := msg["content"].(string); ok {
						return cleanSummaryText(content)
					}
				}
			}
		}
	}
	b, _ := json.Marshal(payload)
	return cleanSummaryText(string(b))
}

func cleanSummaryText(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}

func isErrorPayload(payload map[string]interface{}) bool {
	if payload == nil {
		return false
	}
	if _, ok := payload["error"]; ok {
		return true
	}
	if status, ok := payload["status"].(string); ok && (status == "failed" || status == "error") {
		return true
	}
	return false
}

func comparePayloadParameters(b, t map[string]interface{}) ([]string, float64) {
	var diffs []string
	allKeys := make(map[string]bool)
	for k := range b {
		if k != "timestamp" && k != "id" && k != "duration_ms" {
			allKeys[k] = true
		}
	}
	for k := range t {
		if k != "timestamp" && k != "id" && k != "duration_ms" {
			allKeys[k] = true
		}
	}

	matches := 0
	total := len(allKeys)
	if total == 0 {
		return nil, 1.0
	}

	var sortedKeys []string
	for k := range allKeys {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)

	for _, k := range sortedKeys {
		vB, hasB := b[k]
		vT, hasT := t[k]

		if hasB && !hasT {
			diffs = append(diffs, fmt.Sprintf("Param '%s' removed (was: %v)", k, vB))
		} else if !hasB && hasT {
			diffs = append(diffs, fmt.Sprintf("Param '%s' added (val: %v)", k, vT))
		} else {
			if !reflect.DeepEqual(vB, vT) {
				diffs = append(diffs, fmt.Sprintf("Param '%s' modified: %v -> %v", k, vB, vT))
			} else {
				matches++
			}
		}
	}

	score := float64(matches) / float64(total)
	return diffs, score
}

func computeTextJaccardSimilarity(s1, s2 string) float64 {
	cleaner := func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return ' '
	}
	w1 := strings.Fields(strings.Map(cleaner, strings.ToLower(s1)))
	w2 := strings.Fields(strings.Map(cleaner, strings.ToLower(s2)))

	if len(w1) == 0 && len(w2) == 0 {
		return 1.0
	}
	if len(w1) == 0 || len(w2) == 0 {
		return 0.0
	}

	set1 := make(map[string]bool)
	for _, w := range w1 {
		set1[w] = true
	}

	set2 := make(map[string]bool)
	for _, w := range w2 {
		set2[w] = true
	}

	intersection := 0
	for w := range set1 {
		if set2[w] {
			intersection++
		}
	}

	union := len(set1) + len(set2) - intersection
	if union == 0 {
		return 1.0
	}
	return float64(intersection) / float64(union)
}

func computeSequenceSimilarity(seq1, seq2 []string) float64 {
	if len(seq1) == 0 && len(seq2) == 0 {
		return 1.0
	}
	if len(seq1) == 0 || len(seq2) == 0 {
		return 0.0
	}

	matches := 0
	minLen := len(seq1)
	if len(seq2) < minLen {
		minLen = len(seq2)
	}

	for i := 0; i < minLen; i++ {
		if seq1[i] == seq2[i] {
			matches++
		}
	}

	maxLen := len(seq1)
	if len(seq2) > maxLen {
		maxLen = len(seq2)
	}

	return float64(matches) / float64(maxLen)
}
