package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/kowerkoint/partner-watch/server/internal/store"
)

type output struct {
	PairID    string          `json:"pairId"`
	PairName  string          `json:"pairName"`
	ExpiresAt string          `json:"expiresAt"`
	Devices   [2]deviceInvite `json:"devices"`
}

type deviceInvite struct {
	Slot       int    `json:"slot"`
	ServerURL  string `json:"serverUrl"`
	InviteCode string `json:"inviteCode"`
}

func main() {
	if len(os.Args) < 2 || os.Args[1] != "pair-create" {
		fmt.Fprintln(os.Stderr, "usage: partner-watch-admin pair-create [options]")
		os.Exit(2)
	}

	flags := flag.NewFlagSet("pair-create", flag.ExitOnError)
	dataDir := flags.String("data-dir", envOrDefault("PW_DATA_DIR", "/var/lib/partner-watch"), "database directory")
	name := flags.String("name", "Partner Watch", "pair display name")
	serverURL := flags.String("server-url", os.Getenv("PW_PUBLIC_URL"), "public HTTPS server URL (or PW_PUBLIC_URL)")
	ttl := flags.Duration("ttl", 15*time.Minute, "invitation validity")
	_ = flags.Parse(os.Args[2:])

	normalizedServerURL, err := normalizeServerURL(*serverURL)
	if err != nil {
		fatal(err.Error())
	}
	if *ttl <= 0 || *ttl > 24*time.Hour {
		fatal("ttl must be greater than zero and at most 24h")
	}

	database, err := store.Open(*dataDir)
	if err != nil {
		fatal(err.Error())
	}
	defer func() { _ = database.Close() }()

	pair, err := database.CreatePair(context.Background(), *name, time.Now().Add(*ttl))
	if err != nil {
		fatal(err.Error())
	}
	result := output{
		PairID:    pair.PairID,
		PairName:  pair.PairName,
		ExpiresAt: pair.ExpiresAt.Format(time.RFC3339),
		Devices: [2]deviceInvite{
			{Slot: 1, ServerURL: normalizedServerURL, InviteCode: pair.Invitations[0]},
			{Slot: 2, ServerURL: normalizedServerURL, InviteCode: pair.Invitations[1]},
		},
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		fatal(err.Error())
	}
}

func normalizeServerURL(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return "", fmt.Errorf("server URL must be an HTTPS origin without path, credentials, query, or fragment")
	}
	return "https://" + parsed.Host, nil
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, "error:", message)
	os.Exit(1)
}
