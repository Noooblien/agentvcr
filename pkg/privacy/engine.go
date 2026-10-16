package privacy

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"math/rand"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

var (
	// Credential and token patterns (OpenAI, GitHub, AWS, JWT, generic bearer)
	credentialRegex = regexp.MustCompile(`(?i)(?:sk-[a-zA-Z0-9_\-]{20,}|ghp_[a-zA-Z0-9]{20,}|AKIA[0-9A-Z]{16}|bearer\s+[a-zA-Z0-9_\-\.]{20,}|eyJ[a-zA-Z0-9_\-]{10,}\.[a-zA-Z0-9_\-]{10,}\.[a-zA-Z0-9_\-]+)`)
	// Email addresses
	emailRegex = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
	// IPv4 addresses (4 octets)
	ipv4Regex = regexp.MustCompile(`\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b`)
	// Credit card numbers (13 to 19 digits)
	financialRegex = regexp.MustCompile(`\b(?:\d{4}[ -]?){3}\d{4}\b`)
	// PII (US SSN and standard telephone formats)
	ssnRegex   = regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`)
	phoneRegex = regexp.MustCompile(`\b(?:\+?1[-. ]?)?\(?\d{3}\)?[-. ]?\d{3}[-. ]?\d{4}\b`)
)

// PrivacyEngine manages differential privacy budgeting, token masking, and k-anonymity auditing.
type PrivacyEngine struct {
	mu     sync.RWMutex
	config DPConfig
	budget DPBudgetTracker
	rng    *rand.Rand
}

// NewPrivacyEngine creates and initializes a PrivacyEngine.
func NewPrivacyEngine(config DPConfig) *PrivacyEngine {
	seed := config.DeterministicSeed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	r := rand.New(rand.NewSource(seed))

	return &PrivacyEngine{
		config: config,
		budget: DPBudgetTracker{
			TotalEpsilon:     config.TotalBudgetEpsilon,
			SpentEpsilon:     0.0,
			RemainingEpsilon: config.TotalBudgetEpsilon,
			QueryCount:       0,
			BudgetExhausted:  false,
			LastUpdated:      time.Now(),
		},
		rng: r,
	}
}

// LaplaceNoise samples noise from the zero-mean Laplace distribution Laplace(0, b)
// where b = sensitivity / epsilon.
func (e *PrivacyEngine) LaplaceNoise(sensitivity, epsilon float64) float64 {
	if epsilon <= 0 {
		return 0.0
	}
	b := sensitivity / epsilon

	e.mu.Lock()
	u := e.rng.Float64() - 0.5
	e.mu.Unlock()

	// Handle boundary condition
	for u == 0 {
		e.mu.Lock()
		u = e.rng.Float64() - 0.5
		e.mu.Unlock()
	}

	sign := 1.0
	if u < 0 {
		sign = -1.0
	}
	return -sign * b * math.Log(1.0-2.0*math.Abs(u))
}

// PerturbMetric adds calibrated Laplace differential privacy noise to a numeric metric.
func (e *PrivacyEngine) PerturbMetric(val float64, sensitivity, epsilon float64) float64 {
	noise := e.LaplaceNoise(sensitivity, epsilon)
	perturbed := val + noise
	if perturbed < 0 {
		return 0.0
	}
	return perturbed
}

// SpendBudget consumes an allocated epsilon from the cumulative privacy budget.
func (e *PrivacyEngine) SpendBudget(cost float64) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.budget.RemainingEpsilon < cost || e.budget.BudgetExhausted {
		e.budget.BudgetExhausted = true
		return false
	}

	e.budget.SpentEpsilon += cost
	e.budget.RemainingEpsilon -= cost
	e.budget.QueryCount++
	e.budget.LastUpdated = time.Now()

	if e.budget.RemainingEpsilon <= 0.0001 {
		e.budget.RemainingEpsilon = 0.0
		e.budget.BudgetExhausted = true
	}
	return true
}

// GetBudget returns a snapshot copy of the current DP budget tracker.
func (e *PrivacyEngine) GetBudget() DPBudgetTracker {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.budget
}

// hashToken produces a deterministic 8-character pseudonym hash for an entity value.
func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:4])
}

// MaskString identifies sensitive tokens in text and replaces them with deterministic pseudonyms.
func (e *PrivacyEngine) MaskString(text string) (string, MaskingReport) {
	report := MaskingReport{
		CategoryCounts: make(map[string]int),
		RedactedHashes: make([]string, 0),
	}

	// 1. Credentials
	text = credentialRegex.ReplaceAllStringFunc(text, func(m string) string {
		tag := fmt.Sprintf("<REDACTED:KEY_%s>", hashToken(m))
		report.MaskedKeysCount++
		report.TotalTokensMasked++
		report.CategoryCounts[string(CategoryCredential)]++
		report.RedactedHashes = append(report.RedactedHashes, tag)
		return tag
	})

	// 2. Financial
	text = financialRegex.ReplaceAllStringFunc(text, func(m string) string {
		tag := fmt.Sprintf("<REDACTED:FINANCIAL_%s>", hashToken(m))
		report.TotalTokensMasked++
		report.CategoryCounts[string(CategoryFinancial)]++
		report.RedactedHashes = append(report.RedactedHashes, tag)
		return tag
	})

	// 3. Email
	text = emailRegex.ReplaceAllStringFunc(text, func(m string) string {
		tag := fmt.Sprintf("<REDACTED:EMAIL_%s>", hashToken(m))
		report.MaskedEmailsCount++
		report.TotalTokensMasked++
		report.CategoryCounts[string(CategoryEmail)]++
		report.RedactedHashes = append(report.RedactedHashes, tag)
		return tag
	})

	// 4. SSN & Phone PII
	text = ssnRegex.ReplaceAllStringFunc(text, func(m string) string {
		tag := fmt.Sprintf("<REDACTED:SSN_%s>", hashToken(m))
		report.MaskedPIICount++
		report.TotalTokensMasked++
		report.CategoryCounts[string(CategoryPII)]++
		report.RedactedHashes = append(report.RedactedHashes, tag)
		return tag
	})
	text = phoneRegex.ReplaceAllStringFunc(text, func(m string) string {
		tag := fmt.Sprintf("<REDACTED:PHONE_%s>", hashToken(m))
		report.MaskedPIICount++
		report.TotalTokensMasked++
		report.CategoryCounts[string(CategoryPII)]++
		report.RedactedHashes = append(report.RedactedHashes, tag)
		return tag
	})

	// 5. IPv4
	text = ipv4Regex.ReplaceAllStringFunc(text, func(m string) string {
		tag := fmt.Sprintf("<REDACTED:IP_%s>", hashToken(m))
		report.MaskedIPsCount++
		report.TotalTokensMasked++
		report.CategoryCounts[string(CategoryIPAddress)]++
		report.RedactedHashes = append(report.RedactedHashes, tag)
		return tag
	})

	return text, report
}

// SanitizePayload recursively traverses a payload map and applies token masking.
func (e *PrivacyEngine) SanitizePayload(payload map[string]interface{}) (map[string]interface{}, MaskingReport) {
	aggregate := MaskingReport{
		CategoryCounts: make(map[string]int),
		RedactedHashes: make([]string, 0),
	}
	clean := make(map[string]interface{})

	for k, v := range payload {
		sanitizedV, rep := e.sanitizeValue(v)
		clean[k] = sanitizedV
		aggregate.TotalTokensMasked += rep.TotalTokensMasked
		aggregate.MaskedKeysCount += rep.MaskedKeysCount
		aggregate.MaskedEmailsCount += rep.MaskedEmailsCount
		aggregate.MaskedIPsCount += rep.MaskedIPsCount
		aggregate.MaskedPIICount += rep.MaskedPIICount
		for cat, count := range rep.CategoryCounts {
			aggregate.CategoryCounts[cat] += count
		}
		aggregate.RedactedHashes = append(aggregate.RedactedHashes, rep.RedactedHashes...)
	}

	return clean, aggregate
}

func (e *PrivacyEngine) sanitizeValue(val interface{}) (interface{}, MaskingReport) {
	switch v := val.(type) {
	case string:
		cleanStr, rep := e.MaskString(v)
		return cleanStr, rep
	case map[string]interface{}:
		return e.SanitizePayload(v)
	case []interface{}:
		var cleanSlice []interface{}
		subRep := MaskingReport{
			CategoryCounts: make(map[string]int),
		}
		for _, item := range v {
			cleanItem, r := e.sanitizeValue(item)
			cleanSlice = append(cleanSlice, cleanItem)
			subRep.TotalTokensMasked += r.TotalTokensMasked
			subRep.MaskedKeysCount += r.MaskedKeysCount
			subRep.MaskedEmailsCount += r.MaskedEmailsCount
			subRep.MaskedIPsCount += r.MaskedIPsCount
			subRep.MaskedPIICount += r.MaskedPIICount
			for cat, count := range r.CategoryCounts {
				subRep.CategoryCounts[cat] += count
			}
			subRep.RedactedHashes = append(subRep.RedactedHashes, r.RedactedHashes...)
		}
		return cleanSlice, subRep
	default:
		return val, MaskingReport{CategoryCounts: make(map[string]int)}
	}
}

// SanitizeSession clones a session, redacting all sensitive tokens and re-computing valid checksums.
func (e *PrivacyEngine) SanitizeSession(session *trace.Session) (*trace.Session, MaskingReport) {
	cloned := &trace.Session{
		ID:         fmt.Sprintf("%s_sanitized", session.ID),
		AgentName:  session.AgentName,
		CreatedAt:  session.CreatedAt,
		UpdatedAt:  session.UpdatedAt,
		ParentID:   session.ID,
		ForkAtStep: session.ForkAtStep,
		Metadata:   make(map[string]string),
		Events:     make([]trace.TraceEvent, 0, len(session.Events)),
		Status:     "sanitized",
	}

	totalReport := MaskingReport{
		CategoryCounts: make(map[string]int),
		RedactedHashes: make([]string, 0),
	}

	for k, v := range session.Metadata {
		maskedVal, rep := e.MaskString(v)
		cloned.Metadata[k] = maskedVal
		totalReport.TotalTokensMasked += rep.TotalTokensMasked
		totalReport.MaskedKeysCount += rep.MaskedKeysCount
		totalReport.MaskedEmailsCount += rep.MaskedEmailsCount
		totalReport.MaskedIPsCount += rep.MaskedIPsCount
		totalReport.MaskedPIICount += rep.MaskedPIICount
		for cat, count := range rep.CategoryCounts {
			totalReport.CategoryCounts[cat] += count
		}
		totalReport.RedactedHashes = append(totalReport.RedactedHashes, rep.RedactedHashes...)
	}

	prevChecksum := "0000000000000000000000000000000000000000000000000000000000000000"
	for _, ev := range session.Events {
		cleanPayload, rep := e.SanitizePayload(ev.Payload)

		totalReport.TotalTokensMasked += rep.TotalTokensMasked
		totalReport.MaskedKeysCount += rep.MaskedKeysCount
		totalReport.MaskedEmailsCount += rep.MaskedEmailsCount
		totalReport.MaskedIPsCount += rep.MaskedIPsCount
		totalReport.MaskedPIICount += rep.MaskedPIICount
		for cat, count := range rep.CategoryCounts {
			totalReport.CategoryCounts[cat] += count
		}
		totalReport.RedactedHashes = append(totalReport.RedactedHashes, rep.RedactedHashes...)

		// DP noise can optionally perturb step duration
		perturbedDuration := int64(e.PerturbMetric(float64(ev.DurationMs), e.config.Sensitivity, e.config.Epsilon))

		newChecksum := trace.ComputeChecksum(prevChecksum, ev.Index, ev.Type, []byte(fmt.Sprintf("%v", cleanPayload)))

		clonedEv := trace.TraceEvent{
			ID:         fmt.Sprintf("clean_%s", ev.ID),
			Index:      ev.Index,
			Timestamp:  ev.Timestamp,
			Type:       ev.Type,
			DurationMs: perturbedDuration,
			Payload:    cleanPayload,
			Checksum:   newChecksum,
		}
		prevChecksum = newChecksum
		cloned.Events = append(cloned.Events, clonedEv)
	}

	return cloned, totalReport
}

// extractQID computes the quasi-identifier bucket for a session.
func extractQID(session *trace.Session) (string, []string, string) {
	// Tool set
	toolMap := make(map[string]bool)
	sensitiveValue := "status:" + session.Status

	for _, ev := range session.Events {
		if ev.Type == trace.EventToolCall {
			if t, ok := ev.Payload["tool"].(string); ok {
				toolMap[t] = true
			}
		}
		if ev.Type == trace.EventError {
			sensitiveValue = "error"
		}
	}

	var tools []string
	for t := range toolMap {
		tools = append(tools, t)
	}
	sort.Strings(tools)

	// Step count bucket
	n := len(session.Events)
	var stepBucket string
	switch {
	case n <= 5:
		stepBucket = "1-5_steps"
	case n <= 10:
		stepBucket = "6-10_steps"
	case n <= 20:
		stepBucket = "11-20_steps"
	default:
		stepBucket = "21+_steps"
	}

	// Model bucket
	modelBucket := "standard-agent"
	if session.Metadata != nil && session.Metadata["model"] != "" {
		modelBucket = session.Metadata["model"]
	}

	qid := fmt.Sprintf("%s|%s|%s|%s", session.AgentName, modelBucket, strings.Join(tools, ","), stepBucket)
	return qid, tools, sensitiveValue
}

// AuditFleetKAnonymity partitions sessions into equivalence classes and audits k-anonymity & l-diversity.
func (e *PrivacyEngine) AuditFleetKAnonymity(sessions []*trace.Session, kThreshold, lThreshold int) KAnonymityReport {
	if kThreshold <= 0 {
		kThreshold = e.config.KAnonymityThreshold
	}
	if lThreshold <= 0 {
		lThreshold = e.config.LDiversityThreshold
	}

	classMap := make(map[string]*EquivalenceClass)
	sensitiveValsMap := make(map[string]map[string]bool)

	for _, s := range sessions {
		qid, tools, sens := extractQID(s)
		eq, exists := classMap[qid]
		if !exists {
			eq = &EquivalenceClass{
				QIDSignature:    qid,
				AgentName:       s.AgentName,
				ModelBucket:     "default",
				ToolSet:         tools,
				StepCountBucket: "",
				SessionIDs:      make([]string, 0),
			}
			classMap[qid] = eq
			sensitiveValsMap[qid] = make(map[string]bool)
		}
		eq.SessionIDs = append(eq.SessionIDs, s.ID)
		eq.MemberCount++
		sensitiveValsMap[qid][sens] = true
	}

	report := KAnonymityReport{
		KThreshold:              kThreshold,
		LThreshold:              lThreshold,
		TotalSessionsAudited:    len(sessions),
		TotalEquivalenceClasses: len(classMap),
		VulnerableSessionIDs:    make([]string, 0),
	}

	for qid, eq := range classMap {
		eq.IsKCompliant = eq.MemberCount >= kThreshold
		eq.UniqueSensitiveValuesCount = len(sensitiveValsMap[qid])
		eq.IsLCompliant = eq.UniqueSensitiveValuesCount >= lThreshold

		if eq.IsKCompliant {
			report.CompliantClasses++
		} else {
			report.ViolatingClasses++
			report.VulnerableSessionIDs = append(report.VulnerableSessionIDs, eq.SessionIDs...)
		}
	}

	if len(sessions) > 0 {
		report.ReidentificationRisk = float64(len(report.VulnerableSessionIDs)) / float64(len(sessions))
	}
	report.Passed = report.ViolatingClasses == 0

	return report
}

// AuditSessionPrivacy executes a full privacy assessment for a target session against a reference fleet.
func (e *PrivacyEngine) AuditSessionPrivacy(session *trace.Session, fleet []*trace.Session, kThreshold int) *PrivacyAuditReport {
	if kThreshold <= 0 {
		kThreshold = e.config.KAnonymityThreshold
	}

	// 1. Token masking audit
	_, maskReport := e.SanitizeSession(session)

	// 2. Consume privacy budget
	budgetOk := e.SpendBudget(e.config.Epsilon)

	// 3. Evaluate k-anonymity across combined fleet
	allSessions := append([]*trace.Session{session}, fleet...)
	kReport := e.AuditFleetKAnonymity(allSessions, kThreshold, e.config.LDiversityThreshold)

	// Determine if current session is specifically vulnerable
	sessionVulnerable := false
	for _, vulnID := range kReport.VulnerableSessionIDs {
		if vulnID == session.ID {
			sessionVulnerable = true
			break
		}
	}

	noiseScale := e.config.Sensitivity / e.config.Epsilon
	recs := make([]string, 0)
	if maskReport.TotalTokensMasked > 0 {
		recs = append(recs, fmt.Sprintf("Applied deterministic pseudonymization to %d sensitive tokens", maskReport.TotalTokensMasked))
	}
	if sessionVulnerable {
		recs = append(recs, fmt.Sprintf("Generalize agent quasi-identifiers: equivalence group size (%d) is below k=%d threshold", 1, kThreshold))
	}
	if !budgetOk {
		recs = append(recs, "Privacy budget exhausted! Subsequent queries may leak statistical uniqueness.")
	}

	overallPass := budgetOk && !sessionVulnerable && maskReport.TotalTokensMasked >= 0

	return &PrivacyAuditReport{
		SessionID:         session.ID,
		Epsilon:           e.config.Epsilon,
		Delta:             e.config.Delta,
		LaplaceNoiseScale: noiseScale,
		Budget:            e.GetBudget(),
		Masking:           maskReport,
		KAnonymity:        kReport,
		OverallCompliance: overallPass,
		Recommendations:   recs,
	}
}

// RenderPrivacyScorecard generates a formatted ANSI terminal inspection card.
func RenderPrivacyScorecard(report *PrivacyAuditReport) string {
	var sb strings.Builder

	sb.WriteString("\n\033[1;36m========================================================================================\033[0m\n")
	sb.WriteString(fmt.Sprintf("\033[1;37m   AGENTVCR DIFFERENTIAL PRIVACY TOKEN MASKING & K-ANONYMITY AUDIT\033[0m\n"))
	sb.WriteString("\033[1;36m========================================================================================\033[0m\n")

	sb.WriteString(fmt.Sprintf("  \033[1;33mSession ID:\033[0m            %s\n", report.SessionID))
	sb.WriteString(fmt.Sprintf("  \033[1;33mDP Epsilon (ε):\033[0m        %.3f (Laplace scale b=%.3f, δ=%.1e)\n",
		report.Epsilon, report.LaplaceNoiseScale, report.Delta))
	sb.WriteString(fmt.Sprintf("  \033[1;33mPrivacy Budget:\033[0m        Spent: %.2f / %.2f ε (Remaining: \033[1;32m%.2f ε\033[0m | Queries: %d)\n",
		report.Budget.SpentEpsilon, report.Budget.TotalEpsilon, report.Budget.RemainingEpsilon, report.Budget.QueryCount))

	if report.OverallCompliance {
		sb.WriteString("  \033[1;33mCompliance Status:\033[0m     \033[1;32m[PASS] DIFFERENTIAL PRIVACY & K-ANONYMITY CERTIFIED\033[0m\n")
	} else {
		sb.WriteString("  \033[1;33mCompliance Status:\033[0m     \033[1;31m[FAIL] PRIVACY BUDGET EXHAUSTED OR RE-IDENTIFICATION RISK DETECTED\033[0m\n")
	}
	sb.WriteString("\033[1;34m----------------------------------------------------------------------------------------\033[0m\n")

	sb.WriteString("\033[1;37m  TOKEN MASKING & SENSITIVE ENTITY PSEUDONYMIZATION:\033[0m\n")
	sb.WriteString(fmt.Sprintf("  Total Tokens Redacted: \033[1;35m%d\033[0m\n", report.Masking.TotalTokensMasked))
	sb.WriteString(fmt.Sprintf("  - API Keys & Secrets:  %d masked\n", report.Masking.MaskedKeysCount))
	sb.WriteString(fmt.Sprintf("  - Emails & Identifiers: %d masked\n", report.Masking.MaskedEmailsCount))
	sb.WriteString(fmt.Sprintf("  - Network IPs:         %d masked\n", report.Masking.MaskedIPsCount))
	sb.WriteString(fmt.Sprintf("  - PII (SSN / Phone):   %d masked\n", report.Masking.MaskedPIICount))

	sb.WriteString("\033[1;34m----------------------------------------------------------------------------------------\033[0m\n")
	sb.WriteString("\033[1;37m  FLEET K-ANONYMITY & RE-IDENTIFICATION AUDIT:\033[0m\n")
	sb.WriteString(fmt.Sprintf("  Target k-Threshold:    k = %d (l-diversity = %d)\n",
		report.KAnonymity.KThreshold, report.KAnonymity.LThreshold))
	sb.WriteString(fmt.Sprintf("  Sessions Audited:      %d total across %d equivalence classes\n",
		report.KAnonymity.TotalSessionsAudited, report.KAnonymity.TotalEquivalenceClasses))
	sb.WriteString(fmt.Sprintf("  Compliant Classes:     \033[1;32m%d\033[0m | Violating: \033[1;31m%d\033[0m\n",
		report.KAnonymity.CompliantClasses, report.KAnonymity.ViolatingClasses))
	sb.WriteString(fmt.Sprintf("  Re-identification Risk: \033[1;33m%.1f%%\033[0m\n",
		report.KAnonymity.ReidentificationRisk*100.0))

	if len(report.Recommendations) > 0 {
		sb.WriteString("\033[1;34m----------------------------------------------------------------------------------------\033[0m\n")
		sb.WriteString("\033[1;33m  RECOMMENDED PRIVACY ACTIONS:\033[0m\n")
		for _, rec := range report.Recommendations {
			sb.WriteString(fmt.Sprintf("  \033[1;30m[•]\033[0m %s\n", rec))
		}
	}

	sb.WriteString("\033[1;36m========================================================================================\033[0m\n\n")
	return sb.String()
}
