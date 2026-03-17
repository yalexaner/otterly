package bot

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// maxVoiceFileSize is the maximum allowed size for a voice file download (20 MB,
// matching the Telegram Bot API limit).
const maxVoiceFileSize = 20 * 1024 * 1024

// transcriber converts audio to text.
type transcriber interface {
	Transcribe(wavPath string) (string, error)
}

// httpClient is used for downloading files from Telegram with a timeout.
var httpClient = &http.Client{Timeout: 30 * time.Second}

// handleVoice processes a voice message: downloads OGG from Telegram, converts
// to WAV, transcribes via ElevenLabs, and replies with the transcript. all temp
// files are cleaned up internally.
func (b *Bot) handleVoice(msg *tgbotapi.Message) (string, error) {
	// reject voice messages longer than 90 seconds
	if msg.Voice.Duration > 90 {
		b.reply(msg, "Сообщение слишком длинное (более 90 секунд).")
		return "", fmt.Errorf("voice too long: %ds", msg.Voice.Duration)
	}

	// get file metadata from Telegram
	file, err := b.api.GetFile(tgbotapi.FileConfig{FileID: msg.Voice.FileID})
	if err != nil {
		log.Printf("failed to get file info: %v", err)
		b.reply(msg, "Не удалось загрузить голосовое сообщение.")
		return "", fmt.Errorf("get file info: %w", err)
	}

	// reject files exceeding the size limit
	if file.FileSize > maxVoiceFileSize {
		log.Printf("voice file too large: %d bytes", file.FileSize)
		b.reply(msg, "Не удалось загрузить голосовое сообщение.")
		return "", fmt.Errorf("voice file too large: %d bytes", file.FileSize)
	}

	// build download URL
	url := fmt.Sprintf(b.fileEndpoint, b.api.Token, file.FilePath)

	// download the file
	resp, err := httpClient.Get(url)
	if err != nil {
		log.Printf("failed to download voice file: %v", err)
		b.reply(msg, "Не удалось загрузить голосовое сообщение.")
		return "", fmt.Errorf("download voice file: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		log.Printf("voice file download returned status %d", resp.StatusCode)
		b.reply(msg, "Не удалось загрузить голосовое сообщение.")
		return "", fmt.Errorf("download voice file: status %d", resp.StatusCode)
	}

	// save to temp file
	tmp, err := os.CreateTemp("", "otterly-*.ogg")
	if err != nil {
		log.Printf("failed to create temp file: %v", err)
		b.reply(msg, "Не удалось загрузить голосовое сообщение.")
		return "", fmt.Errorf("create temp file: %w", err)
	}
	oggPath := tmp.Name()

	n, err := io.Copy(tmp, io.LimitReader(resp.Body, maxVoiceFileSize+1))
	if err != nil {
		_ = tmp.Close()
		_ = os.Remove(oggPath)
		log.Printf("failed to write voice file: %v", err)
		b.reply(msg, "Не удалось загрузить голосовое сообщение.")
		return "", fmt.Errorf("write voice file: %w", err)
	}
	if n > maxVoiceFileSize {
		_ = tmp.Close()
		_ = os.Remove(oggPath)
		log.Printf("voice file body exceeds size limit: %d bytes read", n)
		b.reply(msg, "Не удалось загрузить голосовое сообщение.")
		return "", fmt.Errorf("voice file body too large: %d bytes", n)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(oggPath)
		log.Printf("failed to flush voice file: %v", err)
		b.reply(msg, "Не удалось загрузить голосовое сообщение.")
		return "", fmt.Errorf("close voice file: %w", err)
	}

	log.Printf("voice message downloaded to %s", oggPath)

	// convert OGG to WAV
	wavPath, err := b.convertToWAV(oggPath)
	if err != nil {
		_ = os.Remove(oggPath)
		log.Printf("failed to convert voice file: %v", err)
		b.reply(msg, "Не удалось обработать голосовое сообщение.")
		return "", fmt.Errorf("convert to wav: %w", err)
	}

	// OGG no longer needed
	_ = os.Remove(oggPath)

	log.Printf("voice message converted to %s", wavPath)

	// transcribe WAV to text
	text, err := b.transcriber.Transcribe(wavPath)
	_ = os.Remove(wavPath)
	if err != nil {
		log.Printf("failed to transcribe voice: %v", err)
		b.reply(msg, "Не удалось расшифровать сообщение. Попробуйте позже.")
		return "", fmt.Errorf("transcribe: %w", err)
	}

	b.reply(msg, text)
	return text, nil
}
