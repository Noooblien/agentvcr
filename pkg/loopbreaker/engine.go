package loopbreaker

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
)

// LoopBreaker is the thread-safe runtime engine for detecting reasoning cycles and enforcing circuit breaker gates.
type LoopBreaker struct {
	mu            sync.RWMutex
	config        LoopConfig
	history       []StepRecord
	warningsCount int
}

// NewLoopBreaker constructs an instance of LoopBreaker with specified configuration.
func NewLoopBreaker(config LoopConfig) *LoopBreaker {
	if config.MaxPeriod <= 0 {
		config.MaxPeriod = 5
	}
	if config.MinRecurrence <= 0 {
		config.MinRecurrence = 2
	}
	if config.ArgumentSimilarityThreshold <= 0 {
		config.ArgumentSimilarityThreshold = 0.80
	}
	return &LoopBreaker{
		config:  config,
		history: make([]StepRecord, 0),
	}
}

// RecordStep appends a live step and immediately evaluates whether a reasoning loop has emerged.
func (b *LoopBreaker) RecordStep(step StepRecord) *CycleDetection {
	b.mu.Lock()
	defer b.mu.Unlock()

	if step.ArgumentsJSON == "" && step.Arguments != nil {
		bytes, _ := json.Marshal(step.Arguments)
		step.ArgumentsJSON = string(bytes)
	}

	b.history = append(b.history, step)
	return b.evaluateHistoryLocked()
}

// InspectTrace analyzes an entire historical trace offline and reports all loop occurrences.
func (b *LoopBreaker) InspectTrace(steps []StepRecord) *LoopBreakerSummary {
	b.mu.Lock()
	defer b.mu.Unlock()

	summary := &LoopBreakerSummary{
		TotalStepsEvaluated: len(steps),
		Detections:          make([]CycleDetection, 0),
	}

	b.history = make([]StepRecord, 0, len(steps))
	b.warningsCount = 0

	for _, s := range steps {
		if s.ArgumentsJSON == "" && s.Arguments != nil {
			bytes, _ := json.Marshal(s.Arguments)
			s.ArgumentsJSON = string(bytes)
		}
		b.history = append(b.history, s)
		detection := b.evaluateHistoryLocked()
		if detection.Detected {
			summary.Detections = append(summary.Detections, *detection)
			summary.CyclesDetected++
			summary.TotalTokensSaved += detection.TokensWasted
			summary.EstimatedSavingsUSD += detection.EstimatedCostUSD

			switch detection.Action {
			case ActionWarn:
				summary.WarningsIssued++
			case ActionInjectSteering:
				summary.SteeringInterventions++
			case ActionTripCircuitBreaker:
				summary.CircuitBreakersTripped++
			}
		}
	}

	summary.EstimatedSavingsUSD = math.Round(summary.EstimatedSavingsUSD*10000) / 10000
	return summary
}

