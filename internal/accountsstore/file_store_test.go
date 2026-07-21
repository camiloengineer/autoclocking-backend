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

func TestFileStoreLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	store := NewFileStore(path)
	ctx := context.Background()

	acc, err := accounts.NewAccount("cgonzalez@robotia.cl", "robotia..", true)
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
	if len(listed) != 1 || listed[0].Password != "robotia.." {
		t.Fatalf("List = %+v, want one account with decoded password", listed)
	}

	if _, err := store.SetActive(ctx, "cgonzalez@robotia.cl", false); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	listed, _ = store.List(ctx)
	if listed[0].Active {
		t.Error("SetActive(false) did not persist")
	}

	if _, err := store.SetActive(ctx, "ghost@robotia.cl", true); !errors.Is(err, accounts.ErrNotFound) {
		t.Errorf("SetActive(missing) = %v, want ErrNotFound", err)
	}

	if err := store.Delete(ctx, "cgonzalez@robotia.cl"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := store.Delete(ctx, "cgonzalez@robotia.cl"); !errors.Is(err, accounts.ErrNotFound) {
		t.Errorf("Delete(missing) = %v, want ErrNotFound", err)
	}
}

func TestFileStorePersistsPasswordEncoded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	store := NewFileStore(path)
	acc, _ := accounts.NewAccount("cgonzalez@robotia.cl", "robotia..", true)
	if _, err := store.Save(context.Background(), acc); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(data), "robotia..") {
		t.Error("plaintext password leaked into the file")
	}
	var raw []map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if raw[0]["password_b64"] != accounts.EncodePassword("robotia..") {
		t.Errorf("password_b64 = %v, want encoded form", raw[0]["password_b64"])
	}
}
