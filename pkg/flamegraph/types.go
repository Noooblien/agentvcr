package flamegraph

import (
	"time"
)

// SpanCategory represents the operational domain of an execution span.
type SpanCategory string

const (
	CategorySession       SpanCategory = "session"
	CategoryTurn          SpanCategory = "turn"
	CategoryInference     SpanCategory = "llm_inference"
	CategoryTool          SpanCategory = "tool_execution"
	CategoryPrompt        SpanCategory = "prompt_assembly"
	CategoryIO            SpanCategory = "state_io"
	CategoryOrchestration SpanCategory = "orchestration"
)

// Span represents a single timed node in the hierarchical execution flame tree.
type Span struct {
	ID               string                 `json:"id"`
	Name             string                 `json:"name"`
	Category         SpanCategory           `json:"category"`
	StepIndex        int                    `json:"step_index"`
	StartOffsetMs    int64                  `json:"start_offset_ms"`
	DurationMs       int64                  `json:"duration_ms"`
	PromptTokens     int                    `json:"prompt_tokens"`
	CompletionTokens int                    `json:"completion_tokens"`
	TotalTokens      int                    `json:"total_tokens"`
	CostUSD          float64                `json:"cost_usd"`
	IsCriticalPath   bool                   `json:"is_critical_path"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
	Children         []*Span                `json:"children,omitempty"`

	// Derived metrics
	SelfDurationMs int64 `json:"self_duration_ms"`
	SelfTokens     int   `json:"self_tokens"`
}

// CategoryStat captures aggregated resource consumption for a single span category.
type CategoryStat struct {
	Category    SpanCategory `json:"category"`
	DurationMs  int64        `json:"duration_ms"`
	DurationPct float64      `json:"duration_pct"`
	Tokens      int          `json:"tokens"`
	TokensPct   float64      `json:"tokens_pct"`
	Count       int          `json:"count"`
}

// Hotspot represents a bottleneck in execution latency or token consumption.
type Hotspot struct {
	SpanID      string       `json:"span_id"`
	Name        string       `json:"name"`
	Category    SpanCategory `json:"category"`
	Metric      string       `json:"metric"` // "latency" or "tokens"
	Value       float64      `json:"value"`
	Percentage  float64      `json:"percentage"`
	Description string       `json:"description"`
}

// FlameProfile is the complete analyzed flamegraph profile for an agent session.
type FlameProfile struct {
	SessionID             string                     `json:"session_id"`
	AgentName             string                     `json:"agent_name"`
	CreatedAt             time.Time                  `json:"created_at"`
	TotalDurationMs       int64                      `json:"total_duration_ms"`
	TotalTokens           int                        `json:"total_tokens"`
	TotalPromptTokens     int                        `json:"total_prompt_tokens"`
	TotalCompletionTokens int                        `json:"total_completion_tokens"`
	TotalCostUSD          float64                    `json:"total_cost_usd"`
	TotalTurns            int                        `json:"total_turns"`
	TotalToolsCalled      int                        `json:"total_tools_called"`
	Root                  *Span                      `json:"root"`
	CriticalPath          []*Span                    `json:"critical_path"`
	Hotspots              []Hotspot                  `json:"hotspots"`
	CategoryBreakdown     map[SpanCategory]CategoryStat `json:"category_breakdown"`
}

// ExportFormat defines the output serialization target.
type ExportFormat string

const (
	FormatTerminal   ExportFormat = "terminal"
	FormatSpeedscope ExportFormat = "speedscope"
	FormatChrome     ExportFormat = "chrome"
	FormatMermaid    ExportFormat = "mermaid"
	FormatMarkdown   ExportFormat = "markdown"
	FormatJSON       ExportFormat = "json"
)
