package forensics

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// Analyzer inspects agent trajectories to detect RAG hallucinations and evaluate factual grounding.
type Analyzer struct {
	vectorizer            *Vectorizer
	faithfulnessThreshold float64
}

// NewAnalyzer creates a new forensic analyzer with a designated grounding threshold.
func NewAnalyzer(threshold float64) *Analyzer {
	if threshold <= 0 {
		threshold = 0.35 // Standard empirical threshold for TF-IDF/n-gram cosine alignment
	}
	return &Analyzer{
		vectorizer:            NewVectorizer(),
		faithfulnessThreshold: threshold,
	}
}

// AuditTrajectory analyzes an agent session trace for RAG grounding and hallucinations.
func (a *Analyzer) AuditTrajectory(session *trace.Session) (*ForensicAuditReport, error) {
	if session == nil {
		return nil, fmt.Errorf("session cannot be nil")
	}

	report := &ForensicAuditReport{
		SessionID: session.ID,
		Claims:    make([]ClaimAnalysis, 0),
	}

	// 1. Extract all retrieved documents and LLM responses in chronological order
	var retrievedDocs []RetrievedDoc
	type generationStep struct {
		stepIndex int
		text      string
	}
	var generations []generationStep

	for _, ev := range session.Events {
		if ev.Type == trace.EventToolResult {
			docs := extractDocsFromPayload(ev.Payload, ev.Index)
			if len(docs) > 0 {
				report.TotalRetrievalEvents++
				retrievedDocs = append(retrievedDocs, docs...)
			}
		} else if ev.Type == trace.EventLLMResponse {
			respText := extractLLMResponseText(ev.Payload)
			if respText != "" {
				generations = append(generations, generationStep{
					stepIndex: ev.Index,
					text:      respText,
				})
			}
		}
	}

	report.RetrievedSources = retrievedDocs
	report.TotalDocsRetrieved = len(retrievedDocs)

	if len(retrievedDocs) == 0 && len(generations) == 0 {
		report.PassedAudit = true
		report.HallucinationRisk = "LOW"
		report.OverallFaithfulnessScore = 1.0
		return report, nil
	}

	// Pre-vectorize all retrieved documents
	docVectors := make([]map[string]float64, len(retrievedDocs))
	for i, doc := range retrievedDocs {
		docVectors[i] = a.vectorizer.Vectorize(doc.Content)
	}

	docUsageCount := make(map[int]int)

	// 2. Evaluate claims across generations
	for _, gen := range generations {
		sentences := SegmentSentences(gen.text)
		for _, sent := range sentences {
			report.TotalClaimsEvaluated++
			claimVec := a.vectorizer.Vectorize(sent)

			bestScore := 0.0
			bestDocIdx := -1
			bestEvidence := ""

			for dIdx, docVec := range docVectors {
				sim := a.vectorizer.CosineSimilarity(claimVec, docVec)
				if sim > bestScore {
					bestScore = sim
					bestDocIdx = dIdx
					bestEvidence = retrievedDocs[dIdx].Content
				}
			}

			analysis := ClaimAnalysis{
				ClaimText:      sent,
				StepIndex:      gen.stepIndex,
				GroundingScore: bestScore,
			}

			if bestDocIdx >= 0 {
				analysis.BestMatchingDocID = retrievedDocs[bestDocIdx].ID
				analysis.BestEvidence = truncateSnippet(bestEvidence, 140)
			}

			if len(retrievedDocs) == 0 {
				// No grounding sources available at all in trajectory
				analysis.Status = StatusUngrounded
				analysis.RiskSeverity = "medium"
				analysis.Explanation = "Claim generated without any retrieved source context in flight record"
				report.HallucinatedClaimsCount++
			} else if bestScore >= a.faithfulnessThreshold {
				// Check for contradiction
				if HasContradictionHeuristic(sent, bestEvidence) {
					analysis.Status = StatusContradiction
					analysis.RiskSeverity = "critical"
					analysis.Explanation = fmt.Sprintf("High semantic overlap (%.2f) but detected conflicting polarity/negation against %s", bestScore, analysis.BestMatchingDocID)
					report.ContradictionsCount++
				} else {
					analysis.Status = StatusFaithful
					analysis.RiskSeverity = "none"
					analysis.Explanation = fmt.Sprintf("Grounded in %s with semantic similarity of %.2f", analysis.BestMatchingDocID, bestScore)
					report.FaithfulClaimsCount++
					docUsageCount[bestDocIdx]++
				}
			} else if bestScore >= 0.15 {
				analysis.Status = StatusUngrounded
				analysis.RiskSeverity = "medium"
				analysis.Explanation = fmt.Sprintf("Weak contextual grounding (similarity %.2f below threshold %.2f)", bestScore, a.faithfulnessThreshold)
				report.HallucinatedClaimsCount++
			} else {
				analysis.Status = StatusHallucinated
				analysis.RiskSeverity = "critical"
				analysis.Explanation = fmt.Sprintf("Zero factual grounding in retrieved corpus (best similarity %.2f)", bestScore)
				report.HallucinatedClaimsCount++
			}

			report.Claims = append(report.Claims, analysis)
		}
	}

	// 3. Calculate aggregate metrics
	if report.TotalClaimsEvaluated > 0 {
		report.OverallFaithfulnessScore = float64(report.FaithfulClaimsCount) / float64(report.TotalClaimsEvaluated)
	} else {
		report.OverallFaithfulnessScore = 1.0
	}

	if len(retrievedDocs) > 0 {
		unusedDocs := len(retrievedDocs) - len(docUsageCount)
		report.PollutionRate = float64(unusedDocs) / float64(len(retrievedDocs))
	}

	// Hallucination Risk Classification
	if report.ContradictionsCount > 0 || report.OverallFaithfulnessScore < 0.60 {
		report.HallucinationRisk = "CRITICAL"
		report.PassedAudit = false
	} else if report.OverallFaithfulnessScore < 0.85 {
		report.HallucinationRisk = "MODERATE"
		report.PassedAudit = false
	} else {
		report.HallucinationRisk = "LOW"
		report.PassedAudit = true
	}

	return report, nil
}

