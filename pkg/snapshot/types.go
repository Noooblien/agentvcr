package snapshot

import (
	"time"
)

// SnapshotDeltaReport provides a high-level summary of filesystem mutations across layers.
type SnapshotDeltaReport struct {
	SessionID        string        `json:"session_id"`
	TotalLayers      int           `json:"total_layers"`
	CreatedFiles     []string      `json:"created_files"`
	ModifiedFiles    []string      `json:"modified_files"`
	DeletedFiles     []string      `json:"deleted_files"`
	TotalBytesDelta  int64         `json:"total_bytes_delta"`
	RollbackLatency  time.Duration `json:"rollback_latency"`
	Status           string        `json:"status"` // "COMMITTED", "DIRTY", "RESTORED"
	ExecutedAt       time.Time     `json:"executed_at"`
}

// CoWConfig provides runtime configuration for the snapshot supervisor.
type CoWConfig struct {
	BaseWorkspaceDir string `json:"base_workspace_dir"`
	StorageDir       string `json:"storage_dir"`
	MaxSnapshots     int    `json:"max_snapshots"`
	Strict           bool   `json:"strict"`
}
