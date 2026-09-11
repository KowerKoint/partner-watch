package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type State struct {
	DeviceID   string `json:"deviceId"`
	PairID     string `json:"pairId"`
	Slot       int    `json:"slot"`
	Credential string `json:"credential"`
	PrivateKey string `json:"privateKey"`
}

func DefaultPath() (string, error) {
	if root := os.Getenv("XDG_STATE_HOME"); root != "" {
		return filepath.Join(root, "partner-watch", "state.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", "partner-watch", "state.json"), nil
}
func Load(path string) (State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return State{}, err
	}
	var value State
	if json.Unmarshal(data, &value) != nil || value.Credential == "" {
		return State{}, errors.New("invalid state")
	}
	return value, nil
}
func Save(path string, value State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0600)
}
