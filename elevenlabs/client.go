package elevenlabs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.elevenlabs.io"

// Client talks to the ElevenLabs speech-to-text API.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new ElevenLabs client with the given API key.
func NewClient(apiKey string) *Client {
	return &Client{
		apiKey:     apiKey,
		baseURL:    defaultBaseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// transcribeResponse is the JSON response from the speech-to-text endpoint.
type transcribeResponse struct {
	Text string `json:"text"`
}

// Transcribe sends a WAV file to the ElevenLabs Scribe v2 API and returns
// the transcribed text. retries once on 5xx or timeout errors. returns an
// error if the API call fails or the response contains empty text.
func (c *Client) Transcribe(ctx context.Context, wavPath string) (string, error) {
	body, contentType, err := buildMultipartRequest(wavPath)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	bodyBytes := body.Bytes()

	text, err := c.doTranscribe(ctx, bodyBytes, contentType)
	if err != nil && isRetryable(err) {
		text, err = c.doTranscribe(ctx, bodyBytes, contentType)
	}
	return text, err
}

// doTranscribe performs a single transcription request.
func (c *Client) doTranscribe(ctx context.Context, bodyBytes []byte, contentType string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/speech-to-text", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("xi-api-key", c.apiKey)
	req.Header.Set("Content-Type", contentType)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 500 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", &serverError{code: resp.StatusCode, body: string(respBody)}
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result transcribeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	if strings.TrimSpace(result.Text) == "" {
		return "", fmt.Errorf("empty transcription result")
	}

	return result.Text, nil
}

// serverError represents a 5xx error from the API.
type serverError struct {
	code int
	body string
}

func (e *serverError) Error() string {
	return fmt.Sprintf("API error %d: %s", e.code, e.body)
}

// IsTransient returns true if the error is a 5xx server error or a timeout.
func IsTransient(err error) bool {
	var se *serverError
	if errors.As(err, &se) {
		return true
	}
	var t interface{ Timeout() bool }
	if errors.As(err, &t) {
		return t.Timeout()
	}
	return false
}

// NewServerError creates a server error for the given status code.
// errors created with this function are recognized by IsTransient.
func NewServerError(code int, body string) error {
	return &serverError{code: code, body: body}
}

// isRetryable returns true if the error is a 5xx server error or a timeout.
func isRetryable(err error) bool {
	return IsTransient(err)
}

// buildMultipartRequest creates the multipart form body for the transcription request.
func buildMultipartRequest(wavPath string) (*bytes.Buffer, string, error) {
	f, err := os.Open(wavPath)
	if err != nil {
		return nil, "", fmt.Errorf("open wav file: %w", err)
	}
	defer func() { _ = f.Close() }()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	// add the WAV file
	part, err := w.CreateFormFile("file", filepath.Base(wavPath))
	if err != nil {
		return nil, "", fmt.Errorf("create form file: %w", err)
	}
	if _, err := io.Copy(part, f); err != nil {
		return nil, "", fmt.Errorf("copy file data: %w", err)
	}

	// add form fields
	fields := map[string]string{
		"model_id":         "scribe_v2",
		"language_code":    "ru",
		"file_format":      "pcm_s16le_16",
		"tag_audio_events": "false",
	}
	for key, val := range fields {
		if err := w.WriteField(key, val); err != nil {
			return nil, "", fmt.Errorf("write field %s: %w", key, err)
		}
	}

	if err := w.Close(); err != nil {
		return nil, "", fmt.Errorf("close multipart writer: %w", err)
	}

	return &buf, w.FormDataContentType(), nil
}
