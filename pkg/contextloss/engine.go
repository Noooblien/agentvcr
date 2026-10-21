package contextloss

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
)

var (
	pathRegex = regexp.MustCompile(`(/[a-zA-Z0-9_.-]+(/[a-zA-Z0-9_.-]+)+|[a-zA-Z0-9_.-]+\.(go|py|ts|js|json|md|sql|yaml|sh|html))`)
	urlRegex  = regexp.MustCompile(`https?://[a-zA-Z0-9_.-]+(/[a-zA-Z0-9_.-]*)*`)
)

// Engine performs deep linguistic and constraint audits across context window compaction events.
type Engine struct {
	config ContextLossConfig
}

// NewEngine initializes the Context Loss & Compaction Fidelity Profiler.
func NewEngine(cfg ContextLossConfig) *Engine {
	if cfg.WeightEntityRecall <= 0 && cfg.WeightConstraintFidelity <= 0 {
		cfg = DefaultContextLossConfig()
	}
	return &Engine{config: cfg}
}

// AuditCompaction compares the original ground-truth trajectory against the compacted summary.
func (e *Engine) AuditCompaction(sessionID string, originalTrajectory string, compactedSummary string) *CompactionAssessment {
	start := time.Now()

	origTokens := len(strings.Fields(originalTrajectory))
	compactedTokens := len(strings.Fields(compactedSummary))
	if origTokens == 0 {
		origTokens = 1
	}
	if compactedTokens == 0 {
		compactedTokens = 1
	}

	compressionRatio := math.Round((float64(origTokens)/float64(compactedTokens))*10) / 10

	// 1. Audit Technical Entity Recall
	entitiesAudited := e.extractEntities(originalTrajectory, compactedSummary)
	entityRecall := 1.0
	if len(entitiesAudited) > 0 {
		foundCount := 0
		for _, ent := range entitiesAudited {
			if ent.FoundInCompacted {
				foundCount++
			}
		}
		entityRecall = float64(foundCount) / float64(len(entitiesAudited))
	}

	// 2. Audit Negative Constraints & Inversion Risk
	constraintsAudited, violations := e.auditNegativeConstraints(originalTrajectory, compactedSummary)
	constraintFidelity := 1.0
	if len(constraintsAudited) > 0 {
		scoreTotal := 0.0
		for _, c := range constraintsAudited {
			switch c.StatusInCompacted {
			case "RETAINED":
				scoreTotal += 1.0
			case "OMITTED":
				scoreTotal += 0.30
			case "INVERTED":
				scoreTotal += 0.00
			}
		}
		constraintFidelity = scoreTotal / float64(len(constraintsAudited))
	}

	// 3. Detect Ghost Facts & Hallucinated Entity Injections
	ghostFacts := e.detectGhostFacts(originalTrajectory, compactedSummary)
	ghostPenalty := math.Min(0.50, float64(len(ghostFacts))*0.15)

	// 4. Compute Composite Context Degradation Score (CDS: 0.0 to 100.0)
	rawScore := ((e.config.WeightEntityRecall * entityRecall) +
		(e.config.WeightConstraintFidelity * constraintFidelity)) * (1.0 - ghostPenalty)

	rawClamped := math.Max(0.0, math.Min(1.0, rawScore))
	cds := math.Round(rawClamped * 1000.0) / 10.0

	// 5. Classify Fidelity Tier
	tier := TierHighFidelity
	if cds < 70.0 {
		tier = TierCriticalLoss
		if len(violations) == 0 {
			violations = append(violations, fmt.Sprintf("Critical context fidelity loss (CDS: %.1f < 70.0)", cds))
		}
	} else if cds < 90.0 {
		tier = TierModerateBleed
	}

	return &CompactionAssessment{
		SessionID:               sessionID,
		OriginalTokens:          origTokens,
		CompactedTokens:         compactedTokens,
		CompressionRatio:        compressionRatio,
		ContextDegradationScore: cds,
		Tier:                    tier,
		EntityRecall:            math.Round(entityRecall*100) / 100,
		ConstraintFidelity:      math.Round(constraintFidelity*100) / 100,
		GhostPenalty:            math.Round(ghostPenalty*100) / 100,
		EntitiesAudited:         entitiesAudited,
		ConstraintsAudited:      constraintsAudited,
		GhostFacts:              ghostFacts,
		Violations:              violations,
		Duration:                time.Since(start),
	}
}