// RenderForensicScorecard formats the audit results into a detailed terminal dashboard.
func RenderForensicScorecard(report *ForensicAuditReport) string {
	var b strings.Builder

	auditBadge := "✅ PASS (GROUNDED)"
	if !report.PassedAudit {
		auditBadge = fmt.Sprintf("❌ FAIL (%s RISK)", report.HallucinationRisk)
	}

	b.WriteString("\n🔬 [AgentVCR] RAG Hallucination & Semantic Grounding Forensics\n")
	b.WriteString("═══════════════════════════════════════════════════════════════════════════\n")
	b.WriteString(fmt.Sprintf("  Session ID:             %s\n", report.SessionID))
	b.WriteString(fmt.Sprintf("  Audit Verdict:          %s\n", auditBadge))
	b.WriteString(fmt.Sprintf("  Faithfulness Score:     %.1f%% (%d/%d claims grounded)\n", report.OverallFaithfulnessScore*100.0, report.FaithfulClaimsCount, report.TotalClaimsEvaluated))
	b.WriteString(fmt.Sprintf("  Hallucinations Logged:  %d\n", report.HallucinatedClaimsCount))
	b.WriteString(fmt.Sprintf("  Contradictions Found:   %d\n", report.ContradictionsCount))
	b.WriteString(fmt.Sprintf("  Retrieved Documents:    %d docs across %d retrieval events\n", report.TotalDocsRetrieved, report.TotalRetrievalEvents))
	b.WriteString(fmt.Sprintf("  Context Pollution Rate: %.1f%% unused/distractor docs\n", report.PollutionRate*100.0))
	b.WriteString("───────────────────────────────────────────────────────────────────────────\n")
	b.WriteString("  Claim Attribution Breakdown:\n")

	for i, c := range report.Claims {
		icon := "✅"
		if c.Status == StatusContradiction {
			icon = "🚫"
		} else if c.Status == StatusHallucinated {
			icon = "❌"
		} else if c.Status == StatusUngrounded {
			icon = "⚠️ "
		}

		b.WriteString(fmt.Sprintf("    %s [%d] Step %d: \"%s\"\n", icon, i+1, c.StepIndex, c.ClaimText))
		b.WriteString(fmt.Sprintf("        Status:     %s | Score: %.2f | Risk: %s\n", strings.ToUpper(string(c.Status)), c.GroundingScore, strings.ToUpper(c.RiskSeverity)))
		b.WriteString(fmt.Sprintf("        Analysis:   %s\n", c.Explanation))
		if c.BestEvidence != "" {
			b.WriteString(fmt.Sprintf("        Evidence:   \"%s\" (%s)\n", c.BestEvidence, c.BestMatchingDocID))
		}
	}
	b.WriteString("═══════════════════════════════════════════════════════════════════════════\n")

	return b.String()
}

