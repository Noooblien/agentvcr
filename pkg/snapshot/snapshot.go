package snapshot

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// FileEntry represents a file or directory captured in a workspace snapshot.
type FileEntry struct {
	Path     string      `json:"path"`
	Size     int64       `json:"size"`
	Mode     os.FileMode `json:"mode"`
	ModTime  time.Time   `json:"mod_time"`
	IsDir    bool        `json:"is_dir"`
	Checksum string      `json:"checksum,omitempty"`
}

// Snapshot represents a point-in-time state of an agent's workspace.
type Snapshot struct {
	ID        string               `json:"id"`
	SessionID string               `json:"session_id"`
	StepIndex int                  `json:"step_index"`
	CreatedAt time.Time            `json:"created_at"`
	RootPath  string               `json:"root_path"`
	Entries   map[string]FileEntry `json:"entries"`
	Archive   []byte               `json:"-"` // in-memory tar archive for deterministic restore
}

// FileChange describes modifications between two snapshot points.
type FileChange struct {
	Path   string `json:"path"`
	Action string `json:"action"` // "created", "modified", "deleted"
}

// SnapshotDiff records the delta between two filesystem states.
type SnapshotDiff struct {
	FromSnapshotID string       `json:"from_snapshot_id"`
	ToSnapshotID   string       `json:"to_snapshot_id"`
	Changes        []FileChange `json:"changes"`
}

// Manager orchestrates workspace checkpointing and rollbacks.
type Manager struct {
	snapshots map[string]*Snapshot
}

// NewManager creates a snapshot manager.
func NewManager() *Manager {
	return &Manager{
		snapshots: make(map[string]*Snapshot),
	}
}

// CaptureWorkspace creates a point-in-time snapshot of the specified directory.
func (m *Manager) CaptureWorkspace(sessionID string, stepIndex int, rootDir string) (*Snapshot, error) {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, fmt.Errorf("invalid root directory: %w", err)
	}

	snapID := fmt.Sprintf("snap_%s_step%d_%d", sessionID, stepIndex, time.Now().UnixNano())
	snap := &Snapshot{
		ID:        snapID,
		SessionID: sessionID,
		StepIndex: stepIndex,
		CreatedAt: time.Now(),
		RootPath:  absRoot,
		Entries:   make(map[string]FileEntry),
	}

	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)

	err = filepath.Walk(absRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		relPath, err := filepath.Rel(absRoot, path)
		if err != nil {
			return err
		}

		// Skip root directory itself in relative index
		if relPath == "." {
			return nil
		}

		entry := FileEntry{
			Path:    relPath,
			Size:    info.Size(),
			Mode:    info.Mode(),
			ModTime: info.ModTime(),
			IsDir:   info.IsDir(),
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = relPath

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if !info.IsDir() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			h := sha256.Sum256(data)
			entry.Checksum = hex.EncodeToString(h[:])

			if _, err := tw.Write(data); err != nil {
				return err
			}
		}

		snap.Entries[relPath] = entry
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to traverse directory %s: %w", absRoot, err)
	}

	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("failed to finalize snapshot tar: %w", err)
	}

	snap.Archive = tarBuf.Bytes()
	m.snapshots[snapID] = snap
	return snap, nil
}

// RestoreWorkspace reverts the destination directory to match the captured snapshot state.
func (m *Manager) RestoreWorkspace(snapshotID, destDir string) error {
	snap, ok := m.snapshots[snapshotID]
	if !ok {
		return fmt.Errorf("snapshot %s not found", snapshotID)
	}

	absDest, err := filepath.Abs(destDir)
	if err != nil {
		return fmt.Errorf("invalid destination directory: %w", err)
	}

	// Clean out destination directory
	if err := os.RemoveAll(absDest); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to clear destination directory: %w", err)
	}
	if err := os.MkdirAll(absDest, 0755); err != nil {
		return fmt.Errorf("failed to recreate destination directory: %w", err)
	}

	// Extract tar archive
	tr := tar.NewReader(bytes.NewReader(snap.Archive))
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("corrupt snapshot archive: %w", err)
		}

		targetPath := filepath.Join(absDest, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return err
			}
			outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, header.FileInfo().Mode())
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
		}
	}

	return nil
}

// Compare computes the differential change between two snapshots.
func Compare(snapA, snapB *Snapshot) SnapshotDiff {
	diff := SnapshotDiff{
		FromSnapshotID: snapA.ID,
		ToSnapshotID:   snapB.ID,
		Changes:        make([]FileChange, 0),
	}

	// Check entries in B against A (created or modified)
	for path, entryB := range snapB.Entries {
		entryA, exists := snapA.Entries[path]
		if !exists {
			diff.Changes = append(diff.Changes, FileChange{
				Path:   path,
				Action: "created",
			})
		} else if !entryB.IsDir && entryA.Checksum != entryB.Checksum {
			diff.Changes = append(diff.Changes, FileChange{
				Path:   path,
				Action: "modified",
			})
		}
	}

	// Check entries in A not in B (deleted)
	for path := range snapA.Entries {
		if _, exists := snapB.Entries[path]; !exists {
			diff.Changes = append(diff.Changes, FileChange{
				Path:   path,
				Action: "deleted",
			})
		}
	}

	return diff
}
