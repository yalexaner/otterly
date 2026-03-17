package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// DefaultInviteTTL is the default time-to-live for invite tokens.
const DefaultInviteTTL = 72 * time.Hour

// User represents a registered bot user.
type User struct {
	TelegramUserID int64
	Username       string
	Status         string
	AddedBy        int64
	AddedAt        string
}

// ErrNotFound is returned when a user does not exist.
var ErrNotFound = errors.New("user not found")

// ErrInvalidToken is returned when an invite token is not found, expired, or already used.
var ErrInvalidToken = errors.New("invalid or expired invite token")

// ErrBlocked is returned when a blocked user attempts to redeem an invite.
var ErrBlocked = errors.New("user is blocked")

// Store wraps a SQLite database connection for user and invite management.
type Store struct {
	db *sql.DB
}

// Open opens a SQLite database at the given path and configures it with
// recommended pragmas for WAL mode, busy timeout, foreign keys, and
// synchronous mode.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	db.SetMaxOpenConns(1)

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
		"PRAGMA synchronous=NORMAL",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("set pragma %q: %w", p, err)
		}
	}

	return &Store{db: db}, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// EnsureAdmin inserts the given Telegram user ID as an active admin if it
// does not already exist. If the user already exists (even as blocked), their
// status is left unchanged. A zero ID is silently skipped.
func (s *Store) EnsureAdmin(telegramUserID int64) error {
	if telegramUserID == 0 {
		return nil
	}
	_, err := s.db.Exec(
		"INSERT OR IGNORE INTO users (telegram_user_id, status) VALUES (?, 'active')",
		telegramUserID,
	)
	if err != nil {
		return fmt.Errorf("ensure admin: %w", err)
	}
	return nil
}

// Migrate creates the users and invites tables if they do not already exist.
func (s *Store) Migrate() error {
	const usersTable = `
CREATE TABLE IF NOT EXISTS users (
    telegram_user_id INTEGER PRIMARY KEY,
    username         TEXT    NOT NULL DEFAULT '',
    status           TEXT    NOT NULL DEFAULT 'active' CHECK(status IN ('active', 'blocked')),
    added_by         INTEGER NOT NULL DEFAULT 0,
    added_at         TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
)`

	const invitesTable = `
CREATE TABLE IF NOT EXISTS invites (
    token      TEXT    PRIMARY KEY,
    created_by INTEGER NOT NULL,
    created_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    expires_at TEXT    NOT NULL,
    used_by    INTEGER,
    used_at    TEXT
)`

	if _, err := s.db.Exec(usersTable); err != nil {
		return fmt.Errorf("create users table: %w", err)
	}
	if _, err := s.db.Exec(invitesTable); err != nil {
		return fmt.Errorf("create invites table: %w", err)
	}

	return nil
}

// IsActive returns true if the user exists and has status 'active'.
func (s *Store) IsActive(telegramUserID int64) (bool, error) {
	var exists int
	err := s.db.QueryRow(
		"SELECT 1 FROM users WHERE telegram_user_id = ? AND status = 'active'",
		telegramUserID,
	).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("is active: %w", err)
	}
	return true, nil
}

// AllowUser adds the user as active or re-activates them if they were blocked.
func (s *Store) AllowUser(telegramUserID, addedBy int64) error {
	_, err := s.db.Exec(
		`INSERT INTO users (telegram_user_id, added_by, status)
		 VALUES (?, ?, 'active')
		 ON CONFLICT(telegram_user_id) DO UPDATE SET status = 'active'`,
		telegramUserID, addedBy,
	)
	if err != nil {
		return fmt.Errorf("allow user: %w", err)
	}
	return nil
}

// BlockUser sets the user's status to 'blocked'. Returns ErrNotFound if the
// user does not exist.
func (s *Store) BlockUser(telegramUserID int64) error {
	res, err := s.db.Exec(
		"UPDATE users SET status = 'blocked' WHERE telegram_user_id = ?",
		telegramUserID,
	)
	if err != nil {
		return fmt.Errorf("block user: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("block user rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListUsers returns all users ordered by added_at.
func (s *Store) ListUsers() ([]User, error) {
	rows, err := s.db.Query(
		"SELECT telegram_user_id, username, status, added_by, added_at FROM users ORDER BY added_at",
	)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.TelegramUserID, &u.Username, &u.Status, &u.AddedBy, &u.AddedAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate users: %w", err)
	}
	return users, nil
}

// GenerateToken creates a cryptographically random URL-safe token (22 chars).
func GenerateToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// CreateInvite inserts a new invite record.
func (s *Store) CreateInvite(token string, createdBy int64, expiresAt time.Time) error {
	_, err := s.db.Exec(
		"INSERT INTO invites (token, created_by, expires_at) VALUES (?, ?, ?)",
		token, createdBy, expiresAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("create invite: %w", err)
	}
	return nil
}

// RedeemInvite validates the token and creates a user in a single transaction.
// Returns ErrInvalidToken if the token does not exist, is expired, or was already used.
func (s *Store) RedeemInvite(token string, userID int64, username string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("redeem invite begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var expiresAt string
	var usedBy sql.NullInt64
	err = tx.QueryRow(
		"SELECT expires_at, used_by FROM invites WHERE token = ?", token,
	).Scan(&expiresAt, &usedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrInvalidToken
	}
	if err != nil {
		return fmt.Errorf("redeem invite select: %w", err)
	}

	if usedBy.Valid {
		return ErrInvalidToken
	}

	exp, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return fmt.Errorf("redeem invite parse expiry: %w", err)
	}
	if time.Now().UTC().After(exp) {
		return ErrInvalidToken
	}

	// blocked users cannot redeem invites — admin's /deny decision takes priority
	var userStatus sql.NullString
	err = tx.QueryRow(
		"SELECT status FROM users WHERE telegram_user_id = ?", userID,
	).Scan(&userStatus)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("redeem invite check user: %w", err)
	}
	if userStatus.Valid && userStatus.String == "blocked" {
		return ErrBlocked
	}

	_, err = tx.Exec(
		`INSERT INTO users (telegram_user_id, username, status, added_by)
		 VALUES (?, ?, 'active', 0)
		 ON CONFLICT(telegram_user_id) DO UPDATE SET status = 'active', username = ?`,
		userID, username, username,
	)
	if err != nil {
		return fmt.Errorf("redeem invite insert user: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	res, err := tx.Exec(
		"UPDATE invites SET used_by = ?, used_at = ? WHERE token = ? AND used_by IS NULL",
		userID, now, token,
	)
	if err != nil {
		return fmt.Errorf("redeem invite update: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("redeem invite rows affected: %w", err)
	}
	if n == 0 {
		return ErrInvalidToken
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("redeem invite commit: %w", err)
	}
	return nil
}
