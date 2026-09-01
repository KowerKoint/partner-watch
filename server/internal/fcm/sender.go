package fcm

import (
	"context"
	"errors"
	"fmt"
	"os"

	"firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"google.golang.org/api/option"
)

type TokenStore interface {
	FCMToken(context.Context, string) (string, error)
}

type Sender struct {
	client *messaging.Client
	tokens TokenStore
}

func New(ctx context.Context, credentialsFile string, tokens TokenStore) (*Sender, error) {
	if credentialsFile == "" {
		return nil, errors.New("FCM credentials file is required")
	}
	if _, err := os.Stat(credentialsFile); err != nil {
		return nil, fmt.Errorf("stat FCM credentials: %w", err)
	}
	app, err := firebase.NewApp(ctx, nil, option.WithCredentialsFile(credentialsFile))
	if err != nil {
		return nil, fmt.Errorf("initialize Firebase: %w", err)
	}
	client, err := app.Messaging(ctx)
	if err != nil {
		return nil, fmt.Errorf("initialize FCM messaging: %w", err)
	}
	return &Sender{client: client, tokens: tokens}, nil
}

func (s *Sender) SendWakeup(ctx context.Context, deviceID, pairID string) error {
	token, err := s.tokens.FCMToken(ctx, deviceID)
	if err != nil || token == "" {
		return err
	}
	_, err = s.client.Send(ctx, &messaging.Message{Token: token, Data: map[string]string{"type": "capture.wakeup", "pairId": pairID}})
	return err
}
