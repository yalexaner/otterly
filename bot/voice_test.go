package bot

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// voiceServerOpts configures the behavior of the fake voice test server.
type voiceServerOpts struct {
	getFileErr         bool   // if true, getFile returns an error
	fileDownloadStatus int    // HTTP status for file download (0 means 200)
	fileContent        []byte // content to serve for file download
}

// newVoiceServer creates a fake Telegram API server that handles getMe,
// getFile, sendMessage, and file download routes.
func newVoiceServer(t *testing.T, opts voiceServerOpts) (*httptest.Server, *[]capturedMessage) {
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

		case strings.HasSuffix(r.URL.Path, "getFile"):
			if opts.getFileErr {
				json.NewEncoder(w).Encode(map[string]any{
					"ok":          false,
					"error_code":  400,
					"description": "Bad Request: file not found",
				})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": map[string]any{
					"file_id":        "test-file-id",
					"file_unique_id": "unique-123",
					"file_size":      len(opts.fileContent),
					"file_path":      "voice/file_123.ogg",
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

		case strings.Contains(r.URL.Path, "/file/bot"):
			if opts.fileDownloadStatus != 0 && opts.fileDownloadStatus != http.StatusOK {
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(opts.fileDownloadStatus)
				w.Write([]byte("not found"))
				return
			}
			w.Header().Set("Content-Type", "audio/ogg")
			w.Write(opts.fileContent)

		default:
			json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{}})
		}
	}))

	return server, &captured
}

// voiceMessage builds a message with a Voice attachment.
func voiceMessage(chatID int64, fileID string, duration int) *tgbotapi.Message {
	return &tgbotapi.Message{
		MessageID: 100,
		Chat:      &tgbotapi.Chat{ID: chatID},
		Voice:     &tgbotapi.Voice{FileID: fileID, Duration: duration},
	}
}

// fakeTranscriber implements the transcriber interface for tests.
type fakeTranscriber struct {
	text        string
	err         error
	capturedPath string
}

func (f *fakeTranscriber) Transcribe(wavPath string) (string, error) {
	f.capturedPath = wavPath
	return f.text, f.err
}

// mockConverter returns a converter that creates a WAV temp file with the given content.
func mockConverter(t *testing.T) func(string) (string, error) {
	t.Helper()
	return func(inputPath string) (string, error) {
		tmp, err := os.CreateTemp("", "test-*.wav")
		if err != nil {
			t.Fatal(err)
		}
		tmp.Write([]byte("fake-wav-data"))
		tmp.Close()
		return tmp.Name(), nil
	}
}

func TestHandleVoice_Success(t *testing.T) {
	fakeOGG := []byte("fake-ogg-data")
	server, captured := newVoiceServer(t, voiceServerOpts{fileContent: fakeOGG})
	defer server.Close()

	b := newTestBotWithCapture(t, server)

	var convertedInput string
	b.convertToWAV = func(inputPath string) (string, error) {
		convertedInput = inputPath
		tmp, err := os.CreateTemp("", "test-*.wav")
		if err != nil {
			t.Fatal(err)
		}
		tmp.Write([]byte("fake-wav-data"))
		tmp.Close()
		return tmp.Name(), nil
	}
	b.transcriber = &fakeTranscriber{text: "привет мир"}

	msg := voiceMessage(42, "test-file-id", 5)

	text, err := b.handleVoice(msg)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// verify transcript returned
	if text != "привет мир" {
		t.Errorf("text = %q, want %q", text, "привет мир")
	}

	// verify conversion was called with OGG path
	if convertedInput == "" {
		t.Fatal("convertToWAV was not called")
	}
	if !strings.HasSuffix(convertedInput, ".ogg") {
		t.Errorf("conversion input %q does not end with .ogg", convertedInput)
	}

	// verify OGG temp file was cleaned up
	if _, err := os.Stat(convertedInput); !os.IsNotExist(err) {
		t.Errorf("OGG temp file %q was not cleaned up", convertedInput)
	}

	// verify transcript reply was sent
	if len(*captured) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(*captured))
	}
	if (*captured)[0].Text != "привет мир" {
		t.Errorf("reply = %q, want transcript text", (*captured)[0].Text)
	}
	if (*captured)[0].ChatID != 42 {
		t.Errorf("reply chat ID = %d, want 42", (*captured)[0].ChatID)
	}
	if (*captured)[0].ReplyToMessageID != 100 {
		t.Errorf("reply_to_message_id = %d, want 100", (*captured)[0].ReplyToMessageID)
	}
}

