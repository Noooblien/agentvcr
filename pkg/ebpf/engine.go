package ebpf

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// KernelRecorder models an ephemeral eBPF kprobe/tracepoint ring-buffer recorder.
type KernelRecorder struct {
	mu         sync.RWMutex
	sessionID  string
	config     EBPFProbeConfig
	events     []SyscallTraceEvent
	sockets    []SocketEvent
	violations []string
}

// NewKernelRecorder instantiates an ephemeral eBPF tracer.
func NewKernelRecorder(sessionID string, config EBPFProbeConfig) *KernelRecorder {
	return &KernelRecorder{
		sessionID:  sessionID,
		config:     config,
		events:     make([]SyscallTraceEvent, 0),
		sockets:    make([]SocketEvent, 0),
		violations: make([]string, 0),
	}
}

// InterceptSocket captures a socket connection attempt, enforcing SSRF and metadata filters.
func (kr *KernelRecorder) InterceptSocket(stepIndex int, pid int, procName, proto, srcIP string, srcPort int, dstIP string, dstPort int, domain string, bytesSent, bytesRecv int64) SyscallTraceEvent {
	kr.mu.Lock()
	defer kr.mu.Unlock()

	blocked := false
	blockReason := ""
	risk := "none"

	// 1. Check blocked metadata IP / hosts (SSRF)
	for _, bip := range kr.config.BlockedIPs {
		if dstIP == bip || strings.EqualFold(domain, bip) {
			blocked = true
			blockReason = fmt.Sprintf("Blocked metadata SSRF egress destination: %s", bip)
			risk = "critical"
			break
		}
	}

	// 2. Check subnet restrictions
	if !blocked && len(kr.config.BlockedSubnets) > 0 {
		parsedIP := net.ParseIP(dstIP)
		if parsedIP != nil {
			for _, cidr := range kr.config.BlockedSubnets {
				_, subnet, err := net.ParseCIDR(cidr)
				if err == nil && subnet.Contains(parsedIP) {
					blocked = true
					blockReason = fmt.Sprintf("Egress IP %s belongs to restricted subnet %s", dstIP, cidr)
					risk = "high"
					break
				}
			}
		}
	}

	sockEv := SocketEvent{
		PID:         pid,
		TGID:        pid,
		ProcessName: procName,
		Protocol:    proto,
		SrcIP:       srcIP,
		SrcPort:     srcPort,
		DstIP:       dstIP,
		DstPort:     dstPort,
		Domain:      domain,
		BytesSent:   bytesSent,
		BytesRecv:   bytesRecv,
		IsEgress:    true,
		Blocked:     blocked,
		BlockReason: blockReason,
		Timestamp:   time.Now(),
	}

	kr.sockets = append(kr.sockets, sockEv)

	retVal := int64(0)
	if blocked {
		retVal = -1 // EPERM
		kr.violations = append(kr.violations, fmt.Sprintf("Step %d (PID %d): %s", stepIndex, pid, blockReason))
	}

	traceEv := SyscallTraceEvent{
		EventID:       fmt.Sprintf("ebpf_sock_%s_%d_%d", kr.sessionID, stepIndex, len(kr.events)+1),
		StepIndex:     stepIndex,
		Syscall:       SyscallSocketConnect,
		PID:           pid,
		ProcessName:   procName,
		TargetPath:    fmt.Sprintf("%s:%d", dstIP, dstPort),
		Socket:        &sockEv,
		ReturnValue:   retVal,
		LatencyNs:     45000, // Simulated fast kernel hook
		TamperRisk:    risk,
		ActionBlocked: blocked,
		Timestamp:     time.Now(),
	}

	kr.events = append(kr.events, traceEv)
	return traceEv
}

// InterceptExecve captures process spawning, enforcing binary whitelisting/blacklisting.
func (kr *KernelRecorder) InterceptExecve(stepIndex int, pid int, binaryPath string, args []string) SyscallTraceEvent {
	kr.mu.Lock()
	defer kr.mu.Unlock()

	blocked := false
	risk := "none"
	reason := ""

	// Check blocked binaries
	for _, b := range kr.config.BlockedBinaries {
		if strings.HasSuffix(binaryPath, "/"+b) || binaryPath == b {
			blocked = true
			risk = "critical"
			reason = fmt.Sprintf("Execution of forbidden utility detected: %s", b)
			break
		}
	}

	retVal := int64(0)
	if blocked {
		retVal = -13 // -EACCES
		kr.violations = append(kr.violations, fmt.Sprintf("Step %d (PID %d): %s", stepIndex, pid, reason))
	}

	fullCmd := binaryPath
	if len(args) > 0 {
		fullCmd = fmt.Sprintf("%s %s", binaryPath, strings.Join(args, " "))
	}

	traceEv := SyscallTraceEvent{
		EventID:       fmt.Sprintf("ebpf_exec_%s_%d_%d", kr.sessionID, stepIndex, len(kr.events)+1),
		StepIndex:     stepIndex,
		Syscall:       SyscallProcessExec,
		PID:           pid,
		ProcessName:   binaryPath,
		TargetPath:    fullCmd,
		ReturnValue:   retVal,
		LatencyNs:     85000,
		TamperRisk:    risk,
		ActionBlocked: blocked,
		Timestamp:     time.Now(),
	}

	kr.events = append(kr.events, traceEv)
	return traceEv
}

