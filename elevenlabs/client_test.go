package elevenlabs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeTimeoutErr implements the Timeout() bool interface for testing.
type fakeTimeoutErr struct{}

func (e *fakeTimeoutErr) Error() string   { return "timeout" }
func (e *fakeTimeoutErr) Timeout() bool   { return true }
func (e *fakeTimeoutErr) Temporary() bool { return true }

// newTestWAV creates a temporary WAV file and returns its path.
// the caller must remove the file when done.
func newTestWAV(t *testing.T) string {
	t.Helper()
	tmp, err := os.CreateTemp("", "test-*.wav")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = tmp.Write([]byte("fake-wav-data"))
	_ = tmp.Close()
	return tmp.Name()
}

func TestTranscribe_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// verify request method and path
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/speech-to-text" {
			t.Errorf("path = %s, want /v1/speech-to-text", r.URL.Path)
		}

		// verify auth header
		if r.Header.Get("xi-api-key") != "test-key" {
			t.Errorf("xi-api-key = %q, want %q", r.Header.Get("xi-api-key"), "test-key")
		}

		// verify multipart form fields
		err := r.ParseMultipartForm(10 << 20)
		if err != nil {
			t.Fatalf("failed to parse multipart form: %v", err)
		}
		if r.FormValue("model_id") != "scribe_v2" {
			t.Errorf("model_id = %q, want %q", r.FormValue("model_id"), "scribe_v2")
		}
		if r.FormValue("language_code") != "ru" {
			t.Errorf("language_code = %q, want %q", r.FormValue("language_code"), "ru")
		}
		if r.FormValue("file_format") != "pcm_s16le_16" {
			t.Errorf("file_format = %q, want %q", r.FormValue("file_format"), "pcm_s16le_16")
		}
		if r.FormValue("tag_audio_events") != "false" {
			t.Errorf("tag_audio_events = %q, want %q", r.FormValue("tag_audio_events"), "false")
		}

		// verify file was uploaded
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("failed to get uploaded file: %v", err)
		}
		defer func() { _ = file.Close() }()
		if header.Filename == "" {
			t.Error("uploaded file has no filename")
		}
		data, _ := io.ReadAll(file)
		if string(data) != "fake-wav-data" {
			t.Errorf("file content = %q, want %q", string(data), "fake-wav-data")
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"text":                 "привет мир",
			"language_code":        "ru",
			"language_probability": 0.99,
		})
	}))
	defer server.Close()

	wavPath := newTestWAV(t)
	defer func() { _ = os.Remove(wavPath) }()

	c := NewClient("test-key")
	c.baseURL = server.URL

	text, err := c.Transcribe(context.Background(), wavPath)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if text != "привет мир" {
		t.Errorf("text = %q, want %q", text, "привет мир")
	}
}

