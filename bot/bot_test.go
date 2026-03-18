package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/yalexaner/otterly/config"
)

// newTestServer creates a fake Telegram API server that returns a valid getMe response.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"id":123,"is_bot":true,"first_name":"TestBot","username":"test_bot"}}`))
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
		_, _ = w.Write([]byte(`{"ok":false,"error_code":401,"description":"Unauthorized"}`))
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
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": map[string]any{
					"id": 123, "is_bot": true,
					"first_name": "TestBot", "username": "test_bot",
				},
			})
		case strings.HasSuffix(r.URL.Path, "setMyCommands"):
			_ = r.ParseForm()
			var cmds []tgbotapi.BotCommand
			if raw := r.FormValue("commands"); raw != "" {
				_ = json.Unmarshal([]byte(raw), &cmds)
			}
			var scope *tgbotapi.BotCommandScope
			if raw := r.FormValue("scope"); raw != "" {
				scope = &tgbotapi.BotCommandScope{}
				_ = json.Unmarshal([]byte(raw), scope)
			}
			captured = append(captured, capturedSetMyCommands{Commands: cmds, Scope: scope})
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{}})
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

func TestStart_ExitsOnContextCancel(t *testing.T) {
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
		case strings.HasSuffix(r.URL.Path, "getUpdates"):
			// return empty updates so the polling loop stays alive
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":     true,
				"result": []any{},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true})
		}
	}))
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

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		b.Start(ctx)
		close(done)
	}()

	// give the polling loop time to start
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Start() returned — success
	case <-time.After(5 * time.Second):
		t.Fatal("Start() did not return after context cancellation")
	}
}

// slowTranscriber signals when it starts and blocks for the configured duration.
type slowTranscriber struct {
	started chan struct{}
	delay   time.Duration
}

func (s *slowTranscriber) Transcribe(_ context.Context, _ string) (string, error) {
	close(s.started)
	time.Sleep(s.delay)
	return "transcribed text", nil
}

// hangingTranscriber blocks until explicitly unblocked.
type hangingTranscriber struct {
	started chan struct{}
	unblock chan struct{}
}

func (h *hangingTranscriber) Transcribe(_ context.Context, _ string) (string, error) {
	close(h.started)
	<-h.unblock
	return "transcribed text", nil
}

// newVoiceUpdateServer creates a fake Telegram API server that delivers one voice
// message update on the first getUpdates call, then returns empty updates.
func newVoiceUpdateServer(t *testing.T) *httptest.Server {
	t.Helper()
	var updatesSent atomic.Bool

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		case strings.HasSuffix(r.URL.Path, "setMyCommands"):
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true})
		case strings.HasSuffix(r.URL.Path, "getUpdates"):
			if updatesSent.CompareAndSwap(false, true) {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"ok": true,
					"result": []map[string]any{{
						"update_id": 1,
						"message": map[string]any{
							"message_id": 1,
							"date":       1234567890,
							"from":       map[string]any{"id": 42, "is_bot": false, "first_name": "Test"},
							"chat":       map[string]any{"id": 42, "type": "private"},
							"voice":      map[string]any{"file_id": "test-file", "duration": 5, "file_size": 100},
						},
					}},
				})
			} else {
				time.Sleep(500 * time.Millisecond)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": []any{}})
			}
		case strings.HasSuffix(r.URL.Path, "getFile"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": map[string]any{
					"file_id":   "test-file",
					"file_path": "voice/file.ogg",
					"file_size": 100,
				},
			})
		case strings.Contains(r.URL.Path, "/file/"):
			w.Header().Set("Content-Type", "audio/ogg")
			_, _ = w.Write([]byte("fake ogg data"))
		case strings.HasSuffix(r.URL.Path, "sendMessage"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": map[string]any{
					"message_id": 2,
					"date":       1234567890,
					"from":       map[string]any{"id": 123, "is_bot": true},
					"chat":       map[string]any{"id": 42, "type": "private"},
					"text":       "ok",
				},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{}})
		}
	}))
}

