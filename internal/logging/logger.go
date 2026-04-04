package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/jamesreagan/autobacklog/internal/config"
)

// Setup initializes the global slog logger from config.
// Returns the logger and a cleanup function that closes the log file (if any).
// Callers should defer cleanup() after checking for errors.
func Setup(cfg config.LoggingConfig) (*slog.Logger, error) {
	level := parseLevel(cfg.Level)

	var writer io.Writer = os.Stderr
	var cleanup func()
	if cfg.File != "" {
		f, err := os.OpenFile(cfg.File, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			return nil, err
		}
		writer = io.MultiWriter(os.Stderr, f)
		cleanup = func() { f.Close() }
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

	// Store cleanup so callers can close the file.
	// Since changing the return signature would break callers,
	// we register the cleanup via a package-level variable.
	logCleanup = cleanup

	return logger, nil
}

// logCleanup holds the function to close the log file, if any.
var logCleanup func()

// Cleanup closes the log file handle opened by Setup, if any.
func Cleanup() {
	if logCleanup != nil {
		logCleanup()
		logCleanup = nil
	}
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
