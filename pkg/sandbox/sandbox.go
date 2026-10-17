package sandbox

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Sandbox manages isolated agent execution, resource quotas, and security guards.
type Sandbox struct {
	mu          sync.Mutex
	config      SandboxConfig
	state       SandboxState
	startedAt   time.Time
	endedAt     time.Time
	fs          *OverlayFS
	auditor     *SecurityAuditor
	execHistory []ExecResult
	violations  []SecurityViolation
	stats       SandboxStats
}

// NewSandbox initializes a new isolated sandbox container with optional baseline files.
func NewSandbox(config SandboxConfig, baseFiles map[string][]byte) *Sandbox {
	if config.WorkDir == "" {
		config.WorkDir = "/workspace"
	}

	return &Sandbox{
		config:     config,
		state:      StateRunning,
		startedAt:  time.Now().UTC(),
		fs:         NewOverlayFS(baseFiles),
		auditor:    NewSecurityAuditor(config),
		violations: make([]SecurityViolation, 0),
	}
}

// GetFS returns the underlying Copy-on-Write overlay filesystem.
func (sb *Sandbox) GetFS() *OverlayFS {
	return sb.fs
}

// GetState returns current lifecycle state.
func (sb *Sandbox) GetState() SandboxState {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.state
}

// Execute runs a command line within the sandbox with full security and quota auditing.
func (sb *Sandbox) Execute(cmdLine string, stdin []byte) (*ExecResult, error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	if sb.state != StateRunning {
		return nil, fmt.Errorf("sandbox is in %s state; cannot execute commands", sb.state)
	}

	cmdID := fmt.Sprintf("cmd_%d_%04d", time.Now().Unix(), len(sb.execHistory)+1)
	start := time.Now()

	res := &ExecResult{
		CommandID:   cmdID,
		CommandLine: cmdLine,
		WorkingDir:  sb.config.WorkDir,
		Timestamp:   start.UTC(),
		Argv:        tokenizeCmd(cmdLine),
	}

	// 1. Pre-execution Security Audit
	violations := sb.auditor.InspectCommand(cmdLine)
	if len(violations) > 0 {
		res.Violations = violations
		res.ExitCode = 126
		res.Stderr = fmt.Sprintf("Execution blocked by AgentVCR Sandbox Security Guard: %s", violations[0].Description)
		res.DurationMs = 1
		res.CPUTimeMs = 1

		sb.violations = append(sb.violations, violations...)
		sb.stats.ViolationsCount += len(violations)
		sb.stats.Violations = append(sb.stats.Violations, violations...)
		sb.execHistory = append(sb.execHistory, *res)
		sb.stats.CommandsExecuted++
		return res, nil
	}

	// 2. Command Execution Simulation & Filesystem Mutations
	res.ExitCode = 0
	res.CPUTimeMs = 5 + int64(len(cmdLine)%15)
	res.MemoryPeakBytes = 12 * 1024 * 1024 // 12 MB baseline

	// Check CPU time SLA
	if res.CPUTimeMs > sb.config.MaxCPUTimeMs {
		sb.state = StateKilledTimeout
		v := SecurityViolation{
			Timestamp:   time.Now().UTC(),
			Type:        ViolationCPUTimeoutExceeded,
			Severity:    SeverityCritical,
			Target:      cmdLine,
			Description: fmt.Sprintf("Command exceeded CPU quota limit (%d ms > %d ms)", res.CPUTimeMs, sb.config.MaxCPUTimeMs),
			ActionTaken: "KILLED",
		}
		res.Violations = append(res.Violations, v)
		sb.violations = append(sb.violations, v)
		res.ExitCode = 137
		res.Stderr = "Command killed: CPU quota exceeded"
	}

	// Execute built-in agent actions on CoW OverlayFS
	if res.ExitCode == 0 {
		sb.handleCommandOperations(cmdLine, res)
	}

	// Check disk write quota
	_, _, _, totalWritten := sb.fs.GetDelta()
	if totalWritten > sb.config.MaxDiskWriteBytes {
		v := SecurityViolation{
			Timestamp:   time.Now().UTC(),
			Type:        ViolationDestructiveCommand,
			Severity:    SeverityHigh,
			Target:      cmdLine,
			Description: fmt.Sprintf("Disk write quota exceeded (%d bytes > %d bytes limit)", totalWritten, sb.config.MaxDiskWriteBytes),
			ActionTaken: "ALERT",
		}
		res.Violations = append(res.Violations, v)
		sb.violations = append(sb.violations, v)
	}

	res.DurationMs = time.Since(start).Milliseconds()
	if res.DurationMs == 0 {
		res.DurationMs = 1
	}

	sb.stats.CommandsExecuted++
	sb.stats.TotalDurationMs += res.DurationMs
	sb.stats.TotalCPUTimeMs += res.CPUTimeMs
	if res.MemoryPeakBytes > sb.stats.PeakMemoryBytes {
		sb.stats.PeakMemoryBytes = res.MemoryPeakBytes
	}
	sb.stats.TotalDiskWritten = totalWritten

	created, modified, deleted, _ := sb.fs.GetDelta()
	sb.stats.FilesCreated = len(created)
	sb.stats.FilesModified = len(modified)
	sb.stats.FilesDeleted = len(deleted)

	sb.execHistory = append(sb.execHistory, *res)
	return res, nil
}