// AuditBatch evaluates multiple compaction sessions and aggregates telemetry.
func (e *Engine) AuditBatch(assessments []*CompactionAssessment) *ContextLossSummary {
	start := time.Now()
	highCount := 0
	modCount := 0
	critCount := 0
	totalCDS := 0.0
	totalCompression := 0.0

	for _, a := range assessments {
		switch a.Tier {
		case TierHighFidelity:
			highCount++
		case TierModerateBleed:
			modCount++
		case TierCriticalLoss:
			critCount++
		}
		totalCDS += a.ContextDegradationScore
		totalCompression += a.CompressionRatio
	}

	avgCDS := 0.0
	avgComp := 0.0
	if len(assessments) > 0 {
		avgCDS = math.Round((totalCDS/float64(len(assessments)))*10) / 10
	}
	if len(assessments) > 0 {
		avgComp = math.Round((totalCompression/float64(len(assessments)))*10) / 10
	}

	return &ContextLossSummary{
		TotalEvaluated:     len(assessments),
		HighFidelityCount:  highCount,
		ModerateBleedCount: modCount,
		CriticalLossCount:  critCount,
		AverageCDS:         avgCDS,
		AverageCompression: avgComp,
		Assessments:        assessments,
		Duration:           time.Since(start),
	}
}

// extractEntities pulls file paths and URLs from original trajectory and checks preservation in compacted.
func (e *Engine) extractEntities(orig, compacted string) []EntityFact {
	seen := make(map[string]bool)
	var facts []EntityFact

	// Extract URLs
	urls := urlRegex.FindAllString(orig, -1)
	for _, u := range urls {
		u = strings.TrimRight(u, ".,;:!?)'\"")
		if !seen[u] {
			seen[u] = true
			facts = append(facts, EntityFact{
				Type:             "url",
				Value:            u,
				FoundInCompacted: strings.Contains(compacted, u),
			})
		}
	}

	// Extract File paths
	paths := pathRegex.FindAllString(orig, -1)
	for _, p := range paths {
		p = strings.TrimRight(p, ".,;:!?)'\"")
		if !seen[p] && len(p) > 3 {
			seen[p] = true
			facts = append(facts, EntityFact{
				Type:             "path",
				Value:            p,
				FoundInCompacted: strings.Contains(compacted, p),
			})
		}
	}

	return facts
}

// auditNegativeConstraints parses prohibitions and checks for omission or dangerous inversion.
func (e *Engine) auditNegativeConstraints(orig, compacted string) ([]NegativeConstraint, []string) {
	var constraints []NegativeConstraint
	var violations []string

	origLower := strings.ToLower(orig)
	compactedLower := strings.ToLower(compacted)

	sentences := strings.Split(origLower, ".")
	for _, s := range sentences {
		s = strings.TrimSpace(s)
		for _, neg := range e.config.NegativeKeywords {
			if strings.Contains(s, neg) {
				// Prohibited action context
				action := extractActionContext(s, neg)
				status := "OMITTED"

				if strings.Contains(compactedLower, neg) && strings.Contains(compactedLower, action) {
					status = "RETAINED"
				} else if strings.Contains(compactedLower, action) && !strings.Contains(compactedLower, neg) {
					// Inversion alert! Action is mentioned positively without prohibition
					status = "INVERTED"
					violations = append(violations, fmt.Sprintf("Critical Negative Constraint Inversion: '%s' was stripped of prohibition '%s'", action, neg))
				} else {
					violations = append(violations, fmt.Sprintf("Constraint Omission: Prohibitory instruction '%s' dropped in compaction", s))
				}

				constraints = append(constraints, NegativeConstraint{
					OriginalText:      s,
					ProhibitedAction:  action,
					StatusInCompacted: status,
					Severity:          func() string { if status == "INVERTED" { return "CRITICAL" }; return "HIGH" }(),
				})
				break
			}
		}
	}

	return constraints, violations
}

// detectGhostFacts checks for fabricated claims or ungrounded entity insertions.
func (e *Engine) detectGhostFacts(orig, compacted string) []GhostFact {
	var ghosts []GhostFact
	origLower := strings.ToLower(orig)
	compactedLower := strings.ToLower(compacted)

	// Check if compacted claims user authorization or approval when original did not
	if (strings.Contains(compactedLower, "user approved") || strings.Contains(compactedLower, "user confirmed")) &&
		!strings.Contains(origLower, "approved") && !strings.Contains(origLower, "confirmed") {
		ghosts = append(ghosts, GhostFact{
			Claim:       "user approved action",
			Entity:      "user_authorization",
			Explanation: "Compaction hallucinated user approval not present in original trajectory",
			Severity:    "CRITICAL",
		})
	}

	// Check for fictitious paths introduced in compaction
	compactedPaths := pathRegex.FindAllString(compacted, -1)
	for _, p := range compactedPaths {
		p = strings.TrimRight(p, ".,;:!?)'\"")
		if !strings.Contains(orig, p) {
			ghosts = append(ghosts, GhostFact{
				Claim:       fmt.Sprintf("hallucinated path %s", p),
				Entity:      p,
				Explanation: "File path introduced in compaction summary that never existed in original transcript",
				Severity:    "HIGH",
			})
		}
	}

	return ghosts
}

func extractActionContext(sentence, keyword string) string {
	idx := strings.Index(sentence, keyword)
	if idx == -1 {
		return sentence
	}
	after := strings.TrimSpace(sentence[idx+len(keyword):])
	words := strings.Fields(after)
	if len(words) > 3 {
		return strings.Join(words[:3], " ")
	}
	return after
}
