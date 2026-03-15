package bot

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/yalexaner/otterly/config"
)

// capturedMessage holds the chat_id and text from a sendMessage request.
type capturedMessage struct {
	ChatID int64
	Text   string
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
			captured = append(captured, capturedMessage{ChatID: chatID, Text: text})

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
	b, err := newWithEndpoint(cfg, server.URL+"/bot%s/%s")
	if err != nil {
		t.Fatalf("failed to create test bot: %v", err)
	}
	return b
}

// commandMessage builds a message that looks like a bot command.
func commandMessage(chatID int64, text string) *tgbotapi.Message {
	// find command length (from / to first space or end of string)
	cmdLen := len(text)
	for i, c := range text {
		if c == ' ' {
			cmdLen = i
			break
		}
	}
	return &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: chatID},
		Text: text,
		Entities: []tgbotapi.MessageEntity{
			{Type: "bot_command", Offset: 0, Length: cmdLen},
		},
	}
}

func TestHandleCommand_Start(t *testing.T) {
	server, captured := newCaptureServer(t)
	defer server.Close()
	b := newTestBotWithCapture(t, server)

	msg := commandMessage(42, "/start")
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

	msg := commandMessage(99, "/help")
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

	msg := commandMessage(7, "/foo")
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
