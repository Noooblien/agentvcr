package tests

import (
	"strings"
	"testing"

	"github.com/Noooblien/agentvcr/pkg/sandbox"
)

func TestSandbox_OverlayFS_CoWAndRollback(t *testing.T) {
	baseFiles := map[string][]byte{
		"/workspace/main.go":  []byte("package main\n\nfunc main() {}\n"),
		"/workspace/README.md": []byte("# Agent Workspace\n"),
	}

	fs := sandbox.NewOverlayFS(baseFiles)

	// 1. Read baseline files
	mainData, err := fs.ReadFile("/workspace/main.go")
	if err != nil || !strings.Contains(string(mainData), "package main") {
		t.Fatalf("failed to read baseline main.go: %v", err)
	}

	// 2. Create checkpoint before mutations
	cp1 := fs.CreateCheckpoint("checkpoint_base")
	if len(cp1.Files) != 2 {
		t.Fatalf("expected 2 files in checkpoint, got %d", len(cp1.Files))
	}

	// 3. Mutate main.go, add feature.go, delete README.md
	_ = fs.WriteFile("/workspace/main.go", []byte("package main\n\nfunc main() { println(\"v2\") }\n"))
	_ = fs.WriteFile("/workspace/feature.go", []byte("package main\n\nfunc Feature() {}\n"))
	_ = fs.RemoveFile("/workspace/README.md")

	created, modified, deleted, totalWritten := fs.GetDelta()
	if len(created) != 1 || created[0] != "/workspace/feature.go" {
		t.Fatalf("expected 1 created file, got %v", created)
	}
	if len(modified) != 1 || modified[0] != "/workspace/main.go" {
		t.Fatalf("expected 1 modified file, got %v", modified)
	}
	if len(deleted) != 1 || deleted[0] != "/workspace/README.md" {
		t.Fatalf("expected 1 deleted file, got %v", deleted)
	}
	if totalWritten == 0 {
		t.Fatalf("expected non-zero total written bytes")
	}

	// Verify README is tombstoned
	_, err = fs.ReadFile("/workspace/README.md")
	if err == nil {
		t.Fatalf("expected error reading whiteouted file")
	}

	// 4. Rollback to checkpoint
	fs.RollbackToCheckpoint(cp1)

	// Verify state restored
	restoredReadme, err := fs.ReadFile("/workspace/README.md")
	if err != nil || string(restoredReadme) != "# Agent Workspace\n" {
		t.Fatalf("rollback failed to restore README.md")
	}

	_, err = fs.ReadFile("/workspace/feature.go")
	if err == nil {
		t.Fatalf("rollback failed: feature.go should no longer exist")
	}

	restoredMain, _ := fs.ReadFile("/workspace/main.go")
	if string(restoredMain) != "package main\n\nfunc main() {}\n" {
		t.Fatalf("rollback failed to restore original main.go content")
	}
}

func TestSandbox_CommandExecution(t *testing.T) {
	cfg := sandbox.DefaultSandboxConfig("sbx-test-001")
	baseFiles := map[string][]byte{
		"/workspace/app.py": []byte("print('hello world')\n"),
	}

	sb := sandbox.NewSandbox(cfg, baseFiles)

	// 1. Write file via echo
	res1, err := sb.Execute("echo 'version = 2.0' > /workspace/version.txt", nil)
	if err != nil || res1.ExitCode != 0 {
		t.Fatalf("file write execution failed: %v", err)
	}

	// 2. Read file via cat
	res2, err := sb.Execute("cat /workspace/version.txt", nil)
	if err != nil || res2.ExitCode != 0 {
		t.Fatalf("cat execution failed: %v", err)
	}
	if !strings.Contains(res2.Stdout, "version = 2.0") {
		t.Fatalf("unexpected cat output: %s", res2.Stdout)
	}

	// 3. Git status
	res3, err := sb.Execute("git status", nil)
	if err != nil || res3.ExitCode != 0 {
		t.Fatalf("git status failed: %v", err)
	}
	if !strings.Contains(res3.Stdout, "Untracked files") || !strings.Contains(res3.Stdout, "/workspace/version.txt") {
		t.Fatalf("git status missing untracked file: %s", res3.Stdout)
	}

	// 4. List files
	res4, err := sb.Execute("ls", nil)
	if err != nil || res4.ExitCode != 0 {
		t.Fatalf("ls failed: %v", err)
	}
	if !strings.Contains(res4.Stdout, "/workspace/app.py") || !strings.Contains(res4.Stdout, "/workspace/version.txt") {
		t.Fatalf("ls missing files: %s", res4.Stdout)
	}
}

