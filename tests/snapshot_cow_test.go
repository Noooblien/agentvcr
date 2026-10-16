package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Noooblien/agentvcr/pkg/snapshot"
)

func TestCoW_ReadWriteAndLayerIsolation(t *testing.T) {
	tempDir := t.TempDir()
	baseDir := filepath.Join(tempDir, "base_workspace")
	workDir := filepath.Join(tempDir, "work_storage")

	if err := os.MkdirAll(baseDir, 0755); err != nil {
		t.Fatalf("failed to create base dir: %v", err)
	}

	// Create initial file in base
	initialContent := []byte("package main\n\nfunc main() {}\n")
	if err := os.WriteFile(filepath.Join(baseDir, "main.go"), initialContent, 0644); err != nil {
		t.Fatalf("failed to write base file: %v", err)
	}

	cow, err := snapshot.NewCoWFilesystem("session-cow-1", baseDir, workDir)
	if err != nil {
		t.Fatalf("failed to initialize CoW filesystem: %v", err)
	}
	defer cow.Cleanup()

	// 1. Read base file through CoW
	data, err := cow.ReadFile("main.go")
	if err != nil {
		t.Fatalf("expected to read base main.go: %v", err)
	}
	if string(data) != string(initialContent) {
		t.Errorf("content mismatch: %s vs %s", string(data), string(initialContent))
	}

	// 2. Modify existing file and create new file in upper layer
	modifiedContent := []byte("package main\n\nfunc main() { println(\"modified\") }\n")
	if err := cow.WriteFile("main.go", modifiedContent, 0644); err != nil {
		t.Fatalf("failed to write main.go to CoW: %v", err)
	}
	newContent := []byte("{\"port\": 8080}")
	if err := cow.WriteFile("config.json", newContent, 0644); err != nil {
		t.Fatalf("failed to write config.json to CoW: %v", err)
	}

	// 3. Verify CoW returns modified content
	readMod, err := cow.ReadFile("main.go")
	if err != nil {
		t.Fatalf("failed to read modified main.go: %v", err)
	}
	if string(readMod) != string(modifiedContent) {
		t.Errorf("expected modified content from CoW, got: %s", string(readMod))
	}

	// 4. Verify base directory on disk was completely untouched (Copy-on-Write)
	diskBaseData, err := os.ReadFile(filepath.Join(baseDir, "main.go"))
	if err != nil {
		t.Fatalf("failed to read disk base file: %v", err)
	}
	if string(diskBaseData) != string(initialContent) {
		t.Errorf("base directory on disk was mutated! Expected %s, got %s", string(initialContent), string(diskBaseData))
	}
}

func TestCoW_DeletionAndWhiteoutTombs(t *testing.T) {
	tempDir := t.TempDir()
	baseDir := filepath.Join(tempDir, "base_workspace")
	workDir := filepath.Join(tempDir, "work_storage")

	_ = os.MkdirAll(baseDir, 0755)
	_ = os.WriteFile(filepath.Join(baseDir, "legacy.txt"), []byte("obsolete"), 0644)

	cow, err := snapshot.NewCoWFilesystem("session-cow-del", baseDir, workDir)
	if err != nil {
		t.Fatalf("failed to initialize CoW: %v", err)
	}
	defer cow.Cleanup()

	// Verify file exists initially
	_, err = cow.ReadFile("legacy.txt")
	if err != nil {
		t.Fatalf("expected legacy.txt to exist")
	}

	// Delete file
	if err := cow.DeleteFile("legacy.txt"); err != nil {
		t.Fatalf("failed to delete file: %v", err)
	}

	// Read should return ErrNotExist
	_, err = cow.ReadFile("legacy.txt")
	if !os.IsNotExist(err) {
		t.Errorf("expected os.ErrNotExist, got: %v", err)
	}

	// Verify OCI whiteout marker exists in upper layer on disk
	whiteoutPath := filepath.Join(cow.ActiveLayer.RootDir, snapshot.WhiteoutPrefix+"legacy.txt")
	if _, statErr := os.Stat(whiteoutPath); statErr != nil {
		t.Errorf("expected OCI whiteout tombstone at %s, got: %v", whiteoutPath, statErr)
	}

	// Verify Diff records deletion
	diff, err := cow.Diff()
	if err != nil {
		t.Fatalf("diff error: %v", err)
	}
	foundDeleted := false
	for _, ch := range diff.Changes {
		if ch.Path == "legacy.txt" && ch.Action == "deleted" {
			foundDeleted = true
			break
		}
	}
	if !foundDeleted {
		t.Errorf("expected diff to contain legacy.txt marked as deleted, got: %+v", diff.Changes)
	}
}

