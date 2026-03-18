package audio

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// execCommandContext is used to create exec.Cmd instances with a context.
// tests can replace this to mock external commands.
var execCommandContext = exec.CommandContext

// ConvertToWAV converts an OGG/Opus audio file to WAV PCM 16 kHz mono using
// ffmpeg. Returns the path to the output WAV temp file. The caller is
// responsible for removing the output file. The input file is not removed.
// The provided context controls cancellation; a 60-second timeout is applied
// on top of it to bound ffmpeg execution time.
func ConvertToWAV(ctx context.Context, inputPath string) (string, error) {
	if _, err := os.Stat(inputPath); err != nil {
		return "", fmt.Errorf("input file: %w", err)
	}

	tmp, err := os.CreateTemp("", "otterly-*.wav")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	outputPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(outputPath)
		return "", fmt.Errorf("close temp file: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	cmd := execCommandContext(ctx, "ffmpeg", "-i", inputPath, "-ar", "16000", "-ac", "1", "-f", "wav", "-y", outputPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		_ = os.Remove(outputPath)
		return "", fmt.Errorf("ffmpeg conversion failed: %w: %s", err, output)
	}

	return outputPath, nil
}
