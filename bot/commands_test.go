package bot

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/yalexaner/otterly/config"
	"github.com/yalexaner/otterly/store"
)

// capturedMessage holds the chat_id, text, and reply target from a sendMessage request.
type capturedMessage struct {
	ChatID           int64
	Text             string
	ReplyToMessageID int
}

// newCaptureServer creates a fake Telegram API server that records sendMessage calls.
func newCaptureServer(t *testing.T) (*httptest.Server, *[]capturedMessage) {
	t.Helper()
	var captured []capturedMessage

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.HasSuffix(r.URL.Path, "getMe"):
			json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": map[string]any{
					"id": 123, "is_bot": true,
					"first_name": "TestBot", "username": "test_bot",
				},
			})
		case strings.HasSuffix(r.URL.Path, "sendMessage"):
			r.ParseForm()
			chatID, _ := strconv.ParseInt(r.FormValue("chat_id"), 10, 64)
			text := r.FormValue("text")
			replyTo, _ := strconv.Atoi(r.FormValue("reply_to_message_id"))
			captured = append(captured, capturedMessage{ChatID: chatID, Text: text, ReplyToMessageID: replyTo})

			json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": map[string]any{
					"message_id": 1,
					"chat":       map[string]any{"id": chatID, "type": "private"},
					"date":       1234567890,
					"text":       text,
				},
			})
		default:
			json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{}})
		}
	}))

	return server, &captured
}

// newTestBotWithCapture creates a bot connected to the capture server.
func newTestBotWithCapture(t *testing.T, server *httptest.Server) *Bot {
	t.Helper()
	cfg := config.Config{
		TelegramBotToken: "fake-token",
		TelegramAdminID:  12345,
		ElevenLabsAPIKey: "el-key",
	}
	b, err := newWithEndpoint(cfg, nil, server.URL+"/bot%s/%s")
	if err != nil {
		t.Fatalf("failed to create test bot: %v", err)
	}
	return b
}

// newTestBotWithStore creates a bot connected to the capture server with a real store.
func newTestBotWithStore(t *testing.T, server *httptest.Server, s *store.Store) *Bot {
	t.Helper()
	cfg := config.Config{
		TelegramBotToken: "fake-token",
		TelegramAdminID:  12345,
		ElevenLabsAPIKey: "el-key",
	}
	b, err := newWithEndpoint(cfg, s, server.URL+"/bot%s/%s")
	if err != nil {
		t.Fatalf("failed to create test bot: %v", err)
	}
	return b
}

// commandMessage builds a message that looks like a bot command.
func commandMessage(chatID int64, userID int64, text string) *tgbotapi.Message {
	// find command length (from / to first space or end of string)
	cmdLen := len(text)
	for i, c := range text {
		if c == ' ' {
			cmdLen = i
			break
		}
	}
	return &tgbotapi.Message{
		MessageID: 50,
		From:      &tgbotapi.User{ID: userID},
		Chat:      &tgbotapi.Chat{ID: chatID},
		Text:      text,
		Entities: []tgbotapi.MessageEntity{
			{Type: "bot_command", Offset: 0, Length: cmdLen},
		},
	}
}

func TestHandleCommand_Start(t *testing.T) {
	server, captured := newCaptureServer(t)
	defer server.Close()
	b := newTestBotWithCapture(t, server)

	msg := commandMessage(42, 12345, "/start")
	b.handleCommand(msg)

	if len(*captured) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(*captured))
	}
	want := "Добро пожаловать! Отправь голосовое сообщение, и я пришлю текст."
	if (*captured)[0].Text != want {
		t.Errorf("text = %q, want %q", (*captured)[0].Text, want)
	}
	if (*captured)[0].ChatID != 42 {
		t.Errorf("chat ID = %d, want 42", (*captured)[0].ChatID)
	}
}

