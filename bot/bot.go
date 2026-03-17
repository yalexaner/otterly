package bot

import (
	"log"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/yalexaner/otterly/audio"
	"github.com/yalexaner/otterly/config"
	"github.com/yalexaner/otterly/elevenlabs"
	"github.com/yalexaner/otterly/store"
)

// Bot wraps the Telegram bot API client and application config.
type Bot struct {
	api          *tgbotapi.BotAPI
	cfg          config.Config
	store        *store.Store
	fileEndpoint string
	convertToWAV func(string) (string, error)
	transcriber  transcriber
}

// New creates a new Bot instance using the provided config.
// returns an error if the Telegram API client cannot be created.
func New(cfg config.Config, s *store.Store) (*Bot, error) {
	return newWithEndpoint(cfg, s, tgbotapi.APIEndpoint)
}

// newWithEndpoint creates a Bot using a custom API endpoint (used for testing).
func newWithEndpoint(cfg config.Config, s *store.Store, apiEndpoint string) (*Bot, error) {
	api, err := tgbotapi.NewBotAPIWithAPIEndpoint(cfg.TelegramBotToken, apiEndpoint)
	if err != nil {
		return nil, err
	}

	log.Printf("authorized on account %s", api.Self.UserName)

	// derive file endpoint from api endpoint (e.g., /bot%s/%s -> /file/bot%s/%s)
	fileEndpoint := strings.Replace(apiEndpoint, "/bot%s/%s", "/file/bot%s/%s", 1)

	return &Bot{
		api:          api,
		cfg:          cfg,
		store:        s,
		fileEndpoint: fileEndpoint,
		convertToWAV: audio.ConvertToWAV,
		transcriber:  elevenlabs.NewClient(cfg.ElevenLabsAPIKey),
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
			b.handleCommand(update.Message)
		} else if update.Message.Voice != nil {
			if !update.Message.Chat.IsPrivate() {
				log.Printf("[voice] ignored non-private chat %d", update.Message.Chat.ID)
				continue
			}
			log.Printf("[voice] from user %d, duration %ds", update.Message.From.ID, update.Message.Voice.Duration)
			b.handleVoice(update.Message)
		} else if update.Message.Text != "" {
			log.Printf("[text] from user %d", update.Message.From.ID)
		} else {
			log.Printf("[other] from user %d", update.Message.From.ID)
		}
	}
}
