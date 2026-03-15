package bot

import (
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/yalexaner/otterly/config"
)

// Bot wraps the Telegram bot API client and application config.
type Bot struct {
	api *tgbotapi.BotAPI
	cfg config.Config
}

// New creates a new Bot instance using the provided config.
// returns an error if the Telegram API client cannot be created.
func New(cfg config.Config) (*Bot, error) {
	return newWithEndpoint(cfg, tgbotapi.APIEndpoint)
}

// newWithEndpoint creates a Bot using a custom API endpoint (used for testing).
func newWithEndpoint(cfg config.Config, apiEndpoint string) (*Bot, error) {
	api, err := tgbotapi.NewBotAPIWithAPIEndpoint(cfg.TelegramBotToken, apiEndpoint)
	if err != nil {
		return nil, err
	}

	log.Printf("authorized on account %s", api.Self.UserName)

	return &Bot{
		api: api,
		cfg: cfg,
	}, nil
}

// Start begins the long-polling loop, receiving and dispatching updates.
func (b *Bot) Start() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	log.Println("bot started, listening for updates...")

	for update := range updates {
		if update.Message == nil || update.Message.From == nil {
			continue
		}

		if update.Message.IsCommand() {
			log.Printf("[command] %s from user %d", update.Message.Command(), update.Message.From.ID)
		} else if update.Message.Voice != nil {
			log.Printf("[voice] from user %d, duration %ds", update.Message.From.ID, update.Message.Voice.Duration)
		} else if update.Message.Text != "" {
			log.Printf("[text] from user %d", update.Message.From.ID)
		} else {
			log.Printf("[other] from user %d", update.Message.From.ID)
		}
	}
}
