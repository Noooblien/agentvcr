package checkpoint

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// StorageEngine handles atomic, compressed, hash-verified serialization of checkpoint images.
type StorageEngine struct {
	mu        sync.RWMutex
	dir       string
	gzipLevel int
}

// NewStorageEngine creates an initialized storage engine ensuring target directories exist.
func NewStorageEngine(dir string, gzipLevel int) (*StorageEngine, error) {
	if dir == "" {
		dir = "./.vcr_checkpoints"
	}
	if gzipLevel <= 0 || gzipLevel > 9 {
		gzipLevel = 6
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create checkpoint storage dir: %w", err)
	}
	return &StorageEngine{
		dir:       dir,
		gzipLevel: gzipLevel,
	}, nil
}

// Save compresses and atomically writes a SessionImage to disk, calculating its SHA-256 hash.
func (se *StorageEngine) Save(image *SessionImage) error {
	se.mu.Lock()
	defer se.mu.Unlock()

	// Compute Raw JSON with SHA256Hash empty
	image.SHA256Hash = ""
	rawBytes, err := json.Marshal(image)
	if err != nil {
		return fmt.Errorf("failed to serialize session image: %w", err)
	}
	image.UncompressedSize = int64(len(rawBytes))

	// Re-marshal with UncompressedSize included before computing hash
	rawForHash, err := json.Marshal(image)
	if err != nil {
		return fmt.Errorf("failed to serialize session image for hash: %w", err)
	}

	// Compute Cryptographic SHA-256 Integrity Hash
	h := sha256.Sum256(rawForHash)
	image.SHA256Hash = hex.EncodeToString(h[:])

	// Re-marshal with stamped hash
	rawWithHash, err := json.Marshal(image)
	if err != nil {
		return fmt.Errorf("failed to re-serialize session image with hash: %w", err)
	}

	// Gzip Compression
	var buf bytes.Buffer
	gw, err := gzip.NewWriterLevel(&buf, se.gzipLevel)
	if err != nil {
		return fmt.Errorf("failed to initialize gzip writer: %w", err)
	}
	if _, err := gw.Write(rawWithHash); err != nil {
		return fmt.Errorf("gzip compression failed: %w", err)
	}
	if err := gw.Close(); err != nil {
		return fmt.Errorf("failed to flush gzip buffer: %w", err)
	}

	compressedBytes := buf.Bytes()
	image.CompressedSize = int64(len(compressedBytes))

	// Atomic File Write via Temp File + Rename
	finalPath := filepath.Join(se.dir, fmt.Sprintf("%s.ckpt.gz", image.ImageID))
	tempPath := finalPath + fmt.Sprintf(".tmp.%d", time.Now().UnixNano())

	if err := os.WriteFile(tempPath, compressedBytes, 0644); err != nil {
		return fmt.Errorf("failed to write temporary checkpoint file: %w", err)
	}

	if err := os.Rename(tempPath, finalPath); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("atomic rename failed for checkpoint: %w", err)
	}

	return nil
}

// Load decompresses and validates the cryptographic hash of a stored checkpoint image.
func (se *StorageEngine) Load(imageID string) (*SessionImage, error) {
	se.mu.RLock()
	defer se.mu.RUnlock()

	filePath := filepath.Join(se.dir, fmt.Sprintf("%s.ckpt.gz", imageID))
	compressedData, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("checkpoint file not found: %w", err)
	}

	// Decompress Gzip
	gr, err := gzip.NewReader(bytes.NewReader(compressedData))
	if err != nil {
		return nil, fmt.Errorf("invalid gzip header on checkpoint %s: %w", imageID, err)
	}
	defer gr.Close()

	decompressed, err := io.ReadAll(gr)
	if err != nil {
		return nil, fmt.Errorf("decompression error on %s: %w", imageID, err)
	}

	var image SessionImage
	if err := json.Unmarshal(decompressed, &image); err != nil {
		return nil, fmt.Errorf("failed to parse checkpoint JSON: %w", err)
	}

	// Verify Cryptographic Hash Integrity
	savedHash := image.SHA256Hash
	image.SHA256Hash = ""
	checkBytes, _ := json.Marshal(&image)
	h := sha256.Sum256(checkBytes)
	computedHash := hex.EncodeToString(h[:])
	image.SHA256Hash = savedHash

	if savedHash != "" && savedHash != computedHash {
		image.State = StateCorrupted
		return &image, fmt.Errorf("integrity violation on %s: expected SHA-256 %s, got %s", imageID, savedHash, computedHash)
	}

	return &image, nil
}

// Delete removes a stored checkpoint file.
func (se *StorageEngine) Delete(imageID string) error {
	se.mu.Lock()
	defer se.mu.Unlock()

	filePath := filepath.Join(se.dir, fmt.Sprintf("%s.ckpt.gz", imageID))
	return os.Remove(filePath)
}
