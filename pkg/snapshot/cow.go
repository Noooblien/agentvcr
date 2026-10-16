package snapshot

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// WhiteoutPrefix is the standard OCI/Docker container layer whiteout tombstone prefix.
const WhiteoutPrefix = ".wh."

// CoWLayer represents an immutable or writable layer within the layered filesystem stack.
type CoWLayer struct {
	ID          string    `json:"id"`
	ParentID    string    `json:"parent_id,omitempty"`
	RootDir     string    `json:"root_dir"`
	IsReadOnly  bool      `json:"is_read_only"`
	CreatedAt   time.Time `json:"created_at"`
	Digest      string    `json:"digest"`
	Description string    `json:"description,omitempty"`
}

// CoWFilesystem provides an ephemeral, copy-on-write layered filesystem driver
// enabling instantaneous O(1) workspace branching, delta commits, and rollbacks.
type CoWFilesystem struct {
	mu           sync.RWMutex
	BaseDir      string
	Layers       []*CoWLayer
	ActiveLayer  *CoWLayer
	Whiteouts    map[string]bool // normalized relative path -> true
	SessionID    string
	CurrentStep  int
	layerCounter int
}

// NewCoWFilesystem initializes a new copy-on-write workspace layered over a base directory.
func NewCoWFilesystem(sessionID, baseDir, workStorageDir string) (*CoWFilesystem, error) {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("invalid base directory: %w", err)
	}

	if workStorageDir == "" {
		workStorageDir = filepath.Join(os.TempDir(), "agentvcr-cow", sessionID)
	}

	if err := os.MkdirAll(workStorageDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create work storage: %w", err)
	}

	baseLayer := &CoWLayer{
		ID:          "layer_base_0",
		RootDir:     absBase,
		IsReadOnly:  true,
		CreatedAt:   time.Now(),
		Digest:      "base_immutable",
		Description: "Base image workspace",
	}

	upperDir := filepath.Join(workStorageDir, "upper_step_0")
	if err := os.MkdirAll(upperDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create initial upper layer: %w", err)
	}

	initialUpper := &CoWLayer{
		ID:          "layer_upper_0",
		ParentID:    baseLayer.ID,
		RootDir:     upperDir,
		IsReadOnly:  false,
		CreatedAt:   time.Now(),
		Description: "Active scratch delta",
	}

	return &CoWFilesystem{
		BaseDir:     workStorageDir,
		Layers:      []*CoWLayer{baseLayer},
		ActiveLayer: initialUpper,
		Whiteouts:   make(map[string]bool),
		SessionID:   sessionID,
		CurrentStep: 0,
	}, nil
}

// ReadFile resolves a relative path through the layer stack with whiteout checking.
func (c *CoWFilesystem) ReadFile(relPath string) ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	normPath := filepath.Clean(relPath)

	// Check if file is tombstoned
	if c.isTombstoned(normPath) {
		return nil, os.ErrNotExist
	}

	// 1. Check active upper layer
	upperPath := filepath.Join(c.ActiveLayer.RootDir, normPath)
	if data, err := os.ReadFile(upperPath); err == nil {
		return data, nil
	}

	// 2. Check intermediate and base layers in reverse order (newest to oldest)
	for i := len(c.Layers) - 1; i >= 0; i-- {
		layerPath := filepath.Join(c.Layers[i].RootDir, normPath)
		if data, err := os.ReadFile(layerPath); err == nil {
			return data, nil
		}
	}

	return nil, os.ErrNotExist
}

// WriteFile writes mutations exclusively to the active upper delta layer (Copy-on-Write).
func (c *CoWFilesystem) WriteFile(relPath string, data []byte, perm os.FileMode) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	normPath := filepath.Clean(relPath)
	upperPath := filepath.Join(c.ActiveLayer.RootDir, normPath)

	if err := os.MkdirAll(filepath.Dir(upperPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory in upper layer: %w", err)
	}

	if err := os.WriteFile(upperPath, data, perm); err != nil {
		return fmt.Errorf("failed to write to upper layer: %w", err)
	}

	// Clear any tombstone marker if the file is recreated
	delete(c.Whiteouts, normPath)
	whiteoutMarker := filepath.Join(filepath.Dir(upperPath), WhiteoutPrefix+filepath.Base(normPath))
	_ = os.Remove(whiteoutMarker)

	return nil
}