func TestStart_DrainCompletesBeforeTimeout(t *testing.T) {
	server := newVoiceUpdateServer(t)
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

	tr := &slowTranscriber{
		started: make(chan struct{}),
		delay:   300 * time.Millisecond,
	}
	b.transcriber = tr
	b.convertToWAV = func(_ context.Context, path string) (string, error) { return path, nil }
	b.drainTimeout = 5 * time.Second

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		b.Start(ctx)
		close(done)
	}()

	// wait for transcriber to start processing
	select {
	case <-tr.started:
	case <-time.After(5 * time.Second):
		t.Fatal("transcriber did not start within timeout")
	}

	// cancel context while handler is in-flight
	cancel()

	// Start() should return after handler completes (~300ms), not after drain timeout (5s)
	select {
	case <-done:
		// success — handler completed and shutdown was clean
	case <-time.After(3 * time.Second):
		t.Fatal("Start() did not return after in-flight handler completed")
	}
}

func TestStart_DrainTimesOutOnHangingHandler(t *testing.T) {
	server := newVoiceUpdateServer(t)
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

	tr := &hangingTranscriber{
		started: make(chan struct{}),
		unblock: make(chan struct{}),
	}
	b.transcriber = tr
	b.convertToWAV = func(_ context.Context, path string) (string, error) { return path, nil }
	b.drainTimeout = 200 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		b.Start(ctx)
		close(done)
	}()

	// wait for transcriber to start processing
	select {
	case <-tr.started:
	case <-time.After(5 * time.Second):
		t.Fatal("transcriber did not start within timeout")
	}

	// cancel context while handler is hung
	cancel()

	// Start() should return after drain timeout (~200ms), not hang forever
	select {
	case <-done:
		// success — shutdown timed out and Start returned
	case <-time.After(3 * time.Second):
		t.Fatal("Start() did not return after drain timeout")
	}

	// unblock the hanging transcriber to prevent goroutine leak
	close(tr.unblock)
}

// capturedReply records a sendMessage call's chat ID and text.
type capturedReply struct {
	chatID string
	text   string
}

// alwaysPanicTranscriber panics on every call.
type alwaysPanicTranscriber struct{}

func (p *alwaysPanicTranscriber) Transcribe(_ context.Context, _ string) (string, error) {
	panic("test panic in transcriber")
}

// panicOnceTranscriber panics on the first call and succeeds on subsequent calls.
type panicOnceTranscriber struct {
	panicked atomic.Bool
}

func (p *panicOnceTranscriber) Transcribe(_ context.Context, _ string) (string, error) {
	if p.panicked.CompareAndSwap(false, true) {
		panic("test panic in transcriber")
	}
	return "transcribed text", nil
}

// newMessageCaptureServer creates a fake Telegram API server that delivers
// the given number of voice updates on the first getUpdates call, then returns
// empty updates. All sendMessage calls are captured into the returned channel.
func newMessageCaptureServer(t *testing.T, voiceUpdateCount int) (*httptest.Server, chan capturedReply) {
	t.Helper()
	var updatesSent atomic.Bool
	replies := make(chan capturedReply, 20)

	// build voice updates
	updates := make([]map[string]any, voiceUpdateCount)
	for i := range updates {
		updates[i] = map[string]any{
			"update_id": i + 1,
			"message": map[string]any{
				"message_id": i + 1,
				"date":       1234567890,
				"from":       map[string]any{"id": 42, "is_bot": false, "first_name": "Test"},
				"chat":       map[string]any{"id": 42, "type": "private"},
				"voice":      map[string]any{"file_id": fmt.Sprintf("test-file-%d", i+1), "duration": 5, "file_size": 100},
			},
		}
	}

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
		case strings.HasSuffix(r.URL.Path, "setMyCommands"):
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true})
		case strings.HasSuffix(r.URL.Path, "getUpdates"):
			if updatesSent.CompareAndSwap(false, true) {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"ok":     true,
					"result": updates,
				})
			} else {
				time.Sleep(500 * time.Millisecond)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": []any{}})
			}
		case strings.HasSuffix(r.URL.Path, "getFile"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": map[string]any{
					"file_id":   "test-file",
					"file_path": "voice/file.ogg",
					"file_size": 100,
				},
			})
		case strings.Contains(r.URL.Path, "/file/"):
			w.Header().Set("Content-Type", "audio/ogg")
			_, _ = w.Write([]byte("fake ogg data"))
		case strings.HasSuffix(r.URL.Path, "sendMessage"):
			_ = r.ParseForm()
			replies <- capturedReply{
				chatID: r.FormValue("chat_id"),
				text:   r.FormValue("text"),
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": map[string]any{
					"message_id": 99,
					"date":       1234567890,
					"from":       map[string]any{"id": 123, "is_bot": true},
					"chat":       map[string]any{"id": 42, "type": "private"},
					"text":       "ok",
				},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{}})
		}
	}))

	return server, replies
}

