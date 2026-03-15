package bot

import (
	"net/http"
	"net/http/httptest"
	"testing"

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
		TelegramAdminID:  "12345",
		ElevenLabsAPIKey: "el-key",
	}

	b, err := newWithEndpoint(cfg, server.URL+"/bot%s/%s")
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
		TelegramAdminID:  "12345",
		ElevenLabsAPIKey: "el-key",
	}

	b, err := newWithEndpoint(cfg, server.URL+"/bot%s/%s")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
	if b != nil {
		t.Fatal("expected nil bot on error")
	}
}
