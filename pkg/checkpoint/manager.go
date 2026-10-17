package checkpoint

import (
	"fmt"
	"sync"
	"time"
)

// CheckpointManager orchestrates freeze/thaw transitions, delta compaction, and hot recovery.
type CheckpointManager struct {
	mu           sync.RWMutex
	systemID     string
	config       HibernationConfig
	storage      *StorageEngine
	deltaComp    *DeltaCompressor
	activeImages map[string]*SessionImage // ImageID -> In-memory image
	history      []string                 // Chronological list of ImageIDs
}

// NewCheckpointManager creates a new hibernation and checkpoint manager.
func NewCheckpointManager(systemID string, cfg HibernationConfig) (*CheckpointManager, error) {
	if systemID == "" {
		systemID = "agentvcr-checkpoint-system"
	}
	se, err := NewStorageEngine(cfg.StorageDir, cfg.GzipCompressionLevel)
	if err != nil {
		return nil, err
	}

	return &CheckpointManager{
		systemID:     systemID,
		config:       cfg,
		storage:      se,
		deltaComp:    NewDeltaCompressor(),
		activeImages: make(map[string]*SessionImage),
		history:      make([]string, 0),
	}, nil
}

// Hibernate captures, freezes, delta-compresses, and persists an agent state to disk.
func (cm *CheckpointManager) Hibernate(image *SessionImage) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	image.State = StateHibernated
	if image.CreatedAt.IsZero() {
		image.CreatedAt = time.Now()
	}

	toStore := image

	// If delta compression enabled and we have a preceding checkpoint for this session
	if cm.config.UseDeltaCompression && len(cm.history) > 0 {
		lastID := cm.history[len(cm.history)-1]
		if base, ok := cm.activeImages[lastID]; ok && base.SessionID == image.SessionID {
			delta := cm.deltaComp.ComputeDelta(base, image)
			toStore = delta
		}
	}

	// Persist to disk with Gzip and SHA-256
	if err := cm.storage.Save(toStore); err != nil {
		return fmt.Errorf("failed to hibernate checkpoint image: %w", err)
	}

	cm.activeImages[image.ImageID] = image
	cm.history = append(cm.history, image.ImageID)

	// Enforce retention limit
	if len(cm.history) > cm.config.MaxSnapshotsRetained {
		oldest := cm.history[0]
		cm.history = cm.history[1:]
		delete(cm.activeImages, oldest)
	}

	return nil
}

// Restore revives and reconstructs a hibernated agent image from disk, verifying SHA-256 integrity.
func (cm *CheckpointManager) Restore(imageID string) (*SessionImage, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	stored, err := cm.storage.Load(imageID)
	if err != nil {
		return nil, fmt.Errorf("failed to load checkpoint from storage: %w", err)
	}

	// Check if this was stored as an incremental delta
	if stored.DeltaBaseID != "" {
		base, ok := cm.activeImages[stored.DeltaBaseID]
		if !ok {
			// Try loading base from disk
			base, err = cm.storage.Load(stored.DeltaBaseID)
			if err != nil {
				return nil, fmt.Errorf("failed to load delta base image %s: %w", stored.DeltaBaseID, err)
			}
		}

		reconstructed, recErr := cm.deltaComp.Reconstruct(base, stored)
		if recErr != nil {
			return nil, fmt.Errorf("failed to reconstruct delta image: %w", recErr)
		}
		reconstructed.State = StateRestored
		cm.activeImages[imageID] = reconstructed
		return reconstructed, nil
	}

	stored.State = StateRestored
	cm.activeImages[imageID] = stored
	return stored, nil
}

// Audit generates a full diagnostic scorecard across all stored checkpoints.
func (cm *CheckpointManager) Audit() *CheckpointAuditReport {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var images []*SessionImage
	var totalRaw int64
	var totalCompressed int64
	hibernatedCount := 0
	restoredCount := 0
	integrityPassed := true

	for _, id := range cm.history {
		img, ok := cm.activeImages[id]
		if !ok {
			continue
		}
		images = append(images, img)
		totalRaw += img.UncompressedSize
		totalCompressed += img.CompressedSize

		if img.State == StateHibernated {
			hibernatedCount++
		} else if img.State == StateRestored {
			restoredCount++
		} else if img.State == StateCorrupted {
			integrityPassed = false
		}
	}

	ratio := 0.0
	if totalRaw > 0 && totalCompressed < totalRaw {
		ratio = float64(totalRaw-totalCompressed) / float64(totalRaw)
	}

	return &CheckpointAuditReport{
		SystemID:             cm.systemID,
		Timestamp:            time.Now(),
		TotalImages:          len(images),
		TotalHibernated:      hibernatedCount,
		TotalRestored:        restoredCount,
		TotalRawBytes:        totalRaw,
		TotalCompressedBytes: totalCompressed,
		CompressionRatio:     ratio,
		IntegrityVerified:    integrityPassed,
		Images:               images,
	}
}