func (sb *Sandbox) handleCommandOperations(cmdLine string, res *ExecResult) {
	trimmed := strings.TrimSpace(cmdLine)

	// Handling file creation: echo "..." > /workspace/file.txt
	if strings.Contains(trimmed, ">") && !strings.Contains(trimmed, ">>") {
		parts := strings.SplitN(trimmed, ">", 2)
		left := strings.TrimSpace(parts[0])
		targetPath := sb.resolvePath(strings.TrimSpace(parts[1]))

		content := strings.TrimPrefix(left, "echo ")
		content = strings.Trim(content, "\"'")

		_ = sb.fs.WriteFile(targetPath, []byte(content+"\n"))
		res.Stdout = fmt.Sprintf("Wrote %d bytes to %s\n", len(content)+1, targetPath)
		res.DiskBytesWritten = int64(len(content) + 1)
		return
	}

	// Handling file append: echo "..." >> /workspace/file.txt
	if strings.Contains(trimmed, ">>") {
		parts := strings.SplitN(trimmed, ">>", 2)
		left := strings.TrimSpace(parts[0])
		targetPath := sb.resolvePath(strings.TrimSpace(parts[1]))

		content := strings.TrimPrefix(left, "echo ")
		content = strings.Trim(content, "\"'")

		existing, _ := sb.fs.ReadFile(targetPath)
		newContent := append(existing, []byte(content+"\n")...)
		_ = sb.fs.WriteFile(targetPath, newContent)
		res.Stdout = fmt.Sprintf("Appended to %s\n", targetPath)
		res.DiskBytesWritten = int64(len(content) + 1)
		return
	}

	// Handling cat /workspace/file.txt
	if strings.HasPrefix(trimmed, "cat ") {
		path := sb.resolvePath(strings.TrimPrefix(trimmed, "cat "))
		data, err := sb.fs.ReadFile(path)
		if err != nil {
			res.ExitCode = 1
			res.Stderr = fmt.Sprintf("cat: %s: No such file or directory\n", path)
		} else {
			res.Stdout = string(data)
		}
		return
	}

	// Handling rm /workspace/file.txt
	if strings.HasPrefix(trimmed, "rm ") && !strings.Contains(trimmed, "-rf /") {
		path := sb.resolvePath(strings.TrimPrefix(trimmed, "rm "))
		err := sb.fs.RemoveFile(path)
		if err != nil {
			res.ExitCode = 1
			res.Stderr = fmt.Sprintf("rm: cannot remove '%s': %v\n", path, err)
		} else {
			res.Stdout = ""
		}
		return
	}

	// Handling ls or ls -la
	if trimmed == "ls" || strings.HasPrefix(trimmed, "ls ") {
		files := sb.fs.ListFiles()
		var sbOut strings.Builder
		for _, f := range files {
			sbOut.WriteString(f + "\n")
		}
		res.Stdout = sbOut.String()
		return
	}

	// Handling git status
	if trimmed == "git status" {
		created, modified, deleted, _ := sb.fs.GetDelta()
		var out strings.Builder
		out.WriteString("On branch main\nChanges not staged for commit:\n")
		for _, m := range modified {
			out.WriteString(fmt.Sprintf("  modified: %s\n", m))
		}
		for _, d := range deleted {
			out.WriteString(fmt.Sprintf("  deleted:  %s\n", d))
		}
		if len(created) > 0 {
			out.WriteString("Untracked files:\n")
			for _, c := range created {
				out.WriteString(fmt.Sprintf("  %s\n", c))
			}
		}
		res.Stdout = out.String()
		return
	}

	// Default simulated execution output
	res.Stdout = fmt.Sprintf("[sandbox: %s] command executed successfully\n", sb.config.SandboxID)
}

