package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
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
	serverURL := flags.String("server-url", "https://partner-watch.kowerkoint.com", "public HTTPS server URL")
	ttl := flags.Duration("ttl", 15*time.Minute, "invitation validity")
	_ = flags.Parse(os.Args[2:])

	if !strings.HasPrefix(*serverURL, "https://") {
		fatal("server URL must use HTTPS")
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
			{Slot: 1, ServerURL: strings.TrimRight(*serverURL, "/"), InviteCode: pair.Invitations[0]},
			{Slot: 2, ServerURL: strings.TrimRight(*serverURL, "/"), InviteCode: pair.Invitations[1]},
		},
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		fatal(err.Error())
	}
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