func TestHandleCommand_Help(t *testing.T) {
	server, captured := newCaptureServer(t)
	defer server.Close()
	b := newTestBotWithCapture(t, server)

	msg := commandMessage(99, 12345, "/help")
	b.handleCommand(msg)

	if len(*captured) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(*captured))
	}
	want := "Отправьте голосовое сообщение, и бот вернёт текстовую расшифровку."
	if (*captured)[0].Text != want {
		t.Errorf("text = %q, want %q", (*captured)[0].Text, want)
	}
	if (*captured)[0].ChatID != 99 {
		t.Errorf("chat ID = %d, want 99", (*captured)[0].ChatID)
	}
}

func TestHandleCommand_Unknown(t *testing.T) {
	server, captured := newCaptureServer(t)
	defer server.Close()
	b := newTestBotWithCapture(t, server)

	msg := commandMessage(7, 12345, "/foo")
	b.handleCommand(msg)

	if len(*captured) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(*captured))
	}
	want := "Эта команда не поддерживается."
	if (*captured)[0].Text != want {
		t.Errorf("text = %q, want %q", (*captured)[0].Text, want)
	}
	if (*captured)[0].ChatID != 7 {
		t.Errorf("chat ID = %d, want 7", (*captured)[0].ChatID)
	}
}

func TestHandleStart_AuthorizedUser(t *testing.T) {
	server, captured := newCaptureServer(t)
	defer server.Close()
	s := openTestStore(t)
	defer s.Close()

	// admin user is always authorized
	b := newTestBotWithStore(t, server, s)
	msg := commandMessage(42, 12345, "/start")
	b.handleCommand(msg)

	if len(*captured) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(*captured))
	}
	want := "Добро пожаловать! Отправь голосовое сообщение, и я пришлю текст."
	if (*captured)[0].Text != want {
		t.Errorf("text = %q, want %q", (*captured)[0].Text, want)
	}
}

func TestHandleStart_UnauthorizedUser(t *testing.T) {
	server, captured := newCaptureServer(t)
	defer server.Close()
	s := openTestStore(t)
	defer s.Close()

	b := newTestBotWithStore(t, server, s)
	msg := commandMessage(42, 99999, "/start")
	b.handleCommand(msg)

	if len(*captured) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(*captured))
	}
	want := "У вас нет доступа. Обратитесь к администратору."
	if (*captured)[0].Text != want {
		t.Errorf("text = %q, want %q", (*captured)[0].Text, want)
	}
}

func TestHandleStart_ValidToken_Unauthorized(t *testing.T) {
	server, captured := newCaptureServer(t)
	defer server.Close()
	s := openTestStore(t)
	defer s.Close()

	// create an invite token
	token := "test-valid-token-1234"
	if err := s.CreateInvite(token, 12345, time.Now().Add(store.DefaultInviteTTL)); err != nil {
		t.Fatalf("failed to create invite: %v", err)
	}

	b := newTestBotWithStore(t, server, s)
	msg := commandMessage(42, 99999, "/start "+token)
	b.handleCommand(msg)

	if len(*captured) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(*captured))
	}
	want := "Добро пожаловать! Теперь вы можете отправлять голосовые сообщения."
	if (*captured)[0].Text != want {
		t.Errorf("text = %q, want %q", (*captured)[0].Text, want)
	}

	// verify user is now active
	active, err := s.IsActive(99999)
	if err != nil {
		t.Fatalf("IsActive error: %v", err)
	}
	if !active {
		t.Error("user should be active after redeeming invite")
	}
}

func TestHandleStart_InvalidToken(t *testing.T) {
	server, captured := newCaptureServer(t)
	defer server.Close()
	s := openTestStore(t)
	defer s.Close()

	b := newTestBotWithStore(t, server, s)
	msg := commandMessage(42, 99999, "/start bad-token")
	b.handleCommand(msg)

	if len(*captured) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(*captured))
	}
	want := "Недействительная или просроченная ссылка."
	if (*captured)[0].Text != want {
		t.Errorf("text = %q, want %q", (*captured)[0].Text, want)
	}
}

