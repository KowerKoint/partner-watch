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

	"github.com/kowerkoint/partner-watch/server/internal/secret"
	_ "modernc.org/sqlite"
)

var ErrInvitationNotFound = errors.New("invitation not found")
var ErrUnauthorized = errors.New("unauthorized")
var ErrImageNotFound = errors.New("image not found")
var ErrPartnerNotFound = errors.New("partner not found")
var ErrRateLimited = errors.New("capture request rate limited")
var ErrCaptureRequestNotFound = errors.New("capture request not found")
var ErrPairNotFound = errors.New("pair not found")

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

type Enrollment struct {
	DeviceID   string
	PairID     string
	Slot       int
	Credential string
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
	Status            string
	ImageID           string
	Failure           string
	CreatedAt         time.Time
	ExpiresAt         time.Time
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
    created_at INTEGER NOT NULL,
    UNIQUE (pair_id, slot)
);

CREATE TABLE IF NOT EXISTS devices (
    id TEXT PRIMARY KEY,
    pair_id TEXT NOT NULL REFERENCES pairs(id) ON DELETE CASCADE,
    slot INTEGER NOT NULL CHECK (slot IN (1, 2)),
    name TEXT NOT NULL,
    public_key TEXT NOT NULL UNIQUE,
    credential_hash BLOB NOT NULL UNIQUE,
    created_at INTEGER NOT NULL,
    UNIQUE (pair_id, slot)
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

CREATE INDEX IF NOT EXISTS audit_events_created_at_idx ON audit_events(created_at);
CREATE INDEX IF NOT EXISTS images_expires_at_idx ON images(expires_at);
CREATE INDEX IF NOT EXISTS capture_requests_requester_created_idx ON capture_requests(requester_device_id, created_at);
CREATE INDEX IF NOT EXISTS capture_requests_expires_at_idx ON capture_requests(expires_at);

CREATE TABLE IF NOT EXISTS device_fcm_tokens (
    device_id TEXT PRIMARY KEY REFERENCES devices(id) ON DELETE CASCADE,
    token TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);
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
	err := s.db.QueryRowContext(ctx, "SELECT id FROM devices WHERE credential_hash = ?", hash[:]).Scan(&deviceID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrUnauthorized
	}
	if err != nil {
		return "", fmt.Errorf("authenticate device: %w", err)
	}
	return deviceID, nil
}

func (s *Store) CreateCaptureRequest(ctx context.Context, requesterDeviceID string) (CaptureRequest, error) {
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CaptureRequest{}, fmt.Errorf("begin capture request: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var pairID, targetDeviceID string
	err = tx.QueryRowContext(ctx, `
        SELECT requester.pair_id, target.id
        FROM devices requester
        JOIN devices target ON target.pair_id = requester.pair_id AND target.id <> requester.id
        WHERE requester.id = ?`, requesterDeviceID,
	).Scan(&pairID, &targetDeviceID)
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
        FROM capture_requests
        WHERE requester_device_id = ? AND created_at > ?`,
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

func (s *Store) CompleteCaptureRequest(
	ctx context.Context,
	targetDeviceID string,
	requestID string,
	status string,
	imageID string,
	failure string,
) (CaptureRequest, error) {
	if status != "READY" && status != "FAILED" {
		return CaptureRequest{}, ErrCaptureRequestNotFound
	}
	if (status == "READY") != (imageID != "") || (status == "FAILED") != (failure != "") {
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
		var exists int
		err := tx.QueryRowContext(ctx, `
            SELECT 1 FROM images
            WHERE id = ? AND uploader_device_id = ? AND pair_id = ? AND expires_at > ?`,
			imageID, targetDeviceID, result.PairID, now.Unix(),
		).Scan(&exists)
		if errors.Is(err, sql.ErrNoRows) {
			return CaptureRequest{}, ErrCaptureRequestNotFound
		}
		if err != nil {
			return CaptureRequest{}, fmt.Errorf("validate capture image: %w", err)
		}
	}
	update, err := tx.ExecContext(ctx, `
        UPDATE capture_requests SET status = ?, image_id = NULLIF(?, ''), failure = NULLIF(?, ''), completed_at = ?
        WHERE id = ? AND status = 'PENDING'`, status, imageID, failure, now.Unix(), requestID,
	)
	if err != nil {
		return CaptureRequest{}, fmt.Errorf("complete capture request: %w", err)
	}
	rows, err := update.RowsAffected()
	if err != nil || rows != 1 {
		return CaptureRequest{}, ErrCaptureRequestNotFound
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
	result.Status, result.ImageID, result.Failure = status, imageID, failure
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
        JOIN devices ON devices.pair_id = images.pair_id
        WHERE images.id = ? AND devices.id = ?
          AND images.uploader_device_id <> devices.id AND images.expires_at > ?`,
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

func (s *Store) EnrollDevice(
	ctx context.Context,
	invitationToken string,
	deviceName string,
	publicKey string,
) (Enrollment, error) {
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
        INSERT INTO devices (id, pair_id, slot, name, public_key, credential_hash, created_at)
        VALUES (?, ?, ?, ?, ?, ?, ?)`,
		deviceID, pairID, slot, deviceName, publicKey, credentialHash[:], now.Unix(),
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
