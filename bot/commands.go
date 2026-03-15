package bot

import (
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// reply sends a text reply to the message that triggered it.
func (b *Bot) reply(msg *tgbotapi.Message, text string) {
	r := tgbotapi.NewMessage(msg.Chat.ID, text)
	r.ReplyToMessageID = msg.MessageID
	if _, err := b.api.Send(r); err != nil {
		log.Printf("failed to send reply: %v", err)
	}
}

// handleCommand dispatches the command to the appropriate handler.
func (b *Bot) handleCommand(msg *tgbotapi.Message) {
	switch msg.Command() {
	case "start":
		b.reply(msg, "Добро пожаловать! Отправь голосовое сообщение, и я пришлю текст.")
	case "help":
		b.reply(msg, "Отправьте голосовое сообщение, и бот вернёт текстовую расшифровку.")
	default:
		b.reply(msg, "Эта команда не поддерживается.")
	}
}
