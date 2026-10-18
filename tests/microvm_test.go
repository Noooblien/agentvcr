package tests

import (
	"strings"
	"testing"

	"github.com/Noooblien/agentvcr/pkg/microvm"
)

func TestMicroVM_SpawnAndNominalExecution(t *testing.T) {
	hyp := microvm.NewHypervisor()

	cfg := microvm.MicroVMConfig{
		VMID:        "vm-test-nominal",
		VCPUCount:   2,
		MemoryMB:    256,
		Policy:      microvm.DefaultJailPolicy(),
		ChrootPath:  "/tmp/firecracker-jail-nominal",
	}

	vm, err := hyp.SpawnVM(cfg)
	if err != nil {
		t.Fatalf("Failed to spawn microVM: %v", err)
	}

	if vm.State != microvm.VMStateRunning {
		t.Fatalf("Expected running state, got: %s", vm.State)
	}
	if vm.BootDurationMs <= 0 {
		t.Fatalf("Expected non-zero boot latency, got: %d", vm.BootDurationMs)
	}

	res, err := hyp.ExecuteInJail(vm.Config.VMID, "echo 'hello from microvm'")
	if err != nil {
		t.Fatalf("Execution failed: %v", err)
	}

	if res.SecurityBreach {
		t.Fatalf("Expected nominal command to pass without security breach")
	}
	if res.ExitCode != 0 {
		t.Fatalf("Expected exit code 0, got %d", res.ExitCode)
	}
	if !strings.Contains(res.Stdout, "hello from microvm") {
		t.Fatalf("Unexpected stdout: %s", res.Stdout)
	}

	// Verify auto snapshot on turn
	if len(vm.Snapshots) != 1 {
		t.Fatalf("Expected 1 auto snapshot, got %d", len(vm.Snapshots))
	}
}

func TestMicroVM_SeccompSyscallTrap(t *testing.T) {
	hyp := microvm.NewHypervisor()

	cfg := microvm.MicroVMConfig{
		VMID:   "vm-test-seccomp",
		Policy: microvm.DefaultJailPolicy(),
	}

	vm, err := hyp.SpawnVM(cfg)
	if err != nil {
		t.Fatalf("Spawn error: %v", err)
	}

	// Attempt blocked syscall (reboot or ptrace)
	res, err := hyp.ExecuteInJail(vm.Config.VMID, "reboot -f")
	if err != nil {
		t.Fatalf("Expected containment trap rather than go error: %v", err)
	}

	if !res.SecurityBreach {
		t.Fatalf("Expected security breach flag to be TRUE")
	}
	if res.ViolationType != "PROHIBITED_SYSCALL" {
		t.Fatalf("Expected PROHIBITED_SYSCALL violation, got: %s", res.ViolationType)
	}
	if vm.State != microvm.VMStateBreached {
		t.Fatalf("Expected VM state to be BREACHED, got: %s", vm.State)
	}
}

func TestMicroVM_ChrootPathEscapeTrap(t *testing.T) {
	hyp := microvm.NewHypervisor()

	cfg := microvm.MicroVMConfig{
		VMID:   "vm-test-chroot",
		Policy: microvm.DefaultJailPolicy(),
	}

	vm, err := hyp.SpawnVM(cfg)
	if err != nil {
		t.Fatalf("Spawn error: %v", err)
	}

	// Attempt access to host /etc/shadow
	res, err := hyp.ExecuteInJail(vm.Config.VMID, "cat /etc/shadow")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !res.SecurityBreach {
		t.Fatalf("Expected path access breach")
	}
	if res.ViolationType != "RESTRICTED_PATH_ACCESS" {
		t.Fatalf("Expected RESTRICTED_PATH_ACCESS, got: %s", res.ViolationType)
	}
}

func TestMicroVM_SnapshotAndRollbackRestore(t *testing.T) {
	hyp := microvm.NewHypervisor()

	cfg := microvm.MicroVMConfig{
		VMID:   "vm-test-restore",
		Policy: microvm.DefaultJailPolicy(),
	}

	vm, _ := hyp.SpawnVM(cfg)

	// Step 1: Nominal
	_, _ = hyp.ExecuteInJail(vm.Config.VMID, "install_package python3")
	snap, err := hyp.CreateSnapshot(vm.Config.VMID)
	if err != nil {
		t.Fatalf("Failed to create snapshot: %v", err)
	}

	if snap.SnapshotID == "" || snap.MemoryHash == "" {
		t.Fatalf("Invalid snapshot: %+v", snap)
	}

	// Step 2: Faulty command
	_, _ = hyp.ExecuteInJail(vm.Config.VMID, "corrupt_data")

	// Step 3: Rollback restore
	restoredVM, err := hyp.RestoreSnapshot(vm.Config.VMID, snap.SnapshotID)
	if err != nil {
		t.Fatalf("Failed to restore snapshot: %v", err)
	}

	if restoredVM.TotalRestores != 1 {
		t.Fatalf("Expected 1 total restore, got %d", restoredVM.TotalRestores)
	}
	if restoredVM.CurrentTurn != snap.TurnIndex {
		t.Fatalf("Expected turn %d, got %d", snap.TurnIndex, restoredVM.CurrentTurn)
	}
	if restoredVM.State != microvm.VMStateRunning {
		t.Fatalf("Expected running state post-restore, got: %s", restoredVM.State)
	}
}

func TestMicroVM_ScorecardAndMermaid(t *testing.T) {
	hyp := microvm.NewHypervisor()

	cfg := microvm.MicroVMConfig{
		VMID:   "vm-test-render",
		Policy: microvm.DefaultJailPolicy(),
	}

	vm, _ := hyp.SpawnVM(cfg)

	res1, _ := hyp.ExecuteInJail(vm.Config.VMID, "echo 'testing'")
	res2, _ := hyp.ExecuteInJail(vm.Config.VMID, "mount -t ext4 /dev/sdb /mnt") // Prohibited syscall

	report := hyp.GenerateReport(vm.Config.VMID, []microvm.ExecutionResult{*res1, *res2})

	scorecard := microvm.RenderMicroVMScorecard(report)
	if !strings.Contains(scorecard, "AGENTVCR // EPHEMERAL FIRECRACKER MICROVM JAIL") {
		t.Fatalf("Scorecard missing header: %s", scorecard)
	}
	if !strings.Contains(scorecard, "ISOLATION VIOLATIONS INTERCEPTED") {
		t.Fatalf("Scorecard missing isolation violations section: %s", scorecard)
	}

	mermaid := microvm.RenderMicroVMMermaid(report)
	if !strings.Contains(mermaid, "graph TD") || !strings.Contains(mermaid, "Firecracker MicroVM Jail") {
		t.Fatalf("Mermaid missing expected structure: %s", mermaid)
	}
}
