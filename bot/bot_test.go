package bot

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/yalexaner/otterly/config"
)

// newTestServer creates a fake Telegram API server that returns a valid getMe response.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true,"result":{"id":123,"is_bot":true,"first_name":"TestBot","username":"test_bot"}}`))
	}))
}

func TestNew_ValidToken(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	cfg := config.Config{
		TelegramBotToken: "fake-token",
		TelegramAdminID:  12345,
		ElevenLabsAPIKey: "el-key",
	}

	b, err := newWithEndpoint(cfg, nil, server.URL+"/bot%s/%s")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil bot")
	}
	if b.api == nil {
		t.Fatal("expected non-nil api client")
	}
	if b.cfg.TelegramBotToken != "fake-token" {
		t.Errorf("config token = %q, want %q", b.cfg.TelegramBotToken, "fake-token")
	}
}

func TestNew_InvalidToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"ok":false,"error_code":401,"description":"Unauthorized"}`))
	}))
	defer server.Close()

	cfg := config.Config{
		TelegramBotToken: "bad-token",
		TelegramAdminID:  12345,
		ElevenLabsAPIKey: "el-key",
	}

	b, err := newWithEndpoint(cfg, nil, server.URL+"/bot%s/%s")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
	if b != nil {
		t.Fatal("expected nil bot on error")
	}
}

// capturedSetMyCommands holds parameters from a setMyCommands API call.
type capturedSetMyCommands struct {
	Commands []tgbotapi.BotCommand
	Scope    *tgbotapi.BotCommandScope
}

// newSetMyCommandsCaptureServer creates a fake Telegram API server that records setMyCommands calls.
func newSetMyCommandsCaptureServer(t *testing.T) (*httptest.Server, *[]capturedSetMyCommands) {
	t.Helper()
	var captured []capturedSetMyCommands

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
		case strings.HasSuffix(r.URL.Path, "setMyCommands"):
			r.ParseForm()
			var cmds []tgbotapi.BotCommand
			if raw := r.FormValue("commands"); raw != "" {
				json.Unmarshal([]byte(raw), &cmds)
			}
			var scope *tgbotapi.BotCommandScope
			if raw := r.FormValue("scope"); raw != "" {
				scope = &tgbotapi.BotCommandScope{}
				json.Unmarshal([]byte(raw), scope)
			}
			captured = append(captured, capturedSetMyCommands{Commands: cmds, Scope: scope})
			json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true})
		default:
			json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{}})
		}
	}))

	return server, &captured
}

func TestRegisterCommands_DefaultAndAdminScope(t *testing.T) {
	server, captured := newSetMyCommandsCaptureServer(t)
	defer server.Close()

	cfg := config.Config{
		TelegramBotToken: "fake-token",
		TelegramAdminID:  12345,
		ElevenLabsAPIKey: "el-key",
	}
	b, err := newWithEndpoint(cfg, nil, server.URL+"/bot%s/%s")
	if err != nil {
		t.Fatalf("failed to create bot: %v", err)
	}

	b.registerCommands()

	if len(*captured) != 2 {
		t.Fatalf("expected 2 setMyCommands calls, got %d", len(*captured))
	}

	// first call: default scope (no scope set)
	defaultCall := (*captured)[0]
	if defaultCall.Scope != nil {
		t.Errorf("default scope call should have nil scope, got %+v", defaultCall.Scope)
	}
	if len(defaultCall.Commands) != 2 {
		t.Fatalf("expected 2 default commands, got %d", len(defaultCall.Commands))
	}
	if defaultCall.Commands[0].Command != "start" {
		t.Errorf("default command[0] = %q, want 'start'", defaultCall.Commands[0].Command)
	}
	if defaultCall.Commands[1].Command != "help" {
		t.Errorf("default command[1] = %q, want 'help'", defaultCall.Commands[1].Command)
	}

	// second call: admin scope
	adminCall := (*captured)[1]
	if adminCall.Scope == nil {
		t.Fatal("admin scope call should have a scope set")
	}
	if adminCall.Scope.Type != "chat" {
		t.Errorf("admin scope type = %q, want 'chat'", adminCall.Scope.Type)
	}
	if adminCall.Scope.ChatID != 12345 {
		t.Errorf("admin scope chat_id = %d, want 12345", adminCall.Scope.ChatID)
	}
	if len(adminCall.Commands) != 6 {
		t.Fatalf("expected 6 admin commands, got %d", len(adminCall.Commands))
	}
	wantCmds := []string{"start", "help", "allow", "deny", "list", "invite"}
	for i, want := range wantCmds {
		if adminCall.Commands[i].Command != want {
			t.Errorf("admin command[%d] = %q, want %q", i, adminCall.Commands[i].Command, want)
		}
	}
}

func TestRegisterCommands_SkipsAdminScopeWhenNoAdmin(t *testing.T) {
	server, captured := newSetMyCommandsCaptureServer(t)
	defer server.Close()

	cfg := config.Config{
		TelegramBotToken: "fake-token",
		TelegramAdminID:  0,
		ElevenLabsAPIKey: "el-key",
	}
	b, err := newWithEndpoint(cfg, nil, server.URL+"/bot%s/%s")
	if err != nil {
		t.Fatalf("failed to create bot: %v", err)
	}

	b.registerCommands()

	if len(*captured) != 1 {
		t.Fatalf("expected 1 setMyCommands call (default only), got %d", len(*captured))
	}
	if (*captured)[0].Scope != nil {
		t.Error("should only have default scope call when admin ID is 0")
	}
}
