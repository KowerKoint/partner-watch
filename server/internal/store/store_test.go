package store

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenMigratesExistingSingleDeviceSchema(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "partner-watch.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
CREATE TABLE pairs(id TEXT PRIMARY KEY,name TEXT NOT NULL,created_at INTEGER NOT NULL);
CREATE TABLE invitations(token_hash BLOB PRIMARY KEY,pair_id TEXT NOT NULL REFERENCES pairs(id) ON DELETE CASCADE,slot INTEGER NOT NULL,expires_at INTEGER NOT NULL,used_at INTEGER,created_at INTEGER NOT NULL,UNIQUE(pair_id,slot));
CREATE TABLE devices(id TEXT PRIMARY KEY,pair_id TEXT NOT NULL REFERENCES pairs(id) ON DELETE CASCADE,slot INTEGER NOT NULL,name TEXT NOT NULL,public_key TEXT NOT NULL UNIQUE,credential_hash BLOB NOT NULL UNIQUE,created_at INTEGER NOT NULL,UNIQUE(pair_id,slot));
INSERT INTO pairs VALUES('pair','Existing',1);
INSERT INTO devices VALUES('android','pair',1,'Pixel','key',X'01',1);`)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open migration: %v", err)
	}
	defer s.Close()
	devices, err := s.ListDevices(context.Background(), "pair")
	if err != nil || len(devices) != 1 || devices[0].Platform != "ANDROID" {
		t.Fatalf("devices=%+v err=%v", devices, err)
	}
	invite, err := s.CreateDeviceInvitation(context.Background(), "pair", 1, time.Now().Add(time.Hour))
	if err != nil || invite.Token == "" {
		t.Fatalf("additional invitation=%+v err=%v", invite, err)
	}
}

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

func TestCaptureRequestFansOutToEveryPartnerCaptureDevice(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	first, second := enrollTestPair(t, s)
	invite, err := s.CreateDeviceInvitation(ctx, first.PairID, second.Slot, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	linux, err := s.EnrollDeviceWithMetadata(ctx, invite.Token, "niri PC", "linux-capture-key", "LINUX", "notification.send,capture")
	if err != nil {
		t.Fatal(err)
	}

	requests, err := s.CreateCaptureRequests(ctx, first.DeviceID)
	if err != nil {
		t.Fatalf("CreateCaptureRequests: %v", err)
	}
	if len(requests) != 2 {
		t.Fatalf("request count = %d; want 2", len(requests))
	}
	targets := map[string]string{}
	for _, request := range requests {
		targets[request.TargetDeviceID] = request.TargetDeviceName
	}
	if targets[second.DeviceID] != "Galaxy" || targets[linux.DeviceID] != "niri PC" {
		t.Fatalf("targets = %#v", targets)
	}
	if _, err := s.CreateCaptureRequests(ctx, first.DeviceID); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("second fan-out error = %v; want ErrRateLimited", err)
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

func TestForwardedNotificationDeliveryAndExpiry(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	first, second := enrollTestPair(t, s)
	base := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return base }

	created, targets, err := s.CreateForwardedNotification(ctx, first.DeviceID, ForwardedNotification{
		SourcePackage: "com.example.chat", SourceAppName: "Chat", Title: "hello", Body: "world", PostedAt: base.Add(-time.Second),
	})
	if err != nil {
		t.Fatalf("CreateForwardedNotification: %v", err)
	}
	if len(targets) != 1 || targets[0] != second.DeviceID || created.ExpiresAt != base.Add(time.Hour) {
		t.Fatalf("created=%+v targets=%v", created, targets)
	}
	if items, err := s.PendingForwardedNotifications(ctx, first.DeviceID); err != nil || len(items) != 0 {
		t.Fatalf("source pending=%v, %v", items, err)
	}
	items, err := s.PendingForwardedNotifications(ctx, second.DeviceID)
	if err != nil || len(items) != 1 || items[0].Body != "world" {
		t.Fatalf("target pending=%v, %v", items, err)
	}
	if err := s.AcknowledgeForwardedNotification(ctx, second.DeviceID, created.ID); err != nil {
		t.Fatalf("AcknowledgeForwardedNotification: %v", err)
	}
	if err := s.AcknowledgeForwardedNotification(ctx, second.DeviceID, created.ID); !errors.Is(err, ErrForwardedNotificationNotFound) {
		t.Fatalf("duplicate acknowledgement=%v", err)
	}

	s.now = func() time.Time { return base.Add(time.Hour + time.Second) }
	if count, err := s.DeleteExpiredForwardedNotifications(ctx); err != nil || count != 1 {
		t.Fatalf("DeleteExpiredForwardedNotifications=%d, %v", count, err)
	}
}

func TestNotificationFilterBlocksBeforeStorageAndDelivery(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	first, second := enrollTestPair(t, s)
	base := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return base }

	filter, err := s.SaveNotificationFilter(ctx, second.DeviceID, NotificationFilter{
		SourceDeviceID: first.DeviceID, SourceDeviceName: "Pixel", SourcePackage: "com.example.chat",
		SourceAppName: "Chat", TitlePattern: "ＡＬＥＲＴ", TitleMatch: "CONTAINS", MessageMatch: "CONTAINS", Enabled: true,
	})
	if err != nil {
		t.Fatalf("SaveNotificationFilter: %v", err)
	}
	if filters, err := s.ListNotificationFilters(ctx, second.DeviceID); err != nil || len(filters) != 1 || filters[0].ID != filter.ID {
		t.Fatalf("filters=%+v err=%v", filters, err)
	}
	created, targets, err := s.CreateForwardedNotification(ctx, first.DeviceID, ForwardedNotification{
		SourcePackage: "com.example.chat", SourceAppName: "Chat", Title: "System alert", Body: "battery", PostedAt: base,
	})
	if err != nil || created.ID == "" || len(targets) != 0 {
		t.Fatalf("created=%+v targets=%v err=%v", created, targets, err)
	}
	var stored int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM forwarded_notifications WHERE id=?`, created.ID).Scan(&stored); err != nil || stored != 0 {
		t.Fatalf("stored=%d err=%v", stored, err)
	}
	if err := s.DeleteNotificationFilter(ctx, second.DeviceID, filter.ID); err != nil {
		t.Fatal(err)
	}
}

