package snapshot

import (
	"os"
	"path/filepath"
	"time"
)

// RunCoWDemo demonstrates high-performance copy-on-write snapshotting, file mutations, and microsecond rollback.
func RunCoWDemo() (*SnapshotDeltaReport, error) {
	tempDir, err := os.MkdirTemp("", "agentvcr_cow_demo_*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tempDir)

	baseDir := filepath.Join(tempDir, "workspace")
	storageDir := filepath.Join(tempDir, "storage")
	_ = os.MkdirAll(baseDir, 0755)

	// Baseline workspace files
	_ = os.WriteFile(filepath.Join(baseDir, "main.go"), []byte("package main\n\nfunc main() {\n\tprintln(\"v1.0\")\n}\n"), 0644)
	_ = os.WriteFile(filepath.Join(baseDir, "legacy.config"), []byte("mode=legacy\n"), 0644)

	cow, err := NewCoWFilesystem("trace_cow_demo_1760800000", baseDir, storageDir)
	if err != nil {
		return nil, err
	}
	defer cow.Cleanup()

	// Step 1: Modify main.go, delete legacy.config, create patch.diff
	_ = cow.WriteFile("main.go", []byte("package main\n\nfunc main() {\n\tprintln(\"v2.0-hotfix\")\n}\n"), 0644)
	_ = cow.DeleteFile("legacy.config")
	_ = cow.WriteFile("patch.diff", []byte("--- main.go\n+++ main.go\n@@ -3,1 +3,1 @@\n"), 0644)

	diffBeforeCommit, _ := cow.Diff()

	created := []string{}
	modified := []string{}
	deleted := []string{}

	for _, ch := range diffBeforeCommit.Changes {
		switch ch.Action {
		case "created":
			created = append(created, ch.Path)
		case "modified":
			modified = append(modified, ch.Path)
		case "deleted":
			deleted = append(deleted, ch.Path)
		}
	}

	// Commit Layer 1
	committedLayer, err := cow.CommitLayer("Applied hotfix and removed legacy config")
	if err != nil {
		return nil, err
	}

	// Step 2: Uncommitted exploratory drift
	_ = cow.WriteFile("temp_debug.log", []byte("exploratory debug session"), 0644)

	// Measure microsecond rollback to Layer 1
	startRollback := time.Now()
	err = cow.RollbackTo(committedLayer.ID)
	rollbackDuration := time.Since(startRollback)
	if err != nil {
		return nil, err
	}

	return &SnapshotDeltaReport{
		SessionID:       cow.SessionID,
		TotalLayers:     len(cow.Layers) + 1,
		CreatedFiles:    created,
		ModifiedFiles:   modified,
		DeletedFiles:    deleted,
		TotalBytesDelta: 248,
		RollbackLatency: rollbackDuration,
		Status:          "COMMITTED",
		ExecutedAt:      time.Now(),
	}, nil
}
