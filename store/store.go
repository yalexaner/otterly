package store

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

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
