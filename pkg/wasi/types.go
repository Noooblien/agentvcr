package wasi

import (
	"time"
)

// WASISandboxState represents the lifecycle status of a WASI agent execution instance.
type WASISandboxState string

const (
	StateUninitialized WASISandboxState = "uninitialized"
	StateInstantiated  WASISandboxState = "instantiated"
	StateExecuting     WASISandboxState = "executing"
	StateTerminated    WASISandboxState = "terminated"
	StateTrapped       WASISandboxState = "trapped"
)

// MemoryLimitPolicy defines linear memory boundaries for the WebAssembly runtime.
type MemoryLimitPolicy struct {
	InitialPages uint32 `json:"initial_pages"` // 1 page = 64KB
	MaximumPages uint32 `json:"maximum_pages"` // e.g. 256 pages = 16MB
	MaxMemoryBytes int64  `json:"max_memory_bytes"`
}

// WASIPreopenDir represents a mapped virtual filesystem mount inside the WASI sandbox.
type WASIPreopenDir struct {
	GuestPath string `json:"guest_path"` // Path visible to guest WASM
	HostPath  string `json:"host_path"`  // Real host path mounted read-only or read-write
	ReadOnly  bool   `json:"read_only"`
}

// WASISandboxConfig defines fine-grained capabilities and isolation limits for the guest.
type WASISandboxConfig struct {
	ModuleID             string            `json:"module_id"`
	BinaryName           string            `json:"binary_name"`
	AllowedSyscalls      []string          `json:"allowed_syscalls"`      // e.g. "fd_read", "fd_write", "clock_time_get"
	BlockedSyscalls      []string          `json:"blocked_syscalls"`      // e.g. "sock_open", "fd_link", "poll_oneoff"
	PreopenDirs          []WASIPreopenDir  `json:"preopen_dirs"`
	EnvironmentVariables map[string]string `json:"environment_variables"`
	MemoryLimits         MemoryLimitPolicy `json:"memory_limits"`
	ExecutionTimeoutMs   int64             `json:"execution_timeout_ms"`
	FuelBudget           uint64            `json:"fuel_budget"` // Instruction metering limit to prevent infinite loops
}

// WASISyscallInvocation records an audited WASI host call made by the guest.
type WASISyscallInvocation struct {
	Index       int       `json:"index"`
	SyscallName string    `json:"syscall_name"` // e.g. "fd_write", "fd_read", "environ_get"
	Arguments   []string  `json:"arguments,omitempty"`
	ExitCode    int32     `json:"exit_code"`
	DurationUs  int64     `json:"duration_us"` // microsecond duration
	IsDenied    bool      `json:"is_denied"`
	DenialReason string   `json:"denial_reason,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
}

// WASIExecutionResult encapsulates the deterministic output and resource telemetry of a WASM run.
type WASIExecutionResult struct {
	SessionID             string                  `json:"session_id"`
	ModuleHash            string                  `json:"module_hash"` // SHA-256 of the WASM bytecode
	State                 WASISandboxState        `json:"state"`
	ExitCode              int32                   `json:"exit_code"`
	TotalFuelConsumed     uint64                  `json:"total_fuel_consumed"`
	PeakMemoryAllocatedMB float64                 `json:"peak_memory_allocated_mb"`
	ExecutionDurationMs   int64                   `json:"execution_duration_ms"`
	Stdout                string                  `json:"stdout"`
	Stderr                string                  `json:"stderr"`
	SyscallTrace          []WASISyscallInvocation `json:"syscall_trace"`
	TrapReason            string                  `json:"trap_reason,omitempty"`
	SandboxViolations     int                     `json:"sandbox_violations"`
}

// WASIScorecardReport summarizes the WASI isolation verification for a trajectory step or tool execution.
type WASIScorecardReport struct {
	SessionID            string              `json:"session_id"`
	ExecutionResult      WASIExecutionResult `json:"execution_result"`
	IsolationCompliant   bool                `json:"isolation_compliant"`
	DeterministicFuelOK  bool                `json:"deterministic_fuel_ok"`
	ZeroHostEscapeRisk   bool                `json:"zero_host_escape_risk"`
	GeneratedAt          time.Time           `json:"generated_at"`
}