// DeleteFile marks a file as deleted, removing it from the upper layer and creating an OCI whiteout marker.
func (c *CoWFilesystem) DeleteFile(relPath string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	normPath := filepath.Clean(relPath)
	upperPath := filepath.Join(c.ActiveLayer.RootDir, normPath)

	// Remove from upper layer if present
	_ = os.Remove(upperPath)

	// Check if file exists in any lower layer
	existsInLower := false
	for _, layer := range c.Layers {
		lowerPath := filepath.Join(layer.RootDir, normPath)
		if _, err := os.Stat(lowerPath); err == nil {
			existsInLower = true
			break
		}
	}

	// If present in lower layer, create OCI whiteout marker tombstone
	if existsInLower {
		c.Whiteouts[normPath] = true
		upperDir := filepath.Dir(upperPath)
		_ = os.MkdirAll(upperDir, 0755)
		whiteoutPath := filepath.Join(upperDir, WhiteoutPrefix+filepath.Base(normPath))
		_ = os.WriteFile(whiteoutPath, []byte(""), 0644)
	}

	return nil
}

// isTombstoned checks if path is marked deleted in the active layer.
func (c *CoWFilesystem) isTombstoned(normPath string) bool {
	if c.Whiteouts[normPath] {
		return true
	}
	whiteoutPath := filepath.Join(c.ActiveLayer.RootDir, filepath.Dir(normPath), WhiteoutPrefix+filepath.Base(normPath))
	if _, err := os.Stat(whiteoutPath); err == nil {
		return true
	}
	return false
}

// CommitLayer freezes the current upper layer into an immutable layer and spawns a new upper layer.
func (c *CoWFilesystem) CommitLayer(description string) (*CoWLayer, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	digest, err := computeLayerDigest(c.ActiveLayer.RootDir)
	if err != nil {
		return nil, fmt.Errorf("failed to compute layer digest: %w", err)
	}

	c.ActiveLayer.IsReadOnly = true
	c.ActiveLayer.Digest = digest
	c.ActiveLayer.Description = description

	committed := c.ActiveLayer
	c.Layers = append(c.Layers, committed)

	c.CurrentStep++
	c.layerCounter++
	newUpperDir := filepath.Join(c.BaseDir, fmt.Sprintf("upper_step_%d_%d", c.CurrentStep, c.layerCounter))
	if err := os.MkdirAll(newUpperDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to allocate new upper layer: %w", err)
	}

	c.ActiveLayer = &CoWLayer{
		ID:          fmt.Sprintf("layer_upper_%d", c.CurrentStep),
		ParentID:    committed.ID,
		RootDir:     newUpperDir,
		IsReadOnly:  false,
		CreatedAt:   time.Now(),
		Description: fmt.Sprintf("Scratch layer at step %d", c.CurrentStep),
	}

	// Reset whiteouts for new upper layer
	c.Whiteouts = make(map[string]bool)

	return committed, nil
}

// RollbackTo restores the filesystem to a previous layer checkpoint, discarding uncommitted deltas in microseconds.
func (c *CoWFilesystem) RollbackTo(layerID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	targetIdx := -1
	for i, l := range c.Layers {
		if l.ID == layerID {
			targetIdx = i
			break
		}
	}

	if targetIdx == -1 {
		return fmt.Errorf("checkpoint layer %s not found in stack", layerID)
	}

	// Clean out current active layer
	_ = os.RemoveAll(c.ActiveLayer.RootDir)

	// Slice layers to target checkpoint + 1
	c.Layers = c.Layers[:targetIdx+1]

	c.layerCounter++
	newUpperDir := filepath.Join(c.BaseDir, fmt.Sprintf("upper_rollback_%s_%d", layerID, c.layerCounter))
	if err := os.MkdirAll(newUpperDir, 0755); err != nil {
		return fmt.Errorf("failed to create rollback upper layer: %w", err)
	}

	c.ActiveLayer = &CoWLayer{
		ID:          fmt.Sprintf("layer_upper_rb_%d", c.layerCounter),
		ParentID:    layerID,
		RootDir:     newUpperDir,
		IsReadOnly:  false,
		CreatedAt:   time.Now(),
		Description: fmt.Sprintf("Rollback scratch from %s", layerID),
	}
	c.Whiteouts = make(map[string]bool)

	return nil
}

