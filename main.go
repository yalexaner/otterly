package main

import (
	"log"

	"github.com/yalexaner/otterly/bot"
	"github.com/yalexaner/otterly/config"
	"github.com/yalexaner/otterly/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	s, err := store.Open(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer func() { _ = s.Close() }()

	if err := s.Migrate(); err != nil {
		log.Fatalf("failed to migrate database: %v", err)
	}

	if err := s.EnsureAdmin(cfg.TelegramAdminID); err != nil {
		log.Fatalf("failed to seed admin: %v", err)
	}

	b, err := bot.New(cfg, s)
	if err != nil {
		log.Fatalf("failed to create bot: %v", err)
	}

	b.Start()
}
