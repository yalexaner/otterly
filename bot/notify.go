package bot

import (
	"fmt"
	"log"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// adminNotifier sends rate-limited messages to the bot admin via Telegram DM.
// A nil *adminNotifier is safe to call — all methods are no-ops.
type adminNotifier struct {
	api      *tgbotapi.BotAPI
	chatID   int64
	mu       sync.Mutex
	lastSent map[string]time.Time
	cooldown time.Duration
	now      func() time.Time // injectable clock for testing
}

// newAdminNotifier creates a notifier that DMs the given chat ID.
func newAdminNotifier(api *tgbotapi.BotAPI, chatID int64) *adminNotifier {
	return &adminNotifier{
		api:      api,
		chatID:   chatID,
		lastSent: make(map[string]time.Time),
		cooldown: 5 * time.Minute,
		now:      time.Now,
	}
}

// Notify sends a message to the admin, rate-limited per category.
// It is safe to call on a nil receiver (no-op).
func (n *adminNotifier) Notify(category, text string) {
	if n == nil {
		return
	}

	now := n.now()

	n.mu.Lock()
	if now.Sub(n.lastSent[category]) < n.cooldown {
		n.mu.Unlock()
		return
	}
	n.lastSent[category] = now
	n.mu.Unlock()

	msg := fmt.Sprintf("[%s] %s\n%s", category, now.Format("2006-01-02 15:04:05"), text)

	// truncate to Telegram message limit
	runes := []rune(msg)
	if len(runes) > maxMessageLength {
		msg = string(runes[:maxMessageLength])
	}

	m := tgbotapi.NewMessage(n.chatID, msg)
	if _, err := n.api.Send(m); err != nil {
		log.Printf("failed to notify admin: %v", err)
	}
}
