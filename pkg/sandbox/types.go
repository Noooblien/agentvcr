package sandbox

import (
	"time"
)

// SandboxState represents the lifecycle state of a sandbox execution container.
type SandboxState string

const (
	StateCreated       SandboxState = "CREATED"
	StateRunning       SandboxState = "RUNNING"
	StatePaused        SandboxState = "PAUSED"
	StateTerminated    SandboxState = "TERMINATED"
	StateKilledOOM     SandboxState = "KILLED_OOM"
	StateKilledTimeout SandboxState = "KILLED_TIMEOUT"
)

// NetworkPolicy defines network egress rules for code execution in the sandbox.
type NetworkPolicy string

const (
	NetworkPolicyNone      NetworkPolicy = "NONE"       // Complete network isolation
	NetworkPolicyAllowList NetworkPolicy = "ALLOW_LIST" // Outbound only to explicit domains/IPs
	NetworkPolicyDenyList  NetworkPolicy = "DENY_LIST"  // Outbound allowed except blocked destinations
)

// ViolationSeverity defines risk levels for sandbox policy infractions.
type ViolationSeverity string

const (
	SeverityInfo     ViolationSeverity = "INFO"
	SeverityWarning  ViolationSeverity = "WARNING"
	SeverityHigh     ViolationSeverity = "HIGH"
	SeverityCritical ViolationSeverity = "CRITICAL"
)

// ViolationType identifies the security violation category.
type ViolationType string

const (
	ViolationKernelEscape        ViolationType = "KERNEL_ESCAPE"
	ViolationDestructiveCommand   ViolationType = "DESTRUCTIVE_COMMAND"
	ViolationCloudMetadataSSRF   ViolationType = "CLOUD_METADATA_SSRF"
	ViolationCredentialTheft     ViolationType = "CREDENTIAL_THEFT"
	ViolationMemoryQuotaExceeded ViolationType = "MEMORY_QUOTA_EXCEEDED"
	ViolationCPUTimeoutExceeded  ViolationType = "CPU_TIMEOUT_EXCEEDED"
	ViolationNetworkBlockedHost  ViolationType = "NETWORK_BLOCKED_HOST"
	ViolationForkBomb            ViolationType = "FORK_BOMB_DETECTED"
)

// SecurityViolation records an attempt to escape or breach sandbox policy.
type SecurityViolation struct {
	Timestamp   time.Time         `json:"timestamp"`
	Type        ViolationType     `json:"type"`
	Severity    ViolationSeverity `json:"severity"`
	Target      string            `json:"target"`
	Description string            `json:"description"`
	ActionTaken string            `json:"action_taken"` // e.g. "BLOCKED", "KILLED"
}

// SandboxConfig specifies the deterministic constraints and quotas for the sandbox.
type SandboxConfig struct {
	SandboxID         string            `json:"sandbox_id"`
	MaxCPUTimeMs      int64             `json:"max_cpu_time_ms"`
	MaxMemoryBytes    int64             `json:"max_memory_bytes"`
	MaxProcesses      int               `json:"max_processes"`
	MaxDiskWriteBytes int64             `json:"max_disk_write_bytes"`
	NetworkPolicy     NetworkPolicy     `json:"network_policy"`
	AllowedHosts      []string          `json:"allowed_hosts,omitempty"`
	BlockedHosts      []string          `json:"blocked_hosts,omitempty"`
	WorkDir           string            `json:"work_dir"`
	Environment       map[string]string `json:"environment,omitempty"`
}

// DefaultSandboxConfig returns recommended security quotas for autonomous coding agents.
func DefaultSandboxConfig(id string) SandboxConfig {
	return SandboxConfig{
		SandboxID:         id,
		MaxCPUTimeMs:      15000,           // 15 seconds CPU limit per command
		MaxMemoryBytes:    512 * 1024 * 1024, // 512 MB RSS limit
		MaxProcesses:      64,              // 64 PIDs ceiling
		MaxDiskWriteBytes: 100 * 1024 * 1024, // 100 MB write limit
		NetworkPolicy:     NetworkPolicyAllowList,
		AllowedHosts:      []string{"github.com", "api.github.com", "proxy.golang.org", "registry.npmjs.org"},
		BlockedHosts: []string{
			"169.254.169.254",          // AWS / GCP / Azure IMDS
			"metadata.google.internal", // GCP metadata
			"100.100.100.200",          // Alibaba metadata
			"localhost", "127.0.0.1",   // Prevent internal service scanning
		},
		WorkDir: "/workspace",
		Environment: map[string]string{
			"PATH": "/usr/local/bin:/usr/bin:/bin",
			"HOME": "/home/agent",
			"USER": "agent",
		},
	}
}

// ExecResult records the execution trace of a command run inside the sandbox.
type ExecResult struct {
	CommandID        string              `json:"command_id"`
	CommandLine      string              `json:"command_line"`
	Argv             []string            `json:"argv"`
	WorkingDir       string              `json:"working_dir"`
	ExitCode         int                 `json:"exit_code"`
	Stdout           string              `json:"stdout"`
	Stderr           string              `json:"stderr"`
	DurationMs       int64               `json:"duration_ms"`
	CPUTimeMs        int64               `json:"cpu_time_ms"`
	MemoryPeakBytes  int64               `json:"memory_peak_bytes"`
	DiskBytesWritten int64               `json:"disk_bytes_written"`
	Violations       []SecurityViolation `json:"violations,omitempty"`
	Timestamp        time.Time           `json:"timestamp"`
}

// FSCheckpoint stores an immutable snapshot of filesystem state for rollbacks.
type FSCheckpoint struct {
	CheckpointID string            `json:"checkpoint_id"`
	CreatedAt    time.Time         `json:"created_at"`
	Files        map[string][]byte `json:"-"`
	FileHashes   map[string]string `json:"file_hashes"`
	TotalBytes   int64             `json:"total_bytes"`
}

// SandboxStats holds aggregated runtime telemetry for an active or terminated sandbox.
type SandboxStats struct {
	CommandsExecuted int                 `json:"commands_executed"`
	TotalDurationMs  int64               `json:"total_duration_ms"`
	TotalCPUTimeMs   int64               `json:"total_cpu_time_ms"`
	PeakMemoryBytes  int64               `json:"peak_memory_bytes"`
	TotalDiskWritten int64               `json:"total_disk_written"`
	FilesCreated     int                 `json:"files_created"`
	FilesModified    int                 `json:"files_modified"`
	FilesDeleted     int                 `json:"files_deleted"`
	ViolationsCount  int                 `json:"violations_count"`
	Violations       []SecurityViolation `json:"violations,omitempty"`
}

// SandboxAuditReport contains the complete compliance assessment of a sandbox session.
type SandboxAuditReport struct {
	SandboxID        string         `json:"sandbox_id"`
	State            SandboxState   `json:"state"`
	StartedAt        time.Time      `json:"started_at"`
	TerminatedAt     time.Time      `json:"terminated_at"`
	Stats            SandboxStats   `json:"stats"`
	ExecHistory      []ExecResult   `json:"exec_history"`
	ZeroEscapeRisk   bool           `json:"zero_escape_risk"`
	CompliantWithSLA bool           `json:"compliant_with_sla"`
	SecurityScore    float64        `json:"security_score"` // 0.0 - 100.0
}
