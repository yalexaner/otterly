package store

import (
	"testing"
)

func TestOpen_SetsWALMode(t *testing.T) {
	path := t.TempDir() + "/test.db"
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	var mode string
	if err := s.db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want %q", mode, "wal")
	}
}

func TestMigrate_CreatesTablesIdempotently(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	// call Migrate twice — second call must not fail
	for i := range 2 {
		if err := s.Migrate(); err != nil {
			t.Fatalf("Migrate call %d: %v", i+1, err)
		}
	}

	// verify both tables exist
	for _, table := range []string{"users", "invites"} {
		var name string
		err := s.db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}
}

func TestMigrate_StatusCheckConstraint(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	if err := s.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	_, err = s.db.Exec(
		"INSERT INTO users (telegram_user_id, status) VALUES (1, 'invalid')",
	)
	if err == nil {
		t.Fatal("expected error inserting user with invalid status, got nil")
	}
}

func TestEnsureAdmin_InsertsOnce(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	if err := s.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// call twice — should insert only one row
	for i := range 2 {
		if err := s.EnsureAdmin(42); err != nil {
			t.Fatalf("EnsureAdmin call %d: %v", i+1, err)
		}
	}

	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM users WHERE telegram_user_id = 42").Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 1 {
		t.Errorf("user count = %d, want 1", count)
	}

	var status string
	if err := s.db.QueryRow("SELECT status FROM users WHERE telegram_user_id = 42").Scan(&status); err != nil {
		t.Fatalf("status query: %v", err)
	}
	if status != "active" {
		t.Errorf("status = %q, want %q", status, "active")
	}
}

func TestEnsureAdmin_DoesNotOverwriteExistingStatus(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	if err := s.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// manually insert admin as blocked
	_, err = s.db.Exec(
		"INSERT INTO users (telegram_user_id, status) VALUES (42, 'blocked')",
	)
	if err != nil {
		t.Fatalf("manual insert: %v", err)
	}

	// EnsureAdmin should not overwrite the blocked status
	if err := s.EnsureAdmin(42); err != nil {
		t.Fatalf("EnsureAdmin: %v", err)
	}

	var status string
	if err := s.db.QueryRow("SELECT status FROM users WHERE telegram_user_id = 42").Scan(&status); err != nil {
		t.Fatalf("status query: %v", err)
	}
	if status != "blocked" {
		t.Errorf("status = %q, want %q (should not overwrite)", status, "blocked")
	}
}

func TestEnsureAdmin_SkipsZeroID(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	if err := s.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	if err := s.EnsureAdmin(0); err != nil {
		t.Fatalf("EnsureAdmin(0): %v", err)
	}

	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 0 {
		t.Errorf("user count = %d, want 0 (zero ID should be skipped)", count)
	}
}
