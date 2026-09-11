package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kowerkoint/partner-watch/server/internal/secret"
	"golang.org/x/text/unicode/norm"
	_ "modernc.org/sqlite"
)

var ErrInvitationNotFound = errors.New("invitation not found")
var ErrUnauthorized = errors.New("unauthorized")
var ErrImageNotFound = errors.New("image not found")
var ErrPartnerNotFound = errors.New("partner not found")
var ErrRateLimited = errors.New("capture request rate limited")
var ErrCaptureRequestNotFound = errors.New("capture request not found")
var ErrStatusRequestNotFound = errors.New("status request not found")
var ErrStatusSnapshotNotFound = errors.New("status snapshot not found")
var ErrPairNotFound = errors.New("pair not found")
var ErrForwardedNotificationNotFound = errors.New("forwarded notification not found")
var ErrNotificationFilterNotFound = errors.New("notification filter not found")

type Store struct {
	db      *sql.DB
	dataDir string
	now     func() time.Time
}

type WakeupSender interface {
	SendWakeup(context.Context, string, string) error
}

type PairInvitations struct {
	PairID      string
	PairName    string
	ExpiresAt   time.Time
	Invitations [2]string
}

type PairSummary struct {
	PairID      string
	PairName    string
	CreatedAt   time.Time
	DeviceCount int
}

type Enrollment struct {
	DeviceID   string
	PairID     string
	Slot       int
	Credential string
}

type DeviceInvitation struct {
	PairID, Token string
	Slot          int
	ExpiresAt     time.Time
}

type DeviceSummary struct {
	DeviceID     string     `json:"deviceId"`
	PairID       string     `json:"pairId"`
	Name         string     `json:"name"`
	Platform     string     `json:"platform"`
	Capabilities string     `json:"capabilities"`
	Slot         int        `json:"slot"`
	CreatedAt    time.Time  `json:"createdAt"`
	LastSeenAt   *time.Time `json:"lastSeenAt,omitempty"`
	RevokedAt    *time.Time `json:"revokedAt,omitempty"`
}

type Image struct {
	ID        string
	Data      []byte
	CreatedAt time.Time
	ExpiresAt time.Time
}

type CaptureRequest struct {
	ID                string
	PairID            string
	RequesterDeviceID string
	TargetDeviceID    string
	TargetDeviceName  string
	TargetPlatform    string
	Status            string
	ImageID           string
	Failure           string
	CreatedAt         time.Time
	ExpiresAt         time.Time
	Images            []CaptureImage
}

type CaptureImage struct {
	ImageID     string `json:"imageId"`
	DisplayName string `json:"displayName"`
}

type StatusRequest struct {
	ID, PairID, RequesterDeviceID, TargetDeviceID, Status string
	CreatedAt, ExpiresAt                                  time.Time
}

type BatteryReport struct {
	Status        string
	Percent       int
	ChargingState string
}
type LocationReport struct {
	Status                              string
	Latitude, Longitude, AccuracyMeters float64
	ObservedAt                          time.Time
	Source                              string
}

type StatusSnapshot struct {
	DeviceID              string
	Battery               BatteryReport
	Location              LocationReport
	ReportedAt, ExpiresAt time.Time
}

type ForwardedNotification struct {
	ID, PairID, SourceDeviceID, SourceDeviceName, SourcePackage, SourceAppName, Title, Body string
	PostedAt, CreatedAt, ExpiresAt                                                          time.Time
}

type NotificationFilter struct {
	ID, PairID, SourceDeviceID, SourceDeviceName, SourcePackage, SourceAppName string
	TitlePattern, TitleMatch, MessagePattern, MessageMatch                     string
	OwnerSlot                                                                  int
	Enabled                                                                    bool
	CreatedAt, UpdatedAt                                                       time.Time
}

const schema = `
CREATE TABLE IF NOT EXISTS pairs (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS invitations (
    token_hash BLOB PRIMARY KEY,
    pair_id TEXT NOT NULL REFERENCES pairs(id) ON DELETE CASCADE,
    slot INTEGER NOT NULL CHECK (slot IN (1, 2)),
    expires_at INTEGER NOT NULL,
    used_at INTEGER,
    created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS devices (
    id TEXT PRIMARY KEY,
    pair_id TEXT NOT NULL REFERENCES pairs(id) ON DELETE CASCADE,
    slot INTEGER NOT NULL CHECK (slot IN (1, 2)),
    name TEXT NOT NULL,
    public_key TEXT NOT NULL UNIQUE,
    credential_hash BLOB NOT NULL UNIQUE,
    created_at INTEGER NOT NULL,
    platform TEXT NOT NULL DEFAULT 'ANDROID',
    capabilities TEXT NOT NULL DEFAULT 'notification.send,notification.receive,capture,status',
    last_seen_at INTEGER,
    revoked_at INTEGER
);

CREATE TABLE IF NOT EXISTS audit_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    pair_id TEXT NOT NULL REFERENCES pairs(id) ON DELETE CASCADE,
    device_id TEXT REFERENCES devices(id) ON DELETE SET NULL,
    event_type TEXT NOT NULL,
    created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS images (
    id TEXT PRIMARY KEY,
    pair_id TEXT NOT NULL REFERENCES pairs(id) ON DELETE CASCADE,
    uploader_device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    file_name TEXT NOT NULL UNIQUE,
    size_bytes INTEGER NOT NULL,
    width INTEGER NOT NULL,
    height INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS capture_requests (
    id TEXT PRIMARY KEY,
    pair_id TEXT NOT NULL REFERENCES pairs(id) ON DELETE CASCADE,
    requester_device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    target_device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    status TEXT NOT NULL CHECK (status IN ('PENDING', 'READY', 'FAILED', 'TIMEOUT')),
    image_id TEXT REFERENCES images(id) ON DELETE SET NULL,
    failure TEXT,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL,
    completed_at INTEGER
);
CREATE TABLE IF NOT EXISTS capture_request_images (
    request_id TEXT NOT NULL REFERENCES capture_requests(id) ON DELETE CASCADE,
    image_id TEXT NOT NULL REFERENCES images(id) ON DELETE CASCADE,
    display_name TEXT NOT NULL,
    position INTEGER NOT NULL,
    PRIMARY KEY(request_id,image_id)
);

CREATE INDEX IF NOT EXISTS audit_events_created_at_idx ON audit_events(created_at);
CREATE INDEX IF NOT EXISTS images_expires_at_idx ON images(expires_at);
CREATE INDEX IF NOT EXISTS capture_requests_requester_created_idx ON capture_requests(requester_device_id, created_at);
CREATE INDEX IF NOT EXISTS capture_requests_expires_at_idx ON capture_requests(expires_at);

CREATE TABLE IF NOT EXISTS device_fcm_tokens (
    device_id TEXT PRIMARY KEY REFERENCES devices(id) ON DELETE CASCADE,
    token TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS status_requests (
    id TEXT PRIMARY KEY,
    pair_id TEXT NOT NULL REFERENCES pairs(id) ON DELETE CASCADE,
    requester_device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    target_device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    status TEXT NOT NULL CHECK (status IN ('PENDING', 'COMPLETED', 'TIMEOUT')),
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL,
    completed_at INTEGER
);
CREATE INDEX IF NOT EXISTS status_requests_requester_created_idx ON status_requests(requester_device_id, created_at);
CREATE INDEX IF NOT EXISTS status_requests_expires_at_idx ON status_requests(expires_at);

CREATE TABLE IF NOT EXISTS status_snapshots (
    device_id TEXT PRIMARY KEY REFERENCES devices(id) ON DELETE CASCADE,
    pair_id TEXT NOT NULL REFERENCES pairs(id) ON DELETE CASCADE,
    battery_status TEXT NOT NULL,
    battery_percent INTEGER,
    charging_state TEXT,
    location_status TEXT NOT NULL DEFAULT 'DISABLED',
    latitude REAL,
    longitude REAL,
    accuracy_meters REAL,
    location_observed_at INTEGER,
    location_source TEXT,
    reported_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS forwarded_notifications (
    id TEXT PRIMARY KEY,
    pair_id TEXT NOT NULL REFERENCES pairs(id) ON DELETE CASCADE,
    source_device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    source_package TEXT NOT NULL,
    source_app_name TEXT NOT NULL,
    title TEXT NOT NULL,
    body TEXT NOT NULL,
    posted_at INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS forwarded_notifications_source_created_idx ON forwarded_notifications(source_device_id, created_at);
CREATE INDEX IF NOT EXISTS forwarded_notifications_expires_at_idx ON forwarded_notifications(expires_at);

CREATE TABLE IF NOT EXISTS forwarded_notification_deliveries (
    notification_id TEXT NOT NULL REFERENCES forwarded_notifications(id) ON DELETE CASCADE,
    device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    delivered_at INTEGER,
    PRIMARY KEY (notification_id, device_id)
);
CREATE INDEX IF NOT EXISTS forwarded_notification_deliveries_device_idx ON forwarded_notification_deliveries(device_id, delivered_at);

CREATE TABLE IF NOT EXISTS notification_filters (
    id TEXT PRIMARY KEY,
    pair_id TEXT NOT NULL REFERENCES pairs(id) ON DELETE CASCADE,
    owner_slot INTEGER NOT NULL CHECK (owner_slot IN (1, 2)),
    source_device_id TEXT NOT NULL DEFAULT '',
    source_device_name TEXT NOT NULL DEFAULT '',
    source_package TEXT NOT NULL DEFAULT '',
    source_app_name TEXT NOT NULL DEFAULT '',
    title_pattern TEXT NOT NULL DEFAULT '',
    title_match TEXT NOT NULL DEFAULT 'CONTAINS' CHECK (title_match IN ('CONTAINS','EXACT')),
    message_pattern TEXT NOT NULL DEFAULT '',
    message_match TEXT NOT NULL DEFAULT 'CONTAINS' CHECK (message_match IN ('CONTAINS','EXACT')),
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS notification_filters_owner_idx ON notification_filters(pair_id, owner_slot, created_at);
`

