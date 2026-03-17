package store

import (
	"encoding/base64"
	"errors"
	"testing"
	"time"
)

func TestOpen_SetsWALMode(t *testing.T) {
	path := t.TempDir() + "/test.db"
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = s.Close() }()

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
	defer func() { _ = s.Close() }()

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
	defer func() { _ = s.Close() }()

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
	defer func() { _ = s.Close() }()

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
	defer func() { _ = s.Close() }()

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
	defer func() { _ = s.Close() }()

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
	t.Cleanup(func() { _ = s.Close() })
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

// --- GenerateToken tests ---

func TestGenerateToken_LengthAndEncoding(t *testing.T) {
	token, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	// 16 bytes in base64 RawURL = 22 chars
	if len(token) != 22 {
		t.Errorf("token length = %d, want 22", len(token))
	}
	// must be valid base64 RawURL
	_, err = base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Errorf("token is not valid base64 RawURL: %v", err)
	}
}

func TestGenerateToken_Uniqueness(t *testing.T) {
	tokens := make(map[string]bool)
	for range 100 {
		token, err := GenerateToken()
		if err != nil {
			t.Fatalf("GenerateToken: %v", err)
		}
		if tokens[token] {
			t.Fatalf("duplicate token: %s", token)
		}
		tokens[token] = true
	}
}

// --- CreateInvite tests ---

func TestCreateInvite_Success(t *testing.T) {
	s := newTestStore(t)
	expires := time.Now().Add(DefaultInviteTTL)

	if err := s.CreateInvite("test-token", 42, expires); err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}

	var token string
	var createdBy int64
	err := s.db.QueryRow("SELECT token, created_by FROM invites WHERE token = ?", "test-token").Scan(&token, &createdBy)
	if err != nil {
		t.Fatalf("query invite: %v", err)
	}
	if createdBy != 42 {
		t.Errorf("created_by = %d, want 42", createdBy)
	}
}

func TestCreateInvite_DuplicateTokenError(t *testing.T) {
	s := newTestStore(t)
	expires := time.Now().Add(DefaultInviteTTL)

	if err := s.CreateInvite("dup-token", 42, expires); err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	err := s.CreateInvite("dup-token", 42, expires)
	if err == nil {
		t.Fatal("expected error on duplicate token, got nil")
	}
}

// --- RedeemInvite tests ---

func TestRedeemInvite_Success(t *testing.T) {
	s := newTestStore(t)
	expires := time.Now().Add(DefaultInviteTTL)
	if err := s.CreateInvite("redeem-ok", 42, expires); err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}

	if err := s.RedeemInvite("redeem-ok", 100, "alice"); err != nil {
		t.Fatalf("RedeemInvite: %v", err)
	}

	// user should exist and be active
	active, err := s.IsActive(100)
	if err != nil {
		t.Fatalf("IsActive: %v", err)
	}
	if !active {
		t.Error("user should be active after redeem")
	}

	// invite should be marked as used
	var usedBy int64
	err = s.db.QueryRow("SELECT used_by FROM invites WHERE token = ?", "redeem-ok").Scan(&usedBy)
	if err != nil {
		t.Fatalf("query used_by: %v", err)
	}
	if usedBy != 100 {
		t.Errorf("used_by = %d, want 100", usedBy)
	}
}

func TestRedeemInvite_TokenNotFound(t *testing.T) {
	s := newTestStore(t)

	err := s.RedeemInvite("nonexistent", 100, "alice")
	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("err = %v, want ErrInvalidToken", err)
	}
}

func TestRedeemInvite_ExpiredToken(t *testing.T) {
	s := newTestStore(t)
	// create an already-expired invite
	expired := time.Now().Add(-1 * time.Hour)
	if err := s.CreateInvite("expired-token", 42, expired); err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}

	err := s.RedeemInvite("expired-token", 100, "alice")
	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("err = %v, want ErrInvalidToken", err)
	}
}

func TestRedeemInvite_AlreadyUsedToken(t *testing.T) {
	s := newTestStore(t)
	expires := time.Now().Add(DefaultInviteTTL)
	if err := s.CreateInvite("used-token", 42, expires); err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}

	// first redeem succeeds
	if err := s.RedeemInvite("used-token", 100, "alice"); err != nil {
		t.Fatalf("RedeemInvite (first): %v", err)
	}

	// second redeem should fail
	err := s.RedeemInvite("used-token", 200, "bob")
	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("err = %v, want ErrInvalidToken", err)
	}
}

func TestRedeemInvite_UserAlreadyExists(t *testing.T) {
	s := newTestStore(t)
	// create an existing user
	if err := s.AllowUser(100, 1); err != nil {
		t.Fatalf("AllowUser: %v", err)
	}

	expires := time.Now().Add(DefaultInviteTTL)
	if err := s.CreateInvite("existing-user-token", 42, expires); err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}

	// redeem should succeed (ON CONFLICT updates)
	if err := s.RedeemInvite("existing-user-token", 100, "alice"); err != nil {
		t.Fatalf("RedeemInvite: %v", err)
	}

	// user should still be active
	active, err := s.IsActive(100)
	if err != nil {
		t.Fatalf("IsActive: %v", err)
	}
	if !active {
		t.Error("existing user should remain active after redeem")
	}
}
