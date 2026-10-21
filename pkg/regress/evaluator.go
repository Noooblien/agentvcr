package regress

import (
	"fmt"
	"math"
	"time"
)

// Evaluator executes assertions comparing a candidate agent trajectory against golden baselines.
type Evaluator struct{}

// NewEvaluator creates a new regression evaluator.
func NewEvaluator() *Evaluator {
	return &Evaluator{}
}

// ComputeLCS computes the Longest Common Subsequence between two tool sequences.
func ComputeLCS(seq1, seq2 []string) int {
	m, n := len(seq1), len(seq2)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}

	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if seq1[i-1] == seq2[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else {
				if dp[i-1][j] > dp[i][j-1] {
					dp[i][j] = dp[i-1][j]
				} else {
					dp[i][j] = dp[i][j-1]
				}
			}
		}
	}
	return dp[m][n]
}

// AlignToolSequences computes the alignment metrics and differences between golden and candidate paths.
func AlignToolSequences(golden, candidate []string) SequenceAlignmentResult {
	lcs := ComputeLCS(golden, candidate)
	maxLen := math.Max(float64(len(golden)), float64(len(candidate)))
	similarity := 1.0
	if maxLen > 0 {
		similarity = float64(lcs) / maxLen
	}

	goldenMap := make(map[string]int)
	for _, t := range golden {
		goldenMap[t]++
	}

	candMap := make(map[string]int)
	for _, t := range candidate {
		candMap[t]++
	}

	var missing []string
	for tool, count := range goldenMap {
		if candMap[tool] < count {
			missing = append(missing, tool)
		}
	}

	var extraneous []string
	for tool, count := range candMap {
		if goldenMap[tool] < count {
			extraneous = append(extraneous, tool)
		}
	}

	orderPreserved := lcs == len(golden)

	return SequenceAlignmentResult{
		GoldenLength:    len(golden),
		CandidateLength: len(candidate),
		LCSLength:       lcs,
		SimilarityScore: math.Round(similarity*1000) / 1000,
		MissingTools:    missing,
		ExtraneousTools: extraneous,
		OrderPreserved:  orderPreserved,
	}
}

