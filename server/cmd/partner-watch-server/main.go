package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/kowerkoint/partner-watch/server/internal/fcm"
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
	go maintainData(database, logger)

	var handler http.Handler
	credentials := os.Getenv("PW_FIREBASE_CREDENTIALS_FILE")
	if credentials != "" {
		sender, initErr := fcm.New(context.Background(), credentials, database)
		if initErr != nil {
			logger.Error("failed to initialize FCM", "error", initErr)
			os.Exit(1)
		}
		handler = httpapi.NewHandler(database, sender)
	} else {
		logger.Warn("FCM disabled: PW_FIREBASE_CREDENTIALS_FILE is not set")
		handler = httpapi.NewHandler(database)
	}

	server := &http.Server{
		Addr:              address,
		Handler:           handler,
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

func maintainData(database *store.Store, logger *slog.Logger) {
	cleanupImages := func() {
		count, err := database.DeleteExpiredImages(context.Background())
		if err != nil {
			logger.Error("failed to delete expired images", "error", err)
		} else if count > 0 {
			logger.Info("deleted expired images", "count", count)
		}
	}
	expireCaptures := func() {
		count, err := database.ExpireCaptureRequests(context.Background())
		if err != nil {
			logger.Error("failed to expire capture requests", "error", err)
		} else if count > 0 {
			logger.Info("expired capture requests", "count", count)
		}
	}
	expireStatuses := func() {
		if count, err := database.ExpireStatusRequests(context.Background()); err != nil {
			logger.Error("failed to expire status requests", "error", err)
		} else if count > 0 {
			logger.Info("expired status requests", "count", count)
		}
	}
	cleanupImages()
	expireCaptures()
	expireStatuses()
	imageTicker := time.NewTicker(10 * time.Minute)
	captureTicker := time.NewTicker(5 * time.Second)
	defer imageTicker.Stop()
	defer captureTicker.Stop()
	for {
		select {
		case <-imageTicker.C:
			cleanupImages()
		case <-captureTicker.C:
			expireCaptures()
			expireStatuses()
		}
	}
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
