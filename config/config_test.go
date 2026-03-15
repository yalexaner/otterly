package config

import (
	"os"
	"testing"
)

// clearEnv unsets all config-related env vars with automatic restore via t.Setenv.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"TELEGRAM_BOT_TOKEN", "TELEGRAM_ADMIN_ID", "ELEVENLABS_API_KEY"} {
		t.Setenv(k, "")
		os.Unsetenv(k)
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
	if cfg.TelegramAdminID != "12345" {
		t.Errorf("TelegramAdminID = %q, want %q", cfg.TelegramAdminID, "12345")
	}
	if cfg.ElevenLabsAPIKey != "el-key" {
		t.Errorf("ElevenLabsAPIKey = %q, want %q", cfg.ElevenLabsAPIKey, "el-key")
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

func TestLoad_MissingAdminID(t *testing.T) {
	clearEnv(t)
	t.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
	t.Setenv("ELEVENLABS_API_KEY", "el-key")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing TELEGRAM_ADMIN_ID")
	}
}

func TestLoad_MissingElevenLabsKey(t *testing.T) {
	clearEnv(t)
	t.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
	t.Setenv("TELEGRAM_ADMIN_ID", "12345")

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
