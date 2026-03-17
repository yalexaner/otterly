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

// helper to open an in-memory store with migrations applied
func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return s
}

func TestIsActive_ActiveUser(t *testing.T) {
	s := newTestStore(t)
	if err := s.AllowUser(100, 1); err != nil {
		t.Fatalf("AllowUser: %v", err)
	}

	active, err := s.IsActive(100)
	if err != nil {
		t.Fatalf("IsActive: %v", err)
	}
	if !active {
		t.Error("IsActive = false, want true for active user")
	}
}

func TestIsActive_BlockedUser(t *testing.T) {
	s := newTestStore(t)
	if err := s.AllowUser(100, 1); err != nil {
		t.Fatalf("AllowUser: %v", err)
	}
	if err := s.BlockUser(100); err != nil {
		t.Fatalf("BlockUser: %v", err)
	}

	active, err := s.IsActive(100)
	if err != nil {
		t.Fatalf("IsActive: %v", err)
	}
	if active {
		t.Error("IsActive = true, want false for blocked user")
	}
}

func TestIsActive_NonexistentUser(t *testing.T) {
	s := newTestStore(t)

	active, err := s.IsActive(999)
	if err != nil {
		t.Fatalf("IsActive: %v", err)
	}
	if active {
		t.Error("IsActive = true, want false for nonexistent user")
	}
}

func TestAllowUser_NewUser(t *testing.T) {
	s := newTestStore(t)
	if err := s.AllowUser(100, 1); err != nil {
		t.Fatalf("AllowUser: %v", err)
	}

	var status string
	if err := s.db.QueryRow("SELECT status FROM users WHERE telegram_user_id = 100").Scan(&status); err != nil {
		t.Fatalf("query: %v", err)
	}
	if status != "active" {
		t.Errorf("status = %q, want %q", status, "active")
	}
}

func TestAllowUser_ReAllowBlockedUser(t *testing.T) {
	s := newTestStore(t)
	if err := s.AllowUser(100, 1); err != nil {
		t.Fatalf("AllowUser: %v", err)
	}
	if err := s.BlockUser(100); err != nil {
		t.Fatalf("BlockUser: %v", err)
	}

	// re-allow the blocked user
	if err := s.AllowUser(100, 1); err != nil {
		t.Fatalf("AllowUser (re-allow): %v", err)
	}

	var status string
	if err := s.db.QueryRow("SELECT status FROM users WHERE telegram_user_id = 100").Scan(&status); err != nil {
		t.Fatalf("query: %v", err)
	}
	if status != "active" {
		t.Errorf("status = %q, want %q after re-allow", status, "active")
	}
}

func TestAllowUser_IdempotentOnActiveUser(t *testing.T) {
	s := newTestStore(t)
	if err := s.AllowUser(100, 1); err != nil {
		t.Fatalf("AllowUser: %v", err)
	}
	if err := s.AllowUser(100, 1); err != nil {
		t.Fatalf("AllowUser (second): %v", err)
	}

	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM users WHERE telegram_user_id = 100").Scan(&count); err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 1 {
		t.Errorf("user count = %d, want 1", count)
	}
}

func TestBlockUser_ActiveUser(t *testing.T) {
	s := newTestStore(t)
	if err := s.AllowUser(100, 1); err != nil {
		t.Fatalf("AllowUser: %v", err)
	}

	if err := s.BlockUser(100); err != nil {
		t.Fatalf("BlockUser: %v", err)
	}

	var status string
	if err := s.db.QueryRow("SELECT status FROM users WHERE telegram_user_id = 100").Scan(&status); err != nil {
		t.Fatalf("query: %v", err)
	}
	if status != "blocked" {
		t.Errorf("status = %q, want %q", status, "blocked")
	}
}

func TestBlockUser_NonexistentUser(t *testing.T) {
	s := newTestStore(t)

	err := s.BlockUser(999)
	if err == nil {
		t.Fatal("expected error blocking nonexistent user, got nil")
	}
	if err != ErrNotFound {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestListUsers_Empty(t *testing.T) {
	s := newTestStore(t)

	users, err := s.ListUsers()
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 0 {
		t.Errorf("len(users) = %d, want 0", len(users))
	}
}

func TestListUsers_Single(t *testing.T) {
	s := newTestStore(t)
	if err := s.AllowUser(100, 1); err != nil {
		t.Fatalf("AllowUser: %v", err)
	}

	users, err := s.ListUsers()
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("len(users) = %d, want 1", len(users))
	}
	if users[0].TelegramUserID != 100 {
		t.Errorf("TelegramUserID = %d, want 100", users[0].TelegramUserID)
	}
	if users[0].Status != "active" {
		t.Errorf("Status = %q, want %q", users[0].Status, "active")
	}
}

func TestListUsers_Multiple_OrderedByAddedAt(t *testing.T) {
	s := newTestStore(t)

	// insert users with explicit added_at to control ordering
	_, err := s.db.Exec(
		"INSERT INTO users (telegram_user_id, added_by, added_at) VALUES (?, ?, ?)",
		200, 1, "2026-01-02T00:00:00Z",
	)
	if err != nil {
		t.Fatalf("insert user 200: %v", err)
	}
	_, err = s.db.Exec(
		"INSERT INTO users (telegram_user_id, added_by, added_at) VALUES (?, ?, ?)",
		100, 1, "2026-01-01T00:00:00Z",
	)
	if err != nil {
		t.Fatalf("insert user 100: %v", err)
	}
	_, err = s.db.Exec(
		"INSERT INTO users (telegram_user_id, added_by, added_at) VALUES (?, ?, ?)",
		300, 1, "2026-01-03T00:00:00Z",
	)
	if err != nil {
		t.Fatalf("insert user 300: %v", err)
	}

	users, err := s.ListUsers()
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 3 {
		t.Fatalf("len(users) = %d, want 3", len(users))
	}

	// should be ordered: 100, 200, 300 by added_at
	wantIDs := []int64{100, 200, 300}
	for i, want := range wantIDs {
		if users[i].TelegramUserID != want {
			t.Errorf("users[%d].TelegramUserID = %d, want %d", i, users[i].TelegramUserID, want)
		}
	}
}
