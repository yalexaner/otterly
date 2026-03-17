package bot

import (
	"errors"
	"log"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/yalexaner/otterly/store"
)

// reply sends a text reply to the message that triggered it.
func (b *Bot) reply(msg *tgbotapi.Message, text string) {
	r := tgbotapi.NewMessage(msg.Chat.ID, text)
	r.ReplyToMessageID = msg.MessageID
	r.AllowSendingWithoutReply = true
	if _, err := b.api.Send(r); err != nil {
		log.Printf("failed to send reply: %v", err)
	}
}

// handleStart handles the /start command with optional invite token.
func (b *Bot) handleStart(msg *tgbotapi.Message) {
	token := strings.TrimSpace(msg.CommandArguments())
	userID := msg.From.ID
	username := msg.From.UserName

	if token == "" {
		// bare /start — check authorization
		if b.isAuthorized(userID) {
			b.reply(msg, "Добро пожаловать! Отправь голосовое сообщение, и я пришлю текст.")
		} else {
			b.reply(msg, "У вас нет доступа. Обратитесь к администратору.")
		}
		return
	}

	// /start <token> — if already authorized, just welcome (don't consume token)
	if b.isAuthorized(userID) {
		b.reply(msg, "Добро пожаловать! Отправь голосовое сообщение, и я пришлю текст.")
		return
	}

	// try to redeem invite
	if b.store == nil {
		b.reply(msg, "Недействительная или просроченная ссылка.")
		return
	}

	err := b.store.RedeemInvite(token, userID, username)
	if errors.Is(err, store.ErrInvalidToken) {
		b.reply(msg, "Недействительная или просроченная ссылка.")
		return
	}
	if err != nil {
		log.Printf("failed to redeem invite for user %d: %v", userID, err)
		b.reply(msg, "Произошла ошибка. Попробуйте позже.")
		return
	}

	b.reply(msg, "Добро пожаловать! Теперь вы можете отправлять голосовые сообщения.")
}

// handleCommand dispatches the command to the appropriate handler.
func (b *Bot) handleCommand(msg *tgbotapi.Message) {
	switch msg.Command() {
	case "start":
		b.handleStart(msg)
	case "help":
		b.reply(msg, "Отправьте голосовое сообщение, и бот вернёт текстовую расшифровку.")
	default:
		b.reply(msg, "Эта команда не поддерживается.")
	}
}
