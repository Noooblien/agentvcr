package checkpoint

import (
	"time"
)

// CheckpointState represents the lifecycle status of a hibernated agent image.
type CheckpointState string

const (
	StateActive     CheckpointState = "ACTIVE"     // Currently running in memory
	StateHibernated CheckpointState = "HIBERNATED" // Suspended, serialized & compressed on disk
	StateRestored   CheckpointState = "RESTORED"   // Thawed and revived into working memory
	StateCorrupted  CheckpointState = "CORRUPTED"  // Integrity verification failed (hash mismatch)
)

// AgentContextFrame captures the prompt/message stack and tool call history.
type AgentContextFrame struct {
	Role       string                 `json:"role"` // system, user, assistant, tool
	Content    string                 `json:"content"`
	ToolCallID string                 `json:"tool_call_id,omitempty"`
	Action     string                 `json:"action,omitempty"`
	Timestamp  time.Time              `json:"timestamp"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// MemoryVariable captures an individual key-value variable in working scratchpad.
type MemoryVariable struct {
	Key       string      `json:"key"`
	Value     interface{} `json:"value"`
	Type      string      `json:"type"`
	UpdatedAt time.Time   `json:"updated_at"`
}

// ToolCursorState represents the execution cursor of an active tool pipeline.
type ToolCursorState struct {
	ToolName        string                 `json:"tool_name"`
	InvocationID    string                 `json:"invocation_id"`
	ProgressPercent float64                `json:"progress_percent"`
	PendingPayload  map[string]interface{} `json:"pending_payload,omitempty"`
	RetryAttempt    int                    `json:"retry_attempt"`
}

// SessionImage represents the complete serialized state of an agent at an exact microsecond.
type SessionImage struct {
	ImageID         string                 `json:"image_id"`
	SessionID       string                 `json:"session_id"`
	AgentID         string                 `json:"agent_id"`
	StepIndex       int                    `json:"step_index"`
	State           CheckpointState        `json:"state"`
	CreatedAt       time.Time              `json:"created_at"`
	ContextFrames   []AgentContextFrame    `json:"context_frames"`
	WorkingMemory   map[string]interface{} `json:"working_memory"`
	ToolCursors     []ToolCursorState      `json:"tool_cursors"`
	PRNGSeed        int64                  `json:"prng_seed"`
	VirtualClockNow time.Time              `json:"virtual_clock_now"`
	UncompressedSize int64                 `json:"uncompressed_size_bytes"`
	CompressedSize   int64                 `json:"compressed_size_bytes"`
	SHA256Hash      string                 `json:"sha256_hash"`
	DeltaBaseID     string                 `json:"delta_base_id,omitempty"` // ID of parent base image if delta
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// HibernationConfig sets compression, storage directory, and delta parameters.
type HibernationConfig struct {
	StorageDir           string        `json:"storage_dir"`
	UseDeltaCompression  bool          `json:"use_delta_compression"`
	GzipCompressionLevel int           `json:"gzip_compression_level"` // 1-9
	AutoHibernateTimeout time.Duration `json:"auto_hibernate_timeout"`
	MaxSnapshotsRetained int           `json:"max_snapshots_retained"`
}

// DefaultHibernationConfig returns standard production settings.
func DefaultHibernationConfig(dir string) HibernationConfig {
	if dir == "" {
		dir = "./.vcr_checkpoints"
	}
	return HibernationConfig{
		StorageDir:           dir,
		UseDeltaCompression:  true,
		GzipCompressionLevel: 6,
		AutoHibernateTimeout: 5 * time.Minute,
		MaxSnapshotsRetained: 20,
	}
}

// CheckpointAuditReport summarizes all checkpoint images, disk savings, and restore validation.
type CheckpointAuditReport struct {
	SystemID              string          `json:"system_id"`
	Timestamp             time.Time       `json:"timestamp"`
	TotalImages           int             `json:"total_images"`
	TotalHibernated       int             `json:"total_hibernated"`
	TotalRestored         int             `json:"total_restored"`
	TotalRawBytes         int64           `json:"total_raw_bytes"`
	TotalCompressedBytes  int64           `json:"total_compressed_bytes"`
	CompressionRatio      float64         `json:"compression_ratio"` // e.g., 0.75 = 75% space saved
	IntegrityVerified     bool            `json:"integrity_verified"`
	Images                []*SessionImage `json:"images"`
}
