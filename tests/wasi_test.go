package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
	"github.com/Noooblien/agentvcr/pkg/wasi"
)

func TestWASI_ValidModuleExecution_Success(t *testing.T) {
	cfg := wasi.DefaultSandboxPolicy()
	runtime := wasi.NewSandboxRuntime(cfg)

	// Valid WASM binary magic header \x00asm
	validWasm := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	res := runtime.ExecuteGuestBytecode("session_wasi_01", validWasm, "hello from agent test")

	if res.State != wasi.StateTerminated {
		t.Errorf("expected StateTerminated, got: %s", res.State)
	}
	if res.ExitCode != 0 {
		t.Errorf("expected exit code 0, got: %d", res.ExitCode)
	}
	if res.TotalFuelConsumed == 0 {
		t.Errorf("expected instruction fuel to be metered")
	}
	if res.SandboxViolations != 0 {
		t.Errorf("expected 0 sandbox violations, got: %d", res.SandboxViolations)
	}
	if len(res.SyscallTrace) == 0 {
		t.Errorf("expected non-empty syscall trace")
	}
}

func TestWASI_InvalidMagicHeader_Trapped(t *testing.T) {
	cfg := wasi.DefaultSandboxPolicy()
	runtime := wasi.NewSandboxRuntime(cfg)

	// Malformed binary header (not \x00asm)
	invalidBytecode := []byte("INVALID_WASM_BINARY_CODE")
	res := runtime.ExecuteGuestBytecode("session_wasi_trap", invalidBytecode, "")

	if res.State != wasi.StateTrapped {
		t.Errorf("expected StateTrapped, got: %s", res.State)
	}
	if res.ExitCode != -1 {
		t.Errorf("expected exit code -1, got: %d", res.ExitCode)
	}
	if !strings.Contains(res.TrapReason, "magic bytes") {
		t.Errorf("expected magic bytes trap reason, got: %s", res.TrapReason)
	}
}

func TestWASI_BlockedSyscallViolationDetected(t *testing.T) {
	cfg := wasi.DefaultSandboxPolicy()
	// Deliberately block fd_read
	cfg.BlockedSyscalls = append(cfg.BlockedSyscalls, "fd_read")
	runtime := wasi.NewSandboxRuntime(cfg)

	validWasm := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	res := runtime.ExecuteGuestBytecode("session_wasi_blocked", validWasm, "data")

	if res.SandboxViolations == 0 {
		t.Errorf("expected sandbox violations for blocked fd_read")
	}
	if res.ExitCode == 0 {
		t.Errorf("expected non-zero exit code for security policy breach")
	}
}

func TestWASI_IsolateTrajectorySessionAndScorecard(t *testing.T) {
	session := &trace.Session{
		ID:        "session_wasi_trajectory_test",
		AgentName: "wasi-isolated-agent",
		CreatedAt: time.Now().UTC(),
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventToolCall, Payload: map[string]interface{}{"tool": "compile_rust"}},
		},
	}

	cfg := wasi.DefaultSandboxPolicy()
	runtime := wasi.NewSandboxRuntime(cfg)
	report, err := runtime.IsolateTrajectorySession(session)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !report.IsolationCompliant || !report.ZeroHostEscapeRisk {
		t.Errorf("expected compliant isolation with zero host escape risk")
	}

	card := wasi.RenderWASIScorecard(report)
	if !strings.Contains(card, "WEBASSEMBLY (WASI) ISOLATION SANDBOX") {
		t.Errorf("expected title in scorecard")
	}
	if !strings.Contains(card, "session_wasi_trajectory_test") {
		t.Errorf("expected session ID in scorecard")
	}
	if !strings.Contains(card, "ZERO HOST ESCAPE RISK") {
		t.Errorf("expected zero host escape verdict in scorecard")
	}
}
