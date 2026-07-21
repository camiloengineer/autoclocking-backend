package accountsstore

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/camiloengineer/autoclocking-backend/internal/accounts"
)

type fileRecord struct {
	Email       string    `json:"email"`
	PasswordB64 string    `json:"password_b64"`
	JobID       string    `json:"job_id"`
	Active      bool      `json:"active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// FileStore persists accounts as a JSON array on the local filesystem, for
// local development and DEBUG runs. The password is base64 encoded at rest.
type FileStore struct {
	path string
	mu   sync.Mutex
}

// NewFileStore returns a file-backed accounts.Store at the given path.
func NewFileStore(path string) *FileStore {
	return &FileStore{path: path}
}

func (s *FileStore) Seed(_ context.Context, records []accounts.Account) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, err := s.readUnlocked()
	if err != nil {
		return err
	}
	index := make(map[string]struct{}, len(existing))
	for _, record := range existing {
		index[record.Email] = struct{}{}
	}
	for _, record := range records {
		if _, ok := index[record.Email]; ok {
			continue
		}
		existing = append(existing, record)
		index[record.Email] = struct{}{}
	}
	return s.writeUnlocked(existing)
}

func (s *FileStore) List(_ context.Context) ([]accounts.Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readUnlocked()
}

func (s *FileStore) Save(_ context.Context, record accounts.Account) (accounts.Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, err := s.readUnlocked()
	if err != nil {
		return accounts.Account{}, err
	}
	now := time.Now().UTC()
	for i, current := range existing {
		if current.Email == record.Email {
			record.CreatedAt = current.CreatedAt
			record.UpdatedAt = now
			existing[i] = record
			if err := s.writeUnlocked(existing); err != nil {
				return accounts.Account{}, err
			}
			return record, nil
		}
	}
	record.CreatedAt = now
	record.UpdatedAt = now
	existing = append(existing, record)
	if err := s.writeUnlocked(existing); err != nil {
		return accounts.Account{}, err
	}
	return record, nil
}

func (s *FileStore) SetActive(_ context.Context, email string, active bool) (accounts.Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	normalized := accounts.NormalizeEmail(email)
	existing, err := s.readUnlocked()
	if err != nil {
		return accounts.Account{}, err
	}
	for i, current := range existing {
		if current.Email == normalized {
			current.Active = active
			current.UpdatedAt = time.Now().UTC()
			existing[i] = current
			if err := s.writeUnlocked(existing); err != nil {
				return accounts.Account{}, err
			}
			return current, nil
		}
	}
	return accounts.Account{}, accounts.ErrNotFound
}

func (s *FileStore) Delete(_ context.Context, email string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	normalized := accounts.NormalizeEmail(email)
	existing, err := s.readUnlocked()
	if err != nil {
		return err
	}
	filtered := make([]accounts.Account, 0, len(existing))
	found := false
	for _, current := range existing {
		if current.Email == normalized {
			found = true
			continue
		}
		filtered = append(filtered, current)
	}
	if !found {
		return accounts.ErrNotFound
	}
	return s.writeUnlocked(filtered)
}

func (s *FileStore) readUnlocked() ([]accounts.Account, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []accounts.Account{}, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return []accounts.Account{}, nil
	}
	var raw []fileRecord
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	records := make([]accounts.Account, 0, len(raw))
	for _, item := range raw {
		password, err := accounts.DecodePassword(item.PasswordB64)
		if err != nil {
			return nil, err
		}
		records = append(records, accounts.Account{
			Email:     item.Email,
			Password:  password,
			JobID:     item.JobID,
			Active:    item.Active,
			CreatedAt: item.CreatedAt,
			UpdatedAt: item.UpdatedAt,
		})
	}
	return records, nil
}

func (s *FileStore) writeUnlocked(records []accounts.Account) error {
	raw := make([]fileRecord, 0, len(records))
	for _, record := range records {
		raw = append(raw, fileRecord{
			Email:       record.Email,
			PasswordB64: accounts.EncodePassword(record.Password),
			JobID:       record.JobID,
			Active:      record.Active,
			CreatedAt:   record.CreatedAt,
			UpdatedAt:   record.UpdatedAt,
		})
	}
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(s.path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(s.path, data, 0o600)
}
