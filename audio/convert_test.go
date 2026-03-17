package audio

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestHelperProcess is invoked by tests as a fake ffmpeg process.
// it is not a real test.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_TEST_HELPER_PROCESS") != "1" {
		return
	}
	args := os.Args
	for i, arg := range args {
		if arg == "--" {
			args = args[i+1:]
			break
		}
	}

	switch os.Getenv("GO_TEST_HELPER_MODE") {
	case "success":
		// simulate ffmpeg: create the output file (last argument)
		outputPath := args[len(args)-1]
		if err := os.WriteFile(outputPath, []byte("fake-wav-data"), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "helper: %v", err)
			os.Exit(1)
		}
		os.Exit(0)
	case "fail":
		fmt.Fprintf(os.Stderr, "ffmpeg: error while processing")
		os.Exit(1)
	}
}

func fakeExecCommand(mode string) func(context.Context, string, ...string) *exec.Cmd {
	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestHelperProcess", "--", name}
		cs = append(cs, args...)
		cmd := exec.CommandContext(ctx, os.Args[0], cs...)
		cmd.Env = append(os.Environ(),
			"GO_TEST_HELPER_PROCESS=1",
			"GO_TEST_HELPER_MODE="+mode,
		)
		return cmd
	}
}

func TestConvertToWAV_Success(t *testing.T) {
	input, err := os.CreateTemp("", "test-input-*.ogg")
	if err != nil {
		t.Fatal(err)
	}
	input.Write([]byte("fake-ogg-data"))
	input.Close()
	defer os.Remove(input.Name())

	origExecCommand := execCommandContext
	execCommandContext = fakeExecCommand("success")
	defer func() { execCommandContext = origExecCommand }()

	outputPath, err := ConvertToWAV(input.Name())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	defer os.Remove(outputPath)

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}
	if len(data) == 0 {
		t.Error("output file is empty")
	}

	if !strings.HasSuffix(outputPath, ".wav") {
		t.Errorf("output path %q does not end with .wav", outputPath)
	}
}

func TestConvertToWAV_InputNotFound(t *testing.T) {
	outputPath, err := ConvertToWAV("/nonexistent/file.ogg")
	if err == nil {
		t.Fatal("expected error for missing input, got nil")
	}
	if outputPath != "" {
		os.Remove(outputPath)
		t.Errorf("expected empty output path, got %q", outputPath)
	}
	if !strings.Contains(err.Error(), "input file") {
		t.Errorf("error = %q, want it to contain 'input file'", err)
	}
}

func TestConvertToWAV_FfmpegFails(t *testing.T) {
	input, err := os.CreateTemp("", "test-input-*.ogg")
	if err != nil {
		t.Fatal(err)
	}
	input.Write([]byte("fake-ogg-data"))
	input.Close()
	defer os.Remove(input.Name())

	origExecCommand := execCommandContext
	execCommandContext = fakeExecCommand("fail")
	defer func() { execCommandContext = origExecCommand }()

	outputPath, err := ConvertToWAV(input.Name())
	if err == nil {
		t.Fatal("expected error when ffmpeg fails, got nil")
	}
	if outputPath != "" {
		os.Remove(outputPath)
		t.Errorf("expected empty output path, got %q", outputPath)
	}
	if !strings.Contains(err.Error(), "ffmpeg") {
		t.Errorf("error = %q, want it to contain 'ffmpeg'", err)
	}
}
