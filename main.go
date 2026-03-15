package main

import (
	"log"

	"github.com/yalexaner/otterly/bot"
	"github.com/yalexaner/otterly/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	b, err := bot.New(cfg)
	if err != nil {
		log.Fatalf("failed to create bot: %v", err)
	}

	b.Start()
}
