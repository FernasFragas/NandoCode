// Package logging provides structured logging setup using log/slog.
package logging

import (
	"io"
	"log/slog"
	"os"
)

// Format represents the log output format.
type Format string

const (
	// FormatText outputs human-readable text logs.
	FormatText Format = "text"

	// FormatJSON outputs structured JSON logs.
	FormatJSON Format = "json"
)

// New creates a new structured logger with the specified level and format.
// Default format is text for TTY, JSON otherwise.
func New(level slog.Level, format Format) *slog.Logger {
	var handler slog.Handler

	switch format {
	case FormatJSON:
		handler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
			Level: level,
		})
	case FormatText:
		handler = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: level,
		})
	default:
		// Auto-detect based on TTY
		if isTTY() {
			handler = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
				Level: level,
			})
		} else {
			handler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
				Level: level,
			})
		}
	}

	return slog.New(handler)
}

// NewWithWriter creates a logger that writes to the specified writer.
// Useful for testing or redirecting logs.
func NewWithWriter(w io.Writer, level slog.Level, format Format) *slog.Logger {
	var handler slog.Handler

	switch format {
	case FormatJSON:
		handler = slog.NewJSONHandler(w, &slog.HandlerOptions{
			Level: level,
		})
	case FormatText:
		fallthrough
	default:
		handler = slog.NewTextHandler(w, &slog.HandlerOptions{
			Level: level,
		})
	}

	return slog.New(handler)
}

// isTTY checks if stderr is connected to a terminal.
func isTTY() bool {
	stat, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

// ParseLevel parses a log level string into slog.Level.
func ParseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
