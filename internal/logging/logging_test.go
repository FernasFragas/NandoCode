package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestNewWithWriterText(t *testing.T) {
	var out bytes.Buffer
	logger := NewWithWriter(&out, slog.LevelInfo, FormatText)

	logger.Info("hello", "key", "value")

	got := out.String()
	if !strings.Contains(got, "msg=hello") {
		t.Fatalf("text log = %q", got)
	}
	if !strings.Contains(got, "key=value") {
		t.Fatalf("text log = %q", got)
	}
}

func TestNewWithWriterJSON(t *testing.T) {
	var out bytes.Buffer
	logger := NewWithWriter(&out, slog.LevelInfo, FormatJSON)

	logger.Info("hello", "key", "value")

	var entry map[string]any
	if err := json.Unmarshal(out.Bytes(), &entry); err != nil {
		t.Fatalf("json log = %q: %v", out.String(), err)
	}
	if entry["msg"] != "hello" {
		t.Fatalf("json msg = %#v", entry["msg"])
	}
	if entry["key"] != "value" {
		t.Fatalf("json key = %#v", entry["key"])
	}
}

func TestNewWithWriterUnknownFormatFallsBackToText(t *testing.T) {
	var out bytes.Buffer
	logger := NewWithWriter(&out, slog.LevelInfo, Format("unknown"))

	logger.Info("hello")

	if !strings.Contains(out.String(), "msg=hello") {
		t.Fatalf("unknown format log = %q", out.String())
	}
}

func TestNewWithWriterDoesNotChangeDefaultLogger(t *testing.T) {
	before := slog.Default()
	var out bytes.Buffer

	_ = NewWithWriter(&out, slog.LevelInfo, FormatText)

	if slog.Default() != before {
		t.Fatal("NewWithWriter changed the default logger")
	}
}