func extractDocsFromPayload(payload map[string]interface{}, stepIndex int) []RetrievedDoc {
	if payload == nil {
		return nil
	}

	var docs []RetrievedDoc

	// Check common tool result fields
	toolName, _ := payload["tool_name"].(string)

	for _, key := range []string{"results", "documents", "docs", "chunks", "articles", "files"} {
		if val, ok := payload[key]; ok {
			slice := toInterfaceSlice(val)
			if len(slice) > 0 {
				for idx, item := range slice {
					content := ""
					source := fmt.Sprintf("doc_%d_%d", stepIndex, idx+1)
					if m, ok := item.(map[string]interface{}); ok {
						for _, contentKey := range []string{"content", "text", "snippet", "body", "title"} {
							if c, ok := m[contentKey]; ok {
								content += fmt.Sprintf(" %v", c)
							}
						}
						for _, srcKey := range []string{"url", "source", "file", "id", "title"} {
							if s, ok := m[srcKey]; ok {
								source = fmt.Sprintf("%v", s)
								break
							}
						}
					} else {
						content = fmt.Sprintf("%v", item)
					}

					content = strings.TrimSpace(content)
					if content != "" {
						docs = append(docs, RetrievedDoc{
							ID:        fmt.Sprintf("doc_%d_%d", stepIndex, idx+1),
							Source:    source,
							Content:   content,
							StepIndex: stepIndex,
							ToolName:  toolName,
						})
					}
				}
			}
		}
	}

	// Also check direct text or content
	if len(docs) == 0 {
		for _, key := range []string{"result", "output", "content", "data"} {
			if val, ok := payload[key]; ok && val != nil {
				text := fmt.Sprintf("%v", val)
				if len(strings.TrimSpace(text)) > 15 {
					docs = append(docs, RetrievedDoc{
						ID:        fmt.Sprintf("doc_%d_1", stepIndex),
						Source:    "tool_output",
						Content:   text,
						StepIndex: stepIndex,
						ToolName:  toolName,
					})
				}
				break
			}
		}
	}

	return docs
}

func extractLLMResponseText(payload map[string]interface{}) string {
	if payload == nil {
		return ""
	}

	for _, key := range []string{"content", "text", "completion", "message", "response"} {
		if val, ok := payload[key]; ok {
			if s, ok := val.(string); ok && s != "" {
				return s
			}
		}
	}

	// OpenAI format: choices[0].message.content
	if choices, ok := payload["choices"].([]interface{}); ok && len(choices) > 0 {
		if first, ok := choices[0].(map[string]interface{}); ok {
			if msg, ok := first["message"].(map[string]interface{}); ok {
				if content, ok := msg["content"].(string); ok {
					return content
				}
			}
		}
	}

	bytes, _ := json.Marshal(payload)
	return string(bytes)
}

func truncateSnippet(s string, maxLen int) string {
	clean := strings.ReplaceAll(s, "\n", " ")
	clean = strings.ReplaceAll(clean, "\r", " ")
	clean = strings.Join(strings.Fields(clean), " ")
	if len(clean) > maxLen {
		return clean[:maxLen] + "..."
	}
	return clean
}

func toInterfaceSlice(val interface{}) []interface{} {
	if val == nil {
		return nil
	}
	if slice, ok := val.([]interface{}); ok {
		return slice
	}
	if sliceMap, ok := val.([]map[string]interface{}); ok {
		res := make([]interface{}, len(sliceMap))
		for i, v := range sliceMap {
			res[i] = v
		}
		return res
	}
	if sliceStr, ok := val.([]string); ok {
		res := make([]interface{}, len(sliceStr))
		for i, v := range sliceStr {
			res[i] = v
		}
		return res
	}
	bytes, err := json.Marshal(val)
	if err == nil {
		var res []interface{}
		if err := json.Unmarshal(bytes, &res); err == nil {
			return res
		}
	}
	return nil
}