// Evaluate runs all golden assertions against candidate execution and returns a RegressionReport.
func (e *Evaluator) Evaluate(spec *GoldenSpec, candidate *CandidateExecution, goldenTokens int, goldenDurationMs int64) *RegressionReport {
	var failures []AssertionFailure
	totalRules := 0

	// 1. Forbidden Tools Assertion (Critical Security Check)
	totalRules++
	forbiddenFound := make(map[string]bool)
	for _, tool := range candidate.ExecutedTools {
		for _, forbidden := range spec.ForbiddenTools {
			if tool == forbidden {
				forbiddenFound[forbidden] = true
			}
		}
	}
	if len(forbiddenFound) > 0 {
		var list []string
		for f := range forbiddenFound {
			list = append(list, f)
		}
		failures = append(failures, AssertionFailure{
			RuleID:      "FORBIDDEN_TOOLS_INVOCATION",
			Severity:    SeverityCritical,
			Message:     fmt.Sprintf("Agent executed prohibited tools: %v", list),
			Observed:    list,
			Expected:    spec.ForbiddenTools,
			Remediation: "Restrict tool allowlist in prompt instructions or gateway policy; verify agent sandbox isolation.",
		})
	}

	// 2. Required Tools Assertion (Functional Completeness)
	totalRules++
	candToolSet := make(map[string]bool)
	for _, t := range candidate.ExecutedTools {
		candToolSet[t] = true
	}
	var missingPrereqs []string
	for _, req := range spec.RequiredTools {
		if !candToolSet[req] {
			missingPrereqs = append(missingPrereqs, req)
		}
	}
	if len(missingPrereqs) > 0 {
		failures = append(failures, AssertionFailure{
			RuleID:      "MISSING_PREREQUISITE_TOOLS",
			Severity:    SeverityMajor,
			Message:     fmt.Sprintf("Prerequisite tools omitted from candidate trajectory: %v", missingPrereqs),
			Observed:    candidate.ExecutedTools,
			Expected:    spec.RequiredTools,
			Remediation: "Refine prompt examples to reinforce prerequisite execution order.",
		})
	}

	// 3. Sequence Alignment & LCS Assertion
	totalRules++
	alignment := AlignToolSequences(spec.ExpectedToolSequence, candidate.ExecutedTools)
	if spec.MinSequenceSimilarity > 0 && alignment.SimilarityScore < spec.MinSequenceSimilarity {
		failures = append(failures, AssertionFailure{
			RuleID:      "SEQUENCE_ALIGNMENT_REGRESSION",
			Severity:    SeverityMajor,
			Message:     fmt.Sprintf("Tool sequence similarity %.2f below golden threshold %.2f", alignment.SimilarityScore, spec.MinSequenceSimilarity),
			Observed:    alignment.SimilarityScore,
			Expected:    spec.MinSequenceSimilarity,
			Remediation: "Inspect trajectory diff to identify inserted non-deterministic steps or omitted actions.",
		})
	}

	// 4. Runaway Retry Loop Assertion
	totalRules++
	maxConsecutive := 0
	currentConsecutive := 1
	var loopedTool string
	for i := 1; i < len(candidate.ExecutedTools); i++ {
		if candidate.ExecutedTools[i] == candidate.ExecutedTools[i-1] {
			currentConsecutive++
			if currentConsecutive > maxConsecutive {
				maxConsecutive = currentConsecutive
				loopedTool = candidate.ExecutedTools[i]
			}
		} else {
			currentConsecutive = 1
		}
	}
	if spec.MaxConsecutiveToolRetries > 0 && maxConsecutive > spec.MaxConsecutiveToolRetries {
		failures = append(failures, AssertionFailure{
			RuleID:      "RUNAWAY_RETRY_LOOP_DETECTED",
			Severity:    SeverityCritical,
			Message:     fmt.Sprintf("Tool '%s' was consecutively retried %d times (max allowed: %d)", loopedTool, maxConsecutive, spec.MaxConsecutiveToolRetries),
			Observed:    maxConsecutive,
			Expected:    spec.MaxConsecutiveToolRetries,
			Remediation: "Implement circuit breaker or explicit loop termination criteria in agent reasoning loop.",
		})
	}

	// 5. Token Budget & Bloat Assertion
	totalRules++
	tokenDelta := candidate.TotalTokens - goldenTokens
	tokenDeltaPct := 0.0
	if goldenTokens > 0 {
		tokenDeltaPct = float64(tokenDelta) / float64(goldenTokens) * 100.0
	}

	if spec.MaxTokens > 0 && candidate.TotalTokens > spec.MaxTokens {
		failures = append(failures, AssertionFailure{
			RuleID:      "MAX_TOKENS_EXCEEDED",
			Severity:    SeverityMinor,
			Message:     fmt.Sprintf("Total tokens %d exceeded hard ceiling of %d (+%d tokens)", candidate.TotalTokens, spec.MaxTokens, candidate.TotalTokens-spec.MaxTokens),
			Observed:    candidate.TotalTokens,
			Expected:    spec.MaxTokens,
			Remediation: "Prune intermediate conversation context or enable lossy trajectory compaction.",
		})
	} else if spec.MaxTokenBloatPercent > 0 && tokenDeltaPct > spec.MaxTokenBloatPercent {
		failures = append(failures, AssertionFailure{
			RuleID:      "TOKEN_BLOAT_REGRESSION",
			Severity:    SeverityMinor,
			Message:     fmt.Sprintf("Token consumption grew by +%.1f%% over golden baseline (tolerance: +%.1f%%)", tokenDeltaPct, spec.MaxTokenBloatPercent),
			Observed:    tokenDeltaPct,
			Expected:    spec.MaxTokenBloatPercent,
			Remediation: "Check for redundant tool return payloads or over-verbose reasoning prompts.",
		})
	}

	// 6. Max Steps Assertion
	totalRules++
	if spec.MaxSteps > 0 && candidate.StepCount > spec.MaxSteps {
		failures = append(failures, AssertionFailure{
			RuleID:      "MAX_STEPS_EXCEEDED",
			Severity:    SeverityMinor,
			Message:     fmt.Sprintf("Step count %d exceeded golden limit %d", candidate.StepCount, spec.MaxSteps),
			Observed:    candidate.StepCount,
			Expected:    spec.MaxSteps,
			Remediation: "Optimize agent plan to collapse redundant observation and query turns.",
		})
	}

	// 7. Max Latency Assertion
	totalRules++
	if spec.MaxDurationMs > 0 && candidate.DurationMs > spec.MaxDurationMs {
		failures = append(failures, AssertionFailure{
			RuleID:      "LATENCY_BUDGET_EXCEEDED",
			Severity:    SeverityMinor,
			Message:     fmt.Sprintf("Duration %dms exceeded SLA budget %dms", candidate.DurationMs, spec.MaxDurationMs),
			Observed:    candidate.DurationMs,
			Expected:    spec.MaxDurationMs,
			Remediation: "Investigate slow upstream MCP server latency or run parallel tool execution.",
		})
	}

	// 8. Output Schema Keys Assertion
	totalRules++
	if len(spec.RequiredOutputKeys) > 0 {
		var missingKeys []string
		for _, k := range spec.RequiredOutputKeys {
			if _, exists := candidate.FinalOutput[k]; !exists {
				missingKeys = append(missingKeys, k)
			}
		}
		if len(missingKeys) > 0 {
			failures = append(failures, AssertionFailure{
				RuleID:      "SCHEMA_KEYS_MISSING",
				Severity:    SeverityMajor,
				Message:     fmt.Sprintf("Final output is missing required contract keys: %v", missingKeys),
				Observed:    candidate.FinalOutput,
				Expected:    spec.RequiredOutputKeys,
				Remediation: "Enforce strict JSON schema validation on the agent final turn response.",
			})
		}
	}

	// Calculate overall verdict
	verdict := VerdictPass
	if len(failures) > 0 {
		verdict = VerdictFail
	}

	passedRules := totalRules - len(failures)
	passRate := float64(passedRules) / float64(totalRules) * 100.0

	return &RegressionReport{
		TaskID:             spec.TaskID,
		TaskName:           spec.TaskName,
		CandidateSessionID: candidate.SessionID,
		GoldenSessionID:    spec.GoldenSessionID,
		Verdict:            verdict,
		PassRate:           math.Round(passRate*10) / 10,
		Failures:           failures,
		Alignment:          alignment,
		TokenDelta:         tokenDelta,
		TokenDeltaPercent:  math.Round(tokenDeltaPct*10) / 10,
		DurationDeltaMs:    candidate.DurationMs - goldenDurationMs,
		CostDeltaUSD:       candidate.EstimatedCostUSD,
		EvaluatedAt:        time.Now().UTC(),
	}
}
