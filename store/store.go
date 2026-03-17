package store

import (
	"database/sql"
	"errors"
	"fmt"

	_ "modernc.org/sqlite"
)

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
			db.Close()
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
	defer rows.Close()

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
