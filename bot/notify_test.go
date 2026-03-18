package bot

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// newNotifierTestServer creates a fake Telegram API server that counts
// sendMessage calls and records the last message text.
func newNotifierTestServer(t *testing.T) (*httptest.Server, *atomic.Int32, *atomic.Value) {
	t.Helper()
	var sendCount atomic.Int32
	var lastText atomic.Value

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.HasSuffix(r.URL.Path, "getMe"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": map[string]any{
					"id": 123, "is_bot": true,
					"first_name": "TestBot", "username": "test_bot",
				},
			})
		case strings.HasSuffix(r.URL.Path, "sendMessage"):
			_ = r.ParseForm()
			lastText.Store(r.FormValue("text"))
			sendCount.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": map[string]any{
					"message_id": 1,
					"date":       1234567890,
					"from":       map[string]any{"id": 123, "is_bot": true},
					"chat":       map[string]any{"id": 99, "type": "private"},
					"text":       "ok",
				},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{}})
		}
	}))

	return server, &sendCount, &lastText
}

func newTestNotifier(t *testing.T, server *httptest.Server, adminID int64) *adminNotifier {
	t.Helper()
	api, err := tgbotapi.NewBotAPIWithAPIEndpoint("fake-token", server.URL+"/bot%s/%s")
	if err != nil {
		t.Fatalf("failed to create bot api: %v", err)
	}
	return newAdminNotifier(api, adminID)
}

func TestNotify_SendsOnFirstCall(t *testing.T) {
	server, sendCount, lastText := newNotifierTestServer(t)
	defer server.Close()

	n := newTestNotifier(t, server, 99)

	n.Notify("test_category", "something went wrong")

	if got := sendCount.Load(); got != 1 {
		t.Fatalf("expected 1 sendMessage call, got %d", got)
	}

	text, _ := lastText.Load().(string)
	if !strings.Contains(text, "[test_category]") {
		t.Errorf("message should contain category prefix, got: %s", text)
	}
	if !strings.Contains(text, "something went wrong") {
		t.Errorf("message should contain original text, got: %s", text)
	}
}

func TestNotify_SuppressesDuplicateWithinCooldown(t *testing.T) {
	server, sendCount, _ := newNotifierTestServer(t)
	defer server.Close()

	n := newTestNotifier(t, server, 99)
	n.cooldown = 1 * time.Minute

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	n.now = func() time.Time { return now }

	n.Notify("error", "first")
	n.Notify("error", "second") // same category, should be suppressed

	if got := sendCount.Load(); got != 1 {
		t.Fatalf("expected 1 sendMessage call (second suppressed), got %d", got)
	}
}

func TestNotify_SendsAgainAfterCooldownExpires(t *testing.T) {
	server, sendCount, _ := newNotifierTestServer(t)
	defer server.Close()

	n := newTestNotifier(t, server, 99)
	n.cooldown = 1 * time.Minute

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	n.now = func() time.Time { return now }

	n.Notify("error", "first")
	if got := sendCount.Load(); got != 1 {
		t.Fatalf("expected 1 call after first notify, got %d", got)
	}

	// advance past cooldown
	now = now.Add(2 * time.Minute)

	n.Notify("error", "second")
	if got := sendCount.Load(); got != 2 {
		t.Fatalf("expected 2 calls after cooldown expired, got %d", got)
	}
}

func TestNotify_DifferentCategoriesAreIndependent(t *testing.T) {
	server, sendCount, _ := newNotifierTestServer(t)
	defer server.Close()

	n := newTestNotifier(t, server, 99)
	n.cooldown = 1 * time.Minute

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	n.now = func() time.Time { return now }

	n.Notify("panic", "oops")
	n.Notify("elevenlabs", "api down")

	if got := sendCount.Load(); got != 2 {
		t.Fatalf("expected 2 calls for different categories, got %d", got)
	}
}

func TestNotify_NilNotifierIsNoop(t *testing.T) {
	var n *adminNotifier
	// should not panic
	n.Notify("test", "this should be silently ignored")
}

func TestNotify_TruncatesLongMessages(t *testing.T) {
	server, sendCount, lastText := newNotifierTestServer(t)
	defer server.Close()

	n := newTestNotifier(t, server, 99)

	// create a message that exceeds 4096 chars
	long := strings.Repeat("a", 5000)
	n.Notify("test", long)

	if got := sendCount.Load(); got != 1 {
		t.Fatalf("expected 1 sendMessage call, got %d", got)
	}

	text, _ := lastText.Load().(string)
	if len([]rune(text)) > maxMessageLength {
		t.Errorf("message length %d exceeds max %d", len([]rune(text)), maxMessageLength)
	}
}
