package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"cloud.google.com/go/firestore"
	"cloud.google.com/go/storage"

	"github.com/camiloengineer/autoclocking-backend/internal/config"
	"github.com/camiloengineer/autoclocking-backend/internal/marcajes"
	"github.com/camiloengineer/autoclocking-backend/internal/marcajesapi"
	"github.com/camiloengineer/autoclocking-backend/internal/marcajesstore"
	"github.com/camiloengineer/autoclocking-backend/internal/ruts"
)

func main() {
	setupLogging()

	projectID := firstNonEmpty(
		os.Getenv("FIRESTORE_PROJECT_ID"),
		os.Getenv("GOOGLE_CLOUD_PROJECT"),
		os.Getenv("GCP_PROJECT"),
	)
	storageBackend := strings.ToLower(firstNonEmpty(
		os.Getenv("MARCAJES_STORAGE_BACKEND"),
		os.Getenv("MARCJE_STORAGE_BACKEND"),
	))
	rutStorageBackend := strings.ToLower(os.Getenv("RUT_STORAGE_BACKEND"))
	if rutStorageBackend == "" && strings.TrimSpace(os.Getenv("RUT_STORAGE_BUCKET")) != "" {
		rutStorageBackend = "gcs"
	}
	ctx := context.Background()
	var store marcajes.Store
	var rutStore ruts.Store
	if storageBackend != "firestore" {
		storePath := firstNonEmpty(
			os.Getenv("MARCAJE_STORAGE_FILE"),
			os.Getenv("MARCJE_STORAGE_FILE"),
			"./data/marcajes.json",
		)
		slog.Info("Using file-backed marcajes store", "path", storePath)
		store = marcajesstore.NewFileStore(storePath)
	} else {
		if projectID == "" {
			slog.Error("Firestore storage selected without project id")
			os.Exit(1)
		}
		client, err := firestore.NewClient(ctx, projectID)
		if err != nil {
			slog.Error("Failed to create Firestore client", "error", err)
			os.Exit(1)
		}
		defer client.Close()
		store = marcajesstore.NewFirestoreStore(client)
	}

	switch rutStorageBackend {
	case "gcs":
		bucket := strings.TrimSpace(os.Getenv("RUT_STORAGE_BUCKET"))
		if bucket == "" {
			slog.Error("GCS RUT storage selected without bucket")
			os.Exit(1)
		}
		object := firstNonEmpty(os.Getenv("RUT_STORAGE_OBJECT"), "autoclocking/ruts.json")
		client, err := storage.NewClient(ctx)
		if err != nil {
			slog.Error("Failed to create Cloud Storage client", "error", err)
			os.Exit(1)
		}
		defer client.Close()
		slog.Info("Using GCS-backed RUT store", "bucket", bucket, "object", object)
		rutStore = marcajesstore.NewRUTGCSStore(client, bucket, object)
	case "firestore":
		if projectID == "" {
			slog.Error("Firestore RUT storage selected without project id")
			os.Exit(1)
		}
		client, err := firestore.NewClient(ctx, projectID)
		if err != nil {
			slog.Error("Failed to create Firestore client", "error", err)
			os.Exit(1)
		}
		defer client.Close()
		slog.Info("Using Firestore-backed RUT store")
		rutStore = marcajesstore.NewRUTFirestoreStore(client)
	default:
		rutStorePath := firstNonEmpty(
			os.Getenv("RUT_STORAGE_FILE"),
			"./data/ruts.json",
		)
		slog.Info("Using file-backed RUT store", "path", rutStorePath)
		rutStore = marcajesstore.NewRUTFileStore(rutStorePath)
	}

	if err := seedRUTs(rutStore); err != nil {
		slog.Error("Failed to seed RUTs", "error", err)
		os.Exit(1)
	}

	server := marcajesapi.NewServer(store, rutStore)

	port := getEnvOrDefault("PORT", "8080")
	addr := ":" + port

	slog.Info("Starting marcajes API", "addr", addr, "project_id", projectID)
	if err := http.ListenAndServe(addr, server); err != nil {
		slog.Error("HTTP server stopped", "error", err)
		os.Exit(1)
	}
}

func seedRUTs(store ruts.Store) error {
	activeRUTs, err := config.LoadInitialRUTs()
	if err != nil {
		return err
	}
	if len(activeRUTs) == 0 {
		return nil
	}

	records, err := ruts.SeedRecords(activeRUTs)
	if err != nil {
		return err
	}

	return store.SeedRUTs(context.Background(), records)
}

func setupLogging() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
}

func getEnvOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
