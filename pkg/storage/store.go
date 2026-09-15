package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// Store handles persisting and loading traces from disk.
type Store struct {
	baseDir string
	mu      sync.RWMutex
}

// NewStore initializes a new local filesystem trace store.
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create trace directory: %w", err)
	}
	return &Store{baseDir: dir}, nil
}

// SaveSession writes a session and all its events as formatted JSON.
func (s *Store) SaveSession(session *trace.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize session: %w", err)
	}

	path := filepath.Join(s.baseDir, fmt.Sprintf("%s.vcr.json", session.ID))
	return os.WriteFile(path, data, 0644)
}

// LoadSession reads a session from disk by ID.
func (s *Store) LoadSession(id string) (*trace.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	path := filepath.Join(s.baseDir, fmt.Sprintf("%s.vcr.json", id))
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read trace file: %w", err)
	}

	var session trace.Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to parse trace file: %w", err)
	}

	return &session, nil
}

// ListSessions returns all trace session IDs stored locally.
func (s *Store) ListSessions() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		return nil, err
	}

	var sessions []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			sessions = append(sessions, entry.Name())
		}
	}
	return sessions, nil
}