func TestNotificationFilterIsScopedToReceivingSlot(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	first, second := enrollTestPair(t, s)
	filter, err := s.SaveNotificationFilter(ctx, second.DeviceID, NotificationFilter{SourcePackage: "com.example.chat", TitleMatch: "CONTAINS", MessageMatch: "CONTAINS", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if own, err := s.ListNotificationFilters(ctx, first.DeviceID); err != nil || len(own) != 0 {
		t.Fatalf("sender filters=%+v err=%v", own, err)
	}
	filter.SourcePackage = "com.example.other"
	if _, err := s.SaveNotificationFilter(ctx, first.DeviceID, filter); !errors.Is(err, ErrNotificationFilterNotFound) {
		t.Fatalf("cross-slot update error=%v", err)
	}
}

func TestNotificationTextMatchModesNormalizeCaseAndWidth(t *testing.T) {
	if !notificationTextMatches("Ａｌｉｃｅ", "alice", "EXACT") {
		t.Fatal("normalized exact match failed")
	}
	if notificationTextMatches("Alice (group)", "alice", "EXACT") {
		t.Fatal("exact match accepted extra text")
	}
	if !notificationTextMatches("Alice (group)", "alice", "CONTAINS") {
		t.Fatal("contains match failed")
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
	polled, err := s.CaptureRequestForRequester(ctx, first.DeviceID, request.ID)
	if err != nil || polled.Status != "READY" || polled.ImageID != image.ID {
		t.Fatalf("polled=%+v err=%v", polled, err)
	}
	if _, err := s.CaptureRequestForRequester(ctx, second.DeviceID, request.ID); !errors.Is(err, ErrCaptureRequestNotFound) {
		t.Fatalf("target poll=%v", err)
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
	_, err = s.CompleteStatusRequest(ctx, second.DeviceID, request.ID, BatteryReport{Status: "AVAILABLE", Percent: 73, ChargingState: "CHARGING"}, LocationReport{Status: "DISABLED"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.PartnerStatusSnapshot(ctx, first.DeviceID)
	if err != nil || snapshot.Battery.Percent != 73 || snapshot.Battery.ChargingState != "CHARGING" {
		t.Fatalf("snapshot=%+v err=%v", snapshot, err)
	}
	if err := s.ClearStatusField(ctx, second.DeviceID, "battery"); err != nil {
		t.Fatal(err)
	}
	snapshot, err = s.PartnerStatusSnapshot(ctx, first.DeviceID)
	if err != nil || snapshot.Battery.Status != "DISABLED" {
		t.Fatalf("after clear=%+v err=%v", snapshot, err)
	}
}

func TestStatusRequestStoresLocation(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	first, second := enrollTestPair(t, s)
	base := time.Now().UTC()
	s.now = func() time.Time { return base }
	r, err := s.CreateStatusRequest(ctx, first.DeviceID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.CompleteStatusRequest(ctx, second.DeviceID, r.ID, BatteryReport{Status: "DISABLED"}, LocationReport{Status: "AVAILABLE", Latitude: 35.6812, Longitude: 139.7671, AccuracyMeters: 12.5, ObservedAt: base, Source: "FRESH"})
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.PartnerStatusSnapshot(ctx, first.DeviceID)
	if err != nil || v.Location.Status != "AVAILABLE" || v.Location.Latitude != 35.6812 {
		t.Fatalf("snapshot=%+v err=%v", v, err)
	}
	if err = s.ClearStatusField(ctx, second.DeviceID, "location"); err != nil {
		t.Fatal(err)
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

func TestAdditionalDeviceInvitationAndRevocation(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	first, _ := enrollTestPair(t, s)
	invite, err := s.CreateDeviceInvitation(ctx, first.PairID, 1, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("CreateDeviceInvitation: %v", err)
	}
	linux, err := s.EnrollDeviceWithMetadata(ctx, invite.Token, "niri PC", "linux-key", "LINUX", "notification.send,capture")
	if err != nil {
		t.Fatalf("EnrollDeviceWithMetadata: %v", err)
	}
	if linux.Slot != 1 {
		t.Fatalf("slot=%d", linux.Slot)
	}
	devices, err := s.ListDevices(ctx, first.PairID)
	if err != nil || len(devices) != 3 {
		t.Fatalf("devices=%v err=%v", devices, err)
	}
	if devices[1].Platform != "LINUX" || devices[1].Capabilities != "notification.send,capture" {
		t.Fatalf("linux=%+v", devices[1])
	}
	if err := s.RevokeDevice(ctx, linux.DeviceID); err != nil {
		t.Fatalf("RevokeDevice: %v", err)
	}
	if _, err := s.AuthenticateDevice(ctx, linux.Credential); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("authentication after revoke=%v", err)
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
