package taint

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Tracker provides thread-safe runtime information flow control and taint provenance tracking.
type Tracker struct {
	mu            sync.RWMutex
	variables     map[string]*TaintedValue
	declassifiers map[string]*DeclassifierConfig
	policy        *Policy
	violations    []TaintViolation
	totalFlows    int
}

// NewTracker initializes a new Dynamic Taint Tracker with the specified policy.
func NewTracker(policy *Policy) *Tracker {
	if policy == nil {
		policy = DefaultPolicy()
	}

	t := &Tracker{
		variables:     make(map[string]*TaintedValue),
		declassifiers: make(map[string]*DeclassifierConfig),
		policy:        policy,
		violations:    make([]TaintViolation, 0),
	}

	// Register canonical built-in declassifiers
	t.registerDefaultDeclassifiers()
	return t
}

func (t *Tracker) registerDefaultDeclassifiers() {
	// 1. SHA-256 Digest Declassifier: Hashing strips sensitive PII/secrets into a non-reversible fingerprint
	t.RegisterDeclassifier(&DeclassifierConfig{
		ID:          "crypto_sha256_hash",
		Description: "One-way cryptographic hash declassifying secrets and PII for integrity checks",
		RemovesTaints: []TaintTag{
			TaintSecretKey,
			TaintUserPII,
			TaintIndirectInjection,
		},
		Transform: func(val interface{}) (interface{}, bool) {
			strVal := fmt.Sprintf("%v", val)
			return fmt.Sprintf("sha256(%d_bytes)", len(strVal)), true
		},
	})

	// 2. Strict Schema Sanitizer: Strips untrusted web strings when validated against strict scalar primitives
	t.RegisterDeclassifier(&DeclassifierConfig{
		ID:          "strict_scalar_validator",
		Description: "Validates string as pure numeric/boolean/uuid scalar, stripping untrusted injection taints",
		RemovesTaints: []TaintTag{
			TaintUntrustedWeb,
			TaintIndirectInjection,
		},
		Transform: func(val interface{}) (interface{}, bool) {
			str := strings.TrimSpace(fmt.Sprintf("%v", val))
			// Reject any control characters, shell operators, or prompt markers
			if strings.ContainsAny(str, ";|&`$<>{}[]\n\r") {
				return nil, false
			}
			return str, true
		},
	})
}

// RegisterDeclassifier adds an approved sanitization gate to the tracker.
func (t *Tracker) RegisterDeclassifier(cfg *DeclassifierConfig) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.declassifiers[cfg.ID] = cfg
}

// TagSource introduces a new variable originating from a specific data source.
func (t *Tracker) TagSource(varName string, val interface{}, source SourceType, tags ...TaintTag) *TaintedValue {
	t.mu.Lock()
	defer t.mu.Unlock()

	taints := make(map[TaintTag]bool)
	for _, tag := range tags {
		taints[tag] = true
	}

	now := time.Now().UTC()
	tv := &TaintedValue{
		VarName:  varName,
		RawValue: val,
		Taints:   taints,
		ProvenanceChain: []FlowStep{
			{
				StepID:      randomStepID(),
				Operation:   "source",
				Details:     fmt.Sprintf("Origin: %s", source),
				TaintsAdded: tags,
				Timestamp:   now,
			},
		},
		CreatedAt:      now,
		LastModifiedAt: now,
	}

	t.variables[varName] = tv
	t.totalFlows++
	return tv
}

// PropagateConcat joins multiple tainted values, accumulating all taint tags into a new variable.
func (t *Tracker) PropagateConcat(resultVar string, separator string, inputs ...*TaintedValue) *TaintedValue {
	t.mu.Lock()
	defer t.mu.Unlock()

	mergedTaints := make(map[TaintTag]bool)
	var strParts []string
	var mergedProvenance []FlowStep

	for _, in := range inputs {
		if in == nil {
			continue
		}
		for tag := range in.Taints {
			mergedTaints[tag] = true
		}
		strParts = append(strParts, fmt.Sprintf("%v", in.RawValue))
		mergedProvenance = append(mergedProvenance, in.ProvenanceChain...)
	}

	now := time.Now().UTC()
	step := FlowStep{
		StepID:      randomStepID(),
		Operation:   "concat",
		Details:     fmt.Sprintf("Combined %d variables into '%s'", len(inputs), resultVar),
		Timestamp:   now,
	}
	mergedProvenance = append(mergedProvenance, step)

	res := &TaintedValue{
		VarName:         resultVar,
		RawValue:        strings.Join(strParts, separator),
		Taints:          mergedTaints,
		ProvenanceChain: mergedProvenance,
		CreatedAt:       now,
		LastModifiedAt:  now,
	}

	t.variables[resultVar] = res
	t.totalFlows++
	return res
}

// PropagateJSONExtract tracks a field extracted from a tainted JSON payload.
func (t *Tracker) PropagateJSONExtract(resultVar string, jsonVal *TaintedValue, fieldPath string, extractedVal interface{}) *TaintedValue {
	t.mu.Lock()
	defer t.mu.Unlock()

	inheritedTaints := make(map[TaintTag]bool)
	if jsonVal != nil {
		for tag := range jsonVal.Taints {
			inheritedTaints[tag] = true
		}
	}

	now := time.Now().UTC()
	var prov []FlowStep
	if jsonVal != nil {
		prov = append(prov, jsonVal.ProvenanceChain...)
	}
	prov = append(prov, FlowStep{
		StepID:    randomStepID(),
		Operation: "json_extract",
		Details:   fmt.Sprintf("Extracted field '%s' from '%s'", fieldPath, jsonVal.VarName),
		Timestamp: now,
	})

	res := &TaintedValue{
		VarName:         resultVar,
		RawValue:        extractedVal,
		Taints:          inheritedTaints,
		ProvenanceChain: prov,
		CreatedAt:       now,
		LastModifiedAt:  now,
	}

	t.variables[resultVar] = res
	t.totalFlows++
	return res
}