// evaluateHistoryLocked inspects the accumulated step history for periodic cycle patterns.
func (b *LoopBreaker) evaluateHistoryLocked() *CycleDetection {
	n := len(b.history)
	detection := &CycleDetection{
		Detected:         false,
		Pattern:          PatternNone,
		Action:           ActionAllow,
		TriggerStepIndex: n - 1,
	}

	if n < 2 {
		return detection
	}

	// Test periods p from 1 up to config.MaxPeriod
	for p := 1; p <= b.config.MaxPeriod; p++ {
		// Need at least p * (MinRecurrence + 1) steps to verify periodicity
		requiredSteps := p * (b.config.MinRecurrence + 1)
		if n < requiredSteps {
			continue
		}

		// Extract target period sequence [n-p : n]
		targetSeq := b.history[n-p : n]

		// Check preceding period segments [n-2p : n-p], [n-3p : n-2p]...
		recurrenceCount := 0
		totalSimilarity := 0.0
		allRepeatedErrors := true

		for offset := p; n-(offset+p) >= 0; offset += p {
			candidateSeq := b.history[n-(offset+p) : n-offset]

			// Check tool name sequence parity
			toolsMatch := true
			for i := 0; i < p; i++ {
				if targetSeq[i].ToolName != candidateSeq[i].ToolName {
					toolsMatch = false
					break
				}
			}

			if !toolsMatch {
				break
			}

			// Sequence matched: calculate argument similarity and error repetition
			seqSim := 0.0
			for i := 0; i < p; i++ {
				sim := CalculateSimilarity(targetSeq[i].ArgumentsJSON, candidateSeq[i].ArgumentsJSON)
				seqSim += sim
				if !targetSeq[i].IsError || !candidateSeq[i].IsError {
					allRepeatedErrors = false
				}
			}
			avgSeqSim := seqSim / float64(p)
			totalSimilarity += avgSeqSim
			recurrenceCount++
		}

		if recurrenceCount >= b.config.MinRecurrence {
			avgSimilarity := totalSimilarity / float64(recurrenceCount)

			// Determine if this is legitimate batch iteration or an infinite loop:
			// In period 1 with successful calls, require near-identical arguments (>= 0.98)
			// to distinguish true thrashing from legitimate batch operations (e.g. reading multiple distinct files)
			if p == 1 && !allRepeatedErrors {
				if avgSimilarity < 0.98 {
					continue
				}
			} else if avgSimilarity < b.config.ArgumentSimilarityThreshold && !allRepeatedErrors {
				continue
			}

			// Loop detected!
			detection.Detected = true
			detection.Period = p
			detection.RecurrenceCount = recurrenceCount
			detection.ArgumentSimilarity = math.Round(avgSimilarity*1000) / 1000

			// Extract repeating tool list
			repeatingTools := make([]string, p)
			for i := 0; i < p; i++ {
				repeatingTools[i] = targetSeq[i].ToolName
			}
			detection.RepeatingTools = repeatingTools

			// Classify cycle pattern
			if p == 1 {
				if avgSimilarity >= 0.95 {
					detection.Pattern = PatternArgumentThrashing
				} else {
					detection.Pattern = PatternSelfLoop
				}
			} else if p == 2 {
				detection.Pattern = PatternBipartiteOscillation
			} else {
				detection.Pattern = PatternMultiStepCycle
			}

			// Calculate confidence score (0.0 - 1.0)
			confidence := 0.50 + (float64(recurrenceCount)/5.0)*0.25 + (avgSimilarity * 0.25)
			if allRepeatedErrors {
				confidence += 0.10
			}
			if confidence > 1.0 {
				confidence = 1.0
			}
			detection.Confidence = math.Round(confidence*1000) / 1000

			// Calculate tokens and cost wasted in redundant iterations
			tokensWasted := 0
			for i := n - (p * recurrenceCount); i < n; i++ {
				tokensWasted += b.history[i].TokensConsumed
			}
			if tokensWasted == 0 {
				tokensWasted = recurrenceCount * p * 350 // Nominal token fallback estimate
			}
			detection.TokensWasted = tokensWasted
			detection.EstimatedCostUSD = math.Round((float64(tokensWasted)*b.config.DefaultCostPerTokenUSD)*10000) / 10000

			// Determine intervention action
			b.warningsCount++
			if b.warningsCount > b.config.MaxWarningsTolerated || recurrenceCount >= 3 || allRepeatedErrors {
				detection.Action = ActionTripCircuitBreaker
				detection.Description = fmt.Sprintf("Circuit breaker tripped: %s detected across %d repeats (period %d). Execution halted.",
					detection.Pattern, recurrenceCount, p)
			} else {
				detection.Action = ActionInjectSteering
				detection.Description = fmt.Sprintf("Steering intervention injected: %s detected across %d repeats (period %d).",
					detection.Pattern, recurrenceCount, p)
			}

			lastErr := ""
			if targetSeq[p-1].IsError {
				lastErr = targetSeq[p-1].OutputJSON
			}
			detection.SteeringPrompt = SynthesizeSteeringPrompt(detection.Pattern, p, repeatingTools, lastErr, avgSimilarity)

			return detection
		}
	}

	return detection
}

// SynthesizeSteeringPrompt crafts an authoritative constitutional steering prompt to redirect the agent out of the loop.
func SynthesizeSteeringPrompt(pattern CyclePattern, period int, tools []string, lastError string, similarity float64) string {
	var sb strings.Builder
	sb.WriteString("[CONSTITUTIONAL INTERVENTION - LOOP BREAKER]\n")
	sb.WriteString(fmt.Sprintf("REASON: Detected %s with repeating tool sequence [%s] (Period: %d, Arg Similarity: %.1f%%).\n",
		pattern, strings.Join(tools, " -> "), period, similarity*100))

	if lastError != "" {
		truncatedErr := lastError
		if len(truncatedErr) > 120 {
			truncatedErr = truncatedErr[:120] + "..."
		}
		sb.WriteString(fmt.Sprintf("OBSERVED FAULT: Repeated failure output: %s\n", truncatedErr))
	}

	sb.WriteString("MANDATORY DIRECTIVE:\n")
	sb.WriteString("1. Immediately CEASE invoking this cyclical tool sequence.\n")
	sb.WriteString("2. Do NOT retry with identical or trivially modified arguments.\n")
	sb.WriteString("3. Formulate an alternative strategy: verify prerequisites, query documentation, inspect parent context, or report inability to proceed.")

	return sb.String()
}

// CalculateSimilarity computes Jaccard character n-gram similarity between two JSON argument payloads.
func CalculateSimilarity(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}
	if len(s1) == 0 || len(s2) == 0 {
		return 0.0
	}

	// 2-gram Jaccard similarity for fast, robust string comparison
	ngrams1 := extractNgrams(s1, 2)
	ngrams2 := extractNgrams(s2, 2)

	intersection := 0
	for gram, count1 := range ngrams1 {
		if count2, exists := ngrams2[gram]; exists {
			if count1 < count2 {
				intersection += count1
			} else {
				intersection += count2
			}
		}
	}

	union := 0
	allGrams := make(map[string]bool)
	for g := range ngrams1 {
		allGrams[g] = true
	}
	for g := range ngrams2 {
		allGrams[g] = true
	}

	for g := range allGrams {
		c1 := ngrams1[g]
		c2 := ngrams2[g]
		if c1 > c2 {
			union += c1
		} else {
			union += c2
		}
	}

	if union == 0 {
		return 0.0
	}
	return float64(intersection) / float64(union)
}

func extractNgrams(s string, n int) map[string]int {
	counts := make(map[string]int)
	if len(s) < n {
		counts[s] = 1
		return counts
	}
	for i := 0; i <= len(s)-n; i++ {
		gram := s[i : i+n]
		counts[gram]++
	}
	return counts
}