func TestHandleVoice_GetFileError(t *testing.T) {
	server, captured := newVoiceServer(t, voiceServerOpts{getFileErr: true})
	defer server.Close()

	b := newTestBotWithCapture(t, server)
	msg := voiceMessage(42, "test-file-id", 5)

	path, err := b.handleVoice(msg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if path != "" {
		t.Errorf("expected empty path, got %q", path)
	}

	// verify error reply was sent
	if len(*captured) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(*captured))
	}
	if (*captured)[0].Text != "Не удалось загрузить голосовое сообщение." {
		t.Errorf("reply = %q, want error message", (*captured)[0].Text)
	}
}

func TestHandleVoice_DownloadError(t *testing.T) {
	server, captured := newVoiceServer(t, voiceServerOpts{fileDownloadStatus: http.StatusNotFound})
	defer server.Close()

	b := newTestBotWithCapture(t, server)
	msg := voiceMessage(42, "test-file-id", 5)

	path, err := b.handleVoice(msg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if path != "" {
		t.Errorf("expected empty path, got %q", path)
	}

	// verify error reply was sent
	if len(*captured) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(*captured))
	}
	if (*captured)[0].Text != "Не удалось загрузить голосовое сообщение." {
		t.Errorf("reply = %q, want error message", (*captured)[0].Text)
	}
}

func TestHandleVoice_TooLong(t *testing.T) {
	server, captured := newCaptureServer(t)
	defer server.Close()

	b := newTestBotWithCapture(t, server)
	msg := voiceMessage(42, "test-file-id", 91)

	path, err := b.handleVoice(msg)
	if err == nil {
		t.Fatal("expected error for voice >90s, got nil")
	}
	if path != "" {
		t.Errorf("expected empty path, got %q", path)
	}
	if !strings.Contains(err.Error(), "too long") {
		t.Errorf("error = %q, want it to contain 'too long'", err)
	}

	// verify rejection reply was sent
	if len(*captured) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(*captured))
	}
	want := "Сообщение слишком длинное (более 90 секунд)."
	if (*captured)[0].Text != want {
		t.Errorf("reply = %q, want %q", (*captured)[0].Text, want)
	}
	if (*captured)[0].ChatID != 42 {
		t.Errorf("reply chat ID = %d, want 42", (*captured)[0].ChatID)
	}
	if (*captured)[0].ReplyToMessageID != 100 {
		t.Errorf("reply_to_message_id = %d, want 100", (*captured)[0].ReplyToMessageID)
	}
}

func TestHandleVoice_ConversionFails(t *testing.T) {
	fakeOGG := []byte("fake-ogg-data")
	server, captured := newVoiceServer(t, voiceServerOpts{fileContent: fakeOGG})
	defer server.Close()

	b := newTestBotWithCapture(t, server)

	var convertedInput string
	b.convertToWAV = func(inputPath string) (string, error) {
		convertedInput = inputPath
		return "", fmt.Errorf("ffmpeg not found")
	}

	msg := voiceMessage(42, "test-file-id", 5)
	path, err := b.handleVoice(msg)
	if err == nil {
		t.Fatal("expected error when conversion fails, got nil")
	}
	if path != "" {
		os.Remove(path)
		t.Errorf("expected empty path, got %q", path)
	}
	if !strings.Contains(err.Error(), "convert to wav") {
		t.Errorf("error = %q, want it to contain 'convert to wav'", err)
	}

	// verify OGG temp file was cleaned up despite conversion failure
	if convertedInput == "" {
		t.Fatal("convertToWAV was not called")
	}
	if _, err := os.Stat(convertedInput); !os.IsNotExist(err) {
		t.Errorf("OGG temp file %q was not cleaned up after conversion failure", convertedInput)
	}

	// verify error reply was sent (different from download error message)
	if len(*captured) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(*captured))
	}
	if (*captured)[0].Text != "Не удалось обработать голосовое сообщение." {
		t.Errorf("reply = %q, want conversion error message", (*captured)[0].Text)
	}
	if (*captured)[0].ChatID != 42 {
		t.Errorf("reply chat ID = %d, want 42", (*captured)[0].ChatID)
	}
}

