package store

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
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

func TestImageCanOnlyBeTakenOnceByPartner(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	first, second := enrollTestPair(t, s)

	if got, err := s.AuthenticateDevice(ctx, first.Credential); err != nil || got != first.DeviceID {
		t.Fatalf("AuthenticateDevice = %q, %v", got, err)
	}
	created, err := s.SaveImage(ctx, first.DeviceID, []byte("jpeg-data"), 1080, 2400)
	if err != nil {
		t.Fatalf("SaveImage: %v", err)
	}
	if _, err := s.TakeImage(ctx, first.DeviceID, created.ID); !errors.Is(err, ErrImageNotFound) {
		t.Fatalf("uploader TakeImage error = %v, want ErrImageNotFound", err)
	}
	taken, err := s.TakeImage(ctx, second.DeviceID, created.ID)
	if err != nil {
		t.Fatalf("partner TakeImage: %v", err)
	}
	if !bytes.Equal(taken.Data, []byte("jpeg-data")) {
		t.Fatalf("image data = %q", taken.Data)
	}
	if _, err := s.TakeImage(ctx, second.DeviceID, created.ID); !errors.Is(err, ErrImageNotFound) {
		t.Fatalf("second TakeImage error = %v, want ErrImageNotFound", err)
	}
	if _, err := os.Stat(filepath.Join(s.dataDir, "images", created.ID+".jpg")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("claimed image still exists: %v", err)
	}
}

func TestExpiredImagesAreDeleted(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	first, _ := enrollTestPair(t, s)
	created, err := s.SaveImage(ctx, first.DeviceID, []byte("jpeg-data"), 1, 1)
	if err != nil {
		t.Fatalf("SaveImage: %v", err)
	}
	s.now = func() time.Time { return created.ExpiresAt.Add(time.Second) }
	count, err := s.DeleteExpiredImages(ctx)
	if err != nil || count != 1 {
		t.Fatalf("DeleteExpiredImages = %d, %v; want 1, nil", count, err)
	}
	if _, err := os.Stat(filepath.Join(s.dataDir, "images", created.ID+".jpg")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expired image still exists: %v", err)
	}
}

func enrollTestPair(t *testing.T, s *Store) (Enrollment, Enrollment) {
	t.Helper()
	ctx := context.Background()
	pair, err := s.CreatePair(ctx, "Us", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("CreatePair: %v", err)
	}
	first, err := s.EnrollDevice(ctx, pair.Invitations[0], "Pixel", "key-one")
	if err != nil {
		t.Fatalf("EnrollDevice(first): %v", err)
	}
	second, err := s.EnrollDevice(ctx, pair.Invitations[1], "Galaxy", "key-two")
	if err != nil {
		t.Fatalf("EnrollDevice(second): %v", err)
	}
	return first, second
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