func TestTranscribe_APIError400(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"detail":{"message":"invalid request"}}`))
	}))
	defer server.Close()

	wavPath := newTestWAV(t)
	defer func() { _ = os.Remove(wavPath) }()

	c := NewClient("test-key")
	c.baseURL = server.URL

	text, err := c.Transcribe(context.Background(), wavPath)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if text != "" {
		t.Errorf("text = %q, want empty", text)
	}
}

func TestTranscribe_EmptyText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"text":                 "",
			"language_code":        "ru",
			"language_probability": 0.99,
		})
	}))
	defer server.Close()

	wavPath := newTestWAV(t)
	defer func() { _ = os.Remove(wavPath) }()

	c := NewClient("test-key")
	c.baseURL = server.URL

	text, err := c.Transcribe(context.Background(), wavPath)
	if err == nil {
		t.Fatal("expected error for empty text, got nil")
	}
	if text != "" {
		t.Errorf("text = %q, want empty", text)
	}
}

func TestTranscribe_WhitespaceOnlyText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"text":                 "   \n  ",
			"language_code":        "ru",
			"language_probability": 0.99,
		})
	}))
	defer server.Close()

	wavPath := newTestWAV(t)
	defer func() { _ = os.Remove(wavPath) }()

	c := NewClient("test-key")
	c.baseURL = server.URL

	text, err := c.Transcribe(context.Background(), wavPath)
	if err == nil {
		t.Fatal("expected error for whitespace-only text, got nil")
	}
	if text != "" {
		t.Errorf("text = %q, want empty", text)
	}
}

func TestTranscribe_Retry500ThenSuccess(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("server error"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"text":                 "привет мир",
			"language_code":        "ru",
			"language_probability": 0.99,
		})
	}))
	defer server.Close()

	wavPath := newTestWAV(t)
	defer func() { _ = os.Remove(wavPath) }()

	c := NewClient("test-key")
	c.baseURL = server.URL

	text, err := c.Transcribe(context.Background(), wavPath)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if text != "привет мир" {
		t.Errorf("text = %q, want %q", text, "привет мир")
	}
	if calls.Load() != 2 {
		t.Errorf("calls = %d, want 2", calls.Load())
	}
}

func TestTranscribe_Retry500BothFail(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("server error"))
	}))
	defer server.Close()

	wavPath := newTestWAV(t)
	defer func() { _ = os.Remove(wavPath) }()

	c := NewClient("test-key")
	c.baseURL = server.URL

	text, err := c.Transcribe(context.Background(), wavPath)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if text != "" {
		t.Errorf("text = %q, want empty", text)
	}
	if calls.Load() != 2 {
		t.Errorf("calls = %d, want 2 (initial + retry)", calls.Load())
	}
}

func TestTranscribe_RetryTimeoutThenSuccess(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			// delay longer than client timeout to trigger timeout
			time.Sleep(200 * time.Millisecond)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"text":                 "привет мир",
			"language_code":        "ru",
			"language_probability": 0.99,
		})
	}))
	defer server.Close()

	wavPath := newTestWAV(t)
	defer func() { _ = os.Remove(wavPath) }()

	c := NewClient("test-key")
	c.baseURL = server.URL
	c.httpClient.Timeout = 100 * time.Millisecond

	text, err := c.Transcribe(context.Background(), wavPath)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if text != "привет мир" {
		t.Errorf("text = %q, want %q", text, "привет мир")
	}
	if calls.Load() != 2 {
		t.Errorf("calls = %d, want 2", calls.Load())
	}
}

func TestIsTransient_ServerError(t *testing.T) {
	err := NewServerError(500, "internal server error")
	if !IsTransient(err) {
		t.Error("expected IsTransient to return true for server error")
	}
}

func TestIsTransient_NonServerError(t *testing.T) {
	err := fmt.Errorf("API error 400: bad request")
	if IsTransient(err) {
		t.Error("expected IsTransient to return false for non-server error")
	}
}

func TestIsTransient_WrappedServerError(t *testing.T) {
	err := fmt.Errorf("transcribe: %w", NewServerError(503, "service unavailable"))
	if !IsTransient(err) {
		t.Error("expected IsTransient to return true for wrapped server error")
	}
}

func TestIsTransient_TimeoutError(t *testing.T) {
	err := fmt.Errorf("send request: %w", &fakeTimeoutErr{})
	if !IsTransient(err) {
		t.Error("expected IsTransient to return true for timeout error")
	}
}

func TestIsTransient_NonTimeoutNonServerError(t *testing.T) {
	err := fmt.Errorf("some random error")
	if IsTransient(err) {
		t.Error("expected IsTransient to return false for non-timeout non-server error")
	}
}

func TestTranscribe_FileNotFound(t *testing.T) {
	c := NewClient("test-key")

	text, err := c.Transcribe(context.Background(), "/nonexistent/file.wav")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
	if text != "" {
		t.Errorf("text = %q, want empty", text)
	}
	if !strings.Contains(err.Error(), "build request") {
		t.Errorf("error = %q, want it to contain 'build request'", err)
	}
}

func TestTranscribe_InvalidJSONResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	wavPath := newTestWAV(t)
	defer func() { _ = os.Remove(wavPath) }()

	c := NewClient("test-key")
	c.baseURL = server.URL

	text, err := c.Transcribe(context.Background(), wavPath)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	if text != "" {
		t.Errorf("text = %q, want empty", text)
	}
	if !strings.Contains(err.Error(), "decode response") {
		t.Errorf("error = %q, want it to contain 'decode response'", err)
	}
}

func TestTranscribe_CancelledContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"text": "should not reach"})
	}))
	defer server.Close()

	wavPath := newTestWAV(t)
	defer func() { _ = os.Remove(wavPath) }()

	c := NewClient("test-key")
	c.baseURL = server.URL

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	text, err := c.Transcribe(ctx, wavPath)
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
	if text != "" {
		t.Errorf("text = %q, want empty", text)
	}
}