func TestCoW_CommitLayerAndRollback(t *testing.T) {
	tempDir := t.TempDir()
	baseDir := filepath.Join(tempDir, "base_workspace")
	workDir := filepath.Join(tempDir, "work_storage")

	_ = os.MkdirAll(baseDir, 0755)
	cow, err := snapshot.NewCoWFilesystem("session-cow-commit", baseDir, workDir)
	if err != nil {
		t.Fatalf("failed to initialize CoW: %v", err)
	}
	defer cow.Cleanup()

	// Step 1: Write file and commit
	_ = cow.WriteFile("step1.txt", []byte("step 1 output"), 0644)
	layer1, err := cow.CommitLayer("completed step 1 build")
	if err != nil {
		t.Fatalf("commit error: %v", err)
	}
	if layer1.Digest == "" {
		t.Errorf("expected non-empty layer digest")
	}

	// Step 2: Write temporary mutation
	_ = cow.WriteFile("step2_temp.txt", []byte("ephemeral"), 0644)
	data, _ := cow.ReadFile("step2_temp.txt")
	if string(data) != "ephemeral" {
		t.Errorf("expected to read step2_temp.txt")
	}

	// Rollback to layer1 checkpoint
	if err := cow.RollbackTo(layer1.ID); err != nil {
		t.Fatalf("rollback error: %v", err)
	}

	// Verify step2_temp is gone
	_, err = cow.ReadFile("step2_temp.txt")
	if !os.IsNotExist(err) {
		t.Errorf("expected step2_temp.txt to be removed after rollback")
	}

	// Verify step1.txt is still intact
	step1Data, err := cow.ReadFile("step1.txt")
	if err != nil || string(step1Data) != "step 1 output" {
		t.Errorf("expected step1.txt to persist after rollback")
	}
}

func TestCoW_InstantBranchingAndDockerMount(t *testing.T) {
	tempDir := t.TempDir()
	baseDir := filepath.Join(tempDir, "base_workspace")
	workDir := filepath.Join(tempDir, "work_storage")

	_ = os.MkdirAll(baseDir, 0755)
	_ = os.WriteFile(filepath.Join(baseDir, "model.go"), []byte("type Model struct{}"), 0644)

	parentCoW, err := snapshot.NewCoWFilesystem("session-parent", baseDir, workDir)
	if err != nil {
		t.Fatalf("failed to initialize CoW: %v", err)
	}
	defer parentCoW.Cleanup()

	// Commit step 1 in parent
	_ = parentCoW.WriteFile("utils.go", []byte("package main"), 0644)
	_, _ = parentCoW.CommitLayer("add utils.go")

	// Branch child for speculative trajectory
	childBranch, err := parentCoW.Branch("speculate-branch-a")
	if err != nil {
		t.Fatalf("failed to branch: %v", err)
	}

	// Mutate only child branch
	_ = childBranch.WriteFile("speculate.txt", []byte("speculative thought"), 0644)

	// Child should see speculate.txt, parent should NOT
	_, childErr := childBranch.ReadFile("speculate.txt")
	if childErr != nil {
		t.Errorf("child branch should see speculate.txt")
	}
	_, parentErr := parentCoW.ReadFile("speculate.txt")
	if !os.IsNotExist(parentErr) {
		t.Errorf("parent stack should not see speculate.txt from child branch")
	}

	// Test Docker/Overlay mount command string generation
	mountCmd := childBranch.GenerateDockerMountArgs("/mnt/agent-workspace")
	if !strings.Contains(mountCmd, "mount -t overlay overlay -o lowerdir=") {
		t.Errorf("invalid overlay mount command: %s", mountCmd)
	}
	if !strings.Contains(mountCmd, "/mnt/agent-workspace") {
		t.Errorf("missing target mount path in: %s", mountCmd)
	}
}
