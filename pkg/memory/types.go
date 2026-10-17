package memory

import (
	"time"
)

// MemoryLevel indicates the hierarchical depth and abstraction of a memory.
type MemoryLevel string

const (
	LevelEpisodic   MemoryLevel = "EPISODIC"   // Raw turn-by-turn agent event / execution step
	LevelReflection MemoryLevel = "REFLECTION" // Synthesized insight / episodic cluster
	LevelSemantic   MemoryLevel = "SEMANTIC"   // Permanent knowledge triple (Subject-Predicate-Object)
)

// EpisodicTurn captures a single granular action, prompt, tool execution, or observation.
type EpisodicTurn struct {
	ID          string                 `json:"id"`
	SessionID   string                 `json:"session_id"`
	StepIndex   int                    `json:"step_index"`
	Role        string                 `json:"role"` // system, user, assistant, tool
	Content     string                 `json:"content"`
	Action      string                 `json:"action,omitempty"`
	Observation string                 `json:"observation,omitempty"`
	Importance  float64                `json:"importance"` // 0.0 (trivial) to 1.0 (vital decision)
	Timestamp   time.Time              `json:"timestamp"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// MemoryNode represents a unit of consolidated memory inside the agent knowledge mesh.
type MemoryNode struct {
	ID               string                 `json:"id"`
	Level            MemoryLevel            `json:"level"`
	Content          string                 `json:"content"`
	Subject          string                 `json:"subject,omitempty"`   // Entity Subject for semantic triples
	Predicate        string                 `json:"predicate,omitempty"` // Relationship / Action
	Object           string                 `json:"object,omitempty"`    // Target Entity / Value
	Confidence       float64                `json:"confidence"`          // Extraction confidence (0.0 - 1.0)
	Importance       float64                `json:"importance"`          // Intrinsic weight (0.0 - 1.0)
	Stability        float64                `json:"stability"`           // Ebbinghaus memory strength factor
	CreatedAt        time.Time              `json:"created_at"`
	LastAccessedAt   time.Time              `json:"last_accessed_at"`
	AccessCount      int                    `json:"access_count"`
	SourceEpisodeIDs []string               `json:"source_episode_ids"` // Lineage tracing back to raw turns
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
}

// MemoryConfig configures decay rate, thresholds, and memory consolidation cycles.
type MemoryConfig struct {
	EpisodicBufferLimit    int           `json:"episodic_buffer_limit"`   // Max raw turns before mandatory consolidation
	DecayRate              float64       `json:"decay_rate"`              // Ebbinghaus lambda decay factor
	ImportanceThreshold    float64       `json:"importance_threshold"`    // Min importance to promote to semantic level
	MinRetentionScore      float64       `json:"min_retention_score"`     // Threshold below which unreinforced memory is pruned
	AccessReinforcement    float64       `json:"access_reinforcement"`    // Multiplier boosting stability upon recall
	ConsolidationBatchSize int           `json:"consolidation_batch_size"`// Number of episodic turns to cluster
	ClockDrift             time.Duration `json:"clock_drift,omitempty"`   // Synthetic time advancement for testing
}

// DefaultMemoryConfig provides battle-tested defaults for autonomous agent memory hierarchies.
func DefaultMemoryConfig() MemoryConfig {
	return MemoryConfig{
		EpisodicBufferLimit:    100,
		DecayRate:              0.05, // Retains ~60% strength after 10 hours without access
		ImportanceThreshold:    0.50,
		MinRetentionScore:      0.15,
		AccessReinforcement:    0.25, // 25% boost in stability per recall
		ConsolidationBatchSize: 5,
	}
}

// RecallQuery specifies parameters for contextual memory retrieval into prompt context.
type RecallQuery struct {
	QueryText        string        `json:"query_text"`
	TokenBudget      int           `json:"token_budget"` // Strict token cap for retrieved memories
	TargetLevels     []MemoryLevel `json:"target_levels,omitempty"`
	MinRetention     float64       `json:"min_retention"`
	IncludeEpisodic  bool          `json:"include_episodic"`
	CurrentTimestamp time.Time     `json:"current_timestamp"`
}

// ConsolidationReport summarizes memory hierarchy size, pruning, and context compression metrics.
type ConsolidationReport struct {
	SystemID               string        `json:"system_id"`
	Timestamp              time.Time     `json:"timestamp"`
	TotalEpisodicBuffered  int           `json:"total_episodic_buffered"`
	ConsolidatedReflection int           `json:"consolidated_reflection"`
	PermanentSemanticFacts int           `json:"permanent_semantic_facts"`
	PrunedDecayedNodes     int           `json:"pruned_decayed_nodes"`
	ActiveMemoryNodes      int           `json:"active_memory_nodes"`
	RawEpisodicTokens      int           `json:"raw_episodic_tokens"`
	ConsolidatedTokens     int           `json:"consolidated_tokens"`
	TokenSavingsRatio      float64       `json:"token_savings_ratio"` // e.g. 0.72 = 72% token reduction
	TopMemories            []*MemoryNode `json:"top_memories"`
}
