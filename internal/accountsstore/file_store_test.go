package accountsstore

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/camiloengineer/autoclocking-backend/internal/accounts"
)

const testKey = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="

func TestFileStoreLifecycle(t *testing.T) {
	t.Setenv(accounts.EncryptionKeyEnv, testKey)
	path := filepath.Join(t.TempDir(), "accounts.json")
	store := NewFileStore(path)
	ctx := context.Background()

	acc, err := accounts.NewAccount("user@example.com", "test-password", true)
	if err != nil {
		t.Fatalf("NewAccount: %v", err)
	}
	if _, err := store.Save(ctx, acc); err != nil {
		t.Fatalf("Save: %v", err)
	}

	listed, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listed) != 1 || listed[0].Password != "test-password" {
		t.Fatalf("List = %+v, want one account with decoded password", listed)
	}

	if _, err := store.SetActive(ctx, "user@example.com", false); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	listed, _ = store.List(ctx)
	if listed[0].Active {
		t.Error("SetActive(false) did not persist")
	}

	if _, err := store.SetActive(ctx, "ghost@example.com", true); !errors.Is(err, accounts.ErrNotFound) {
		t.Errorf("SetActive(missing) = %v, want ErrNotFound", err)
	}

	if err := store.Delete(ctx, "user@example.com"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := store.Delete(ctx, "user@example.com"); !errors.Is(err, accounts.ErrNotFound) {
		t.Errorf("Delete(missing) = %v, want ErrNotFound", err)
	}
}

func TestFileStorePersistsPasswordEncrypted(t *testing.T) {
	t.Setenv(accounts.EncryptionKeyEnv, testKey)
	path := filepath.Join(t.TempDir(), "accounts.json")
	store := NewFileStore(path)
	acc, _ := accounts.NewAccount("user@example.com", "test-password", true)
	if _, err := store.Save(context.Background(), acc); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(data), "test-password") {
		t.Error("plaintext password leaked into the file")
	}
	var raw []map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	stored, ok := raw[0]["password_enc"].(string)
	if !ok {
		t.Fatalf("password_enc = %v, want a string", raw[0]["password_enc"])
	}
	decrypted, err := accounts.DecryptPassword("user@example.com", stored)
	if err != nil {
		t.Fatalf("DecryptPassword: %v", err)
	}
	if decrypted != "test-password" {
		t.Errorf("decrypted = %q, want the original password", decrypted)
	}
}
