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
}

// Load reads configuration from .env file (if present) and environment variables.
// returns an error if any required variable is missing.
func Load() (Config, error) {
	// .env file is optional, but parse/read errors should be surfaced
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("failed to load .env file: %w", err)
	}

	cfg := Config{
		TelegramBotToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		ElevenLabsAPIKey: os.Getenv("ELEVENLABS_API_KEY"),
	}

	if cfg.TelegramBotToken == "" {
		return Config{}, fmt.Errorf("TELEGRAM_BOT_TOKEN is required but not set")
	}

	if raw := os.Getenv("TELEGRAM_ADMIN_ID"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return Config{}, fmt.Errorf("TELEGRAM_ADMIN_ID must be a valid integer: %w", err)
		}
		cfg.TelegramAdminID = id
	}

	return cfg, nil
}
