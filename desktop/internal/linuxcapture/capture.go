package linuxcapture

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
)

type Image struct {
	DisplayName string
	JPEG        []byte
}

// AllDisplays uses grim's wlr-screencopy backend and returns one JPEG per
// connected Wayland output.
func AllDisplays(ctx context.Context) ([]Image, error) {
	output, err := exec.CommandContext(ctx, "niri", "msg", "--json", "outputs").Output()
	if err != nil {
		return nil, fmt.Errorf("list displays: %w", err)
	}
	var outputs map[string]json.RawMessage
	if err := json.Unmarshal(output, &outputs); err != nil {
		return nil, fmt.Errorf("parse displays: %w", err)
	}
	names := make([]string, 0, len(outputs))
	for name := range outputs {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, errors.New("no displays found")
	}
	if len(names) > 8 {
		return nil, errors.New("more than 8 displays are not supported")
	}
	images := make([]Image, 0, len(names))
	for _, name := range names {
		var jpeg bytes.Buffer
		command := exec.CommandContext(ctx, "grim", "-o", name, "-t", "jpeg", "-q", "85", "-")
		command.Stdout = &jpeg
		if err := command.Run(); err != nil {
			continue
		}
		if jpeg.Len() > 0 {
			images = append(images, Image{DisplayName: name, JPEG: jpeg.Bytes()})
		}
	}
	if len(images) == 0 {
		return nil, errors.New("all display captures failed")
	}
	return images, nil
}
