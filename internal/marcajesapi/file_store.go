package marcajesapi

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type FileStore struct {
	mu      sync.Mutex
	path    string
	loaded  bool
	records []MarcajeRecord
}

func NewFileStore(path string) *FileStore {
	return &FileStore{path: path}
}

func (s *FileStore) AddMarcaje(_ context.Context, record MarcajeRecord) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.loadLocked(); err != nil {
		return "", err
	}

	if record.ID == "" {
		record.ID = time.Now().UTC().Format("20060102150405.000000000")
	}
	record.CreatedAt = record.CreatedAt.UTC()
	s.records = append(s.records, record)

	if err := s.persistLocked(); err != nil {
		return "", err
	}

	return record.ID, nil
}

func (s *FileStore) ListMarcajes(_ context.Context, limit int) ([]MarcajeRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.loadLocked(); err != nil {
		return nil, err
	}

	items := make([]MarcajeRecord, len(s.records))
	copy(items, s.records)
	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})

	if limit < len(items) {
		items = items[:limit]
	}

	return items, nil
}

func (s *FileStore) loadLocked() error {
	if s.loaded {
		return nil
	}

	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s.records = []MarcajeRecord{}
			s.loaded = true
			return nil
		}
		return err
	}

	if len(data) == 0 {
		s.records = []MarcajeRecord{}
		s.loaded = true
		return nil
	}

	if err := json.Unmarshal(data, &s.records); err != nil {
		return err
	}

	s.loaded = true
	return nil
}

func (s *FileStore) persistLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}

	payload, err := json.MarshalIndent(s.records, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, payload, 0o644); err != nil {
		return err
	}

	return os.Rename(tmpPath, s.path)
}