func Open(dataDir string) (*Store, error) {
	if strings.TrimSpace(dataDir) == "" {
		return nil, errors.New("data directory is required")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}

	databasePath := filepath.Join(dataDir, "partner-watch.db")
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(1)

	store := &Store{db: db, dataDir: dataDir, now: time.Now}
	if err := store.initialize(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) SetFCMToken(ctx context.Context, deviceID, token string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO device_fcm_tokens(device_id, token, updated_at) VALUES (?, ?, ?) ON CONFLICT(device_id) DO UPDATE SET token=excluded.token, updated_at=excluded.updated_at`, deviceID, token, s.now().UTC().Unix())
	return err
}

func (s *Store) FCMToken(ctx context.Context, deviceID string) (string, error) {
	var token string
	err := s.db.QueryRowContext(ctx, `SELECT token FROM device_fcm_tokens WHERE device_id = ?`, deviceID).Scan(&token)
	return token, err
}

func (s *Store) AuthenticateDevice(ctx context.Context, credential string) (string, error) {
	if credential == "" {
		return "", ErrUnauthorized
	}
	hash := secret.Hash(credential)
	var deviceID string
	err := s.db.QueryRowContext(ctx, "SELECT id FROM devices WHERE credential_hash = ? AND revoked_at IS NULL", hash[:]).Scan(&deviceID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrUnauthorized
	}
	if err != nil {
		return "", fmt.Errorf("authenticate device: %w", err)
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE devices SET last_seen_at=? WHERE id=?`, s.now().UTC().Unix(), deviceID)
	return deviceID, nil
}

func (s *Store) CreateDeviceInvitation(ctx context.Context, pairID string, slot int, expiresAt time.Time) (DeviceInvitation, error) {
	if slot != 1 && slot != 2 || !expiresAt.After(s.now().UTC()) {
		return DeviceInvitation{}, errors.New("invalid device invitation")
	}
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT 1 FROM pairs WHERE id=?`, strings.TrimSpace(pairID)).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return DeviceInvitation{}, ErrPairNotFound
	} else if err != nil {
		return DeviceInvitation{}, err
	}
	token, err := secret.Generate(32)
	if err != nil {
		return DeviceInvitation{}, err
	}
	hash := secret.Hash(token)
	_, err = s.db.ExecContext(ctx, `INSERT INTO invitations(token_hash,pair_id,slot,expires_at,created_at) VALUES(?,?,?,?,?)`, hash[:], pairID, slot, expiresAt.UTC().Unix(), s.now().UTC().Unix())
	if err != nil {
		return DeviceInvitation{}, err
	}
	return DeviceInvitation{PairID: pairID, Token: token, Slot: slot, ExpiresAt: expiresAt.UTC()}, nil
}

func (s *Store) ListDevices(ctx context.Context, pairID string) ([]DeviceSummary, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,pair_id,slot,name,platform,capabilities,created_at,last_seen_at,revoked_at FROM devices WHERE pair_id=? ORDER BY slot,created_at`, pairID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []DeviceSummary
	for rows.Next() {
		var item DeviceSummary
		var created int64
		var seen, revoked sql.NullInt64
		if err := rows.Scan(&item.DeviceID, &item.PairID, &item.Slot, &item.Name, &item.Platform, &item.Capabilities, &created, &seen, &revoked); err != nil {
			return nil, err
		}
		item.CreatedAt = time.Unix(created, 0).UTC()
		if seen.Valid {
			v := time.Unix(seen.Int64, 0).UTC()
			item.LastSeenAt = &v
		}
		if revoked.Valid {
			v := time.Unix(revoked.Int64, 0).UTC()
			item.RevokedAt = &v
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) RevokeDevice(ctx context.Context, deviceID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `UPDATE devices SET revoked_at=? WHERE id=? AND revoked_at IS NULL`, s.now().UTC().Unix(), strings.TrimSpace(deviceID))
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return ErrUnauthorized
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM device_fcm_tokens WHERE device_id=?`, deviceID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) CreateCaptureRequest(ctx context.Context, requesterDeviceID string) (CaptureRequest, error) {
	return s.CreateCaptureRequestForTarget(ctx, requesterDeviceID, "")
}

func (s *Store) CreateCaptureRequestForTarget(ctx context.Context, requesterDeviceID, requestedTargetID string) (CaptureRequest, error) {
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CaptureRequest{}, fmt.Errorf("begin capture request: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var pairID, targetDeviceID string
	if requestedTargetID == "" {
		err = tx.QueryRowContext(ctx, `
        SELECT requester.pair_id, target.id
        FROM devices requester
		JOIN devices target ON target.pair_id=requester.pair_id AND target.slot<>requester.slot
		WHERE requester.id=? AND requester.revoked_at IS NULL AND target.revoked_at IS NULL AND target.platform='ANDROID'
		ORDER BY target.created_at LIMIT 1`, requesterDeviceID).Scan(&pairID, &targetDeviceID)
	} else {
		err = tx.QueryRowContext(ctx, `SELECT requester.pair_id,target.id FROM devices requester JOIN devices target ON target.pair_id=requester.pair_id AND target.slot<>requester.slot WHERE requester.id=? AND target.id=? AND requester.revoked_at IS NULL AND target.revoked_at IS NULL AND instr(','||target.capabilities||',',',capture,')>0`, requesterDeviceID, requestedTargetID).Scan(&pairID, &targetDeviceID)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return CaptureRequest{}, ErrPartnerNotFound
	}
	if err != nil {
		return CaptureRequest{}, fmt.Errorf("find capture target: %w", err)
	}
	var recent, hourly int
	if err := tx.QueryRowContext(ctx, `
        SELECT
          COUNT(CASE WHEN created_at > ? THEN 1 END),
          COUNT(*)
        FROM audit_events
        WHERE device_id = ? AND event_type = 'capture.requested' AND created_at > ?`,
		now.Add(-10*time.Second).Unix(), requesterDeviceID, now.Add(-time.Hour).Unix(),
	).Scan(&recent, &hourly); err != nil {
		return CaptureRequest{}, fmt.Errorf("count capture requests: %w", err)
	}
	if recent > 0 || hourly >= 60 {
		return CaptureRequest{}, ErrRateLimited
	}
	id, err := secret.Generate(16)
	if err != nil {
		return CaptureRequest{}, err
	}
	expiresAt := now.Add(time.Minute)
	if _, err := tx.ExecContext(ctx, `
        INSERT INTO capture_requests
          (id, pair_id, requester_device_id, target_device_id, status, created_at, expires_at)
        VALUES (?, ?, ?, ?, 'PENDING', ?, ?)`,
		id, pairID, requesterDeviceID, targetDeviceID, now.Unix(), expiresAt.Unix(),
	); err != nil {
		return CaptureRequest{}, fmt.Errorf("insert capture request: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
        INSERT INTO audit_events (pair_id, device_id, event_type, created_at)
        VALUES (?, ?, 'capture.requested', ?)`, pairID, requesterDeviceID, now.Unix(),
	); err != nil {
		return CaptureRequest{}, fmt.Errorf("record capture request: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return CaptureRequest{}, fmt.Errorf("commit capture request: %w", err)
	}
	return CaptureRequest{
		ID: id, PairID: pairID, RequesterDeviceID: requesterDeviceID, TargetDeviceID: targetDeviceID,
		Status: "PENDING", CreatedAt: now, ExpiresAt: expiresAt,
	}, nil
}

// CreateCaptureRequests creates one request for every capture-capable device on
// the other side. The whole fan-out counts as one user action for rate limiting.
func (s *Store) CreateCaptureRequests(ctx context.Context, requesterDeviceID string) ([]CaptureRequest, error) {
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin capture requests: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var pairID string
	var requesterSlot int
	if err := tx.QueryRowContext(ctx, `SELECT pair_id, slot FROM devices WHERE id=? AND revoked_at IS NULL`, requesterDeviceID).Scan(&pairID, &requesterSlot); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPartnerNotFound
	} else if err != nil {
		return nil, fmt.Errorf("find capture requester: %w", err)
	}

	rows, err := tx.QueryContext(ctx, `
        SELECT id, name, platform FROM devices
        WHERE pair_id=? AND slot<>? AND revoked_at IS NULL
          AND instr(','||capabilities||',', ',capture,')>0
        ORDER BY created_at, id`, pairID, requesterSlot)
	if err != nil {
		return nil, fmt.Errorf("find capture targets: %w", err)
	}
	var targets []CaptureRequest
	for rows.Next() {
		var target CaptureRequest
		if err := rows.Scan(&target.TargetDeviceID, &target.TargetDeviceName, &target.TargetPlatform); err != nil {
			_ = rows.Close()
			return nil, err
		}
		targets = append(targets, target)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		return nil, ErrPartnerNotFound
	}

	var recent, hourly int
	if err := tx.QueryRowContext(ctx, `
        SELECT COUNT(CASE WHEN created_at > ? THEN 1 END), COUNT(*)
        FROM audit_events
        WHERE device_id=? AND event_type='capture.requested' AND created_at>?`,
		now.Add(-10*time.Second).Unix(), requesterDeviceID, now.Add(-time.Hour).Unix(),
	).Scan(&recent, &hourly); err != nil {
		return nil, fmt.Errorf("count capture requests: %w", err)
	}
	if recent > 0 || hourly >= 60 {
		return nil, ErrRateLimited
	}

	expiresAt := now.Add(time.Minute)
	for index := range targets {
		id, err := secret.Generate(16)
		if err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO capture_requests
            (id,pair_id,requester_device_id,target_device_id,status,created_at,expires_at)
            VALUES(?,?,?,?,'PENDING',?,?)`, id, pairID, requesterDeviceID, targets[index].TargetDeviceID, now.Unix(), expiresAt.Unix()); err != nil {
			return nil, fmt.Errorf("insert capture request: %w", err)
		}
		targets[index].ID = id
		targets[index].PairID = pairID
		targets[index].RequesterDeviceID = requesterDeviceID
		targets[index].Status = "PENDING"
		targets[index].CreatedAt = now
		targets[index].ExpiresAt = expiresAt
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(pair_id,device_id,event_type,created_at) VALUES(?,?,'capture.requested',?)`, pairID, requesterDeviceID, now.Unix()); err != nil {
		return nil, fmt.Errorf("record capture request: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit capture requests: %w", err)
	}
	return targets, nil
}