func TestHandleVoice_TranscriptionSuccess(t *testing.T) {
	fakeOGG := []byte("fake-ogg-data")
	server, captured := newVoiceServer(t, voiceServerOpts{fileContent: fakeOGG})
	defer server.Close()

	b := newTestBotWithCapture(t, server)

	var wavPath string
	b.convertToWAV = func(inputPath string) (string, error) {
		tmp, err := os.CreateTemp("", "test-*.wav")
		if err != nil {
			t.Fatal(err)
		}
		tmp.Write([]byte("fake-wav-data"))
		tmp.Close()
		wavPath = tmp.Name()
		return wavPath, nil
	}
	ft := &fakeTranscriber{text: "расшифрованный текст"}
	b.transcriber = ft

	msg := voiceMessage(42, "test-file-id", 10)

	text, err := b.handleVoice(msg)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if text != "расшифрованный текст" {
		t.Errorf("text = %q, want %q", text, "расшифрованный текст")
	}

	// verify transcriber received the WAV path from converter
	if ft.capturedPath != wavPath {
		t.Errorf("transcriber got path %q, want %q", ft.capturedPath, wavPath)
	}

	// verify WAV temp file was cleaned up
	if _, err := os.Stat(wavPath); !os.IsNotExist(err) {
		t.Errorf("WAV temp file %q was not cleaned up", wavPath)
	}

	// verify transcript reply was sent
	if len(*captured) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(*captured))
	}
	if (*captured)[0].Text != "расшифрованный текст" {
		t.Errorf("reply = %q, want transcript text", (*captured)[0].Text)
	}
	if (*captured)[0].ReplyToMessageID != 100 {
		t.Errorf("reply_to_message_id = %d, want 100", (*captured)[0].ReplyToMessageID)
	}
}

func TestHandleVoice_TranscriptionFails(t *testing.T) {
	fakeOGG := []byte("fake-ogg-data")
	server, captured := newVoiceServer(t, voiceServerOpts{fileContent: fakeOGG})
	defer server.Close()

	b := newTestBotWithCapture(t, server)

	var wavPath string
	b.convertToWAV = func(inputPath string) (string, error) {
		tmp, err := os.CreateTemp("", "test-*.wav")
		if err != nil {
			t.Fatal(err)
		}
		tmp.Write([]byte("fake-wav-data"))
		tmp.Close()
		wavPath = tmp.Name()
		return wavPath, nil
	}
	b.transcriber = &fakeTranscriber{err: fmt.Errorf("API error 500: server error")}

	msg := voiceMessage(42, "test-file-id", 10)

	text, err := b.handleVoice(msg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if text != "" {
		t.Errorf("text = %q, want empty", text)
	}
	if !strings.Contains(err.Error(), "transcribe") {
		t.Errorf("error = %q, want it to contain 'transcribe'", err)
	}

	// verify WAV temp file was cleaned up despite error
	if _, err := os.Stat(wavPath); !os.IsNotExist(err) {
		t.Errorf("WAV temp file %q was not cleaned up after transcription failure", wavPath)
	}

	// verify error reply was sent
	if len(*captured) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(*captured))
	}
	want := "Не удалось расшифровать сообщение. Попробуйте позже."
	if (*captured)[0].Text != want {
		t.Errorf("reply = %q, want %q", (*captured)[0].Text, want)
	}
	if (*captured)[0].ChatID != 42 {
		t.Errorf("reply chat ID = %d, want 42", (*captured)[0].ChatID)
	}
}

func TestHandleVoice_ExactLimit(t *testing.T) {
	fakeOGG := []byte("fake-ogg-data")
	server, captured := newVoiceServer(t, voiceServerOpts{fileContent: fakeOGG})
	defer server.Close()

	b := newTestBotWithCapture(t, server)
	b.convertToWAV = mockConverter(t)
	b.transcriber = &fakeTranscriber{text: "тест"}
	msg := voiceMessage(42, "test-file-id", 90)

	text, err := b.handleVoice(msg)
	if err != nil {
		t.Fatalf("expected no error for 90s voice, got: %v", err)
	}

	// verify transcript returned (not rejected)
	if text != "тест" {
		t.Errorf("text = %q, want %q", text, "тест")
	}

	// verify transcript reply (not rejection)
	if len(*captured) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(*captured))
	}
	if (*captured)[0].Text != "тест" {
		t.Errorf("reply = %q, want transcript text", (*captured)[0].Text)
	}
}
