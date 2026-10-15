package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Noooblien/agentvcr/pkg/snapshot"
)

func TestSnapshot_CaptureAndRestore(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agentvcr_snap_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	srcDir := filepath.Join(tempDir, "workspace")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("failed to create src dir: %v", err)
	}

	file1 := filepath.Join(srcDir, "main.go")
	if err := os.WriteFile(file1, []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatalf("failed to write file1: %v", err)
	}

	mgr := snapshot.NewManager()
	snap1, err := mgr.CaptureWorkspace("session-test-1", 1, srcDir)
	if err != nil {
		t.Fatalf("failed to capture workspace: %v", err)
	}

	if len(snap1.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(snap1.Entries))
	}

	// Mutate workspace (modify file1, create file2)
	if err := os.WriteFile(file1, []byte("package main\n\nfunc mutated() {}\n"), 0644); err != nil {
		t.Fatalf("failed to mutate file1: %v", err)
	}
	file2 := filepath.Join(srcDir, "config.json")
	if err := os.WriteFile(file2, []byte(`{"debug":true}`), 0644); err != nil {
		t.Fatalf("failed to write file2: %v", err)
	}

	snap2, err := mgr.CaptureWorkspace("session-test-1", 2, srcDir)
	if err != nil {
		t.Fatalf("failed to capture snap2: %v", err)
	}

	diff := snapshot.Compare(snap1, snap2)
	if len(diff.Changes) != 2 {
		t.Fatalf("expected 2 changes (1 modified, 1 created), got %d: %+v", len(diff.Changes), diff.Changes)
	}

	// Restore back to snap1
	restoreDir := filepath.Join(tempDir, "restored")
	if err := mgr.RestoreWorkspace(snap1.ID, restoreDir); err != nil {
		t.Fatalf("failed to restore workspace: %v", err)
	}

	restoredContent, err := os.ReadFile(filepath.Join(restoreDir, "main.go"))
	if err != nil {
		t.Fatalf("failed to read restored file: %v", err)
	}
	if string(restoredContent) != "package main\n\nfunc main() {}\n" {
		t.Fatalf("restored content mismatch: got %s", string(restoredContent))
	}

	// Verify file2 does not exist in restored snapshot
	if _, err := os.Stat(filepath.Join(restoreDir, "config.json")); !os.IsNotExist(err) {
		t.Fatalf("expected config.json to not exist in restored snap1")
	}
}
