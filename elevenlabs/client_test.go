package elevenlabs

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// newTestWAV creates a temporary WAV file and returns its path.
// the caller must remove the file when done.
func newTestWAV(t *testing.T) string {
	t.Helper()
	tmp, err := os.CreateTemp("", "test-*.wav")
	if err != nil {
		t.Fatal(err)
	}
	tmp.Write([]byte("fake-wav-data"))
	tmp.Close()
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
		defer file.Close()
		if header.Filename == "" {
			t.Error("uploaded file has no filename")
		}
		data, _ := io.ReadAll(file)
		if string(data) != "fake-wav-data" {
			t.Errorf("file content = %q, want %q", string(data), "fake-wav-data")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"text":                 "привет мир",
			"language_code":        "ru",
			"language_probability": 0.99,
		})
	}))
	defer server.Close()

	wavPath := newTestWAV(t)
	defer os.Remove(wavPath)

	c := NewClient("test-key")
	c.baseURL = server.URL

	text, err := c.Transcribe(wavPath)
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
		w.Write([]byte(`{"detail":{"message":"invalid request"}}`))
	}))
	defer server.Close()

	wavPath := newTestWAV(t)
	defer os.Remove(wavPath)

	c := NewClient("test-key")
	c.baseURL = server.URL

	text, err := c.Transcribe(wavPath)
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
		json.NewEncoder(w).Encode(map[string]any{
			"text":                 "",
			"language_code":        "ru",
			"language_probability": 0.99,
		})
	}))
	defer server.Close()

	wavPath := newTestWAV(t)
	defer os.Remove(wavPath)

	c := NewClient("test-key")
	c.baseURL = server.URL

	text, err := c.Transcribe(wavPath)
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
		json.NewEncoder(w).Encode(map[string]any{
			"text":                 "   \n  ",
			"language_code":        "ru",
			"language_probability": 0.99,
		})
	}))
	defer server.Close()

	wavPath := newTestWAV(t)
	defer os.Remove(wavPath)

	c := NewClient("test-key")
	c.baseURL = server.URL

	text, err := c.Transcribe(wavPath)
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
			w.Write([]byte("server error"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"text":                 "привет мир",
			"language_code":        "ru",
			"language_probability": 0.99,
		})
	}))
	defer server.Close()

	wavPath := newTestWAV(t)
	defer os.Remove(wavPath)

	c := NewClient("test-key")
	c.baseURL = server.URL

	text, err := c.Transcribe(wavPath)
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
		w.Write([]byte("server error"))
	}))
	defer server.Close()

	wavPath := newTestWAV(t)
	defer os.Remove(wavPath)

	c := NewClient("test-key")
	c.baseURL = server.URL

	text, err := c.Transcribe(wavPath)
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
		json.NewEncoder(w).Encode(map[string]any{
			"text":                 "привет мир",
			"language_code":        "ru",
			"language_probability": 0.99,
		})
	}))
	defer server.Close()

	wavPath := newTestWAV(t)
	defer os.Remove(wavPath)

	c := NewClient("test-key")
	c.baseURL = server.URL
	c.httpClient.Timeout = 100 * time.Millisecond

	text, err := c.Transcribe(wavPath)
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