// InterceptFileIO captures file access, detecting reads to sensitive credentials or system files.
func (kr *KernelRecorder) InterceptFileIO(stepIndex int, pid int, procName, filePath string, isWrite bool) SyscallTraceEvent {
	kr.mu.Lock()
	defer kr.mu.Unlock()

	blocked := false
	risk := "none"
	syscall := SyscallFileOpen
	if isWrite {
		syscall = SyscallFileWrite
	}

	for _, sPath := range kr.config.SensitivePaths {
		if strings.HasPrefix(filePath, sPath) || filePath == sPath {
			blocked = true
			risk = "critical"
			kr.violations = append(kr.violations,
				fmt.Sprintf("Step %d (PID %d): Unauthorized access attempt to protected kernel/system path: %s",
					stepIndex, pid, filePath))
			break
		}
	}

	retVal := int64(3) // fd 3
	if blocked {
		retVal = -13 // -EACCES
	}

	traceEv := SyscallTraceEvent{
		EventID:       fmt.Sprintf("ebpf_file_%s_%d_%d", kr.sessionID, stepIndex, len(kr.events)+1),
		StepIndex:     stepIndex,
		Syscall:       syscall,
		PID:           pid,
		ProcessName:   procName,
		TargetPath:    filePath,
		ReturnValue:   retVal,
		LatencyNs:     12000,
		TamperRisk:    risk,
		ActionBlocked: blocked,
		Timestamp:     time.Now(),
	}

	kr.events = append(kr.events, traceEv)
	return traceEv
}

// GenerateAuditReport produces an aggregated forensic report of kernel and socket activity.
func (kr *KernelRecorder) GenerateAuditReport() *EBPFAuditReport {
	kr.mu.RLock()
	defer kr.mu.RUnlock()

	report := &EBPFAuditReport{
		SessionID:           kr.sessionID,
		TotalSyscalls:       len(kr.events),
		KernelTamperProofOK: true,
		Syscalls:            kr.events,
		Violations:          kr.violations,
	}

	for _, ev := range kr.events {
		if ev.ActionBlocked {
			report.BlockedSyscallsCount++
		}
		if ev.TamperRisk == "critical" {
			report.KernelTamperProofOK = false
		}

		switch ev.Syscall {
		case SyscallSocketConnect, SyscallSocketAccept:
			report.SocketEventsCount++
			if ev.ActionBlocked && ev.Socket != nil && strings.Contains(ev.Socket.BlockReason, "SSRF") {
				report.SSRFAttemptsCount++
			}
		case SyscallProcessExec, SyscallProcessClone:
			report.ExecEventsCount++
			if ev.ActionBlocked {
				report.UnauthorizedEscalations++
			}
		case SyscallFileOpen, SyscallFileRead, SyscallFileWrite:
			report.FileIOEventsCount++
		}
	}

	report.Passed = report.BlockedSyscallsCount == 0 && len(report.Violations) == 0
	return report
}

