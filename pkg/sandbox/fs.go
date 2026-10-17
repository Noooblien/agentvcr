package sandbox

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// OverlayFS provides an in-memory Copy-on-Write (CoW) layered filesystem with instant rollback.
type OverlayFS struct {
	mu           sync.RWMutex
	baseFiles    map[string][]byte // Read-only lower layer
	upperFiles   map[string][]byte // Ephemeral write layer
	whiteouts    map[string]bool   // Deleted files tombstones
	totalWritten int64
}

// NewOverlayFS initializes a CoW filesystem with initial baseline files.
func NewOverlayFS(baseFiles map[string][]byte) *OverlayFS {
	copiedBase := make(map[string][]byte)
	for k, v := range baseFiles {
		cleanPath := normalizePath(k)
		buf := make([]byte, len(v))
		copy(buf, v)
		copiedBase[cleanPath] = buf
	}

	return &OverlayFS{
		baseFiles:  copiedBase,
		upperFiles: make(map[string][]byte),
		whiteouts:  make(map[string]bool),
	}
}

// ReadFile reads a file from upper layer (if present and not whiteouted) or base layer.
func (ofs *OverlayFS) ReadFile(path string) ([]byte, error) {
	ofs.mu.RLock()
	defer ofs.mu.RUnlock()

	clean := normalizePath(path)

	if ofs.whiteouts[clean] {
		return nil, fmt.Errorf("file not found (deleted): %s", path)
	}

	if data, ok := ofs.upperFiles[clean]; ok {
		res := make([]byte, len(data))
		copy(res, data)
		return res, nil
	}

	if data, ok := ofs.baseFiles[clean]; ok {
		res := make([]byte, len(data))
		copy(res, data)
		return res, nil
	}

	return nil, fmt.Errorf("file not found: %s", path)
}

// WriteFile writes a file to the ephemeral upper layer.
func (ofs *OverlayFS) WriteFile(path string, data []byte) error {
	ofs.mu.Lock()
	defer ofs.mu.Unlock()

	clean := normalizePath(path)
	buf := make([]byte, len(data))
	copy(buf, data)

	ofs.upperFiles[clean] = buf
	delete(ofs.whiteouts, clean)
	ofs.totalWritten += int64(len(data))
	return nil
}

// RemoveFile adds a whiteout tombstone in the upper layer.
func (ofs *OverlayFS) RemoveFile(path string) error {
	ofs.mu.Lock()
	defer ofs.mu.Unlock()

	clean := normalizePath(path)

	existsInUpper := false
	if _, ok := ofs.upperFiles[clean]; ok {
		existsInUpper = true
		delete(ofs.upperFiles, clean)
	}

	_, existsInBase := ofs.baseFiles[clean]
	if !existsInUpper && !existsInBase {
		return fmt.Errorf("cannot delete non-existent file: %s", path)
	}

	ofs.whiteouts[clean] = true
	return nil
}

// ListFiles returns all visible files sorted alphabetically.
func (ofs *OverlayFS) ListFiles() []string {
	ofs.mu.RLock()
	defer ofs.mu.RUnlock()

	seen := make(map[string]bool)

	for k := range ofs.baseFiles {
		if !ofs.whiteouts[k] {
			seen[k] = true
		}
	}

	for k := range ofs.upperFiles {
		if !ofs.whiteouts[k] {
			seen[k] = true
		}
	}

	var list []string
	for k := range seen {
		list = append(list, k)
	}
	sort.Strings(list)
	return list
}

// CreateCheckpoint creates an immutable snapshot of all current files and hashes.
func (ofs *OverlayFS) CreateCheckpoint(checkpointID string) *FSCheckpoint {
	ofs.mu.RLock()
	defer ofs.mu.RUnlock()

	cp := &FSCheckpoint{
		CheckpointID: checkpointID,
		CreatedAt:    time.Now().UTC(),
		Files:        make(map[string][]byte),
		FileHashes:   make(map[string]string),
	}

	for _, p := range ofs.ListFiles() {
		var content []byte
		if data, ok := ofs.upperFiles[p]; ok {
			content = make([]byte, len(data))
			copy(content, data)
		} else if data, ok := ofs.baseFiles[p]; ok {
			content = make([]byte, len(data))
			copy(content, data)
		}

		cp.Files[p] = content
		hash := sha256.Sum256(content)
		cp.FileHashes[p] = hex.EncodeToString(hash[:])
		cp.TotalBytes += int64(len(content))
	}

	return cp
}

// RollbackToCheckpoint resets the upper layer so filesystem matches the checkpoint exactly.
func (ofs *OverlayFS) RollbackToCheckpoint(cp *FSCheckpoint) {
	ofs.mu.Lock()
	defer ofs.mu.Unlock()

	ofs.upperFiles = make(map[string][]byte)
	ofs.whiteouts = make(map[string]bool)

	// Determine which base files were not in the checkpoint -> whiteout them
	for bp := range ofs.baseFiles {
		if _, ok := cp.Files[bp]; !ok {
			ofs.whiteouts[bp] = true
		}
	}

	// Restore files from checkpoint into upper layer if different from base
	for p, data := range cp.Files {
		baseData, inBase := ofs.baseFiles[p]
		if !inBase || string(baseData) != string(data) {
			buf := make([]byte, len(data))
			copy(buf, data)
			ofs.upperFiles[p] = buf
		}
	}
}

// GetDelta computes categorized file mutations compared to the base lower layer.
func (ofs *OverlayFS) GetDelta() (created, modified, deleted []string, totalBytesWritten int64) {
	ofs.mu.RLock()
	defer ofs.mu.RUnlock()

	for up, uData := range ofs.upperFiles {
		if bData, ok := ofs.baseFiles[up]; ok {
			if string(bData) != string(uData) {
				modified = append(modified, up)
			}
		} else {
			created = append(created, up)
		}
	}

	for wp := range ofs.whiteouts {
		if _, inBase := ofs.baseFiles[wp]; inBase {
			deleted = append(deleted, wp)
		}
	}

	sort.Strings(created)
	sort.Strings(modified)
	sort.Strings(deleted)
	return created, modified, deleted, ofs.totalWritten
}

func normalizePath(p string) string {
	clean := filepath.Clean(strings.TrimSpace(p))
	clean = strings.ReplaceAll(clean, "\\", "/")
	if !strings.HasPrefix(clean, "/") {
		clean = "/" + clean
	}
	return clean
}
