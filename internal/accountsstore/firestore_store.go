package accountsstore

import (
	"context"
	"errors"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"

	"github.com/camiloengineer/autoclocking-backend/internal/accounts"
)

const accountsCollectionName = "accounts"

type firestoreDoc struct {
	Email       string    `firestore:"email"`
	PasswordB64 string    `firestore:"password_b64"`
	JobID       string    `firestore:"job_id"`
	Active      bool      `firestore:"active"`
	CreatedAt   time.Time `firestore:"created_at"`
	UpdatedAt   time.Time `firestore:"updated_at"`
}

// FirestoreStore persists accounts in the "accounts" Firestore collection,
// keyed by normalized email, with the password base64 encoded at rest.
type FirestoreStore struct {
	client *firestore.Client
}

// NewFirestoreStore wraps a Firestore client as an accounts.Store.
func NewFirestoreStore(client *firestore.Client) *FirestoreStore {
	return &FirestoreStore{client: client}
}

func (s *FirestoreStore) Seed(ctx context.Context, records []accounts.Account) error {
	if s.client == nil {
		return errors.New("firestore client is nil")
	}
	for _, record := range records {
		ref := s.client.Collection(accountsCollectionName).Doc(record.Email)
		snapshot, err := ref.Get(ctx)
		if err == nil && snapshot.Exists() {
			continue
		}
		if err != nil && !isNotFound(err) {
			return err
		}
		if _, err := ref.Set(ctx, toDoc(record)); err != nil {
			return err
		}
	}
	return nil
}

func (s *FirestoreStore) List(ctx context.Context) ([]accounts.Account, error) {
	if s.client == nil {
		return nil, errors.New("firestore client is nil")
	}
	iter := s.client.Collection(accountsCollectionName).Documents(ctx)
	defer iter.Stop()

	records := []accounts.Account{}
	for {
		doc, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			return records, nil
		}
		if err != nil {
			return nil, err
		}
		var raw firestoreDoc
		if err := doc.DataTo(&raw); err != nil {
			return nil, err
		}
		record, err := fromDoc(raw)
		if err != nil {
			return nil, err
		}
		if record.Email == "" {
			record.Email = doc.Ref.ID
		}
		records = append(records, record)
	}
}

func (s *FirestoreStore) Save(ctx context.Context, record accounts.Account) (accounts.Account, error) {
	if s.client == nil {
		return accounts.Account{}, errors.New("firestore client is nil")
	}
	ref := s.client.Collection(accountsCollectionName).Doc(record.Email)
	now := time.Now().UTC()
	snapshot, err := ref.Get(ctx)
	if err == nil && snapshot.Exists() {
		var current firestoreDoc
		if err := snapshot.DataTo(&current); err != nil {
			return accounts.Account{}, err
		}
		record.CreatedAt = current.CreatedAt
		record.UpdatedAt = now
	} else {
		if err != nil && !isNotFound(err) {
			return accounts.Account{}, err
		}
		record.CreatedAt = now
		record.UpdatedAt = now
	}
	if _, err := ref.Set(ctx, toDoc(record)); err != nil {
		return accounts.Account{}, err
	}
	return record, nil
}

func (s *FirestoreStore) SetActive(ctx context.Context, email string, active bool) (accounts.Account, error) {
	if s.client == nil {
		return accounts.Account{}, errors.New("firestore client is nil")
	}
	normalized := accounts.NormalizeEmail(email)
	ref := s.client.Collection(accountsCollectionName).Doc(normalized)
	snapshot, err := ref.Get(ctx)
	if err != nil && isNotFound(err) {
		return accounts.Account{}, accounts.ErrNotFound
	}
	if err != nil {
		return accounts.Account{}, err
	}
	if !snapshot.Exists() {
		return accounts.Account{}, accounts.ErrNotFound
	}
	var raw firestoreDoc
	if err := snapshot.DataTo(&raw); err != nil {
		return accounts.Account{}, err
	}
	record, err := fromDoc(raw)
	if err != nil {
		return accounts.Account{}, err
	}
	record.Active = active
	record.UpdatedAt = time.Now().UTC()
	if _, err := ref.Set(ctx, toDoc(record)); err != nil {
		return accounts.Account{}, err
	}
	return record, nil
}

func (s *FirestoreStore) Delete(ctx context.Context, email string) error {
	if s.client == nil {
		return errors.New("firestore client is nil")
	}
	normalized := accounts.NormalizeEmail(email)
	ref := s.client.Collection(accountsCollectionName).Doc(normalized)
	snapshot, err := ref.Get(ctx)
	if err != nil && isNotFound(err) {
		return accounts.ErrNotFound
	}
	if err != nil {
		return err
	}
	if !snapshot.Exists() {
		return accounts.ErrNotFound
	}
	_, err = ref.Delete(ctx)
	return err
}

func toDoc(record accounts.Account) firestoreDoc {
	return firestoreDoc{
		Email:       record.Email,
		PasswordB64: accounts.EncodePassword(record.Password),
		JobID:       record.JobID,
		Active:      record.Active,
		CreatedAt:   record.CreatedAt,
		UpdatedAt:   record.UpdatedAt,
	}
}

func fromDoc(raw firestoreDoc) (accounts.Account, error) {
	password, err := accounts.DecodePassword(raw.PasswordB64)
	if err != nil {
		return accounts.Account{}, err
	}
	createdAt := raw.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Unix(0, 0).UTC()
	}
	updatedAt := raw.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = createdAt
	}
	return accounts.Account{
		Email:     raw.Email,
		Password:  password,
		JobID:     raw.JobID,
		Active:    raw.Active,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, nil
}

func isNotFound(err error) bool {
	return strings.Contains(err.Error(), "NotFound")
}
