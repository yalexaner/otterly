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

// httpClient is used for downloading files from Telegram with a timeout.
var httpClient = &http.Client{Timeout: 30 * time.Second}

// handleVoice downloads the voice message OGG file from Telegram to a temp file
// and returns the path to the downloaded file. the caller is responsible for
// cleaning up the temp file.
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

	// build download URL
	url := fmt.Sprintf(b.fileEndpoint, b.api.Token, file.FilePath)

	// download the file
	resp, err := httpClient.Get(url)
	if err != nil {
		log.Printf("failed to download voice file: %v", err)
		b.reply(msg, "Не удалось загрузить голосовое сообщение.")
		return "", fmt.Errorf("download voice file: %w", err)
	}
	defer resp.Body.Close()

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

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		os.Remove(oggPath)
		log.Printf("failed to write voice file: %v", err)
		b.reply(msg, "Не удалось загрузить голосовое сообщение.")
		return "", fmt.Errorf("write voice file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(oggPath)
		log.Printf("failed to flush voice file: %v", err)
		b.reply(msg, "Не удалось загрузить голосовое сообщение.")
		return "", fmt.Errorf("close voice file: %w", err)
	}

	log.Printf("voice message downloaded to %s", oggPath)

	// convert OGG to WAV
	wavPath, err := b.convertToWAV(oggPath)
	if err != nil {
		os.Remove(oggPath)
		log.Printf("failed to convert voice file: %v", err)
		b.reply(msg, "Не удалось обработать голосовое сообщение.")
		return "", fmt.Errorf("convert to wav: %w", err)
	}

	// OGG no longer needed
	os.Remove(oggPath)

	log.Printf("voice message converted to %s", wavPath)
	b.reply(msg, "Голосовое сообщение получено.")

	return wavPath, nil
}