// Branch creates an instantaneous O(1) isolated branch of the current stack with a new scratch layer.
func (c *CoWFilesystem) Branch(branchName string) (*CoWFilesystem, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	branchWorkDir := filepath.Join(c.BaseDir, "branches", branchName)
	if err := os.MkdirAll(branchWorkDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create branch workdir: %w", err)
	}

	// Copy committed layer pointers (immutable)
	sharedLayers := make([]*CoWLayer, len(c.Layers))
	copy(sharedLayers, c.Layers)

	upperDir := filepath.Join(branchWorkDir, "upper_scratch")
	if err := os.MkdirAll(upperDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create branch upper layer: %w", err)
	}

	parentID := "root"
	if len(sharedLayers) > 0 {
		parentID = sharedLayers[len(sharedLayers)-1].ID
	}

	branchUpper := &CoWLayer{
		ID:          fmt.Sprintf("branch_%s_upper", branchName),
		ParentID:    parentID,
		RootDir:     upperDir,
		IsReadOnly:  false,
		CreatedAt:   time.Now(),
		Description: fmt.Sprintf("Branch %s scratch layer", branchName),
	}

	return &CoWFilesystem{
		BaseDir:     branchWorkDir,
		Layers:      sharedLayers,
		ActiveLayer: branchUpper,
		Whiteouts:   make(map[string]bool),
		SessionID:   fmt.Sprintf("%s-branch-%s", c.SessionID, branchName),
		CurrentStep: c.CurrentStep,
	}, nil
}

// Diff analyzes modifications present in the active layer compared to all lower layers.
func (c *CoWFilesystem) Diff() (*SnapshotDiff, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	diff := &SnapshotDiff{
		FromSnapshotID: c.ActiveLayer.ParentID,
		ToSnapshotID:   c.ActiveLayer.ID,
		Changes:        make([]FileChange, 0),
	}

	// 1. Process whiteouts (Deleted files)
	for path := range c.Whiteouts {
		diff.Changes = append(diff.Changes, FileChange{
			Path:   path,
			Action: "deleted",
		})
	}

	// 2. Walk upper layer for created or modified files
	err := filepath.Walk(c.ActiveLayer.RootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}

		rel, err := filepath.Rel(c.ActiveLayer.RootDir, path)
		if err != nil {
			return err
		}

		// Skip whiteout markers themselves from direct file list
		if strings.HasPrefix(filepath.Base(rel), WhiteoutPrefix) {
			return nil
		}

		// Check if file existed in lower layers
		existedInLower := false
		for _, layer := range c.Layers {
			lowerPath := filepath.Join(layer.RootDir, rel)
			if _, statErr := os.Stat(lowerPath); statErr == nil {
				existedInLower = true
				break
			}
		}

		action := "created"
		if existedInLower {
			action = "modified"
		}

		diff.Changes = append(diff.Changes, FileChange{
			Path:   rel,
			Action: action,
		})
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk active layer for diff: %w", err)
	}

	return diff, nil
}

// GenerateDockerMountArgs constructs standard Linux OverlayFS mount options.
func (c *CoWFilesystem) GenerateDockerMountArgs(targetMountPath string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var lowerDirs []string
	for i := len(c.Layers) - 1; i >= 0; i-- {
		lowerDirs = append(lowerDirs, c.Layers[i].RootDir)
	}

	workDir := filepath.Join(c.BaseDir, "workdir")
	_ = os.MkdirAll(workDir, 0755)

	options := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s",
		strings.Join(lowerDirs, ":"),
		c.ActiveLayer.RootDir,
		workDir,
	)

	return fmt.Sprintf("mount -t overlay overlay -o %s %s", options, targetMountPath)
}

// Cleanup removes temporary scratch layers from disk.
func (c *CoWFilesystem) Cleanup() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return os.RemoveAll(c.BaseDir)
}

func computeLayerDigest(rootDir string) (string, error) {
	h := sha256.New()
	hasFiles := false

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		hasFiles = true
		rel, _ := filepath.Rel(rootDir, path)
		h.Write([]byte(rel))

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, _ = io.Copy(h, f)
		return nil
	})

	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	if !hasFiles {
		return "empty_layer_sha256", nil
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
