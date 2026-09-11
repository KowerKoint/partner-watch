package config

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	ServerURL, DeviceName                string
	AcceptCaptures, ForwardNotifications bool
}

func DefaultPath() (string, error) {
	if root := os.Getenv("XDG_CONFIG_HOME"); root != "" {
		return filepath.Join(root, "partner-watch", "config.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "partner-watch", "config.toml"), nil
}

func Load(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer f.Close()
	var result Config
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(strings.SplitN(scanner.Text(), "#", 2)[0])
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return Config{}, errors.New("invalid config line")
		}
		key, raw := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		switch key {
		case "server_url", "device_name":
			value, err := strconv.Unquote(raw)
			if err != nil {
				return Config{}, err
			}
			if key == "server_url" {
				result.ServerURL = value
			} else {
				result.DeviceName = value
			}
		case "accept_captures", "forward_notifications":
			value, err := strconv.ParseBool(raw)
			if err != nil {
				return Config{}, err
			}
			if key == "accept_captures" {
				result.AcceptCaptures = value
			} else {
				result.ForwardNotifications = value
			}
		default:
			return Config{}, errors.New("unknown config key: " + key)
		}
	}
	if err := scanner.Err(); err != nil {
		return Config{}, err
	}
	if !strings.HasPrefix(result.ServerURL, "https://") || result.DeviceName == "" {
		return Config{}, errors.New("server_url and device_name are required")
	}
	return result, nil
}
