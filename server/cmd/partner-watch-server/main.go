package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/kowerkoint/partner-watch/server/internal/httpapi"
	"github.com/kowerkoint/partner-watch/server/internal/store"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	address := envOrDefault("PW_LISTEN_ADDR", "127.0.0.1:8080")
	dataDir := envOrDefault("PW_DATA_DIR", "./data")
	database, err := store.Open(dataDir)
	if err != nil {
		logger.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer func() { _ = database.Close() }()
	go cleanupExpiredImages(database, logger)

	server := &http.Server{
		Addr:              address,
		Handler:           httpapi.NewHandler(database),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	logger.Info("starting Partner Watch server", "address", address)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func cleanupExpiredImages(database *store.Store, logger *slog.Logger) {
	cleanup := func() {
		count, err := database.DeleteExpiredImages(context.Background())
		if err != nil {
			logger.Error("failed to delete expired images", "error", err)
		} else if count > 0 {
			logger.Info("deleted expired images", "count", count)
		}
	}
	cleanup()
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cleanup()
	}
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