func TestSandbox_SecurityGuard_BlockedOperations(t *testing.T) {
	cfg := sandbox.DefaultSandboxConfig("sbx-sec-002")
	sb := sandbox.NewSandbox(cfg, nil)

	dangerousCommands := []struct {
		cmd          string
		expectedType sandbox.ViolationType
	}{
		{
			cmd:          "curl -s http://169.254.169.254/latest/meta-data/iam/security-credentials/",
			expectedType: sandbox.ViolationCloudMetadataSSRF,
		},
		{
			cmd:          "cat /proc/kcore",
			expectedType: sandbox.ViolationKernelEscape,
		},
		{
			cmd:          "nsenter --target 1 --mount /bin/bash",
			expectedType: sandbox.ViolationKernelEscape,
		},
		{
			cmd:          "rm -rf /",
			expectedType: sandbox.ViolationDestructiveCommand,
		},
		{
			cmd:          ":(){ :|:& };:",
			expectedType: sandbox.ViolationDestructiveCommand,
		},
		{
			cmd:          "cat ~/.ssh/id_rsa",
			expectedType: sandbox.ViolationCredentialTheft,
		},
	}

	for _, tc := range dangerousCommands {
		res, err := sb.Execute(tc.cmd, nil)
		if err != nil {
			t.Fatalf("execute error: %v", err)
		}
		if res.ExitCode != 126 {
			t.Fatalf("expected blocked exit code 126 for '%s', got %d", tc.cmd, res.ExitCode)
		}
		if len(res.Violations) == 0 {
			t.Fatalf("expected security violation for '%s'", tc.cmd)
		}
		if res.Violations[0].Type != tc.expectedType {
			t.Fatalf("expected violation %s, got %s", tc.expectedType, res.Violations[0].Type)
		}
	}

	report := sb.Terminate()
	if report.ZeroEscapeRisk {
		t.Fatalf("expected ZeroEscapeRisk to be false due to blocked escape attempts")
	}
	if report.SecurityScore >= 100.0 {
		t.Fatalf("expected security score penalty for violations, got %f", report.SecurityScore)
	}
}

func TestSandbox_AuditScorecardRendering(t *testing.T) {
	cfg := sandbox.DefaultSandboxConfig("sbx-clean-003")
	sb := sandbox.NewSandbox(cfg, map[string][]byte{
		"/workspace/service.go": []byte("package service\n"),
	})

	_, _ = sb.Execute("echo 'ready' > /workspace/status.txt", nil)
	report := sb.Terminate()

	if !report.ZeroEscapeRisk {
		t.Fatalf("clean sandbox should have zero escape risk")
	}
	if !report.CompliantWithSLA {
		t.Fatalf("clean sandbox should be compliant with SLA")
	}
	if report.SecurityScore != 100.0 {
		t.Fatalf("expected 100.0 score, got %f", report.SecurityScore)
	}

	scorecard := sandbox.RenderSandboxScorecard(report)
	if !strings.Contains(scorecard, "AGENTVCR // DETERMINISTIC SANDBOX EXECUTION HYPERVISOR") {
		t.Fatalf("scorecard missing header")
	}
	if !strings.Contains(scorecard, "[PASS] Zero privilege escalation") {
		t.Fatalf("scorecard missing pass badge")
	}
}