func TestHandleStart_TokenAlreadyAuthorized(t *testing.T) {
	server, captured := newCaptureServer(t)
	defer server.Close()
	s := openTestStore(t)
	defer s.Close()

	// create an invite token
	token := "test-token-not-consumed"
	if err := s.CreateInvite(token, 12345, time.Now().Add(store.DefaultInviteTTL)); err != nil {
		t.Fatalf("failed to create invite: %v", err)
	}

	// allow the user first
	if err := s.AllowUser(99999, 12345); err != nil {
		t.Fatalf("failed to allow user: %v", err)
	}

	b := newTestBotWithStore(t, server, s)
	msg := commandMessage(42, 99999, "/start "+token)
	b.handleCommand(msg)

	if len(*captured) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(*captured))
	}
	// authorized user gets welcome, token not consumed
	want := "Добро пожаловать! Отправь голосовое сообщение, и я пришлю текст."
	if (*captured)[0].Text != want {
		t.Errorf("text = %q, want %q", (*captured)[0].Text, want)
	}

	// verify token was NOT consumed (used_by should be NULL)
	// redeem it again to prove it's still valid
	if err := s.RedeemInvite(token, 88888, "another_user"); err != nil {
		t.Errorf("token should still be redeemable, got: %v", err)
	}
}

func TestHandleAllow_AdminSuccess(t *testing.T) {
	server, captured := newCaptureServer(t)
	defer server.Close()
	s := openTestStore(t)
	defer s.Close()

	b := newTestBotWithStore(t, server, s)
	msg := commandMessage(42, 12345, "/allow 99999")
	b.handleCommand(msg)

	if len(*captured) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(*captured))
	}
	want := "Пользователь 99999 добавлен."
	if (*captured)[0].Text != want {
		t.Errorf("text = %q, want %q", (*captured)[0].Text, want)
	}

	// verify user is now active in the store
	active, err := s.IsActive(99999)
	if err != nil {
		t.Fatalf("IsActive error: %v", err)
	}
	if !active {
		t.Error("user should be active after /allow")
	}
}

func TestHandleAllow_NonAdmin(t *testing.T) {
	server, captured := newCaptureServer(t)
	defer server.Close()
	s := openTestStore(t)
	defer s.Close()

	// allow user 99999 so they pass the auth gate, but they are NOT admin
	if err := s.AllowUser(99999, 12345); err != nil {
		t.Fatalf("failed to allow user: %v", err)
	}

	b := newTestBotWithStore(t, server, s)
	msg := commandMessage(42, 99999, "/allow 12345")
	b.handleCommand(msg)

	if len(*captured) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(*captured))
	}
	want := "Эта команда не поддерживается."
	if (*captured)[0].Text != want {
		t.Errorf("text = %q, want %q", (*captured)[0].Text, want)
	}
}

func TestHandleAllow_NoArg(t *testing.T) {
	server, captured := newCaptureServer(t)
	defer server.Close()
	s := openTestStore(t)
	defer s.Close()

	b := newTestBotWithStore(t, server, s)
	msg := commandMessage(42, 12345, "/allow")
	b.handleCommand(msg)

	if len(*captured) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(*captured))
	}
	want := "Использование: /allow <telegram_user_id>"
	if (*captured)[0].Text != want {
		t.Errorf("text = %q, want %q", (*captured)[0].Text, want)
	}
}

func TestHandleAllow_InvalidFormat(t *testing.T) {
	server, captured := newCaptureServer(t)
	defer server.Close()
	s := openTestStore(t)
	defer s.Close()

	b := newTestBotWithStore(t, server, s)
	msg := commandMessage(42, 12345, "/allow abc")
	b.handleCommand(msg)

	if len(*captured) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(*captured))
	}
	want := "Неверный формат ID. Укажите числовой Telegram ID."
	if (*captured)[0].Text != want {
		t.Errorf("text = %q, want %q", (*captured)[0].Text, want)
	}
}

