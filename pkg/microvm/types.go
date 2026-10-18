package microvm

import (
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// VMState represents the lifecycle status of an ephemeral microVM.
type VMState string

const (
	VMStateCreated    VMState = "created"
	VMStateBooting    VMState = "booting"
	VMStateRunning    VMState = "running"
	VMStatePaused     VMState = "paused"
	VMStateRestoring  VMState = "restoring"
	VMStateTerminated VMState = "terminated"
	VMStateBreached   VMState = "breached_jailed"
)

// JailSecurityPolicy defines the seccomp, capability, and filesystem isolation rules.
type JailSecurityPolicy struct {
	AllowRawSockets    bool     `json:"allow_raw_sockets"`
	AllowRootUser      bool     `json:"allow_root_user"`
	BlockedSyscalls    []string `json:"blocked_syscalls"`
	RestrictedPaths    []string `json:"restricted_paths"`
	EgressAllowList    []string `json:"egress_allow_list"`
	MaxCPUTimeMs       int64    `json:"max_cpu_time_ms"`
	MaxMemoryBytes     int64    `json:"max_memory_bytes"`
	AutoSnapshotOnTurn bool     `json:"auto_snapshot_on_turn"`
}

// DefaultJailPolicy returns strict zero-trust sandbox containment rules.
func DefaultJailPolicy() JailSecurityPolicy {
	return JailSecurityPolicy{
		AllowRawSockets: false,
		AllowRootUser:   false,
		BlockedSyscalls: []string{
			"ptrace", "reboot", "sys_chroot", "kexec_load", "bpf",
			"mount", "umount2", "pivot_root", "init_module", "finit_module",
		},
		RestrictedPaths: []string{
			"/etc/shadow", "/proc/kcore", "/dev/mem", "/sys/firmware",
			"/root/.ssh", "/home/slayer28/.config/gh",
		},
		EgressAllowList: []string{
			"api.openai.com", "api.anthropic.com", "github.com",
		},
		MaxCPUTimeMs:       5000,
		MaxMemoryBytes:     512 * 1024 * 1024, // 512MB
		AutoSnapshotOnTurn: true,
	}
}

// MicroVMConfig defines machine specs and storage overlays.
type MicroVMConfig struct {
	VMID           string             `json:"vm_id"`
	KernelImage    string             `json:"kernel_image"`
	RootFSDrive    string             `json:"rootfs_drive"`
	VCPUCount      int                `json:"vcpu_count"`
	MemoryMB       int                `json:"memory_mb"`
	Policy         JailSecurityPolicy `json:"policy"`
	ChrootPath     string             `json:"chroot_path"`
	UID            int                `json:"uid"`
	GID            int                `json:"gid"`
}

// VMSnapshot captures a microsecond-granularity memory & register dump.
type VMSnapshot struct {
	SnapshotID     string    `json:"snapshot_id"`
	VMID           string    `json:"vm_id"`
	TurnIndex      int       `json:"turn_index"`
	MemoryHash     string    `json:"memory_hash"`
	MemorySizeBytes int64    `json:"memory_size_bytes"`
	CPUStateHash   string    `json:"cpu_state_hash"`
	CreatedAt      time.Time `json:"created_at"`
	RestorationMs  int64     `json:"restoration_ms"`
}

// ExecutionResult records the outcome of code or tool commands run within the microVM.
type ExecutionResult struct {
	Command        string        `json:"command"`
	Stdout         string        `json:"stdout"`
	Stderr         string        `json:"stderr"`
	ExitCode       int           `json:"exit_code"`
	DurationMs     int64         `json:"duration_ms"`
	SecurityBreach bool          `json:"security_breach"`
	ViolationType  string        `json:"violation_type,omitempty"`
	ViolationDetail string       `json:"violation_detail,omitempty"`
}

// MicroVMInstance holds live microVM execution state.
type MicroVMInstance struct {
	Config         MicroVMConfig         `json:"config"`
	State          VMState               `json:"state"`
	BootDurationMs int64                 `json:"boot_duration_ms"`
	Snapshots      map[string]VMSnapshot `json:"snapshots"`
	CurrentTurn    int                   `json:"current_turn"`
	TotalRestores  int                   `json:"total_restores"`
	BreachesCaught int                   `json:"breaches_caught"`
	CreatedAt      time.Time             `json:"created_at"`
	UpdatedAt      time.Time             `json:"updated_at"`
}

// MicroVMReport summarizes flight recorded microVM execution and containment forensics.
type MicroVMReport struct {
	ReportID             string            `json:"report_id"`
	VMID                 string            `json:"vm_id"`
	Status               string            `json:"status"` // "HEALTHY", "RESTORED", "BREACH_JAILED", "TERMINATED"
	BootLatencyMs        int64             `json:"boot_latency_ms"`
	TotalSnapshots       int               `json:"total_snapshots"`
	TotalRestores        int               `json:"total_restores"`
	AverageRestoreMs     float64           `json:"average_restore_ms"`
	BreachesDetected     int               `json:"breaches_detected"`
	Violations           []string          `json:"violations,omitempty"`
	Executions           []ExecutionResult `json:"executions"`
	TraceSession         *trace.Session    `json:"trace_session,omitempty"`
	Passed               bool              `json:"passed"`
}
