package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("server_url = \"https://watch.example.com\"\ndevice_name = \"niri PC\"\naccept_captures = false\nforward_notifications = true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || got.DeviceName != "niri PC" || !got.ForwardNotifications || got.AcceptCaptures {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestLoadRejectsUnknownKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	_ = os.WriteFile(path, []byte("server_url=\"https://example.com\"\ndevice_name=\"PC\"\nsecret=true\n"), 0600)
	if _, err := Load(path); err == nil {
		t.Fatal("unknown key was accepted")
	}
}
