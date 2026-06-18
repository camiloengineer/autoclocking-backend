package marcajesstore

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"time"

	"cloud.google.com/go/storage"
	"github.com/camiloengineer/autoclocking-backend/internal/ruts"
)

type RUTGCSStore struct {
	mu      sync.Mutex
	client  *storage.Client
	bucket  string
	object  string
	loaded  bool
	records []ruts.Record
}

func NewRUTGCSStore(client *storage.Client, bucket string, object string) *RUTGCSStore {
	return &RUTGCSStore{client: client, bucket: bucket, object: object}
}

func (s *RUTGCSStore) SeedRUTs(ctx context.Context, records []ruts.Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.loadLocked(ctx); err != nil {
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

	return s.persistLocked(ctx)
}

func (s *RUTGCSStore) ListRUTs(ctx context.Context) ([]ruts.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.loadLocked(ctx); err != nil {
		return nil, err
	}

	items := make([]ruts.Record, len(s.records))
	copy(items, s.records)
	return items, nil
}

func (s *RUTGCSStore) SaveRUT(ctx context.Context, record ruts.Record) (ruts.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.loadLocked(ctx); err != nil {
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
		return record, s.persistLocked(ctx)
	}

	record.CreatedAt = now
	record.UpdatedAt = now
	s.records = append(s.records, record)
	return record, s.persistLocked(ctx)
}

func (s *RUTGCSStore) DeleteRUT(ctx context.Context, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.loadLocked(ctx); err != nil {
		return err
	}

	normalized := ruts.Normalize(value)
	for index, record := range s.records {
		if record.RUT != normalized {
			continue
		}
		s.records = append(s.records[:index], s.records[index+1:]...)
		return s.persistLocked(ctx)
	}

	return ruts.ErrNotFound
}

func (s *RUTGCSStore) loadLocked(ctx context.Context) error {
	if s.loaded {
		return nil
	}
	if s.client == nil {
		return errors.New("storage client is nil")
	}

	reader, err := s.client.Bucket(s.bucket).Object(s.object).NewReader(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			s.records = []ruts.Record{}
			s.loaded = true
			return nil
		}
		return err
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
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

func (s *RUTGCSStore) persistLocked(ctx context.Context) error {
	writer := s.client.Bucket(s.bucket).Object(s.object).NewWriter(ctx)
	payload, err := json.MarshalIndent(s.records, "", "  ")
	if err != nil {
		_ = writer.Close()
		return err
	}
	if _, err := writer.Write(payload); err != nil {
		_ = writer.Close()
		return err
	}
	return writer.Close()
}
