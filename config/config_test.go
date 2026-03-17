package config

import (
	"os"
	"strings"
	"testing"
)

// clearEnv unsets all config-related env vars with automatic restore via t.Setenv.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"TELEGRAM_BOT_TOKEN", "TELEGRAM_ADMIN_ID", "ELEVENLABS_API_KEY", "DATABASE_PATH"} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
}

func setAllEnv(t *testing.T) {
	t.Helper()
	t.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
	t.Setenv("TELEGRAM_ADMIN_ID", "12345")
	t.Setenv("ELEVENLABS_API_KEY", "el-key")
}

func TestLoad_AllVarsSet(t *testing.T) {
	clearEnv(t)
	setAllEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.TelegramBotToken != "test-token" {
		t.Errorf("TelegramBotToken = %q, want %q", cfg.TelegramBotToken, "test-token")
	}
	if cfg.TelegramAdminID != 12345 {
		t.Errorf("TelegramAdminID = %d, want %d", cfg.TelegramAdminID, 12345)
	}
	if cfg.ElevenLabsAPIKey != "el-key" {
		t.Errorf("ElevenLabsAPIKey = %q, want %q", cfg.ElevenLabsAPIKey, "el-key")
	}
	if cfg.DatabasePath != "otterly.db" {
		t.Errorf("DatabasePath = %q, want %q", cfg.DatabasePath, "otterly.db")
	}
}

func TestLoad_MissingToken(t *testing.T) {
	clearEnv(t)
	t.Setenv("TELEGRAM_ADMIN_ID", "12345")
	t.Setenv("ELEVENLABS_API_KEY", "el-key")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing TELEGRAM_BOT_TOKEN")
	}
}

func TestLoad_OptionalAdminID(t *testing.T) {
	clearEnv(t)
	t.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
	t.Setenv("ELEVENLABS_API_KEY", "el-key")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.TelegramAdminID != 0 {
		t.Errorf("TelegramAdminID = %d, want 0", cfg.TelegramAdminID)
	}
}

func TestLoad_InvalidAdminID(t *testing.T) {
	clearEnv(t)
	t.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
	t.Setenv("ELEVENLABS_API_KEY", "el-key")
	t.Setenv("TELEGRAM_ADMIN_ID", "not-a-number")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid TELEGRAM_ADMIN_ID")
	}
	if !strings.Contains(err.Error(), "TELEGRAM_ADMIN_ID") {
		t.Errorf("error = %q, want it to mention TELEGRAM_ADMIN_ID", err)
	}
}

func TestLoad_MissingElevenLabsKey(t *testing.T) {
	clearEnv(t)
	t.Setenv("TELEGRAM_BOT_TOKEN", "test-token")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing ELEVENLABS_API_KEY")
	}
}

func TestLoad_AllMissing(t *testing.T) {
	clearEnv(t)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when all vars are missing")
	}
}

func TestLoad_DefaultDatabasePath(t *testing.T) {
	clearEnv(t)
	setAllEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.DatabasePath != "otterly.db" {
		t.Errorf("DatabasePath = %q, want %q", cfg.DatabasePath, "otterly.db")
	}
}

func TestLoad_CustomDatabasePath(t *testing.T) {
	clearEnv(t)
	setAllEnv(t)
	t.Setenv("DATABASE_PATH", "/tmp/custom.db")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.DatabasePath != "/tmp/custom.db" {
		t.Errorf("DatabasePath = %q, want %q", cfg.DatabasePath, "/tmp/custom.db")
	}
}
