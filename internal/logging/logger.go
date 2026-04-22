package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/jvreagan/autobacklog/internal/config"
)

// #210: protect logCleanup with a mutex for goroutine safety
var (
	logMu      sync.Mutex
	logCleanup func()
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

	// #154: call previous cleanup before storing new one
	logMu.Lock()
	if logCleanup != nil {
		logCleanup()
	}
	logCleanup = cleanup
	logMu.Unlock()

	return logger, nil
}

// SetupWithExtraWriter is identical to Setup but also writes log output to
// the provided extra writer. The logging package never imports webui — it
// just accepts a plain io.Writer.
func SetupWithExtraWriter(cfg config.LoggingConfig, extra io.Writer) (*slog.Logger, error) {
	level := parseLevel(cfg.Level)

	var writer io.Writer = io.MultiWriter(os.Stderr, extra)
	var cleanup func()
	if cfg.File != "" {
		f, err := os.OpenFile(cfg.File, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			return nil, err
		}
		writer = io.MultiWriter(os.Stderr, f, extra)
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

	// #154: call previous cleanup before storing new one
	logMu.Lock()
	if logCleanup != nil {
		logCleanup()
	}
	logCleanup = cleanup
	logMu.Unlock()

	return logger, nil
}

// Cleanup closes the log file handle opened by Setup, if any.
func Cleanup() {
	logMu.Lock()
	defer logMu.Unlock()
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
