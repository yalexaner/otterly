package bot

import (
	"path/filepath"
	"testing"

	"github.com/yalexaner/otterly/config"
	"github.com/yalexaner/otterly/store"
)

func TestIsAuthorized_AdminAlwaysTrue(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	cfg := config.Config{
		TelegramBotToken: "fake-token",
		TelegramAdminID:  12345,
		ElevenLabsAPIKey: "el-key",
	}
	b, err := newWithEndpoint(cfg, nil, server.URL+"/bot%s/%s")
	if err != nil {
		t.Fatalf("failed to create bot: %v", err)
	}

	if !b.isAuthorized(12345) {
		t.Error("admin should always be authorized")
	}
}

func TestIsAuthorized_ActiveUserTrue(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	s := openTestStore(t)
	defer s.Close()

	if err := s.AllowUser(99999, 12345); err != nil {
		t.Fatalf("failed to allow user: %v", err)
	}

	cfg := config.Config{
		TelegramBotToken: "fake-token",
		TelegramAdminID:  12345,
		ElevenLabsAPIKey: "el-key",
	}
	b, err := newWithEndpoint(cfg, s, server.URL+"/bot%s/%s")
	if err != nil {
		t.Fatalf("failed to create bot: %v", err)
	}

	if !b.isAuthorized(99999) {
		t.Error("active user should be authorized")
	}
}

func TestIsAuthorized_BlockedUserFalse(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	s := openTestStore(t)
	defer s.Close()

	if err := s.AllowUser(99999, 12345); err != nil {
		t.Fatalf("failed to allow user: %v", err)
	}
	if err := s.BlockUser(99999); err != nil {
		t.Fatalf("failed to block user: %v", err)
	}

	cfg := config.Config{
		TelegramBotToken: "fake-token",
		TelegramAdminID:  12345,
		ElevenLabsAPIKey: "el-key",
	}
	b, err := newWithEndpoint(cfg, s, server.URL+"/bot%s/%s")
	if err != nil {
		t.Fatalf("failed to create bot: %v", err)
	}

	if b.isAuthorized(99999) {
		t.Error("blocked user should not be authorized")
	}
}

func TestIsAuthorized_NonexistentUserFalse(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	s := openTestStore(t)
	defer s.Close()

	cfg := config.Config{
		TelegramBotToken: "fake-token",
		TelegramAdminID:  12345,
		ElevenLabsAPIKey: "el-key",
	}
	b, err := newWithEndpoint(cfg, s, server.URL+"/bot%s/%s")
	if err != nil {
		t.Fatalf("failed to create bot: %v", err)
	}

	if b.isAuthorized(77777) {
		t.Error("nonexistent user should not be authorized")
	}
}

func TestIsAuthorized_NilStoreTrue(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	cfg := config.Config{
		TelegramBotToken: "fake-token",
		TelegramAdminID:  12345,
		ElevenLabsAPIKey: "el-key",
	}
	b, err := newWithEndpoint(cfg, nil, server.URL+"/bot%s/%s")
	if err != nil {
		t.Fatalf("failed to create bot: %v", err)
	}

	// non-admin user with nil store should be authorized (fallback)
	if !b.isAuthorized(99999) {
		t.Error("with nil store, any user should be authorized")
	}
}

func TestIsAdmin_ZeroAdminID(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	cfg := config.Config{
		TelegramBotToken: "fake-token",
		TelegramAdminID:  0,
		ElevenLabsAPIKey: "el-key",
	}
	b, err := newWithEndpoint(cfg, nil, server.URL+"/bot%s/%s")
	if err != nil {
		t.Fatalf("failed to create bot: %v", err)
	}

	if b.isAdmin(12345) {
		t.Error("isAdmin should return false when admin ID is 0")
	}
}

// openTestStore creates a temporary in-memory store for testing.
func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test store: %v", err)
	}
	if err := s.Migrate(); err != nil {
		t.Fatalf("failed to migrate test store: %v", err)
	}
	return s
}
