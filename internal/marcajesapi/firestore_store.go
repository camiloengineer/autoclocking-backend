package marcajesapi

import (
	"context"
	"errors"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

const collectionName = "marcajes"

type FirestoreStore struct {
	client *firestore.Client
}

func NewFirestoreStore(client *firestore.Client) *FirestoreStore {
	return &FirestoreStore{client: client}
}

func (s *FirestoreStore) AddMarcaje(ctx context.Context, record MarcajeRecord) (string, error) {
	if s.client == nil {
		return "", errors.New("firestore client is nil")
	}

	record.CreatedAt = record.CreatedAt.UTC()
	ref, _, err := s.client.Collection(collectionName).Add(ctx, record)
	if err != nil {
		return "", err
	}

	return ref.ID, nil
}

func (s *FirestoreStore) ListMarcajes(ctx context.Context, limit int) ([]MarcajeRecord, error) {
	if s.client == nil {
		return nil, errors.New("firestore client is nil")
	}

	iter := s.client.Collection(collectionName).
		OrderBy("created_at", firestore.Desc).
		Limit(limit).
		Documents(ctx)
	defer iter.Stop()

	items := make([]MarcajeRecord, 0, limit)
	for {
		doc, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}

		var record MarcajeRecord
		if err := doc.DataTo(&record); err != nil {
			return nil, err
		}

		record.ID = doc.Ref.ID
		record.CreatedAt = record.CreatedAt.UTC()
		if record.CreatedAt.IsZero() {
			record.CreatedAt = time.Unix(0, 0).UTC()
		}
		items = append(items, record)
	}

	return items, nil
}
