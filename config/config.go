package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// Config holds all required environment variables for the bot.
type Config struct {
	TelegramBotToken string
	TelegramAdminID  string
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
		TelegramAdminID:  os.Getenv("TELEGRAM_ADMIN_ID"),
		ElevenLabsAPIKey: os.Getenv("ELEVENLABS_API_KEY"),
	}

	if cfg.TelegramBotToken == "" {
		return Config{}, fmt.Errorf("TELEGRAM_BOT_TOKEN is required but not set")
	}
	if cfg.TelegramAdminID == "" {
		return Config{}, fmt.Errorf("TELEGRAM_ADMIN_ID is required but not set")
	}
	if cfg.ElevenLabsAPIKey == "" {
		return Config{}, fmt.Errorf("ELEVENLABS_API_KEY is required but not set")
	}

	return cfg, nil
}