func (s *Store) CompleteCaptureRequest(
	ctx context.Context,
	targetDeviceID string,
	requestID string,
	status string,
	imageID string,
	failure string,
) (CaptureRequest, error) {
	images := []CaptureImage(nil)
	if imageID != "" {
		images = []CaptureImage{{ImageID: imageID}}
	}
	return s.CompleteCaptureRequestWithImages(ctx, targetDeviceID, requestID, status, images, failure)
}

func (s *Store) CompleteCaptureRequestWithImages(ctx context.Context, targetDeviceID, requestID, status string, images []CaptureImage, failure string) (CaptureRequest, error) {
	if status != "READY" && status != "FAILED" {
		return CaptureRequest{}, ErrCaptureRequestNotFound
	}
	if (status == "READY") != (len(images) > 0) || (status == "FAILED") != (failure != "") || len(images) > 8 {
		return CaptureRequest{}, ErrCaptureRequestNotFound
	}
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CaptureRequest{}, fmt.Errorf("begin complete capture: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var result CaptureRequest
	var createdAt, expiresAt int64
	err = tx.QueryRowContext(ctx, `
        SELECT id, pair_id, requester_device_id, target_device_id, created_at, expires_at
        FROM capture_requests
        WHERE id = ? AND target_device_id = ? AND status = 'PENDING' AND expires_at > ?`,
		requestID, targetDeviceID, now.Unix(),
	).Scan(&result.ID, &result.PairID, &result.RequesterDeviceID, &result.TargetDeviceID, &createdAt, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return CaptureRequest{}, ErrCaptureRequestNotFound
	}
	if err != nil {
		return CaptureRequest{}, fmt.Errorf("find capture request: %w", err)
	}
	if status == "READY" {
		seen := map[string]bool{}
		for _, captureImage := range images {
			if captureImage.ImageID == "" || seen[captureImage.ImageID] || utf8.RuneCountInString(captureImage.DisplayName) > 120 {
				return CaptureRequest{}, ErrCaptureRequestNotFound
			}
			seen[captureImage.ImageID] = true
			var exists int
			err := tx.QueryRowContext(ctx, `
            SELECT 1 FROM images
            WHERE id = ? AND uploader_device_id = ? AND pair_id = ? AND expires_at > ?`,
				captureImage.ImageID, targetDeviceID, result.PairID, now.Unix(),
			).Scan(&exists)
			if errors.Is(err, sql.ErrNoRows) {
				return CaptureRequest{}, ErrCaptureRequestNotFound
			}
			if err != nil {
				return CaptureRequest{}, fmt.Errorf("validate capture image: %w", err)
			}
		}
	}
	legacyImageID := ""
	if len(images) == 1 {
		legacyImageID = images[0].ImageID
	}
	update, err := tx.ExecContext(ctx, `
        UPDATE capture_requests SET status = ?, image_id = NULLIF(?, ''), failure = NULLIF(?, ''), completed_at = ?
		WHERE id = ? AND status = 'PENDING'`, status, legacyImageID, failure, now.Unix(), requestID,
	)
	if err != nil {
		return CaptureRequest{}, fmt.Errorf("complete capture request: %w", err)
	}
	rows, err := update.RowsAffected()
	if err != nil || rows != 1 {
		return CaptureRequest{}, ErrCaptureRequestNotFound
	}
	for position, item := range images {
		if _, err := tx.ExecContext(ctx, `INSERT INTO capture_request_images(request_id,image_id,display_name,position) VALUES(?,?,?,?)`, requestID, item.ImageID, item.DisplayName, position); err != nil {
			return CaptureRequest{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `
        INSERT INTO audit_events (pair_id, device_id, event_type, created_at)
        VALUES (?, ?, ?, ?)`, result.PairID, targetDeviceID, "capture."+strings.ToLower(status), now.Unix(),
	); err != nil {
		return CaptureRequest{}, fmt.Errorf("record capture result: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return CaptureRequest{}, fmt.Errorf("commit capture result: %w", err)
	}
	result.Status, result.ImageID, result.Failure, result.Images = status, legacyImageID, failure, images
	result.CreatedAt, result.ExpiresAt = time.Unix(createdAt, 0).UTC(), time.Unix(expiresAt, 0).UTC()
	return result, nil
}

func (s *Store) ExpireCaptureRequests(ctx context.Context) (int, error) {
	now := s.now().UTC().Unix()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin expire captures: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `
        SELECT id, pair_id, requester_device_id
        FROM capture_requests WHERE status = 'PENDING' AND expires_at <= ?`, now)
	if err != nil {
		return 0, fmt.Errorf("list expired captures: %w", err)
	}
	type expiredCapture struct{ id, pairID, requesterDeviceID string }
	var expired []expiredCapture
	for rows.Next() {
		var item expiredCapture
		if err := rows.Scan(&item.id, &item.pairID, &item.requesterDeviceID); err != nil {
			_ = rows.Close()
			return 0, fmt.Errorf("scan expired capture: %w", err)
		}
		expired = append(expired, item)
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("close expired captures: %w", err)
	}
	for _, item := range expired {
		result, err := tx.ExecContext(ctx, `
            UPDATE capture_requests SET status = 'TIMEOUT', failure = 'TIMEOUT', completed_at = ?
            WHERE id = ? AND status = 'PENDING'`, now, item.id)
		if err != nil {
			return 0, fmt.Errorf("expire capture: %w", err)
		}
		count, err := result.RowsAffected()
		if err != nil {
			return 0, fmt.Errorf("count expired capture: %w", err)
		}
		if count == 1 {
			if _, err := tx.ExecContext(ctx, `
                INSERT INTO audit_events (pair_id, device_id, event_type, created_at)
                VALUES (?, ?, 'capture.timeout', ?)`, item.pairID, item.requesterDeviceID, now); err != nil {
				return 0, fmt.Errorf("record capture timeout: %w", err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit expired captures: %w", err)
	}
	return len(expired), nil
}

func (s *Store) SaveImage(ctx context.Context, deviceID string, data []byte, width, height int) (Image, error) {
	now := s.now().UTC()
	imageID, err := secret.Generate(16)
	if err != nil {
		return Image{}, err
	}
	var pairID string
	if err := s.db.QueryRowContext(ctx, "SELECT pair_id FROM devices WHERE id = ?", deviceID).Scan(&pairID); errors.Is(err, sql.ErrNoRows) {
		return Image{}, ErrUnauthorized
	} else if err != nil {
		return Image{}, fmt.Errorf("find image uploader: %w", err)
	}

	imageDir := filepath.Join(s.dataDir, "images")
	if err := os.MkdirAll(imageDir, 0o700); err != nil {
		return Image{}, fmt.Errorf("create image directory: %w", err)
	}
	fileName := imageID + ".jpg"
	path := filepath.Join(imageDir, fileName)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return Image{}, fmt.Errorf("write image: %w", err)
	}
	expiresAt := now.Add(time.Hour)
	_, err = s.db.ExecContext(ctx, `
        INSERT INTO images (id, pair_id, uploader_device_id, file_name, size_bytes, width, height, created_at, expires_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		imageID, pairID, deviceID, fileName, len(data), width, height, now.Unix(), expiresAt.Unix(),
	)
	if err != nil {
		_ = os.Remove(path)
		return Image{}, fmt.Errorf("record image: %w", err)
	}
	return Image{ID: imageID, CreatedAt: now, ExpiresAt: expiresAt}, nil
}

func (s *Store) TakeImage(ctx context.Context, deviceID, imageID string) (Image, error) {
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Image{}, fmt.Errorf("begin take image: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var fileName string
	var createdAt, expiresAt int64
	err = tx.QueryRowContext(ctx, `
        SELECT images.file_name, images.created_at, images.expires_at
        FROM images
        JOIN devices receiver ON receiver.pair_id = images.pair_id
        JOIN devices uploader ON uploader.id = images.uploader_device_id
        WHERE images.id = ? AND receiver.id = ? AND receiver.revoked_at IS NULL
          AND uploader.slot <> receiver.slot AND images.expires_at > ?`,
		imageID, deviceID, now.Unix(),
	).Scan(&fileName, &createdAt, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Image{}, ErrImageNotFound
	}
	if err != nil {
		return Image{}, fmt.Errorf("find image: %w", err)
	}
	result, err := tx.ExecContext(ctx, "DELETE FROM images WHERE id = ?", imageID)
	if err != nil {
		return Image{}, fmt.Errorf("claim image: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return Image{}, ErrImageNotFound
	}
	path := filepath.Join(s.dataDir, "images", fileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return Image{}, fmt.Errorf("read image: %w", err)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Image{}, fmt.Errorf("delete claimed image: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Image{}, fmt.Errorf("commit take image: %w", err)
	}
	return Image{ID: imageID, Data: data, CreatedAt: time.Unix(createdAt, 0).UTC(), ExpiresAt: time.Unix(expiresAt, 0).UTC()}, nil
}

func (s *Store) DeleteExpiredImages(ctx context.Context) (int, error) {
	now := s.now().UTC().Unix()
	rows, err := s.db.QueryContext(ctx, "SELECT id, file_name FROM images WHERE expires_at <= ?", now)
	if err != nil {
		return 0, fmt.Errorf("list expired images: %w", err)
	}
	type expiredImage struct{ id, fileName string }
	var expired []expiredImage
	for rows.Next() {
		var item expiredImage
		if err := rows.Scan(&item.id, &item.fileName); err != nil {
			_ = rows.Close()
			return 0, fmt.Errorf("scan expired image: %w", err)
		}
		expired = append(expired, item)
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("close expired image rows: %w", err)
	}
	for _, item := range expired {
		if _, err := s.db.ExecContext(ctx, "DELETE FROM images WHERE id = ? AND expires_at <= ?", item.id, now); err != nil {
			return 0, fmt.Errorf("delete expired image record: %w", err)
		}
		if err := os.Remove(filepath.Join(s.dataDir, "images", item.fileName)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return 0, fmt.Errorf("delete expired image file: %w", err)
		}
	}
	return len(expired), nil
}

func (s *Store) CreateForwardedNotification(ctx context.Context, sourceDeviceID string, item ForwardedNotification) (ForwardedNotification, []string, error) {
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ForwardedNotification{}, nil, fmt.Errorf("begin forwarded notification: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var pairID, sourceDeviceName string
	var sourceSlot int
	if err := tx.QueryRowContext(ctx, `SELECT pair_id, slot, name FROM devices WHERE id=?`, sourceDeviceID).Scan(&pairID, &sourceSlot, &sourceDeviceName); errors.Is(err, sql.ErrNoRows) {
		return ForwardedNotification{}, nil, ErrUnauthorized
	} else if err != nil {
		return ForwardedNotification{}, nil, fmt.Errorf("find notification source: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `SELECT id FROM devices WHERE pair_id=? AND slot<>? AND revoked_at IS NULL AND instr(','||capabilities||',', ',notification.receive,')>0 ORDER BY created_at`, pairID, sourceSlot)
	if err != nil {
		return ForwardedNotification{}, nil, fmt.Errorf("find notification targets: %w", err)
	}
	var targets []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return ForwardedNotification{}, nil, err
		}
		targets = append(targets, id)
	}
	if err := rows.Close(); err != nil {
		return ForwardedNotification{}, nil, err
	}
	if len(targets) == 0 {
		return ForwardedNotification{}, nil, ErrPartnerNotFound
	}
	blocked, err := notificationBlockedTx(ctx, tx, pairID, 3-sourceSlot, sourceDeviceID, item)
	if err != nil {
		return ForwardedNotification{}, nil, err
	}
	var recent, hourly int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(CASE WHEN created_at>? THEN 1 END), COUNT(*) FROM forwarded_notifications WHERE source_device_id=? AND created_at>?`, now.Add(-10*time.Second).Unix(), sourceDeviceID, now.Add(-time.Hour).Unix()).Scan(&recent, &hourly); err != nil {
		return ForwardedNotification{}, nil, fmt.Errorf("count forwarded notifications: %w", err)
	}
	if recent >= 20 || hourly >= 500 {
		return ForwardedNotification{}, nil, ErrRateLimited
	}
	id, err := secret.Generate(16)
	if err != nil {
		return ForwardedNotification{}, nil, err
	}
	item.ID, item.PairID, item.SourceDeviceID, item.SourceDeviceName = id, pairID, sourceDeviceID, sourceDeviceName
	item.CreatedAt, item.ExpiresAt = now, now.Add(time.Hour)
	if blocked {
		if err := tx.Commit(); err != nil {
			return ForwardedNotification{}, nil, fmt.Errorf("commit filtered notification: %w", err)
		}
		return item, []string{}, nil
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO forwarded_notifications(id,pair_id,source_device_id,source_package,source_app_name,title,body,posted_at,created_at,expires_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, item.ID, item.PairID, item.SourceDeviceID, item.SourcePackage, item.SourceAppName, item.Title, item.Body, item.PostedAt.UTC().Unix(), item.CreatedAt.Unix(), item.ExpiresAt.Unix()); err != nil {
		return ForwardedNotification{}, nil, fmt.Errorf("insert forwarded notification: %w", err)
	}
	for _, target := range targets {
		if _, err := tx.ExecContext(ctx, `INSERT INTO forwarded_notification_deliveries(notification_id,device_id) VALUES(?,?)`, id, target); err != nil {
			return ForwardedNotification{}, nil, fmt.Errorf("insert notification delivery: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(pair_id,device_id,event_type,created_at) VALUES(?,?,'notification.forwarded',?)`, pairID, sourceDeviceID, now.Unix()); err != nil {
		return ForwardedNotification{}, nil, err
	}
	if err := tx.Commit(); err != nil {
		return ForwardedNotification{}, nil, fmt.Errorf("commit forwarded notification: %w", err)
	}
	return item, targets, nil
}

func (s *Store) PendingForwardedNotifications(ctx context.Context, deviceID string) ([]ForwardedNotification, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT n.id,n.pair_id,n.source_device_id,COALESCE(sd.name,''),n.source_package,n.source_app_name,n.title,n.body,n.posted_at,n.created_at,n.expires_at FROM forwarded_notifications n JOIN forwarded_notification_deliveries d ON d.notification_id=n.id LEFT JOIN devices sd ON sd.id=n.source_device_id WHERE d.device_id=? AND d.delivered_at IS NULL AND n.expires_at>? ORDER BY n.created_at`, deviceID, s.now().UTC().Unix())
	if err != nil {
		return nil, fmt.Errorf("list forwarded notifications: %w", err)
	}
	defer rows.Close()
	var result []ForwardedNotification
	for rows.Next() {
		var item ForwardedNotification
		var posted, created, expires int64
		if err := rows.Scan(&item.ID, &item.PairID, &item.SourceDeviceID, &item.SourceDeviceName, &item.SourcePackage, &item.SourceAppName, &item.Title, &item.Body, &posted, &created, &expires); err != nil {
			return nil, err
		}
		item.PostedAt, item.CreatedAt, item.ExpiresAt = time.Unix(posted, 0).UTC(), time.Unix(created, 0).UTC(), time.Unix(expires, 0).UTC()
		result = append(result, item)
	}
	return result, rows.Err()
}

func normalizedNotificationText(value string) string { return strings.ToLower(norm.NFKC.String(value)) }
func notificationTextMatches(value, pattern, mode string) bool {
	value, pattern = normalizedNotificationText(value), normalizedNotificationText(pattern)
	if mode == "EXACT" {
		return value == pattern
	}
	return strings.Contains(value, pattern)
}

func notificationBlockedTx(ctx context.Context, tx *sql.Tx, pairID string, ownerSlot int, sourceDeviceID string, item ForwardedNotification) (bool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT source_device_id,source_package,title_pattern,title_match,message_pattern,message_match FROM notification_filters WHERE pair_id=? AND owner_slot=? AND enabled=1`, pairID, ownerSlot)
	if err != nil {
		return false, fmt.Errorf("list notification filters: %w", err)
	}
	defer rows.Close()
	message := item.Title + "\n" + item.Body
	for rows.Next() {
		var device, pkg, titlePattern, titleMatch, messagePattern, messageMatch string
		if err := rows.Scan(&device, &pkg, &titlePattern, &titleMatch, &messagePattern, &messageMatch); err != nil {
			return false, err
		}
		if device != "" && device != sourceDeviceID {
			continue
		}
		if pkg != "" && pkg != item.SourcePackage {
			continue
		}
		if titlePattern != "" && !notificationTextMatches(item.Title, titlePattern, titleMatch) {
			continue
		}
		if messagePattern != "" && !notificationTextMatches(message, messagePattern, messageMatch) {
			continue
		}
		return true, nil
	}
	return false, rows.Err()
}

func (s *Store) ListNotificationFilters(ctx context.Context, deviceID string) ([]NotificationFilter, error) {
	pairID, slot, err := s.devicePairSlot(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,pair_id,owner_slot,source_device_id,source_device_name,source_package,source_app_name,title_pattern,title_match,message_pattern,message_match,enabled,created_at,updated_at FROM notification_filters WHERE pair_id=? AND owner_slot=? ORDER BY created_at`, pairID, slot)
	if err != nil {
		return nil, fmt.Errorf("list notification filters: %w", err)
	}
	defer rows.Close()
	var result []NotificationFilter
	for rows.Next() {
		var v NotificationFilter
		var enabled int
		var created, updated int64
		if err := rows.Scan(&v.ID, &v.PairID, &v.OwnerSlot, &v.SourceDeviceID, &v.SourceDeviceName, &v.SourcePackage, &v.SourceAppName, &v.TitlePattern, &v.TitleMatch, &v.MessagePattern, &v.MessageMatch, &enabled, &created, &updated); err != nil {
			return nil, err
		}
		v.Enabled = enabled != 0
		v.CreatedAt = time.Unix(created, 0).UTC()
		v.UpdatedAt = time.Unix(updated, 0).UTC()
		result = append(result, v)
	}
	return result, rows.Err()
}

func (s *Store) SaveNotificationFilter(ctx context.Context, deviceID string, value NotificationFilter) (NotificationFilter, error) {
	pairID, slot, err := s.devicePairSlot(ctx, deviceID)
	if err != nil {
		return NotificationFilter{}, err
	}
	value.SourceDeviceID = strings.TrimSpace(value.SourceDeviceID)
	value.SourceDeviceName = strings.TrimSpace(value.SourceDeviceName)
	value.SourcePackage = strings.TrimSpace(value.SourcePackage)
	value.SourceAppName = strings.TrimSpace(value.SourceAppName)
	value.TitlePattern, value.MessagePattern = strings.TrimSpace(value.TitlePattern), strings.TrimSpace(value.MessagePattern)
	if value.TitleMatch == "" {
		value.TitleMatch = "CONTAINS"
	}
	if value.MessageMatch == "" {
		value.MessageMatch = "CONTAINS"
	}
	if (value.TitleMatch != "CONTAINS" && value.TitleMatch != "EXACT") || (value.MessageMatch != "CONTAINS" && value.MessageMatch != "EXACT") {
		return NotificationFilter{}, errors.New("invalid notification match mode")
	}
	if value.SourceDeviceID == "" && value.SourcePackage == "" && value.TitlePattern == "" && value.MessagePattern == "" {
		return NotificationFilter{}, errors.New("notification filter requires a condition")
	}
	if value.SourceDeviceID != "" {
		var count int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM devices WHERE id=? AND pair_id=? AND slot<>? AND revoked_at IS NULL`, value.SourceDeviceID, pairID, slot).Scan(&count); err != nil {
			return NotificationFilter{}, err
		}
		if count != 1 {
			return NotificationFilter{}, ErrNotificationFilterNotFound
		}
	}
	now := s.now().UTC()
	value.PairID = pairID
	value.OwnerSlot = slot
	value.UpdatedAt = now
	if value.ID == "" {
		value.ID, err = secret.Generate(16)
		if err != nil {
			return NotificationFilter{}, err
		}
		value.CreatedAt = now
		_, err = s.db.ExecContext(ctx, `INSERT INTO notification_filters(id,pair_id,owner_slot,source_device_id,source_device_name,source_package,source_app_name,title_pattern,title_match,message_pattern,message_match,enabled,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, value.ID, pairID, slot, value.SourceDeviceID, value.SourceDeviceName, value.SourcePackage, value.SourceAppName, value.TitlePattern, value.TitleMatch, value.MessagePattern, value.MessageMatch, boolInt(value.Enabled), now.Unix(), now.Unix())
	} else {
		var created int64
		err = s.db.QueryRowContext(ctx, `SELECT created_at FROM notification_filters WHERE id=? AND pair_id=? AND owner_slot=?`, value.ID, pairID, slot).Scan(&created)
		if errors.Is(err, sql.ErrNoRows) {
			return NotificationFilter{}, ErrNotificationFilterNotFound
		}
		if err == nil {
			value.CreatedAt = time.Unix(created, 0).UTC()
			_, err = s.db.ExecContext(ctx, `UPDATE notification_filters SET source_device_id=?,source_device_name=?,source_package=?,source_app_name=?,title_pattern=?,title_match=?,message_pattern=?,message_match=?,enabled=?,updated_at=? WHERE id=? AND pair_id=? AND owner_slot=?`, value.SourceDeviceID, value.SourceDeviceName, value.SourcePackage, value.SourceAppName, value.TitlePattern, value.TitleMatch, value.MessagePattern, value.MessageMatch, boolInt(value.Enabled), now.Unix(), value.ID, pairID, slot)
		}
	}
	if err != nil {
		return NotificationFilter{}, fmt.Errorf("save notification filter: %w", err)
	}
	return value, nil
}

func (s *Store) DeleteNotificationFilter(ctx context.Context, deviceID, filterID string) error {
	pairID, slot, err := s.devicePairSlot(ctx, deviceID)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM notification_filters WHERE id=? AND pair_id=? AND owner_slot=?`, filterID, pairID, slot)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return ErrNotificationFilterNotFound
	}
	return nil
}
func (s *Store) devicePairSlot(ctx context.Context, deviceID string) (string, int, error) {
	var pair string
	var slot int
	err := s.db.QueryRowContext(ctx, `SELECT pair_id,slot FROM devices WHERE id=? AND revoked_at IS NULL`, deviceID).Scan(&pair, &slot)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, ErrUnauthorized
	}
	return pair, slot, err
}
func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (s *Store) AcknowledgeForwardedNotification(ctx context.Context, deviceID, notificationID string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE forwarded_notification_deliveries SET delivered_at=? WHERE notification_id=? AND device_id=? AND delivered_at IS NULL`, s.now().UTC().Unix(), notificationID, deviceID)
	if err != nil {
		return fmt.Errorf("acknowledge forwarded notification: %w", err)
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return ErrForwardedNotificationNotFound
	}
	return nil
}

func (s *Store) DeleteExpiredForwardedNotifications(ctx context.Context) (int, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM forwarded_notifications WHERE expires_at<=?`, s.now().UTC().Unix())
	if err != nil {
		return 0, fmt.Errorf("delete expired forwarded notifications: %w", err)
	}
	count, err := result.RowsAffected()
	return int(count), err
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) initialize(ctx context.Context) error {
	for _, pragma := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		"PRAGMA busy_timeout = 5000",
	} {
		if _, err := s.db.ExecContext(ctx, pragma); err != nil {
			return fmt.Errorf("configure database: %w", err)
		}
	}
	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	if err := s.migrateMultiDevice(ctx); err != nil {
		return err
	}
	for name, definition := range map[string]string{"location_status": "TEXT NOT NULL DEFAULT 'DISABLED'", "latitude": "REAL", "longitude": "REAL", "accuracy_meters": "REAL", "location_observed_at": "INTEGER", "location_source": "TEXT"} {
		if _, err := s.db.ExecContext(ctx, "ALTER TABLE status_snapshots ADD COLUMN "+name+" "+definition); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			return fmt.Errorf("migrate status snapshots: %w", err)
		}
	}
	return nil
}

func (s *Store) migrateMultiDevice(ctx context.Context) error {
	var invitationsSQL, devicesSQL string
	if err := s.db.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type='table' AND name='invitations'`).Scan(&invitationsSQL); err != nil {
		return fmt.Errorf("inspect invitations schema: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type='table' AND name='devices'`).Scan(&devicesSQL); err != nil {
		return fmt.Errorf("inspect devices schema: %w", err)
	}
	normalizedInvitations := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(invitationsSQL, " ", ""), "\n", ""))
	normalizedDevices := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(devicesSQL, " ", ""), "\n", ""))
	oldUnique := strings.Contains(normalizedInvitations, "unique(pair_id,slot)") || strings.Contains(normalizedDevices, "unique(pair_id,slot)")
	if !oldUnique {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA foreign_keys=OFF`); err != nil {
		return err
	}
	defer func() { _, _ = s.db.ExecContext(context.Background(), `PRAGMA foreign_keys=ON`) }()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	statements := []string{
		`CREATE TABLE invitations_new (token_hash BLOB PRIMARY KEY, pair_id TEXT NOT NULL REFERENCES pairs(id) ON DELETE CASCADE, slot INTEGER NOT NULL CHECK(slot IN (1,2)), expires_at INTEGER NOT NULL, used_at INTEGER, created_at INTEGER NOT NULL)`,
		`INSERT INTO invitations_new SELECT token_hash,pair_id,slot,expires_at,used_at,created_at FROM invitations`,
		`CREATE TABLE devices_new (id TEXT PRIMARY KEY, pair_id TEXT NOT NULL REFERENCES pairs(id) ON DELETE CASCADE, slot INTEGER NOT NULL CHECK(slot IN (1,2)), name TEXT NOT NULL, public_key TEXT NOT NULL UNIQUE, credential_hash BLOB NOT NULL UNIQUE, created_at INTEGER NOT NULL, platform TEXT NOT NULL DEFAULT 'ANDROID', capabilities TEXT NOT NULL DEFAULT 'notification.send,notification.receive,capture,status', last_seen_at INTEGER, revoked_at INTEGER)`,
		`INSERT INTO devices_new(id,pair_id,slot,name,public_key,credential_hash,created_at) SELECT id,pair_id,slot,name,public_key,credential_hash,created_at FROM devices`,
		`DROP TABLE invitations`, `DROP TABLE devices`,
		`ALTER TABLE invitations_new RENAME TO invitations`, `ALTER TABLE devices_new RENAME TO devices`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("multi-device migration: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit multi-device migration: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA foreign_keys=ON`); err != nil {
		return err
	}
	var violation string
	if err := s.db.QueryRowContext(ctx, `PRAGMA foreign_key_check`).Scan(&violation); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check multi-device migration: %w", err)
	} else if err == nil {
		return errors.New("multi-device migration produced a foreign key violation")
	}
	return nil
}

