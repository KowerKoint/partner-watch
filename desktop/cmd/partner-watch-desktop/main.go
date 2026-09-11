package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/coder/websocket"
	"github.com/kowerkoint/partner-watch/desktop/internal/client"
	"github.com/kowerkoint/partner-watch/desktop/internal/config"
	"github.com/kowerkoint/partner-watch/desktop/internal/linuxcapture"
	"github.com/kowerkoint/partner-watch/desktop/internal/linuxnotify"
	"github.com/kowerkoint/partner-watch/desktop/internal/state"
)

func main() {
	if len(os.Args) < 2 {
		fatal("usage: partner-watch-desktop enroll|run [options]")
	}
	switch os.Args[1] {
	case "enroll":
		enroll(os.Args[2:])
	case "run":
		run(os.Args[2:])
	default:
		fatal("unknown command")
	}
}

func paths(configFlag, stateFlag string) (string, string) {
	configPath := configFlag
	if configPath == "" {
		configPath, _ = config.DefaultPath()
	}
	statePath := stateFlag
	if statePath == "" {
		statePath, _ = state.DefaultPath()
	}
	return configPath, statePath
}
func enroll(args []string) {
	flags := flag.NewFlagSet("enroll", flag.ExitOnError)
	configFlag := flags.String("config", "", "config path")
	stateFlag := flags.String("state", "", "state path")
	invite := flags.String("invite-code", "", "single-use invitation code")
	_ = flags.Parse(args)
	cp, sp := paths(*configFlag, *stateFlag)
	cfg, err := config.Load(cp)
	if err != nil {
		fatal(err.Error())
	}
	if *invite == "" {
		fmt.Fprint(os.Stderr, "Invite code: ")
		value, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		*invite = strings.TrimSpace(value)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := client.Enroll(ctx, cfg.ServerURL, *invite, cfg.DeviceName)
	if err != nil {
		fatal(err.Error())
	}
	if err := state.Save(sp, result); err != nil {
		fatal(err.Error())
	}
	fmt.Printf("registered device %s in slot %d\n", result.DeviceID, result.Slot)
}

func run(args []string) {
	flags := flag.NewFlagSet("run", flag.ExitOnError)
	configFlag := flags.String("config", "", "config path")
	stateFlag := flags.String("state", "", "state path")
	_ = flags.Parse(args)
	cp, sp := paths(*configFlag, *stateFlag)
	cfg, err := config.Load(cp)
	if err != nil {
		fatal(err.Error())
	}
	saved, err := state.Load(sp)
	if err != nil {
		fatal(err.Error())
	}
	api, err := client.New(cfg.ServerURL, saved.Credential)
	if err != nil {
		fatal(err.Error())
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if cfg.ForwardNotifications {
		go forwardNotifications(ctx, logger, api)
	} else {
		logger.Info("notification forwarding disabled")
	}
	connectionLoop(ctx, logger, cfg.ServerURL, saved.Credential, cfg.AcceptCaptures, api)
}

func forwardNotifications(ctx context.Context, logger *slog.Logger, api *client.Client) {
	items := make(chan linuxnotify.Notification, 32)
	go func() {
		if err := linuxnotify.Monitor(ctx, items); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("notification monitor stopped", "error", err)
		}
	}()
	recent := map[string]time.Time{}
	for {
		select {
		case <-ctx.Done():
			return
		case item := <-items:
			key := item.AppName + "\x00" + item.Title + "\x00" + item.Body
			if at, ok := recent[key]; ok && time.Since(at) < 5*time.Second {
				continue
			}
			recent[key] = time.Now()
			go func() {
				requestCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
				defer cancel()
				if err := api.ForwardNotification(requestCtx, item.AppName, item.DesktopEntry, item.Title, item.Body, item.PostedAt); err != nil {
					logger.Warn("notification forwarding failed", "error", err)
				}
			}()
		}
	}
}

func connectionLoop(ctx context.Context, logger *slog.Logger, origin, credential string, acceptCaptures bool, api *client.Client) {
	delay := time.Second
	var activeMu sync.Mutex
	active := map[string]bool{}
	startCapture := func(requestID string, expiresAt time.Time) {
		if !acceptCaptures || requestID == "" || time.Now().After(expiresAt) {
			return
		}
		activeMu.Lock()
		if active[requestID] {
			activeMu.Unlock()
			return
		}
		active[requestID] = true
		activeMu.Unlock()
		go func() {
			defer func() { activeMu.Lock(); delete(active, requestID); activeMu.Unlock() }()
			captureCtx, cancel := context.WithDeadline(ctx, expiresAt)
			defer cancel()
			captured, err := linuxcapture.AllDisplays(captureCtx)
			if err != nil {
				logger.Warn("display capture failed", "request_id", requestID, "error", err)
				_ = api.ReportCapture(captureCtx, requestID, nil, "INTERNAL_ERROR")
				return
			}
			result := make([]client.CaptureImage, 0, len(captured))
			for _, image := range captured {
				if len(image.JPEG) > 10*1024*1024 {
					logger.Warn("captured display is too large", "display", image.DisplayName)
					continue
				}
				imageID, err := api.UploadImage(captureCtx, image.JPEG)
				if err != nil {
					logger.Warn("capture upload failed", "display", image.DisplayName, "error", err)
					continue
				}
				result = append(result, client.CaptureImage{ImageID: imageID, DisplayName: image.DisplayName})
			}
			failure := ""
			if len(result) == 0 {
				failure = "INTERNAL_ERROR"
			}
			if err := api.ReportCapture(captureCtx, requestID, result, failure); err != nil {
				logger.Warn("capture result failed", "request_id", requestID, "error", err)
				return
			}
			logger.Info("capture completed", "request_id", requestID, "images", len(result))
		}()
	}
	for ctx.Err() == nil {
		url := strings.TrimSuffix(origin, "/") + "/v1/events"
		headers := http.Header{"Authorization": []string{"Bearer " + credential}}
		conn, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: headers})
		if err == nil {
			logger.Info("connected")
			delay = time.Second
			if acceptCaptures {
				pendingCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
				pending, pendingErr := api.PendingCaptures(pendingCtx)
				cancel()
				if pendingErr != nil {
					logger.Warn("pending capture lookup failed", "error", pendingErr)
				} else {
					for _, request := range pending {
						startCapture(request.RequestID, request.ExpiresAt)
					}
				}
			}
			for {
				_, message, readErr := conn.Read(ctx)
				err = readErr
				if err != nil {
					break
				}
				var event struct {
					Type      string    `json:"type"`
					RequestID string    `json:"requestId"`
					ExpiresAt time.Time `json:"expiresAt"`
				}
				if json.Unmarshal(message, &event) == nil && event.Type == "capture.requested" {
					startCapture(event.RequestID, event.ExpiresAt)
				}
			}
			_ = conn.CloseNow()
		}
		if ctx.Err() != nil {
			return
		}
		logger.Warn("connection lost; retrying", "error", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		if delay < time.Minute {
			delay *= 2
		}
	}
}
func fatal(message string) { fmt.Fprintln(os.Stderr, "error:", message); os.Exit(1) }
