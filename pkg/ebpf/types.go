package ebpf

import (
	"time"
)

// SyscallType categorizes intercepted operating system syscalls.
type SyscallType string

const (
	SyscallSocketConnect SyscallType = "sys_enter_connect"
	SyscallSocketAccept  SyscallType = "sys_enter_accept"
	SyscallFileOpen      SyscallType = "sys_enter_openat"
	SyscallFileRead      SyscallType = "sys_enter_read"
	SyscallFileWrite     SyscallType = "sys_enter_write"
	SyscallProcessExec   SyscallType = "sys_enter_execve"
	SyscallProcessClone  SyscallType = "sys_enter_clone"
	SyscallMemoryProtect SyscallType = "sys_enter_mprotect"
)

// SocketEvent records a kernel-level network connection intercepted by eBPF probe.
type SocketEvent struct {
	PID          int       `json:"pid"`
	TGID         int       `json:"tgid"`
	ProcessName  string    `json:"process_name"`
	Protocol     string    `json:"protocol"` // "tcp", "udp"
	SrcIP        string    `json:"src_ip"`
	SrcPort      int       `json:"src_port"`
	DstIP        string    `json:"dst_ip"`
	DstPort      int       `json:"dst_port"`
	Domain       string    `json:"domain,omitempty"`
	BytesSent    int64     `json:"bytes_sent"`
	BytesRecv    int64     `json:"bytes_recv"`
	IsEgress     bool      `json:"is_egress"`
	Blocked      bool      `json:"blocked"`
	BlockReason  string    `json:"block_reason,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
}

// SyscallTraceEvent captures low-level kernel execution trace details for an agent step.
type SyscallTraceEvent struct {
	EventID       string      `json:"event_id"`
	StepIndex     int         `json:"step_index"`
	Syscall       SyscallType `json:"syscall"`
	PID           int         `json:"pid"`
	ProcessName   string      `json:"process_name"`
	TargetPath    string      `json:"target_path,omitempty"`    // e.g. "/etc/shadow", "/bin/sh"
	Socket        *SocketEvent `json:"socket,omitempty"`
	ReturnValue   int64       `json:"return_value"`
	LatencyNs     int64       `json:"latency_ns"`
	TamperRisk    string      `json:"tamper_risk"`              // "none", "low", "medium", "critical"
	ActionBlocked bool        `json:"action_blocked"`
	Timestamp     time.Time   `json:"timestamp"`
}

// EBPFProbeConfig configures kernel tracing probes and security filters.
type EBPFProbeConfig struct {
	CaptureSockets    bool     `json:"capture_sockets"`
	CaptureExecve     bool     `json:"capture_execve"`
	CaptureFileIO     bool     `json:"capture_file_io"`
	BlockedIPs        []string `json:"blocked_ips"`         // e.g. ["169.254.169.254", "10.0.0.1"]
	BlockedSubnets    []string `json:"blocked_subnets"`
	BlockedBinaries   []string `json:"blocked_binaries"`    // e.g. ["nc", "ncat", "curl", "bash"]
	SensitivePaths    []string `json:"sensitive_paths"`     // e.g. ["/etc/passwd", "/root/.ssh"]
	MaxPayloadCapture int      `json:"max_payload_capture"`
}

// DefaultEBPFConfig provides recommended sandbox probe settings.
func DefaultEBPFConfig() EBPFProbeConfig {
	return EBPFProbeConfig{
		CaptureSockets:  true,
		CaptureExecve:   true,
		CaptureFileIO:   true,
		BlockedIPs:      []string{"169.254.169.254", "metadata.google.internal"}, // Cloud metadata SSRF
		BlockedBinaries: []string{"ncat", "nc", "socat"},
		SensitivePaths:  []string{"/etc/shadow", "/etc/sudoers", "/root/.ssh", "/etc/passwd"},
		MaxPayloadCapture: 4096,
	}
}

// EBPFAuditReport summarizes kernel-level security observations and socket activities across a trace.
type EBPFAuditReport struct {
	SessionID            string              `json:"session_id"`
	TotalSyscalls        int                 `json:"total_syscalls"`
	SocketEventsCount    int                 `json:"socket_events_count"`
	ExecEventsCount      int                 `json:"exec_events_count"`
	FileIOEventsCount    int                 `json:"file_io_events_count"`
	BlockedSyscallsCount int                 `json:"blocked_syscalls_count"`
	SSRFAttemptsCount    int                 `json:"ssrf_attempts_count"`
	UnauthorizedEscalations int              `json:"unauthorized_escalations"`
	KernelTamperProofOK  bool                `json:"kernel_tamper_proof_ok"`
	Syscalls             []SyscallTraceEvent `json:"syscalls"`
	Violations           []string            `json:"violations"`
	Passed               bool                `json:"passed"`
}
