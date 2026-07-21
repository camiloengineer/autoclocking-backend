package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"cloud.google.com/go/firestore"

	"github.com/camiloengineer/autoclocking-backend/internal/accountsstore"
	"github.com/camiloengineer/autoclocking-backend/internal/marcajes"
	"github.com/camiloengineer/autoclocking-backend/internal/marcajesapi"
	"github.com/camiloengineer/autoclocking-backend/internal/marcajesstore"
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

	ctx := context.Background()
	var store marcajes.Store
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

	accountStore, closeAccounts, err := accountsstore.FromEnv(ctx)
	if err != nil {
		slog.Error("Failed to build accounts store", "error", err)
		os.Exit(1)
	}
	defer func() { _ = closeAccounts() }()

	server := marcajesapi.NewServer(store, accountStore)

	port := getEnvOrDefault("PORT", "8080")
	addr := ":" + port

	slog.Info("Starting marcajes API", "addr", addr, "project_id", projectID)
	if err := http.ListenAndServe(addr, server); err != nil {
		slog.Error("HTTP server stopped", "error", err)
		os.Exit(1)
	}
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
