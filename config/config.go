package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds all required environment variables for the bot.
type Config struct {
	TelegramBotToken string
	TelegramAdminID  int64
	ElevenLabsAPIKey string
	DatabasePath     string
}

// Load reads configuration from .env file (if present) and environment variables.
// returns an error if any required variable is missing.
func Load() (Config, error) {
	// .env file is optional, but parse/read errors should be surfaced
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("failed to load .env file: %w", err)
	}

	dbPath := os.Getenv("DATABASE_PATH")
	if dbPath == "" {
		dbPath = "otterly.db"
	}

	cfg := Config{
		TelegramBotToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		ElevenLabsAPIKey: os.Getenv("ELEVENLABS_API_KEY"),
		DatabasePath:     dbPath,
	}

	if cfg.TelegramBotToken == "" {
		return Config{}, fmt.Errorf("TELEGRAM_BOT_TOKEN is required but not set")
	}

	if cfg.ElevenLabsAPIKey == "" {
		return Config{}, fmt.Errorf("ELEVENLABS_API_KEY is required but not set")
	}

	raw := os.Getenv("TELEGRAM_ADMIN_ID")
	if raw == "" {
		return Config{}, fmt.Errorf("TELEGRAM_ADMIN_ID is required but not set")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return Config{}, fmt.Errorf("TELEGRAM_ADMIN_ID must be a valid integer: %w", err)
	}
	if id <= 0 {
		return Config{}, fmt.Errorf("TELEGRAM_ADMIN_ID must be a positive integer, got %d", id)
	}
	cfg.TelegramAdminID = id

	return cfg, nil
}
