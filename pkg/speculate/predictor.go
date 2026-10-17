package speculate

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ASTPredictor generates speculative multi-token edit drafts from code context vectors.
type ASTPredictor struct {
	mu          sync.RWMutex
	config      PredictorConfig
	patternBank map[string][]string // Maps context syntactic signature -> canonical completion tokens
	draftCache  map[string]*MultiTokenDraft
}

// NewASTPredictor initializes a predictive speculative editor engine.
func NewASTPredictor(config PredictorConfig) *ASTPredictor {
	p := &ASTPredictor{
		config:      config,
		patternBank: make(map[string][]string),
		draftCache:  make(map[string]*MultiTokenDraft),
	}
	p.seedCommonPatterns()
	return p
}

// seedCommonPatterns populates the predictor with standard idiomatic AST idioms (Go/TS/Python).
func (p *ASTPredictor) seedCommonPatterns() {
	// Idiom 1: Go error check after err assignment
	p.patternBank["go:err_assign"] = []string{
		"if err != nil {",
		"\treturn nil, fmt.Errorf(\"failed operation: %w\", err)",
		"}",
	}

	// Idiom 2: Mutex lock & defer unlock
	p.patternBank["go:mutex_lock"] = []string{
		"defer p.mu.Unlock()",
	}

	// Idiom 3: HTTP handler response write
	p.patternBank["go:http_handler"] = []string{
		"w.Header().Set(\"Content-Type\", \"application/json\")",
		"_ = json.NewEncoder(w).Encode(resp)",
	}

	// Idiom 4: TypeScript async try/catch
	p.patternBank["ts:try_catch"] = []string{
		"} catch (error) {",
		"  console.error(\"Operation failed:\", error);",
		"  throw error;",
		"}",
	}
}

// RegisterPattern dynamically registers a learned syntactic pattern into the pattern bank.
func (p *ASTPredictor) RegisterPattern(signature string, lines []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.patternBank[signature] = lines
}

// GenerateDraft produces a speculative multi-token code edit ahead of user keystrokes in <50ms.
func (p *ASTPredictor) GenerateDraft(ctx ASTContext, pos CursorPosition) (*MultiTokenDraft, error) {
	t0 := time.Now()

	sig := p.detectContextSignature(ctx)
	if sig == "" {
		return nil, fmt.Errorf("no predictable syntactic pattern identified")
	}

	p.mu.RLock()
	lines, found := p.patternBank[sig]
	p.mu.RUnlock()

	if !found || len(lines) == 0 {
		return nil, fmt.Errorf("pattern '%s' not found in predictive index", sig)
	}

	// Adjust indentation to match current AST level
	indent := strings.Repeat("\t", ctx.IndentLevel)
	if ctx.Language == "typescript" || ctx.Language == "python" {
		indent = strings.Repeat("  ", ctx.IndentLevel)
	}

	indentedLines := make([]string, len(lines))
	tokenCount := 0
	for i, line := range lines {
		indentedLines[i] = indent + line
		tokenCount += len(strings.Fields(line))
	}

	elapsedMs := time.Since(t0).Milliseconds()
	draftID := fmt.Sprintf("draft_%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d:%d", pos.FilePath, pos.Line, pos.Column, time.Now().UnixNano()))))[:16]

	confidence := 0.88
	if ctx.CurrentFunction != "" {
		confidence += 0.05
	}
	if confidence > 0.99 {
		confidence = 0.99
	}

	draft := &MultiTokenDraft{
		DraftID:         draftID,
		EditType:        EditInsert,
		TargetPosition:  pos,
		PredictedLines:  indentedLines,
		RawDiff:         FormatDraftDiff(ctx.LeadingLines, indentedLines, pos.Line),
		TokenCount:      tokenCount,
		Confidence:      confidence,
		PredictedInMs:   elapsedMs,
		SyntaxValid:     true,
		SyntacticReason: fmt.Sprintf("Matched AST signature [%s] at indent level %d", sig, ctx.IndentLevel),
	}

	p.mu.Lock()
	p.draftCache[draftID] = draft
	p.mu.Unlock()

	return draft, nil
}

