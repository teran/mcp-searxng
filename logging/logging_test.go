package logging

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestParseLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in      string
		want    logrus.Level
		wantErr bool
	}{
		{"debug", logrus.DebugLevel, false},
		{"info", logrus.InfoLevel, false},
		{"warn", logrus.WarnLevel, false},
		{"warning", logrus.WarnLevel, false},
		{"error", logrus.ErrorLevel, false},
		{"fatal", logrus.FatalLevel, false},
		{"panic", logrus.PanicLevel, false},
		{"DEBUG", logrus.DebugLevel, false}, // case-insensitive
		{"verbose", 0, true},
		{"", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseLevel(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseLevel(%q) expected error, got nil", tt.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseLevel(%q) returned error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("ParseLevel(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseFormat(t *testing.T) {
	t.Parallel()

	t.Run("text format", func(t *testing.T) {
		f, err := ParseFormat("text")
		if err != nil {
			t.Fatalf("ParseFormat(text) returned error: %v", err)
		}
		if _, ok := f.(*logrus.TextFormatter); !ok {
			t.Errorf("ParseFormat(text) = %T, want *logrus.TextFormatter", f)
		}
	})

	t.Run("empty format defaults to text", func(t *testing.T) {
		f, err := ParseFormat("")
		if err != nil {
			t.Fatalf("ParseFormat(\"\") returned error: %v", err)
		}
		if _, ok := f.(*logrus.TextFormatter); !ok {
			t.Errorf("ParseFormat(\"\") = %T, want *logrus.TextFormatter", f)
		}
	})

	t.Run("json format", func(t *testing.T) {
		f, err := ParseFormat("json")
		if err != nil {
			t.Fatalf("ParseFormat(json) returned error: %v", err)
		}
		if _, ok := f.(*logrus.JSONFormatter); !ok {
			t.Errorf("ParseFormat(json) = %T, want *logrus.JSONFormatter", f)
		}
	})

	t.Run("json uppercase is accepted", func(t *testing.T) {
		if _, err := ParseFormat("JSON"); err != nil {
			t.Errorf("ParseFormat(JSON) returned error: %v", err)
		}
	})

	t.Run("invalid format", func(t *testing.T) {
		if _, err := ParseFormat("xml"); err == nil {
			t.Error("ParseFormat(xml) expected error, got nil")
		}
	})
}

func TestNew(t *testing.T) {
	t.Run("defaults to stdout with text and info level", func(t *testing.T) {
		logger, err := New("info", "text", "")
		if err != nil {
			t.Fatalf("New() returned error: %v", err)
		}
		if logger == nil {
			t.Fatal("New() returned nil logger")
		}
		if logger.Level != logrus.InfoLevel {
			t.Errorf("logger.Level = %v, want info", logger.Level)
		}
		if logger.Out != os.Stdout {
			t.Errorf("logger.Out = %v, want os.Stdout", logger.Out)
		}
	})

	t.Run("json format emits valid JSON", func(t *testing.T) {
		logger, err := New("info", "json", "")
		if err != nil {
			t.Fatalf("New() returned error: %v", err)
		}

		var buf bytes.Buffer
		logger.SetOutput(&buf)
		logger.Infof("hello %s", "world")

		var entry map[string]any
		if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
			t.Fatalf("log output is not valid JSON: %v\nraw: %s", err, buf.String())
		}
		if entry["level"] != "info" {
			t.Errorf("level = %v, want info", entry["level"])
		}
		if entry["msg"] != "hello world" {
			t.Errorf("msg = %v, want %q", entry["msg"], "hello world")
		}
	})

	t.Run("invalid level returns error", func(t *testing.T) {
		if _, err := New("verbose", "text", ""); err == nil {
			t.Error("New() expected error for invalid level, got nil")
		}
	})

	t.Run("invalid format returns error", func(t *testing.T) {
		if _, err := New("info", "xml", ""); err == nil {
			t.Error("New() expected error for invalid format, got nil")
		}
	})

	t.Run("unwritable file path returns error", func(t *testing.T) {
		if _, err := New("info", "text", "/nonexistent-dir-xyz/server.log"); err == nil {
			t.Error("New() expected error for unwritable file path, got nil")
		}
	})
}

func TestNew_FileOutput(t *testing.T) {
	t.Run("file output writes to the file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "server.log")

		logger, err := New("info", "json", path)
		if err != nil {
			t.Fatalf("New() returned error: %v", err)
		}
		logger.Infof("written to file")

		// #nosec G304 -- path points to a file created by this test in t.TempDir().
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		if !strings.Contains(string(data), "written to file") {
			t.Errorf("log file does not contain message: %q", string(data))
		}
	})

	t.Run("file is created with 0600 permissions", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "server.log")

		if _, err := New("info", "text", path); err != nil {
			t.Fatalf("New() returned error: %v", err)
		}

		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("file permissions = %o, want 600", perm)
		}
	})
}