func (s *Store) CreatePair(ctx context.Context, name string, expiresAt time.Time) (PairInvitations, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 80 {
		return PairInvitations{}, errors.New("pair name must contain 1 to 80 bytes")
	}
	now := s.now().UTC()
	if !expiresAt.After(now) {
		return PairInvitations{}, errors.New("invitation expiry must be in the future")
	}

	pairID, err := secret.Generate(16)
	if err != nil {
		return PairInvitations{}, err
	}
	result := PairInvitations{PairID: pairID, PairName: name, ExpiresAt: expiresAt.UTC()}
	for index := range result.Invitations {
		result.Invitations[index], err = secret.Generate(32)
		if err != nil {
			return PairInvitations{}, err
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PairInvitations{}, fmt.Errorf("begin create pair: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		"INSERT INTO pairs (id, name, created_at) VALUES (?, ?, ?)",
		pairID, name, now.Unix(),
	); err != nil {
		return PairInvitations{}, fmt.Errorf("insert pair: %w", err)
	}
	for index, invitation := range result.Invitations {
		hash := secret.Hash(invitation)
		if _, err := tx.ExecContext(ctx, `
            INSERT INTO invitations (token_hash, pair_id, slot, expires_at, created_at)
            VALUES (?, ?, ?, ?, ?)`,
			hash[:], pairID, index+1, expiresAt.Unix(), now.Unix(),
		); err != nil {
			return PairInvitations{}, fmt.Errorf("insert invitation: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return PairInvitations{}, fmt.Errorf("commit create pair: %w", err)
	}
	return result, nil
}

func (s *Store) DeletePair(ctx context.Context, pairID string) error {
	pairID = strings.TrimSpace(pairID)
	if pairID == "" {
		return ErrPairNotFound
	}
	rows, err := s.db.QueryContext(ctx, "SELECT file_name FROM images WHERE pair_id = ?", pairID)
	if err != nil {
		return fmt.Errorf("list pair images: %w", err)
	}
	var files []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			return err
		}
		files = append(files, name)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, "DELETE FROM pairs WHERE id = ?", pairID)
	if err != nil {
		return fmt.Errorf("delete pair: %w", err)
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return ErrPairNotFound
	}
	for _, name := range files {
		if err := os.Remove(filepath.Join(s.dataDir, "images", name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("delete pair image file: %w", err)
		}
	}
	return nil
}

func (s *Store) ListPairs(ctx context.Context) ([]PairSummary, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT pairs.id, pairs.name, pairs.created_at, COUNT(devices.id) FROM pairs LEFT JOIN devices ON devices.pair_id = pairs.id GROUP BY pairs.id, pairs.name, pairs.created_at ORDER BY pairs.created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list pairs: %w", err)
	}
	defer rows.Close()
	var result []PairSummary
	for rows.Next() {
		var item PairSummary
		var created int64
		if err := rows.Scan(&item.PairID, &item.PairName, &created, &item.DeviceCount); err != nil {
			return nil, err
		}
		item.CreatedAt = time.Unix(created, 0).UTC()
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Store) PendingCaptureRequests(ctx context.Context, deviceID string) ([]CaptureRequest, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, pair_id, requester_device_id, target_device_id, created_at, expires_at FROM capture_requests WHERE target_device_id = ? AND status = 'PENDING' AND expires_at > ? ORDER BY created_at`, deviceID, s.now().UTC().Unix())
	if err != nil {
		return nil, fmt.Errorf("list pending captures: %w", err)
	}
	defer rows.Close()
	var result []CaptureRequest
	for rows.Next() {
		var item CaptureRequest
		var created, expires int64
		if err := rows.Scan(&item.ID, &item.PairID, &item.RequesterDeviceID, &item.TargetDeviceID, &created, &expires); err != nil {
			return nil, err
		}
		item.Status = "PENDING"
		item.CreatedAt = time.Unix(created, 0).UTC()
		item.ExpiresAt = time.Unix(expires, 0).UTC()
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) CaptureRequestForRequester(ctx context.Context, deviceID, requestID string) (CaptureRequest, error) {
	var v CaptureRequest
	var created, expires int64
	var image, failure sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT id,pair_id,requester_device_id,target_device_id,status,image_id,failure,created_at,expires_at FROM capture_requests WHERE id=? AND requester_device_id=?`, requestID, deviceID).Scan(&v.ID, &v.PairID, &v.RequesterDeviceID, &v.TargetDeviceID, &v.Status, &image, &failure, &created, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return CaptureRequest{}, ErrCaptureRequestNotFound
	}
	if err != nil {
		return CaptureRequest{}, err
	}
	v.ImageID = image.String
	v.Failure = failure.String
	v.CreatedAt = time.Unix(created, 0).UTC()
	v.ExpiresAt = time.Unix(expires, 0).UTC()
	rows, err := s.db.QueryContext(ctx, `SELECT image_id,display_name FROM capture_request_images WHERE request_id=? ORDER BY position`, requestID)
	if err != nil {
		return CaptureRequest{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var image CaptureImage
		if err := rows.Scan(&image.ImageID, &image.DisplayName); err != nil {
			return CaptureRequest{}, err
		}
		v.Images = append(v.Images, image)
	}
	if err := rows.Err(); err != nil {
		return CaptureRequest{}, err
	}
	return v, nil
}

func (s *Store) CreateStatusRequest(ctx context.Context, requesterDeviceID string) (StatusRequest, error) {
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return StatusRequest{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var pairID, targetID string
	err = tx.QueryRowContext(ctx, `SELECT a.pair_id, b.id FROM devices a JOIN devices b ON b.pair_id=a.pair_id AND b.id<>a.id WHERE a.id=?`, requesterDeviceID).Scan(&pairID, &targetID)
	if errors.Is(err, sql.ErrNoRows) {
		return StatusRequest{}, ErrPartnerNotFound
	}
	if err != nil {
		return StatusRequest{}, fmt.Errorf("find status target: %w", err)
	}
	var recent, hourly int
	err = tx.QueryRowContext(ctx, `SELECT COUNT(CASE WHEN created_at > ? THEN 1 END), COUNT(*) FROM status_requests WHERE requester_device_id=? AND created_at>?`, now.Add(-time.Minute).Unix(), requesterDeviceID, now.Add(-time.Hour).Unix()).Scan(&recent, &hourly)
	if err != nil {
		return StatusRequest{}, fmt.Errorf("count status requests: %w", err)
	}
	if recent > 0 || hourly >= 20 {
		return StatusRequest{}, ErrRateLimited
	}
	id, err := secret.Generate(16)
	if err != nil {
		return StatusRequest{}, err
	}
	expires := now.Add(time.Minute)
	if _, err = tx.ExecContext(ctx, `INSERT INTO status_requests(id,pair_id,requester_device_id,target_device_id,status,created_at,expires_at) VALUES(?,?,?,?,'PENDING',?,?)`, id, pairID, requesterDeviceID, targetID, now.Unix(), expires.Unix()); err != nil {
		return StatusRequest{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO audit_events(pair_id,device_id,event_type,created_at) VALUES(?,?,'status.requested',?)`, pairID, requesterDeviceID, now.Unix()); err != nil {
		return StatusRequest{}, err
	}
	if err = tx.Commit(); err != nil {
		return StatusRequest{}, err
	}
	return StatusRequest{ID: id, PairID: pairID, RequesterDeviceID: requesterDeviceID, TargetDeviceID: targetID, Status: "PENDING", CreatedAt: now, ExpiresAt: expires}, nil
}

func (s *Store) PendingStatusRequests(ctx context.Context, deviceID string) ([]StatusRequest, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,pair_id,requester_device_id,target_device_id,created_at,expires_at FROM status_requests WHERE target_device_id=? AND status='PENDING' AND expires_at>? ORDER BY created_at`, deviceID, s.now().UTC().Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []StatusRequest
	for rows.Next() {
		var v StatusRequest
		var created, expires int64
		if err := rows.Scan(&v.ID, &v.PairID, &v.RequesterDeviceID, &v.TargetDeviceID, &created, &expires); err != nil {
			return nil, err
		}
		v.Status = "PENDING"
		v.CreatedAt = time.Unix(created, 0).UTC()
		v.ExpiresAt = time.Unix(expires, 0).UTC()
		result = append(result, v)
	}
	return result, rows.Err()
}

func (s *Store) CompleteStatusRequest(ctx context.Context, targetID, requestID string, battery BatteryReport, location LocationReport) (StatusRequest, error) {
	validCharging := battery.ChargingState == "CHARGING" || battery.ChargingState == "DISCHARGING" || battery.ChargingState == "FULL" || battery.ChargingState == "NOT_CHARGING" || battery.ChargingState == "UNKNOWN"
	validAvailable := battery.Status == "AVAILABLE" && battery.Percent >= 0 && battery.Percent <= 100 && validCharging
	validDisabled := battery.Status == "DISABLED" && battery.Percent == 0 && battery.ChargingState == ""
	if !validAvailable && !validDisabled {
		return StatusRequest{}, ErrStatusRequestNotFound
	}
	validLocationStatus := location.Status == "DISABLED" || location.Status == "PERMISSION_DENIED" || location.Status == "SETTING_OFF" || location.Status == "UNAVAILABLE" || location.Status == "TIMEOUT"
	validLocation := location.Status == "AVAILABLE" && location.Latitude >= -90 && location.Latitude <= 90 && location.Longitude >= -180 && location.Longitude <= 180 && location.AccuracyMeters >= 0 && location.AccuracyMeters <= 100000 && (location.Source == "FRESH" || location.Source == "LAST_KNOWN") && !location.ObservedAt.IsZero() && !location.ObservedAt.After(s.now().UTC().Add(time.Minute))
	if !validLocation && !validLocationStatus {
		return StatusRequest{}, ErrStatusRequestNotFound
	}
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return StatusRequest{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var v StatusRequest
	var created, expires int64
	err = tx.QueryRowContext(ctx, `SELECT id,pair_id,requester_device_id,target_device_id,created_at,expires_at FROM status_requests WHERE id=? AND target_device_id=? AND status='PENDING' AND expires_at>?`, requestID, targetID, now.Unix()).Scan(&v.ID, &v.PairID, &v.RequesterDeviceID, &v.TargetDeviceID, &created, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return StatusRequest{}, ErrStatusRequestNotFound
	}
	if err != nil {
		return StatusRequest{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE status_requests SET status='COMPLETED',completed_at=? WHERE id=? AND status='PENDING'`, now.Unix(), requestID); err != nil {
		return StatusRequest{}, err
	}
	percent, charging := any(nil), any(nil)
	if battery.Status == "AVAILABLE" {
		percent = battery.Percent
		charging = battery.ChargingState
	}
	lat, lon, accuracy, observed, source := any(nil), any(nil), any(nil), any(nil), any(nil)
	if location.Status == "AVAILABLE" {
		lat = location.Latitude
		lon = location.Longitude
		accuracy = location.AccuracyMeters
		observed = location.ObservedAt.UTC().Unix()
		source = location.Source
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO status_snapshots(device_id,pair_id,battery_status,battery_percent,charging_state,location_status,latitude,longitude,accuracy_meters,location_observed_at,location_source,reported_at,expires_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(device_id) DO UPDATE SET battery_status=excluded.battery_status,battery_percent=excluded.battery_percent,charging_state=excluded.charging_state,location_status=excluded.location_status,latitude=excluded.latitude,longitude=excluded.longitude,accuracy_meters=excluded.accuracy_meters,location_observed_at=excluded.location_observed_at,location_source=excluded.location_source,reported_at=excluded.reported_at,expires_at=excluded.expires_at`, targetID, v.PairID, battery.Status, percent, charging, location.Status, lat, lon, accuracy, observed, source, now.Unix(), now.Add(time.Hour).Unix()); err != nil {
		return StatusRequest{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO audit_events(pair_id,device_id,event_type,created_at) VALUES(?,?,'status.completed',?)`, v.PairID, targetID, now.Unix()); err != nil {
		return StatusRequest{}, err
	}
	if err = tx.Commit(); err != nil {
		return StatusRequest{}, err
	}
	v.Status = "COMPLETED"
	v.CreatedAt = time.Unix(created, 0).UTC()
	v.ExpiresAt = time.Unix(expires, 0).UTC()
	return v, nil
}

func (s *Store) PartnerStatusSnapshot(ctx context.Context, requesterID string) (StatusSnapshot, error) {
	var v StatusSnapshot
	var percent sql.NullInt64
	var charging sql.NullString
	var latitude, longitude, accuracy sql.NullFloat64
	var observed sql.NullInt64
	var source sql.NullString
	var reported, expires int64
	err := s.db.QueryRowContext(ctx, `SELECT s.device_id,s.battery_status,s.battery_percent,s.charging_state,s.location_status,s.latitude,s.longitude,s.accuracy_meters,s.location_observed_at,s.location_source,s.reported_at,s.expires_at FROM devices a JOIN devices b ON b.pair_id=a.pair_id AND b.id<>a.id JOIN status_snapshots s ON s.device_id=b.id WHERE a.id=? AND s.expires_at>?`, requesterID, s.now().UTC().Unix()).Scan(&v.DeviceID, &v.Battery.Status, &percent, &charging, &v.Location.Status, &latitude, &longitude, &accuracy, &observed, &source, &reported, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return StatusSnapshot{}, ErrStatusSnapshotNotFound
	}
	if err != nil {
		return StatusSnapshot{}, err
	}
	if percent.Valid {
		v.Battery.Percent = int(percent.Int64)
	}
	v.Battery.ChargingState = charging.String
	if latitude.Valid {
		v.Location.Latitude = latitude.Float64
	}
	if longitude.Valid {
		v.Location.Longitude = longitude.Float64
	}
	if accuracy.Valid {
		v.Location.AccuracyMeters = accuracy.Float64
	}
	if observed.Valid {
		v.Location.ObservedAt = time.Unix(observed.Int64, 0).UTC()
	}
	v.Location.Source = source.String
	v.ReportedAt = time.Unix(reported, 0).UTC()
	v.ExpiresAt = time.Unix(expires, 0).UTC()
	return v, nil
}

func (s *Store) ClearStatusField(ctx context.Context, deviceID, field string) error {
	var query string
	if field == "battery" {
		query = `UPDATE status_snapshots SET battery_status='DISABLED',battery_percent=NULL,charging_state=NULL WHERE device_id=?`
	} else if field == "location" {
		query = `UPDATE status_snapshots SET location_status='DISABLED',latitude=NULL,longitude=NULL,accuracy_meters=NULL,location_observed_at=NULL,location_source=NULL WHERE device_id=?`
	} else {
		return errors.New("invalid status field")
	}
	_, err := s.db.ExecContext(ctx, query, deviceID)
	return err
}

func (s *Store) ExpireStatusRequests(ctx context.Context) (int, error) {
	now := s.now().UTC().Unix()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `INSERT INTO audit_events(pair_id,device_id,event_type,created_at) SELECT pair_id,requester_device_id,'status.timeout',? FROM status_requests WHERE status='PENDING' AND expires_at<=?`, now, now); err != nil {
		return 0, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE status_requests SET status='TIMEOUT',completed_at=? WHERE status='PENDING' AND expires_at<=?`, now, now)
	if err != nil {
		return 0, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return int(count), nil
}

func (s *Store) EnrollDevice(
	ctx context.Context,
	invitationToken string,
	deviceName string,
	publicKey string,
) (Enrollment, error) {
	return s.EnrollDeviceWithMetadata(ctx, invitationToken, deviceName, publicKey, "ANDROID", "notification.send,notification.receive,capture,status")
}

func (s *Store) EnrollDeviceWithMetadata(ctx context.Context, invitationToken, deviceName, publicKey, platform, capabilities string) (Enrollment, error) {
	now := s.now().UTC()
	invitationHash := secret.Hash(invitationToken)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Enrollment{}, fmt.Errorf("begin enrollment: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var pairID string
	var slot int
	err = tx.QueryRowContext(ctx, `
        SELECT pair_id, slot
        FROM invitations
        WHERE token_hash = ? AND used_at IS NULL AND expires_at > ?`,
		invitationHash[:], now.Unix(),
	).Scan(&pairID, &slot)
	if errors.Is(err, sql.ErrNoRows) {
		return Enrollment{}, ErrInvitationNotFound
	}
	if err != nil {
		return Enrollment{}, fmt.Errorf("find invitation: %w", err)
	}

	deviceID, err := secret.Generate(16)
	if err != nil {
		return Enrollment{}, err
	}
	credential, err := secret.Generate(32)
	if err != nil {
		return Enrollment{}, err
	}
	credentialHash := secret.Hash(credential)

	if _, err := tx.ExecContext(ctx, `
        INSERT INTO devices (id, pair_id, slot, name, public_key, credential_hash, created_at, platform, capabilities)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		deviceID, pairID, slot, deviceName, publicKey, credentialHash[:], now.Unix(), platform, capabilities,
	); err != nil {
		return Enrollment{}, ErrInvitationNotFound
	}
	result, err := tx.ExecContext(ctx,
		"UPDATE invitations SET used_at = ? WHERE token_hash = ? AND used_at IS NULL",
		now.Unix(), invitationHash[:],
	)
	if err != nil {
		return Enrollment{}, fmt.Errorf("consume invitation: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil || rowsAffected != 1 {
		return Enrollment{}, ErrInvitationNotFound
	}
	if _, err := tx.ExecContext(ctx, `
        INSERT INTO audit_events (pair_id, device_id, event_type, created_at)
        VALUES (?, ?, 'device.enrolled', ?)`,
		pairID, deviceID, now.Unix(),
	); err != nil {
		return Enrollment{}, fmt.Errorf("record enrollment audit event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Enrollment{}, fmt.Errorf("commit enrollment: %w", err)
	}

	return Enrollment{
		DeviceID:   deviceID,
		PairID:     pairID,
		Slot:       slot,
		Credential: credential,
	}, nil
}
