package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/kowerkoint/partner-watch/server/internal/httpapi"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	address := envOrDefault("PW_LISTEN_ADDR", "127.0.0.1:8080")

	server := &http.Server{
		Addr:              address,
		Handler:           httpapi.NewHandler(),
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

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
