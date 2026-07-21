package accountsstore

import (
	"context"
	"errors"
	"os"
	"strings"

	"cloud.google.com/go/firestore"

	"github.com/camiloengineer/autoclocking-backend/internal/accounts"
)

// FromEnv builds the accounts store selected by ACCOUNTS_STORAGE_BACKEND
// (firestore|file, default file) and returns it together with a cleanup func.
func FromEnv(ctx context.Context) (accounts.Store, func() error, error) {
	backend := strings.ToLower(strings.TrimSpace(os.Getenv("ACCOUNTS_STORAGE_BACKEND")))
	if backend == "firestore" {
		projectID := firstNonEmpty(
			os.Getenv("FIRESTORE_PROJECT_ID"),
			os.Getenv("GOOGLE_CLOUD_PROJECT"),
			os.Getenv("GCP_PROJECT"),
		)
		if projectID == "" {
			return nil, nil, errors.New("firestore accounts storage selected without project id")
		}
		client, err := firestore.NewClient(ctx, projectID)
		if err != nil {
			return nil, nil, err
		}
		return NewFirestoreStore(client), client.Close, nil
	}

	path := firstNonEmpty(os.Getenv("ACCOUNTS_STORAGE_FILE"), "./data/accounts.json")
	return NewFileStore(path), func() error { return nil }, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