func TestHandleAllow_StoreError(t *testing.T) {
	server, captured := newCaptureServer(t)
	defer server.Close()
	s := openTestStore(t)

	// close the store to force an error
	s.Close()

	b := newTestBotWithStore(t, server, s)
	msg := commandMessage(42, 12345, "/allow 99999")
	b.handleCommand(msg)

	if len(*captured) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(*captured))
	}
	want := "Ошибка при добавлении пользователя."
	if (*captured)[0].Text != want {
		t.Errorf("text = %q, want %q", (*captured)[0].Text, want)
	}
}

func TestHandleDeny_AdminSuccess(t *testing.T) {
	server, captured := newCaptureServer(t)
	defer server.Close()
	s := openTestStore(t)
	defer s.Close()

	// allow user first so they can be denied
	if err := s.AllowUser(99999, 12345); err != nil {
		t.Fatalf("AllowUser: %v", err)
	}

	b := newTestBotWithStore(t, server, s)
	msg := commandMessage(42, 12345, "/deny 99999")
	b.handleCommand(msg)

	if len(*captured) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(*captured))
	}
	want := "Пользователь 99999 заблокирован."
	if (*captured)[0].Text != want {
		t.Errorf("text = %q, want %q", (*captured)[0].Text, want)
	}

	// verify user is now blocked
	active, err := s.IsActive(99999)
	if err != nil {
		t.Fatalf("IsActive error: %v", err)
	}
	if active {
		t.Error("user should not be active after /deny")
	}
}

func TestHandleDeny_NonAdmin(t *testing.T) {
	server, captured := newCaptureServer(t)
	defer server.Close()
	s := openTestStore(t)
	defer s.Close()

	if err := s.AllowUser(99999, 12345); err != nil {
		t.Fatalf("AllowUser: %v", err)
	}

	b := newTestBotWithStore(t, server, s)
	msg := commandMessage(42, 99999, "/deny 12345")
	b.handleCommand(msg)

	if len(*captured) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(*captured))
	}
	want := "Эта команда не поддерживается."
	if (*captured)[0].Text != want {
		t.Errorf("text = %q, want %q", (*captured)[0].Text, want)
	}
}

func TestHandleDeny_SelfDeny(t *testing.T) {
	server, captured := newCaptureServer(t)
	defer server.Close()
	s := openTestStore(t)
	defer s.Close()

	b := newTestBotWithStore(t, server, s)
	msg := commandMessage(42, 12345, "/deny 12345")
	b.handleCommand(msg)

	if len(*captured) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(*captured))
	}
	want := "Нельзя заблокировать администратора."
	if (*captured)[0].Text != want {
		t.Errorf("text = %q, want %q", (*captured)[0].Text, want)
	}
}

func TestHandleList_AdminWithUsers(t *testing.T) {
	server, captured := newCaptureServer(t)
	defer server.Close()
	s := openTestStore(t)
	defer s.Close()

	// add users with known usernames
	if err := s.AllowUser(111, 12345); err != nil {
		t.Fatalf("AllowUser: %v", err)
	}
	// add a user with a username by redeeming an invite
	token := "list-test-token"
	if err := s.CreateInvite(token, 12345, time.Now().Add(store.DefaultInviteTTL)); err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	if err := s.RedeemInvite(token, 222, "alice"); err != nil {
		t.Fatalf("RedeemInvite: %v", err)
	}

	b := newTestBotWithStore(t, server, s)
	msg := commandMessage(42, 12345, "/list")
	b.handleCommand(msg)

	if len(*captured) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(*captured))
	}
	text := (*captured)[0].Text
	// user 111 has no username
	if !strings.Contains(text, "111") {
		t.Errorf("expected text to contain user 111, got %q", text)
	}
	// user 222 has username alice
	if !strings.Contains(text, "@alice") {
		t.Errorf("expected text to contain @alice, got %q", text)
	}
	if !strings.Contains(text, "active") {
		t.Errorf("expected text to contain status 'active', got %q", text)
	}
}

