package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCreatePairAndEnrollBothDevices(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	pair, err := store.CreatePair(ctx, "Us", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("CreatePair: %v", err)
	}
	first, err := store.EnrollDevice(ctx, pair.Invitations[0], "Pixel 8a", "first-public-key")
	if err != nil {
		t.Fatalf("EnrollDevice(first): %v", err)
	}
	second, err := store.EnrollDevice(ctx, pair.Invitations[1], "Galaxy A25 5G", "second-public-key")
	if err != nil {
		t.Fatalf("EnrollDevice(second): %v", err)
	}

	if first.PairID != pair.PairID || second.PairID != pair.PairID {
		t.Fatal("devices were not enrolled into the created pair")
	}
	if first.Slot != 1 || second.Slot != 2 {
		t.Fatalf("slots = %d, %d; want 1, 2", first.Slot, second.Slot)
	}
	if first.Credential == "" || second.Credential == "" || first.Credential == second.Credential {
		t.Fatal("expected unique device credentials")
	}
}

func TestInvitationCanOnlyBeUsedOnce(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	pair, err := store.CreatePair(ctx, "Us", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("CreatePair: %v", err)
	}
	if _, err := store.EnrollDevice(ctx, pair.Invitations[0], "Pixel", "key-one"); err != nil {
		t.Fatalf("first enrollment: %v", err)
	}
	_, err = store.EnrollDevice(ctx, pair.Invitations[0], "Other", "key-two")
	if !errors.Is(err, ErrInvitationNotFound) {
		t.Fatalf("second enrollment error = %v, want ErrInvitationNotFound", err)
	}
}

func TestExpiredInvitationIsRejected(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	pair, err := store.CreatePair(ctx, "Us", time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("CreatePair: %v", err)
	}
	store.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
	_, err = store.EnrollDevice(ctx, pair.Invitations[0], "Pixel", "key-one")
	if !errors.Is(err, ErrInvitationNotFound) {
		t.Fatalf("enrollment error = %v, want ErrInvitationNotFound", err)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}