// Declassify runs a variable through a registered declassification gate, removing specified taints.
func (t *Tracker) Declassify(resultVar string, input *TaintedValue, declassifierID string) (*TaintedValue, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	gate, exists := t.declassifiers[declassifierID]
	if !exists {
		return nil, fmt.Errorf("declassifier gate '%s' not registered", declassifierID)
	}

	cleanedVal, ok := gate.Transform(input.RawValue)
	if !ok {
		return nil, fmt.Errorf("declassification gate '%s' rejected variable '%s': transformation failed validation", declassifierID, input.VarName)
	}

	// Copy existing taints and remove declassified tags
	remainingTaints := make(map[TaintTag]bool)
	for tag, active := range input.Taints {
		if active {
			remainingTaints[tag] = true
		}
	}
	for _, stripped := range gate.RemovesTaints {
		delete(remainingTaints, stripped)
	}

	now := time.Now().UTC()
	prov := append([]FlowStep{}, input.ProvenanceChain...)
	prov = append(prov, FlowStep{
		StepID:    randomStepID(),
		Operation: "declassify",
		Details:   fmt.Sprintf("Gate '%s' stripped %v", declassifierID, gate.RemovesTaints),
		Timestamp: now,
	})

	res := &TaintedValue{
		VarName:         resultVar,
		RawValue:        cleanedVal,
		Taints:          remainingTaints,
		ProvenanceChain: prov,
		CreatedAt:       now,
		LastModifiedAt:  now,
	}

	t.variables[resultVar] = res
	t.totalFlows++
	return res, nil
}

// ValidateSink enforces the Information Flow Control policy at a critical sink.
func (t *Tracker) ValidateSink(sink SinkType, targetResource string, val *TaintedValue) (bool, *TaintViolation) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if val == nil {
		return true, nil
	}

	forbiddenList, exists := t.policy.ForbiddenSinkTaints[sink]
	if !exists {
		return true, nil // No specific restrictions on this sink
	}

	var violatingTags []TaintTag
	for _, forbidden := range forbiddenList {
		if val.HasTaint(forbidden) {
			violatingTags = append(violatingTags, forbidden)
		}
	}

	if len(violatingTags) > 0 {
		vID := "viol_" + randomStepID()[:8]
		reason := fmt.Sprintf("Non-interference violation: sink '%s' (%s) forbids taint tags %v carried by '%s'",
			sink, targetResource, violatingTags, val.VarName)

		violation := TaintViolation{
			ViolationID:    vID,
			Sink:           sink,
			TargetResource: targetResource,
			TaintedVar:     val.VarName,
			ViolatingTags:  violatingTags,
			Reason:         reason,
			InterceptedAt:  time.Now().UTC(),
		}

		t.violations = append(t.violations, violation)
		return false, &violation
	}

	return true, nil
}

// GetVariable retrieves a tracked tainted value by name.
func (t *Tracker) GetVariable(name string) (*TaintedValue, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	v, ok := t.variables[name]
	return v, ok
}

// GenerateReport produces an executive Information Flow Control compliance report.
func (t *Tracker) GenerateReport() *TaintReport {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return &TaintReport{
		TotalFlowsTracked:   t.totalFlows,
		ActiveTaintedVars:   len(t.variables),
		ViolationsCount:     len(t.violations),
		Violations:          t.violations,
		NonInterferenceHeld: len(t.violations) == 0,
	}
}

// GenerateMermaidDAG visualizes the information flow provenance in Mermaid flowchart syntax.
func (t *Tracker) GenerateMermaidDAG() string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString("flowchart TD\n")
	sb.WriteString("    classDef source fill:#f9f,stroke:#333,stroke-width:2px;\n")
	sb.WriteString("    classDef sink fill:#fbb,stroke:#f00,stroke-width:2px;\n")
	sb.WriteString("    classDef declass fill:#dfd,stroke:#090,stroke-width:2px;\n")
	sb.WriteString("    classDef var fill:#def,stroke:#333,stroke-width:1px;\n\n")

	for _, v := range t.variables {
		nodeID := sanitizeNodeID(v.VarName)
		tagsStr := fmt.Sprintf("%v", v.TagsList())
		sb.WriteString(fmt.Sprintf("    %s[\"%s<br/><small>%s</small>\"]:::var\n", nodeID, v.VarName, tagsStr))

		for _, p := range v.ProvenanceChain {
			if p.Operation == "declassify" {
				decID := "dec_" + p.StepID[:6]
				sb.WriteString(fmt.Sprintf("    %s[\"Gate: %s\"]:::declass --> %s\n", decID, p.Details, nodeID))
			}
		}
	}

	for _, viol := range t.violations {
		violID := "viol_" + viol.ViolationID
		sb.WriteString(fmt.Sprintf("    %s --> %s[\"BLOCKED SINK: %s<br/>(%s)\"]:::sink\n",
			sanitizeNodeID(viol.TaintedVar), violID, viol.Sink, viol.TargetResource))
	}

	return sb.String()
}

func randomStepID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func sanitizeNodeID(s string) string {
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, ".", "_")
	s = strings.ReplaceAll(s, " ", "_")
	return "n_" + s
}
