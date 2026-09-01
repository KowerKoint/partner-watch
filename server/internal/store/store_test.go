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

func TestCaptureRequestRateLimitAndCompletion(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	first, second := enrollTestPair(t, s)
	base := time.Now().UTC()
	s.now = func() time.Time { return base }

	request, err := s.CreateCaptureRequest(ctx, first.DeviceID)
	if err != nil {
		t.Fatalf("CreateCaptureRequest: %v", err)
	}
	if request.TargetDeviceID != second.DeviceID || request.Status != "PENDING" {
		t.Fatalf("request = %+v", request)
	}
	if _, err := s.CreateCaptureRequest(ctx, first.DeviceID); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("immediate request error = %v, want ErrRateLimited", err)
	}

	image, err := s.SaveImage(ctx, second.DeviceID, []byte("jpeg"), 1, 1)
	if err != nil {
		t.Fatalf("SaveImage: %v", err)
	}
	completed, err := s.CompleteCaptureRequest(ctx, second.DeviceID, request.ID, "READY", image.ID, "")
	if err != nil {
		t.Fatalf("CompleteCaptureRequest: %v", err)
	}
	if completed.Status != "READY" || completed.ImageID != image.ID {
		t.Fatalf("completed = %+v", completed)
	}
	if _, err := s.CompleteCaptureRequest(ctx, second.DeviceID, request.ID, "READY", image.ID, ""); !errors.Is(err, ErrCaptureRequestNotFound) {
		t.Fatalf("duplicate completion error = %v", err)
	}

	s.now = func() time.Time { return base.Add(11 * time.Second) }
	if _, err := s.CreateCaptureRequest(ctx, first.DeviceID); err != nil {
		t.Fatalf("request after 10 seconds: %v", err)
	}
}

func TestCaptureRequestRejectsWrongTargetAndExpiredResult(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	first, second := enrollTestPair(t, s)
	base := time.Now().UTC()
	s.now = func() time.Time { return base }
	request, err := s.CreateCaptureRequest(ctx, first.DeviceID)
	if err != nil {
		t.Fatalf("CreateCaptureRequest: %v", err)
	}
	if _, err := s.CompleteCaptureRequest(ctx, first.DeviceID, request.ID, "FAILED", "", "DISABLED"); !errors.Is(err, ErrCaptureRequestNotFound) {
		t.Fatalf("requester completion error = %v", err)
	}
	s.now = func() time.Time { return base.Add(time.Minute + time.Second) }
	if _, err := s.CompleteCaptureRequest(ctx, second.DeviceID, request.ID, "FAILED", "", "DISABLED"); !errors.Is(err, ErrCaptureRequestNotFound) {
		t.Fatalf("expired completion error = %v", err)
	}
	count, err := s.ExpireCaptureRequests(ctx)
	if err != nil || count != 1 {
		t.Fatalf("ExpireCaptureRequests = %d, %v; want 1, nil", count, err)
	}
	if count, err := s.ExpireCaptureRequests(ctx); err != nil || count != 0 {
		t.Fatalf("second ExpireCaptureRequests = %d, %v; want 0, nil", count, err)
	}
}

func TestStatusRequestStoresLatestPartnerBatteryAndClearsIt(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	first, second := enrollTestPair(t, s)
	base := time.Now().UTC()
	s.now = func() time.Time { return base }
	request, err := s.CreateStatusRequest(ctx, first.DeviceID)
	if err != nil {
		t.Fatal(err)
	}
	if request.TargetDeviceID != second.DeviceID {
		t.Fatalf("target=%q", request.TargetDeviceID)
	}
	if _, err := s.CreateStatusRequest(ctx, first.DeviceID); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("rate limit=%v", err)
	}
	pending, err := s.PendingStatusRequests(ctx, second.DeviceID)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending=%v,%v", pending, err)
	}
	_, err = s.CompleteStatusRequest(ctx, second.DeviceID, request.ID, BatteryReport{Status: "AVAILABLE", Percent: 73, ChargingState: "CHARGING"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.PartnerStatusSnapshot(ctx, first.DeviceID)
	if err != nil || snapshot.Battery.Percent != 73 || snapshot.Battery.ChargingState != "CHARGING" {
		t.Fatalf("snapshot=%+v err=%v", snapshot, err)
	}
	if err := s.ClearStatusSnapshot(ctx, second.DeviceID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PartnerStatusSnapshot(ctx, first.DeviceID); !errors.Is(err, ErrStatusSnapshotNotFound) {
		t.Fatalf("after clear=%v", err)
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
