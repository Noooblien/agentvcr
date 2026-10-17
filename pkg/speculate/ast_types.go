package speculate

import (
	"fmt"
	"strings"
)

// EditType defines the categorical intent of a code modification.
type EditType string

const (
	EditInsert   EditType = "insert"
	EditReplace  EditType = "replace"
	EditDelete   EditType = "delete"
	EditComplete EditType = "complete"
)

// CursorPosition models line and column coordinates in an active editor buffer.
type CursorPosition struct {
	FilePath string `json:"file_path"`
	Line     int    `json:"line"` // 1-indexed
	Column   int    `json:"column"` // 1-indexed
	Offset   int    `json:"offset"`
}

// ASTContext captures the immediate syntactic context surrounding the cursor.
type ASTContext struct {
	Language        string   `json:"language"` // "go", "python", "typescript", etc.
	CurrentFunction string   `json:"current_function,omitempty"`
	EnclosingBlock  string   `json:"enclosing_block,omitempty"` // "if", "for", "switch", "type", etc.
	Imports         []string `json:"imports,omitempty"`
	LeadingLines    []string `json:"leading_lines"` // Lines preceding cursor
	TrailingLines   []string `json:"trailing_lines"` // Lines following cursor
	IndentLevel     int      `json:"indent_level"`
}

// MultiTokenDraft represents a speculatively predicted sequence of code tokens/lines.
type MultiTokenDraft struct {
	DraftID         string         `json:"draft_id"`
	EditType        EditType       `json:"edit_type"`
	TargetPosition  CursorPosition `json:"target_position"`
	PredictedLines  []string       `json:"predicted_lines"`
	RawDiff         string         `json:"raw_diff"`
	TokenCount      int            `json:"token_count"`
	Confidence      float64        `json:"confidence"` // 0.0 to 1.0
	PredictedInMs   int64          `json:"predicted_in_ms"`
	SyntaxValid     bool           `json:"syntax_valid"`
	SyntacticReason string         `json:"syntactic_reason,omitempty"`
}

// PredictorConfig configures drafting parameters, latency bounds, and acceptance thresholds.
type PredictorConfig struct {
	MaxDraftTokens       int     `json:"max_draft_tokens"` // Max tokens/lines to predict ahead (e.g. 50)
	MinConfidence        float64 `json:"min_confidence"`   // Minimum threshold to suggest (e.g. 0.65)
	MaxPredictionTimeMs int64   `json:"max_prediction_time_ms"` // SLA (e.g. 50ms)
	EnableHeuristics     bool    `json:"enable_heuristics"`
}

// DefaultPredictorConfig returns production-ready Cursor-style fast-tab defaults.
func DefaultPredictorConfig() PredictorConfig {
	return PredictorConfig{
		MaxDraftTokens:       64,
		MinConfidence:        0.70,
		MaxPredictionTimeMs:  45, // <45ms for instant interactive Tab feel
		EnableHeuristics:     true,
	}
}

// DraftValidationResult checks whether ground truth user keystrokes match the speculative draft.
type DraftValidationResult struct {
	DraftID         string  `json:"draft_id"`
	Accepted        bool    `json:"accepted"`
	MatchedTokens   int     `json:"matched_tokens"`
	TotalTokens     int     `json:"total_tokens"`
	AcceptanceRatio float64 `json:"acceptance_ratio"`
	TimeSavedMs     int64   `json:"time_saved_ms"`
}

// FormatDraftDiff generates unified-style diff view of the predicted tokens.
func FormatDraftDiff(orig []string, draft []string, lineNum int) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n", lineNum, len(orig), lineNum, len(orig)+len(draft)))
	for _, l := range orig {
		sb.WriteString(fmt.Sprintf(" %s\n", l))
	}
	for _, l := range draft {
		sb.WriteString(fmt.Sprintf("+%s\n", l))
	}
	return sb.String()
}
