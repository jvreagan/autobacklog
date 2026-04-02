package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/jamesreagan/autobacklog/internal/config"
)

// Setup initializes the global slog logger from config.
func Setup(cfg config.LoggingConfig) (*slog.Logger, error) {
	level := parseLevel(cfg.Level)

	var writer io.Writer = os.Stderr
	if cfg.File != "" {
		f, err := os.OpenFile(cfg.File, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, err
		}
		writer = io.MultiWriter(os.Stderr, f)
	}

	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	if strings.ToLower(cfg.Format) == "json" {
		handler = slog.NewJSONHandler(writer, opts)
	} else {
		handler = slog.NewTextHandler(writer, opts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger, nil
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
