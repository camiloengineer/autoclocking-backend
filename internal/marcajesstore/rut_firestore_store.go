package marcajesstore

import (
	"context"
	"errors"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/camiloengineer/autoclocking-backend/internal/ruts"
	"google.golang.org/api/iterator"
)

const rutCollectionName = "ruts"

type RUTFirestoreStore struct {
	client *firestore.Client
}

func NewRUTFirestoreStore(client *firestore.Client) *RUTFirestoreStore {
	return &RUTFirestoreStore{client: client}
}

func (s *RUTFirestoreStore) SeedRUTs(ctx context.Context, records []ruts.Record) error {
	if s.client == nil {
		return errors.New("firestore client is nil")
	}

	for _, record := range records {
		ref := s.client.Collection(rutCollectionName).Doc(record.RUT)
		snapshot, err := ref.Get(ctx)
		if err == nil && snapshot.Exists() {
			continue
		}
		if err != nil && !isNotFound(err) {
			return err
		}
		if _, err := ref.Set(ctx, record); err != nil {
			return err
		}
	}

	return nil
}

func (s *RUTFirestoreStore) ListRUTs(ctx context.Context) ([]ruts.Record, error) {
	if s.client == nil {
		return nil, errors.New("firestore client is nil")
	}

	iter := s.client.Collection(rutCollectionName).Documents(ctx)
	defer iter.Stop()

	records := []ruts.Record{}
	for {
		doc, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			return records, nil
		}
		if err != nil {
			return nil, err
		}

		var record ruts.Record
		if err := doc.DataTo(&record); err != nil {
			return nil, err
		}
		if record.CreatedAt.IsZero() {
			record.CreatedAt = time.Unix(0, 0).UTC()
		}
		if record.UpdatedAt.IsZero() {
			record.UpdatedAt = record.CreatedAt
		}
		if record.RUT == "" {
			record.RUT = doc.Ref.ID
		}
		records = append(records, record)
	}
}

func (s *RUTFirestoreStore) SaveRUT(ctx context.Context, record ruts.Record) (ruts.Record, error) {
	if s.client == nil {
		return ruts.Record{}, errors.New("firestore client is nil")
	}

	ref := s.client.Collection(rutCollectionName).Doc(record.RUT)
	now := time.Now().UTC()
	snapshot, err := ref.Get(ctx)
	if err == nil && snapshot.Exists() {
		var current ruts.Record
		if err := snapshot.DataTo(&current); err != nil {
			return ruts.Record{}, err
		}
		record.CreatedAt = current.CreatedAt
		record.UpdatedAt = now
	} else {
		if err != nil && !isNotFound(err) {
			return ruts.Record{}, err
		}
		record.CreatedAt = now
		record.UpdatedAt = now
	}

	if _, err := ref.Set(ctx, record); err != nil {
		return ruts.Record{}, err
	}

	return record, nil
}

func (s *RUTFirestoreStore) DeleteRUT(ctx context.Context, value string) error {
	if s.client == nil {
		return errors.New("firestore client is nil")
	}

	normalized := ruts.Normalize(value)
	ref := s.client.Collection(rutCollectionName).Doc(normalized)
	snapshot, err := ref.Get(ctx)
	if err != nil && isNotFound(err) {
		return ruts.ErrNotFound
	}
	if err != nil {
		return err
	}
	if !snapshot.Exists() {
		return ruts.ErrNotFound
	}

	_, err = ref.Delete(ctx)
	return err
}

func isNotFound(err error) bool {
	return strings.Contains(err.Error(), "NotFound")
}