func (sb *Sandbox) resolvePath(p string) string {
	clean := strings.TrimSpace(p)
	if strings.HasPrefix(clean, "/") {
		return clean
	}
	return filepath.Join(sb.config.WorkDir, clean)
}

// CreateCheckpoint creates an immutable filesystem checkpoint.
func (sb *Sandbox) CreateCheckpoint(checkpointID string) *FSCheckpoint {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.fs.CreateCheckpoint(checkpointID)
}

// RollbackToCheckpoint resets the filesystem state to the specified checkpoint.
func (sb *Sandbox) RollbackToCheckpoint(cp *FSCheckpoint) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	sb.fs.RollbackToCheckpoint(cp)

	created, modified, deleted, _ := sb.fs.GetDelta()
	sb.stats.FilesCreated = len(created)
	sb.stats.FilesModified = len(modified)
	sb.stats.FilesDeleted = len(deleted)
}

// Terminate stops the sandbox container and generates the final audit report.
func (sb *Sandbox) Terminate() *SandboxAuditReport {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	if sb.state == StateRunning {
		sb.state = StateTerminated
	}
	sb.endedAt = time.Now().UTC()
	return sb.generateReportLocked()
}

// Audit generates a compliance report without modifying sandbox state.
func (sb *Sandbox) Audit() *SandboxAuditReport {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.generateReportLocked()
}

func (sb *Sandbox) generateReportLocked() *SandboxAuditReport {
	criticalCount := 0
	for _, v := range sb.violations {
		if v.Severity == SeverityCritical {
			criticalCount++
		}
	}

	zeroEscapeRisk := (criticalCount == 0 && sb.state != StateKilledOOM)
	compliantSLA := (sb.stats.TotalCPUTimeMs <= int64(sb.stats.CommandsExecuted)*sb.config.MaxCPUTimeMs)

	score := 100.0 - (float64(criticalCount) * 35.0) - (float64(len(sb.violations)-criticalCount) * 10.0)
	if score < 0.0 {
		score = 0.0
	}

	return &SandboxAuditReport{
		SandboxID:        sb.config.SandboxID,
		State:            sb.state,
		StartedAt:        sb.startedAt,
		TerminatedAt:     sb.endedAt,
		Stats:            sb.stats,
		ExecHistory:      sb.execHistory,
		ZeroEscapeRisk:   zeroEscapeRisk,
		CompliantWithSLA: compliantSLA,
		SecurityScore:    score,
	}
}

func tokenizeCmd(cmdLine string) []string {
	parts := strings.Fields(cmdLine)
	if len(parts) == 0 {
		return []string{""}
	}
	return parts
}
