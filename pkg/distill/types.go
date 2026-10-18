package distill

import (
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// DistillFormat defines the target fine-tuning or alignment schema.
type DistillFormat string

const (
	FormatSFT      DistillFormat = "sft"      // OpenAI Chat Completion Fine-Tuning JSONL
	FormatDPO      DistillFormat = "dpo"      // HuggingFace Direct Preference Optimization JSONL
	FormatShareGPT DistillFormat = "sharegpt" // Multi-turn ShareGPT format (human/gpt)
)

// ChatMessage represents a structured turn in an LLM conversation.
type ChatMessage struct {
	Role       string                 `json:"role"` // system, user, assistant, tool
	Content    string                 `json:"content"`
	Name       string                 `json:"name,omitempty"`       // For tool results
	ToolCallID string                 `json:"tool_call_id,omitempty"`
	Payload    map[string]interface{} `json:"payload,omitempty"`
}

// SFTRow represents an OpenAI-compatible Supervised Fine-Tuning JSONL row.
type SFTRow struct {
	Messages []ChatMessage `json:"messages"`
}

// DPORow represents a Direct Preference Optimization JSONL row with chosen and rejected trajectories.
type DPORow struct {
	Prompt       string        `json:"prompt"`
	Chosen       []ChatMessage `json:"chosen"`
	Rejected     []ChatMessage `json:"rejected"`
	RewardMargin float64       `json:"reward_margin"` // Delta score (e.g. +1.5)
	DivergenceAt int           `json:"divergence_step"`
}

// ShareGPTMessage represents a single message in ShareGPT format.
type ShareGPTMessage struct {
	From  string `json:"from"`  // "human", "gpt", "system"
	Value string `json:"value"`
}

// ShareGPTRow represents a multi-turn ShareGPT dataset record.
type ShareGPTRow struct {
	ID            string            `json:"id"`
	Conversations []ShareGPTMessage `json:"conversations"`
}

// DistillConfig configures trajectory filtering, pruning, and alignment pair synthesis.
type DistillConfig struct {
	Format               DistillFormat `json:"format"`
	MinRewardMargin      float64       `json:"min_reward_margin"`     // Minimum delta reward to accept DPO pair
	FilterErrors         bool          `json:"filter_errors"`         // Omit turns containing unhandled tool errors
	StripToolNoise       bool          `json:"strip_tool_noise"`      // Remove excessive logs or repetitive tool output
	MaxTurnsPerSession   int           `json:"max_turns_per_session"` // Cap max turns to fit context windows
	SystemPromptOverride string        `json:"system_prompt_override,omitempty"`
	OrgID                string        `json:"org_id"`
}

// DefaultDistillConfig returns standard production defaults for trajectory distillation.
func DefaultDistillConfig() DistillConfig {
	return DistillConfig{
		Format:             FormatDPO,
		MinRewardMargin:    0.20,
		FilterErrors:       true,
		StripToolNoise:     true,
		MaxTurnsPerSession: 32,
		OrgID:              "agentvcr-distill-corp",
	}
}

// DistillationReport summarizes dataset generation metrics, token savings, and quality score.
type DistillationReport struct {
	ReportID              string         `json:"report_id"`
	GeneratedAt           time.Time      `json:"generated_at"`
	Format                DistillFormat  `json:"format"`
	TotalSessionsScanned  int            `json:"total_sessions_scanned"`
	SFTExamplesGenerated  int            `json:"sft_examples_generated"`
	DPOPairsGenerated     int            `json:"dpo_pairs_generated"`
	PrunedTurnsCount      int            `json:"pruned_turns_count"`
	RawTokensTotal        int            `json:"raw_tokens_total"`
	DistilledTokensTotal  int            `json:"distilled_tokens_total"`
	CompressionRatio      float64        `json:"compression_ratio"` // e.g. 0.45 (45% smaller)
	AverageRewardMargin   float64        `json:"average_reward_margin"`
	Passed                bool           `json:"passed"`
	DPOEntries            []DPORow       `json:"dpo_entries,omitempty"`
	SFTEntries            []SFTRow       `json:"sft_entries,omitempty"`
	SamplePrompt          string         `json:"sample_prompt,omitempty"`
	SampleChosenSummary   string         `json:"sample_chosen_summary,omitempty"`
	SampleRejectedSummary string         `json:"sample_rejected_summary,omitempty"`
	TraceSession          *trace.Session `json:"trace_session,omitempty"`
}
