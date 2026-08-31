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

type Store struct {
	db  *sql.DB
	now func() time.Time
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

CREATE INDEX IF NOT EXISTS audit_events_created_at_idx ON audit_events(created_at);
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

	store := &Store{db: db, now: time.Now}
	if err := store.initialize(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
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
