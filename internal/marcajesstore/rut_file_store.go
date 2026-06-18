package marcajesstore

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/camiloengineer/autoclocking-backend/internal/ruts"
)

type RUTFileStore struct {
	mu      sync.Mutex
	path    string
	loaded  bool
	records []ruts.Record
}

func NewRUTFileStore(path string) *RUTFileStore {
	return &RUTFileStore{path: path}
}

func (s *RUTFileStore) SeedRUTs(_ context.Context, records []ruts.Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.loadLocked(); err != nil {
		return err
	}

	if len(records) == 0 {
		return nil
	}

	existing := make(map[string]struct{}, len(s.records))
	for _, record := range s.records {
		existing[record.RUT] = struct{}{}
	}

	changed := false
	for _, record := range records {
		if _, ok := existing[record.RUT]; ok {
			continue
		}
		existing[record.RUT] = struct{}{}
		s.records = append(s.records, record)
		changed = true
	}

	if !changed {
		return nil
	}

	return s.persistLocked()
}

func (s *RUTFileStore) ListRUTs(_ context.Context) ([]ruts.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.loadLocked(); err != nil {
		return nil, err
	}

	items := make([]ruts.Record, len(s.records))
	copy(items, s.records)
	return items, nil
}

func (s *RUTFileStore) SaveRUT(_ context.Context, record ruts.Record) (ruts.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.loadLocked(); err != nil {
		return ruts.Record{}, err
	}

	now := time.Now().UTC()
	for index, current := range s.records {
		if current.RUT != record.RUT {
			continue
		}
		if current.CreatedAt.IsZero() {
			current.CreatedAt = now
		}
		record.CreatedAt = current.CreatedAt
		record.UpdatedAt = now
		s.records[index] = record
		return record, s.persistLocked()
	}

	record.CreatedAt = now
	record.UpdatedAt = now
	s.records = append(s.records, record)
	return record, s.persistLocked()
}

func (s *RUTFileStore) DeleteRUT(_ context.Context, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.loadLocked(); err != nil {
		return err
	}

	normalized := ruts.Normalize(value)
	for index, record := range s.records {
		if record.RUT != normalized {
			continue
		}
		s.records = append(s.records[:index], s.records[index+1:]...)
		return s.persistLocked()
	}

	return ruts.ErrNotFound
}

func (s *RUTFileStore) loadLocked() error {
	if s.loaded {
		return nil
	}

	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s.records = []ruts.Record{}
			s.loaded = true
			return nil
		}
		return err
	}

	if len(data) == 0 {
		s.records = []ruts.Record{}
		s.loaded = true
		return nil
	}

	if err := json.Unmarshal(data, &s.records); err != nil {
		return err
	}

	s.loaded = true
	return nil
}

func (s *RUTFileStore) persistLocked() error {
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
