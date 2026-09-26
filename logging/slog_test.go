package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

// newTestLogger builds a logrus logger writing to a bytes.Buffer.
func newTestLogger(t *testing.T) (*logrus.Logger, *bytes.Buffer) {
	t.Helper()

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	logger.SetFormatter(&logrus.JSONFormatter{})

	var buf bytes.Buffer
	logger.SetOutput(&buf)
	return logger, &buf
}

// decodeLog returns the last JSON log line written to buf.
func decodeLog(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) == 0 {
		t.Fatal("no log output captured")
	}
	var entry map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &entry); err != nil {
		t.Fatalf("log output is not valid JSON: %v\nraw: %s", err, buf.String())
	}
	return entry
}

func TestSlog_LevelForwarding(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		emit     func(*slog.Logger)
		wantLvl  string
		wantSubj string
	}{
		{"debug", func(l *slog.Logger) { l.Debug("dbg msg") }, "debug", "dbg msg"},
		{"info", func(l *slog.Logger) { l.Info("info msg") }, "info", "info msg"},
		{"warn", func(l *slog.Logger) { l.Warn("warn msg") }, "warning", "warn msg"},
		{"error", func(l *slog.Logger) { l.Error("err msg") }, "error", "err msg"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			logger, buf := newTestLogger(t)
			sl := Slog(logger)

			tt.emit(sl)

			entry := decodeLog(t, buf)
			if entry["level"] != tt.wantLvl {
				t.Errorf("level = %v, want %v", entry["level"], tt.wantLvl)
			}
			if entry["msg"] != tt.wantSubj {
				t.Errorf("msg = %v, want %v", entry["msg"], tt.wantSubj)
			}
		})
	}
}

func TestSlog_AttributesForwarded(t *testing.T) {
	t.Parallel()

	logger, buf := newTestLogger(t)
	Slog(logger).Info("search done", "engine", "google", "hits", 42, "ok", true)

	entry := decodeLog(t, buf)
	if entry["engine"] != "google" {
		t.Errorf("engine = %v, want google", entry["engine"])
	}
	if entry["hits"] != float64(42) {
		t.Errorf("hits = %v, want 42", entry["hits"])
	}
	if entry["ok"] != true {
		t.Errorf("ok = %v, want true", entry["ok"])
	}
	if entry["slog_level"] != "INFO" {
		t.Errorf("slog_level = %v, want INFO", entry["slog_level"])
	}
}

func TestSlog_WithAttrsAndGroup(t *testing.T) {
	t.Parallel()

	logger, buf := newTestLogger(t)
	sl := Slog(logger).
		With("server", "main").
		WithGroup("req").
		With("session_id", "s-1")
	sl.Info("handled", "method", "tools/list")

	entry := decodeLog(t, buf)
	if entry["server"] != "main" {
		t.Errorf("server = %v, want main", entry["server"])
	}
	if entry["req.session_id"] != "s-1" {
		t.Errorf("req.session_id = %v, want s-1", entry["req.session_id"])
	}
	if entry["req.method"] != "tools/list" {
		t.Errorf("req.method = %v, want tools/list", entry["req.method"])
	}
}

func TestSlog_GroupAttrFlattened(t *testing.T) {
	t.Parallel()

	logger, buf := newTestLogger(t)
	Slog(logger).Info("with group attr", slog.Group("req", slog.String("id", "abc")))

	entry := decodeLog(t, buf)
	if entry["req.id"] != "abc" {
		t.Errorf("req.id = %v, want abc", entry["req.id"])
	}
}

func TestSlog_LevelFiltering(t *testing.T) {
	t.Parallel()

	t.Run("handler honours logrus level", func(t *testing.T) {
		logger := logrus.New()
		logger.SetLevel(logrus.WarnLevel)

		sl := Slog(logger)
		if sl.Enabled(context.Background(), slog.LevelDebug) {
			t.Error("debug should be disabled at warn level")
		}
		if sl.Enabled(context.Background(), slog.LevelInfo) {
			t.Error("info should be disabled at warn level")
		}
		if !sl.Enabled(context.Background(), slog.LevelWarn) {
			t.Error("warn should be enabled at warn level")
		}
		if !sl.Enabled(context.Background(), slog.LevelError) {
			t.Error("error should be enabled at warn level")
		}
	})
}

func TestSlog_DisabledLevelNotEmitted(t *testing.T) {
	t.Parallel()

	logger, buf := newTestLogger(t)
	logger.SetLevel(logrus.ErrorLevel)

	Slog(logger).Info("should be dropped")

	if buf.Len() != 0 {
		t.Errorf("expected no output at error level, got: %q", buf.String())
	}
}

func TestNewSlogHandler_WithEmptyGroupIsIdentity(t *testing.T) {
	t.Parallel()

	logger := logrus.New()
	h := NewSlogHandler(logger)
	if g := h.WithGroup(""); g != h {
		t.Error("WithGroup(\"\") should return the same handler")
	}
}