func TestHandleList_AdminEmpty(t *testing.T) {
	server, captured := newCaptureServer(t)
	defer server.Close()
	s := openTestStore(t)
	defer s.Close()

	b := newTestBotWithStore(t, server, s)
	msg := commandMessage(42, 12345, "/list")
	b.handleCommand(msg)

	if len(*captured) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(*captured))
	}
	want := "Нет зарегистрированных пользователей."
	if (*captured)[0].Text != want {
		t.Errorf("text = %q, want %q", (*captured)[0].Text, want)
	}
}

func TestHandleList_NonAdmin(t *testing.T) {
	server, captured := newCaptureServer(t)
	defer server.Close()
	s := openTestStore(t)
	defer s.Close()

	if err := s.AllowUser(99999, 12345); err != nil {
		t.Fatalf("AllowUser: %v", err)
	}

	b := newTestBotWithStore(t, server, s)
	msg := commandMessage(42, 99999, "/list")
	b.handleCommand(msg)

	if len(*captured) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(*captured))
	}
	want := "Эта команда не поддерживается."
	if (*captured)[0].Text != want {
		t.Errorf("text = %q, want %q", (*captured)[0].Text, want)
	}
}

func TestHandleList_StoreError(t *testing.T) {
	server, captured := newCaptureServer(t)
	defer server.Close()
	s := openTestStore(t)

	// close the store to force an error
	s.Close()

	b := newTestBotWithStore(t, server, s)
	msg := commandMessage(42, 12345, "/list")
	b.handleCommand(msg)

	if len(*captured) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(*captured))
	}
	want := "Ошибка при получении списка пользователей."
	if (*captured)[0].Text != want {
		t.Errorf("text = %q, want %q", (*captured)[0].Text, want)
	}
}

func TestHandleInvite_AdminSuccess(t *testing.T) {
	server, captured := newCaptureServer(t)
	defer server.Close()
	s := openTestStore(t)
	defer s.Close()

	b := newTestBotWithStore(t, server, s)
	msg := commandMessage(42, 12345, "/invite")
	b.handleCommand(msg)

	if len(*captured) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(*captured))
	}
	text := (*captured)[0].Text
	if !strings.Contains(text, "https://t.me/test_bot?start=") {
		t.Errorf("expected deep link in reply, got %q", text)
	}
	if !strings.Contains(text, "72 ч") {
		t.Errorf("expected expiration note in reply, got %q", text)
	}
}

func TestHandleInvite_NonAdmin(t *testing.T) {
	server, captured := newCaptureServer(t)
	defer server.Close()
	s := openTestStore(t)
	defer s.Close()

	if err := s.AllowUser(99999, 12345); err != nil {
		t.Fatalf("AllowUser: %v", err)
	}

	b := newTestBotWithStore(t, server, s)
	msg := commandMessage(42, 99999, "/invite")
	b.handleCommand(msg)

	if len(*captured) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(*captured))
	}
	want := "Эта команда не поддерживается."
	if (*captured)[0].Text != want {
		t.Errorf("text = %q, want %q", (*captured)[0].Text, want)
	}
}

func TestHandleInvite_StoreError(t *testing.T) {
	server, captured := newCaptureServer(t)
	defer server.Close()
	s := openTestStore(t)

	// close the store to force an error
	s.Close()

	b := newTestBotWithStore(t, server, s)
	msg := commandMessage(42, 12345, "/invite")
	b.handleCommand(msg)

	if len(*captured) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(*captured))
	}
	want := "Ошибка при создании приглашения."
	if (*captured)[0].Text != want {
		t.Errorf("text = %q, want %q", (*captured)[0].Text, want)
	}
}

func TestHandleDeny_UserNotFound(t *testing.T) {
	server, captured := newCaptureServer(t)
	defer server.Close()
	s := openTestStore(t)
	defer s.Close()

	b := newTestBotWithStore(t, server, s)
	msg := commandMessage(42, 12345, "/deny 99999")
	b.handleCommand(msg)

	if len(*captured) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(*captured))
	}
	want := "Пользователь не найден."
	if (*captured)[0].Text != want {
		t.Errorf("text = %q, want %q", (*captured)[0].Text, want)
	}
}
