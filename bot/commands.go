package bot

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

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

// requireStore checks that the store is available and replies with an error if not.
func (b *Bot) requireStore(msg *tgbotapi.Message) bool {
	if b.store == nil {
		b.reply(msg, "Функция недоступна.")
		return false
	}
	return true
}

// handleAllow handles the /allow command to add a user to the whitelist.
func (b *Bot) handleAllow(msg *tgbotapi.Message) {
	if !b.isAdmin(msg.From.ID) {
		b.reply(msg, "Эта команда не поддерживается.")
		return
	}
	if !b.requireStore(msg) {
		return
	}

	arg := strings.TrimSpace(msg.CommandArguments())
	if arg == "" {
		b.reply(msg, "Использование: /allow <telegram_user_id>")
		return
	}

	userID, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		b.reply(msg, "Неверный формат ID. Укажите числовой Telegram ID.")
		return
	}

	if err := b.store.AllowUser(userID, msg.From.ID); err != nil {
		log.Printf("failed to allow user %d: %v", userID, err)
		b.reply(msg, "Ошибка при добавлении пользователя.")
		return
	}

	b.reply(msg, fmt.Sprintf("Пользователь %d добавлен.", userID))
}

// handleDeny handles the /deny command to block a user.
func (b *Bot) handleDeny(msg *tgbotapi.Message) {
	if !b.isAdmin(msg.From.ID) {
		b.reply(msg, "Эта команда не поддерживается.")
		return
	}
	if !b.requireStore(msg) {
		return
	}

	arg := strings.TrimSpace(msg.CommandArguments())
	if arg == "" {
		b.reply(msg, "Использование: /deny <telegram_user_id>")
		return
	}

	userID, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		b.reply(msg, "Неверный формат ID. Укажите числовой Telegram ID.")
		return
	}

	if userID == msg.From.ID {
		b.reply(msg, "Нельзя заблокировать администратора.")
		return
	}

	if err := b.store.BlockUser(userID); errors.Is(err, store.ErrNotFound) {
		b.reply(msg, "Пользователь не найден.")
		return
	} else if err != nil {
		log.Printf("failed to block user %d: %v", userID, err)
		b.reply(msg, "Ошибка при блокировке пользователя.")
		return
	}

	b.reply(msg, fmt.Sprintf("Пользователь %d заблокирован.", userID))
}

// handleList handles the /list command to show all registered users.
func (b *Bot) handleList(msg *tgbotapi.Message) {
	if !b.isAdmin(msg.From.ID) {
		b.reply(msg, "Эта команда не поддерживается.")
		return
	}
	if !b.requireStore(msg) {
		return
	}

	users, err := b.store.ListUsers()
	if err != nil {
		log.Printf("failed to list users: %v", err)
		b.reply(msg, "Ошибка при получении списка пользователей.")
		return
	}

	if len(users) == 0 {
		b.reply(msg, "Нет зарегистрированных пользователей.")
		return
	}

	var sb strings.Builder
	for _, u := range users {
		if u.Username != "" {
			fmt.Fprintf(&sb, "@%s (%d) — %s\n", u.Username, u.TelegramUserID, u.Status)
		} else {
			fmt.Fprintf(&sb, "%d — %s\n", u.TelegramUserID, u.Status)
		}
	}
	b.reply(msg, strings.TrimRight(sb.String(), "\n"))
}

// handleInvite handles the /invite command to generate an invite link.
func (b *Bot) handleInvite(msg *tgbotapi.Message) {
	if !b.isAdmin(msg.From.ID) {
		b.reply(msg, "Эта команда не поддерживается.")
		return
	}
	if !b.requireStore(msg) {
		return
	}

	token, err := store.GenerateToken()
	if err != nil {
		log.Printf("failed to generate invite token: %v", err)
		b.reply(msg, "Ошибка при создании приглашения.")
		return
	}

	expiresAt := time.Now().Add(store.DefaultInviteTTL)
	if err := b.store.CreateInvite(token, msg.From.ID, expiresAt); err != nil {
		log.Printf("failed to create invite: %v", err)
		b.reply(msg, "Ошибка при создании приглашения.")
		return
	}

	link := fmt.Sprintf("https://t.me/%s?start=%s", b.api.Self.UserName, token)
	b.reply(msg, fmt.Sprintf("Ссылка-приглашение (действует 72 ч):\n%s", link))
}

// handleCommand dispatches the command to the appropriate handler.
func (b *Bot) handleCommand(msg *tgbotapi.Message) {
	switch msg.Command() {
	case "start":
		b.handleStart(msg)
	case "help":
		b.reply(msg, "Отправьте голосовое сообщение, и бот вернёт текстовую расшифровку.")
	case "allow":
		b.handleAllow(msg)
	case "deny":
		b.handleDeny(msg)
	case "list":
		b.handleList(msg)
	case "invite":
		b.handleInvite(msg)
	default:
		b.reply(msg, "Эта команда не поддерживается.")
	}
}
