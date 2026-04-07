package logging

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/jamesreagan/autobacklog/internal/config"
)

func TestSetup_DefaultLevel(t *testing.T) {
	cfg := config.LoggingConfig{Level: "info", Format: "text"}
	logger, err := Setup(cfg)
	// #186: always cleanup to avoid leaking file handles
	t.Cleanup(Cleanup)
	if err != nil {
		t.Fatal(err)
	}
	if logger == nil {
		t.Error("logger should not be nil")
	}
}

func TestSetup_AllLevels(t *testing.T) {
	levels := []string{"debug", "info", "warn", "error"}
	for _, level := range levels {
		t.Run(level, func(t *testing.T) {
			cfg := config.LoggingConfig{Level: level, Format: "text"}
			_, err := Setup(cfg)
			if err != nil {
				t.Fatalf("Setup(%q): %v", level, err)
			}
		})
	}
}

func TestSetup_JSONFormat(t *testing.T) {
	cfg := config.LoggingConfig{Level: "info", Format: "json"}
	_, err := Setup(cfg)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSetup_TextFormat(t *testing.T) {
	cfg := config.LoggingConfig{Level: "info", Format: "text"}
	_, err := Setup(cfg)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSetup_FileOutput(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "test.log")
	cfg := config.LoggingConfig{Level: "info", Format: "text", File: logFile}

	logger, err := Setup(cfg)
	// #186: always cleanup to avoid leaking file handles
	t.Cleanup(Cleanup)
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("test message")

	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(content) == 0 {
		t.Error("log file should have content")
	}
}

// #184: test SetupWithExtraWriter
func TestSetupWithExtraWriter(t *testing.T) {
	var buf bytes.Buffer
	cfg := config.LoggingConfig{Level: "info", Format: "text"}

	logger, err := SetupWithExtraWriter(cfg, &buf)
	t.Cleanup(Cleanup)
	if err != nil {
		t.Fatal(err)
	}
	if logger == nil {
		t.Error("logger should not be nil")
	}

	logger.Info("extra writer test")
	if buf.Len() == 0 {
		t.Error("extra writer should have received log output")
	}
}

// #184: test SetupWithExtraWriter with file output
func TestSetupWithExtraWriter_WithFile(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "test.log")
	var buf bytes.Buffer
	cfg := config.LoggingConfig{Level: "info", Format: "text", File: logFile}

	logger, err := SetupWithExtraWriter(cfg, &buf)
	t.Cleanup(Cleanup)
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("file and extra writer test")

	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(content) == 0 {
		t.Error("log file should have content")
	}
	if buf.Len() == 0 {
		t.Error("extra writer should have received output")
	}
}

func TestSetup_InvalidFilePath(t *testing.T) {
	cfg := config.LoggingConfig{
		Level:  "info",
		Format: "text",
		File:   "/nonexistent/path/to/log.log",
	}
	_, err := Setup(cfg)
	if err == nil {
		t.Error("expected error for invalid file path")
	}
}

func TestParseLevel_AllLevels(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"INFO", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"WARN", slog.LevelWarn},
		{"error", slog.LevelError},
		{"ERROR", slog.LevelError},
		{"", slog.LevelInfo},
		{"unknown", slog.LevelInfo},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseLevel(tt.input)
			if got != tt.want {
				t.Errorf("parseLevel(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