// collectReplies reads from the channel until the expected count is reached or timeout.
func collectReplies(ch chan capturedReply, count int, timeout time.Duration) []capturedReply {
	var collected []capturedReply
	deadline := time.After(timeout)
	for len(collected) < count {
		select {
		case m := <-ch:
			collected = append(collected, m)
		case <-deadline:
			return collected
		}
	}
	return collected
}

func TestPanicRecovery_BotContinuesProcessing(t *testing.T) {
	server, replies := newMessageCaptureServer(t, 2)
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
	b.transcriber = &panicOnceTranscriber{}
	b.convertToWAV = func(_ context.Context, path string) (string, error) { return path, nil }

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		b.Start(ctx)
		close(done)
	}()

	// expect 3 messages: error reply to user, admin notification, transcription reply
	collected := collectReplies(replies, 3, 10*time.Second)
	cancel()
	<-done

	if len(collected) < 3 {
		t.Fatalf("expected 3 messages, got %d: %+v", len(collected), collected)
	}

	// verify at least one message to user contains the transcription
	hasTranscription := false
	for _, m := range collected {
		if m.chatID == "42" && m.text == "transcribed text" {
			hasTranscription = true
		}
	}
	if !hasTranscription {
		t.Errorf("expected transcription reply after panic recovery, got: %+v", collected)
	}
}

func TestPanicRecovery_AdminNotified(t *testing.T) {
	server, replies := newMessageCaptureServer(t, 1)
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
	b.transcriber = &alwaysPanicTranscriber{}
	b.convertToWAV = func(_ context.Context, path string) (string, error) { return path, nil }

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		b.Start(ctx)
		close(done)
	}()

	// expect 2 messages: admin notification + user error reply
	collected := collectReplies(replies, 2, 10*time.Second)
	cancel()
	<-done

	if len(collected) < 2 {
		t.Fatalf("expected 2 messages, got %d: %+v", len(collected), collected)
	}

	adminNotified := false
	for _, m := range collected {
		if m.chatID == "12345" && strings.Contains(m.text, "[panic]") && strings.Contains(m.text, "test panic") {
			adminNotified = true
		}
	}
	if !adminNotified {
		t.Errorf("expected admin notification with panic info, got: %+v", collected)
	}
}

func TestPanicRecovery_UserGetsErrorReply(t *testing.T) {
	server, replies := newMessageCaptureServer(t, 1)
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
	b.transcriber = &alwaysPanicTranscriber{}
	b.convertToWAV = func(_ context.Context, path string) (string, error) { return path, nil }

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		b.Start(ctx)
		close(done)
	}()

	// expect 2 messages: admin notification + user error reply
	collected := collectReplies(replies, 2, 10*time.Second)
	cancel()
	<-done

	if len(collected) < 2 {
		t.Fatalf("expected 2 messages, got %d: %+v", len(collected), collected)
	}

	userGotError := false
	for _, m := range collected {
		if m.chatID == "42" && m.text == "Произошла внутренняя ошибка. Попробуйте позже." {
			userGotError = true
		}
	}
	if !userGotError {
		t.Errorf("expected error reply to user, got: %+v", collected)
	}
}