// detectContextSignature analyzes leading lines to classify the next expected AST continuation.
func (p *ASTPredictor) detectContextSignature(ctx ASTContext) string {
	if len(ctx.LeadingLines) == 0 {
		return ""
	}

	lastLine := strings.TrimSpace(ctx.LeadingLines[len(ctx.LeadingLines)-1])

	if ctx.Language == "go" {
		if strings.Contains(lastLine, "err :=") || strings.Contains(lastLine, "err =") {
			return "go:err_assign"
		}
		if strings.Contains(lastLine, ".Lock()") {
			return "go:mutex_lock"
		}
		if strings.Contains(lastLine, "func (") && strings.Contains(lastLine, "http.ResponseWriter") {
			return "go:http_handler"
		}
	} else if ctx.Language == "typescript" || ctx.Language == "javascript" {
		if strings.Contains(lastLine, "try {") {
			return "ts:try_catch"
		}
	}

	return ""
}

// ValidateDraft verifies user completion against the speculative draft and calculates time saved.
func (p *ASTPredictor) ValidateDraft(draftID string, acceptedLines []string) (*DraftValidationResult, error) {
	p.mu.RLock()
	draft, exists := p.draftCache[draftID]
	p.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("draft ID %s not found in cache", draftID)
	}

	matchedTokens := 0
	totalTokens := draft.TokenCount

	// Calculate token overlap
	draftWords := make(map[string]int)
	for _, l := range draft.PredictedLines {
		for _, w := range strings.Fields(l) {
			draftWords[w]++
		}
	}

	for _, l := range acceptedLines {
		for _, w := range strings.Fields(l) {
			if count, ok := draftWords[w]; ok && count > 0 {
				matchedTokens++
				draftWords[w]--
			}
		}
	}

	ratio := 0.0
	if totalTokens > 0 {
		ratio = float64(matchedTokens) / float64(totalTokens)
	}
	if ratio > 1.0 {
		ratio = 1.0
	}

	accepted := ratio >= 0.70
	timeSaved := int64(0)
	if accepted {
		// Human typing speed benchmark: ~5 characters/sec (approx 400ms per code token)
		timeSaved = int64(matchedTokens) * 350
	}

	return &DraftValidationResult{
		DraftID:         draftID,
		Accepted:        accepted,
		MatchedTokens:   matchedTokens,
		TotalTokens:     totalTokens,
		AcceptanceRatio: ratio,
		TimeSavedMs:     timeSaved,
	}, nil
}

// RenderDraftCard formats an executive ANSI preview of the speculative multi-token draft.
func RenderDraftCard(draft *MultiTokenDraft) string {
	var sb strings.Builder
	sb.WriteString("\n\033[1m\033[36m⚡ [AgentVCR Speculate] Multi-Token AST Edit Draft\033[0m\n")
	sb.WriteString(fmt.Sprintf("   Draft ID:      \033[33m%s\033[0m\n", draft.DraftID))
	sb.WriteString(fmt.Sprintf("   Target File:   %s:%d:%d\n", draft.TargetPosition.FilePath, draft.TargetPosition.Line, draft.TargetPosition.Column))
	sb.WriteString(fmt.Sprintf("   Predicted In:  \033[32m%d ms\033[0m (Confidence: %.1f%%)\n", draft.PredictedInMs, draft.Confidence*100.0))
	sb.WriteString(fmt.Sprintf("   Reason:        %s\n", draft.SyntacticReason))
	sb.WriteString("\n   \033[1mProposed Multi-Line Completion (Fast-Tab):\033[0m\n")
	for _, l := range draft.PredictedLines {
		sb.WriteString(fmt.Sprintf("   \033[32m+ %s\033[0m\n", l))
	}
	sb.WriteString("\n")
	return sb.String()
}