// ComputeSyscallChecksum computes a SHA-256 integrity hash over all captured kernel syscall events.
func (kr *KernelRecorder) ComputeSyscallChecksum() string {
	kr.mu.RLock()
	defer kr.mu.RUnlock()

	h := sha256.New()
	for _, ev := range kr.events {
		line := fmt.Sprintf("%d:%s:%d:%s:%s:%d\n",
			ev.StepIndex, ev.Syscall, ev.PID, ev.ProcessName, ev.TargetPath, ev.ReturnValue)
		h.Write([]byte(line))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// SimulateSessionEBPF traces an agent trajectory by parsing tool calls and commands into kernel syscall probes.
func SimulateSessionEBPF(session *trace.Session, config EBPFProbeConfig) (*KernelRecorder, *EBPFAuditReport) {
	kr := NewKernelRecorder(session.ID, config)

	pid := 41920
	for _, ev := range session.Events {
		// Normal application file reads
		kr.InterceptFileIO(ev.Index, pid, "agentvcr-runtime", "/home/slayer28/workspace/project.go", false)

		if ev.Type == trace.EventToolCall {
			toolName, _ := ev.Payload["tool"].(string)

			// 1. Shell/Bash executions
			if strings.Contains(toolName, "bash") || strings.Contains(toolName, "exec") || strings.Contains(toolName, "terminal") {
				cmdStr := ""
				if args, ok := ev.Payload["args"].(map[string]interface{}); ok {
					if c, ok := args["command"].(string); ok {
						cmdStr = c
					}
				}
				if cmdStr == "" {
					cmdStr = "ls -la"
				}

				parts := strings.Fields(cmdStr)
				bin := "/bin/bash"
				if len(parts) > 0 {
					bin = parts[0]
				}
				kr.InterceptExecve(ev.Index, pid+1, bin, parts[1:])

				// Check if command references sensitive system files
				if strings.Contains(cmdStr, "/etc/shadow") || strings.Contains(cmdStr, "/root/.ssh") {
					kr.InterceptFileIO(ev.Index, pid+1, bin, "/etc/shadow", false)
				}
			}

			// 2. Network/HTTP calls
			if strings.Contains(toolName, "http") || strings.Contains(toolName, "fetch") || strings.Contains(toolName, "search") {
				targetURL := "https://api.openai.com/v1"
				if args, ok := ev.Payload["args"].(map[string]interface{}); ok {
					if u, ok := args["url"].(string); ok {
						targetURL = u
					}
				}

				destIP := "104.18.7.192"
				port := 443
				domain := "api.openai.com"

				if strings.Contains(targetURL, "169.254.169.254") {
					destIP = "169.254.169.254"
					port = 80
					domain = "169.254.169.254"
				}

				kr.InterceptSocket(ev.Index, pid, "agent-net-client", "tcp", "192.168.1.10", 52140, destIP, port, domain, 1024, 4096)
			}
		}
	}

	report := kr.GenerateAuditReport()
	return kr, report
}

// RenderEBPFScorecard formats kernel and socket syscall metrics into an ANSI terminal scorecard.
func RenderEBPFScorecard(report *EBPFAuditReport) string {
	var sb strings.Builder

	sb.WriteString("\n\033[1;36m========================================================================================\033[0m\n")
	sb.WriteString(fmt.Sprintf("\033[1;37m   AGENTVCR EPHEMERAL eBPF KERNEL SOCKET & SYSCALL INTERCEPTION AUDIT\033[0m\n"))
	sb.WriteString("\033[1;36m========================================================================================\033[0m\n")

	sb.WriteString(fmt.Sprintf("  \033[1;33mSession ID:\033[0m            %s\n", report.SessionID))
	sb.WriteString(fmt.Sprintf("  \033[1;33mTotal Syscalls:\033[0m        %d intercepted (Sockets: %d | Exec: %d | Files: %d)\n",
		report.TotalSyscalls, report.SocketEventsCount, report.ExecEventsCount, report.FileIOEventsCount))

	if report.Passed {
		sb.WriteString("  \033[1;33mKernel Security:\033[0m       \033[1;32m[PASS] ZERO KERNEL ESCALATIONS & CLEAN SOCKET EGRESS\033[0m\n")
	} else {
		sb.WriteString(fmt.Sprintf("  \033[1;33mKernel Security:\033[0m       \033[1;31m[FAIL] %d BLOCKED SYSCALLS (SSRF: %d | Escalations: %d)\033[0m\n",
			report.BlockedSyscallsCount, report.SSRFAttemptsCount, report.UnauthorizedEscalations))
	}
	sb.WriteString("\033[1;34m----------------------------------------------------------------------------------------\033[0m\n")

	sb.WriteString("\033[1;37m  INTERCEPTED KERNEL PROBE SAMPLES (kprobe / sys_enter):\033[0m\n")
	sb.WriteString("  \033[1;30mSTEP  PID    SYSCALL            PROCESS          TARGET / SOCKET DESTINATION    STATUS\033[0m\n")

	for _, s := range report.Syscalls {
		statusStr := "\033[1;32mALLOWED\033[0m"
		if s.ActionBlocked {
			statusStr = "\033[1;31mBLOCKED\033[0m"
		}
		target := s.TargetPath
		if len(target) > 30 {
			target = target[:27] + "..."
		}
		proc := s.ProcessName
		if len(proc) > 16 {
			proc = proc[:13] + "..."
		}

		sb.WriteString(fmt.Sprintf("  #%-4d %-6d %-18s %-16s %-30s %s\n",
			s.StepIndex, s.PID, s.Syscall, proc, target, statusStr))
	}

	if len(report.Violations) > 0 {
		sb.WriteString("\033[1;34m----------------------------------------------------------------------------------------\033[0m\n")
		sb.WriteString("\033[1;31m  KERNEL SECURITY VIOLATIONS & BLOCKED HAZARDS:\033[0m\n")
		for _, v := range report.Violations {
			sb.WriteString(fmt.Sprintf("  \033[1;31m[!] %s\033[0m\n", v))
		}
	}

	sb.WriteString("\033[1;36m========================================================================================\033[0m\n\n")
	return sb.String()
}
