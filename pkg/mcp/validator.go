package mcp

import (
	"fmt"
	"regexp"
	"strings"
)

// SecurityViolation flags prompt injection or malicious payloads in tool arguments.
type SecurityViolation struct {
	FieldName string `json:"field_name"`
	Pattern   string `json:"pattern"`
	Severity  string `json:"severity"` // "CRITICAL", "HIGH", "MEDIUM"
	Message   string `json:"message"`
}

// ValidationResult summarizes schema validity and security findings.
type ValidationResult struct {
	Valid      bool                `json:"valid"`
	SchemaErrs []string            `json:"schema_errors,omitempty"`
	Violations []SecurityViolation `json:"security_violations,omitempty"`
}

// Validator checks tool arguments against schemas and scans for malicious prompt injections.
type Validator struct {
	injectionPatterns []*regexp.Regexp
}

// NewValidator initializes a validator with standard OWASP-LLM prompt injection patterns.
func NewValidator() *Validator {
	patterns := []string{
		`(?i)ignore\s+(all\s+)?(previous|prior)\s+instructions`,
		`(?i)disregard\s+(the\s+)?(above|previous|system)\s+prompt`,
		`(?i)you\s+are\s+now\s+in\s+(developer|unrestricted|god)\s+mode`,
		`(?i)output\s+(the\s+)?system\s+prompt`,
		`(?i)send\s+(secrets|passwords|keys|tokens)\s+to\s+https?://`,
		`(?i)curl\s+.*-d\s+@/etc/passwd`,
		`(?i)<script[\s>]`,
		`(?i)\bbase64\s*--decode\b`,
	}

	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err == nil {
			compiled = append(compiled, re)
		}
	}

	return &Validator{
		injectionPatterns: compiled,
	}
}

// ValidateArguments verifies arguments match the tool's schema and contain no dangerous injections.
func (v *Validator) ValidateArguments(schema ToolInputSchema, args map[string]interface{}) ValidationResult {
	res := ValidationResult{
		Valid:      true,
		SchemaErrs: make([]string, 0),
		Violations: make([]SecurityViolation, 0),
	}

	// 1. Verify required properties
	for _, req := range schema.Required {
		if _, ok := args[req]; !ok {
			res.Valid = false
			res.SchemaErrs = append(res.SchemaErrs, fmt.Sprintf("missing required argument '%s'", req))
		}
	}

	// 2. Scan all string values for prompt injection and type compliance
	for key, val := range args {
		// Type check against declared property schema if available
		if propDef, ok := schema.Properties[key]; ok {
			if propMap, ok := propDef.(map[string]interface{}); ok {
				if expectedType, ok := propMap["type"].(string); ok {
					if !v.checkType(expectedType, val) {
						res.Valid = false
						res.SchemaErrs = append(res.SchemaErrs, fmt.Sprintf("argument '%s' expected type '%s', got %T", key, expectedType, val))
					}
				}
			}
		}

		// Security scan
		v.scanSecurityRecursive(key, val, &res)
	}

	if len(res.Violations) > 0 {
		res.Valid = false
	}

	return res
}

func (v *Validator) checkType(expected string, val interface{}) bool {
	if val == nil {
		return false
	}
	switch expected {
	case "string":
		_, ok := val.(string)
		return ok
	case "number", "integer":
		switch val.(type) {
		case int, int32, int64, float32, float64:
			return true
		default:
			return false
		}
	case "boolean":
		_, ok := val.(bool)
		return ok
	case "array":
		_, ok := val.([]interface{})
		return ok
	case "object":
		_, ok := val.(map[string]interface{})
		return ok
	}
	return true
}

func (v *Validator) scanSecurityRecursive(prefix string, val interface{}, res *ValidationResult) {
	switch vTyped := val.(type) {
	case string:
		for _, re := range v.injectionPatterns {
			if re.MatchString(vTyped) {
				res.Violations = append(res.Violations, SecurityViolation{
					FieldName: prefix,
					Pattern:   re.String(),
					Severity:  "CRITICAL",
					Message:   fmt.Sprintf("Prompt injection signature detected in '%s'", prefix),
				})
			}
		}
	case map[string]interface{}:
		for k, child := range vTyped {
			childPrefix := fmt.Sprintf("%s.%s", prefix, k)
			v.scanSecurityRecursive(childPrefix, child, res)
		}
	case []interface{}:
		for idx, item := range vTyped {
			childPrefix := fmt.Sprintf("%s[%d]", prefix, idx)
			v.scanSecurityRecursive(childPrefix, item, res)
		}
	}
}

// RenderSecurityReport formats a terminal-friendly security audit scorecard.
func (v *Validator) RenderSecurityReport(res ValidationResult) string {
	var sb strings.Builder
	sb.WriteString("\n┌─────────────────────────────────────────────────────────────┐\n")
	sb.WriteString("│          AGENTVCR MCP ARGUMENT SECURITY AUDIT               │\n")
	sb.WriteString("├─────────────────────────────────────────────────────────────┤\n")
	if res.Valid {
		sb.WriteString("│ STATUS: \033[32mPASSED\033[0m (Zero Schema Errors, Zero Injections)        │\n")
	} else {
		sb.WriteString("│ STATUS: \033[31mBLOCKED\033[0m (Violations or Schema Errors Detected)      │\n")
	}
	sb.WriteString("├─────────────────────────────────────────────────────────────┤\n")
	if len(res.SchemaErrs) > 0 {
		sb.WriteString("│ Schema Validation Errors:                                   │\n")
		for _, err := range res.SchemaErrs {
			sb.WriteString(fmt.Sprintf("│  - \033[33m%s\033[0m\n", err))
		}
	}
	if len(res.Violations) > 0 {
		sb.WriteString("│ Security Violations:                                        │\n")
		for _, vio := range res.Violations {
			sb.WriteString(fmt.Sprintf("│  - [%s] Field: %s -> %s\n", vio.Severity, vio.FieldName, vio.Message))
		}
	}
	sb.WriteString("└─────────────────────────────────────────────────────────────┘\n")
	return sb.String()
}
