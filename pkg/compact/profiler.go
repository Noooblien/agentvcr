package compact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// Profiler analyzes token curves, duplicates, and KV-cache efficiency across agent traces.
type Profiler struct {
	config CompactorConfig
}

// NewProfiler creates an initialized token waste profiler.
func NewProfiler(config CompactorConfig) *Profiler {
	return &Profiler{config: config}
}

// EstimateTokens calculates an approximate token count from arbitrary JSON payload or text.
func EstimateTokens(val interface{}) int {
	if val == nil {
		return 0
	}
	switch v := val.(type) {
	case string:
		tokens := len(v) / 4
		if tokens == 0 && len(v) > 0 {
			return 1
		}
		return tokens
	case map[string]interface{}:
		// Check for explicit usage fields first
		if usage, ok := v["usage"].(map[string]interface{}); ok {
			if tot, ok := usage["total_tokens"].(float64); ok && tot > 0 {
				return int(tot)
			}
		}
		raw, err := json.Marshal(v)
		if err != nil {
			return 0
		}
		return len(raw) / 4
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return 0
		}
		return len(raw) / 4
	}
}

// hashPayload computes a SHA-256 digest of an event payload for equality checks.
func hashPayload(val interface{}) string {
	raw, err := json.Marshal(val)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// extractToolName tries to pull tool name from event payload.
func extractToolName(evt trace.TraceEvent) string {
	if name, ok := evt.Payload["tool_name"].(string); ok && name != "" {
		return name
	}
	if name, ok := evt.Payload["name"].(string); ok && name != "" {
		return name
	}
	return ""
}

// ProfileSession scans a session's execution events to profile token distributions and detect waste.
func (p *Profiler) ProfileSession(session *trace.Session) (*TokenProfile, error) {
	if session == nil {
		return nil, fmt.Errorf("session is nil")
	}

	profile := &TokenProfile{
		SessionID:          session.ID,
		Anomalies:          make([]RedundancyAnomaly, 0),
		AnalyzedAt:         time.Now(),
		KVCacheHitRatioEst: 0.85, // Default theoretical max baseline
	}

	seenToolResultHashes := make(map[string]int) // hash -> stepIndex
	lastFailedTool := ""
	lastFailedStep := -1

	// Regex to spot volatile prefixes (e.g. Current Time: 2026-10-17T...)
	volatilePrefixRegex := regexp.MustCompile(`(?i)(timestamp|current local time|session_id|nonce)[\s:=]+[0-9a-zA-Z\-_:.]+`)

	for _, evt := range session.Events {
		eventTokens := EstimateTokens(evt.Payload)
		profile.TotalTokens += eventTokens

		switch evt.Type {
		case trace.EventLLMRequest:
			profile.PromptTokens += eventTokens
			// Check prompt body for KV-cache jitter
			if promptStr, ok := evt.Payload["prompt"].(string); ok {
				if loc := volatilePrefixRegex.FindStringIndex(promptStr); loc != nil && loc[0] < 200 {
					profile.Anomalies = append(profile.Anomalies, RedundancyAnomaly{
						StepIndex:    evt.Index,
						EventType:    string(evt.Type),
						Category:     "KV_CACHE_JITTER",
						TokensWasted: eventTokens / 4,
						Details:      "Volatile timestamp/nonce found in prompt header; invalidates upstream KV-cache prefix hits.",
					})
					profile.RedundantTokens += (eventTokens / 4)
					profile.KVCacheHitRatioEst -= 0.15
				}
			}

		case trace.EventLLMResponse:
			profile.CompletionTokens += eventTokens

		case trace.EventToolCall:
			toolName := extractToolName(evt)
			if toolName == lastFailedTool && lastFailedStep >= 0 {
				wasted := eventTokens
				profile.Anomalies = append(profile.Anomalies, RedundancyAnomaly{
					StepIndex:    evt.Index,
					EventType:    string(evt.Type),
					ToolName:     toolName,
					Category:     "RETRY_LOOP",
					TokensWasted: wasted,
					Details:      fmt.Sprintf("Repeated tool invocation for '%s' following failure at step #%d.", toolName, lastFailedStep),
				})
				profile.RedundantTokens += wasted
			}

		case trace.EventToolResult:
			profile.ToolPayloadTokens += eventTokens
			toolName := extractToolName(evt)
			h := hashPayload(evt.Payload)

			if origStep, exists := seenToolResultHashes[h]; exists {
				// Exact duplicate result payload detected!
				profile.Anomalies = append(profile.Anomalies, RedundancyAnomaly{
					StepIndex:    evt.Index,
					EventType:    string(evt.Type),
					ToolName:     toolName,
					Category:     "DUPLICATE_TOOL",
					TokensWasted: eventTokens,
					Details:      fmt.Sprintf("Duplicate tool payload identical to output from step #%d.", origStep),
				})
				profile.RedundantTokens += eventTokens
			} else if h != "" {
				seenToolResultHashes[h] = evt.Index
			}

			// Check for overly verbose tool output
			if p.config.MaxToolResultTokens > 0 && eventTokens > p.config.MaxToolResultTokens {
				excess := eventTokens - p.config.MaxToolResultTokens
				profile.Anomalies = append(profile.Anomalies, RedundancyAnomaly{
					StepIndex:    evt.Index,
					EventType:    string(evt.Type),
					ToolName:     toolName,
					Category:     "VERBOSE_OUTPUT",
					TokensWasted: excess,
					Details:      fmt.Sprintf("Tool output exceeds configured ceiling of %d tokens (%d actual tokens).", p.config.MaxToolResultTokens, eventTokens),
				})
				profile.RedundantTokens += excess
			}

			// Check if tool resulted in error
			if isErr, ok := evt.Payload["isError"].(bool); ok && isErr {
				lastFailedTool = toolName
				lastFailedStep = evt.Index
			} else {
				lastFailedTool = ""
				lastFailedStep = -1
			}

		case trace.EventError:
			lastFailedStep = evt.Index
		}
	}

	if profile.KVCacheHitRatioEst < 0.10 {
		profile.KVCacheHitRatioEst = 0.10
	}

	if profile.TotalTokens > 0 {
		profile.WastePercentage = (float64(profile.RedundantTokens) / float64(profile.TotalTokens)) * 100.0
	}

	// Cost model: $3.00/1M prompt, $15.00/1M completion
	promptCost := (float64(profile.PromptTokens) / 1000000.0) * 3.00
	compCost := (float64(profile.CompletionTokens) / 1000000.0) * 15.00
	profile.EstimatedCostUSD = promptCost + compCost

	return profile, nil
}
