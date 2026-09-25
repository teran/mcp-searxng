// Package logging provides a configurable logrus logger for the server.
//
// It is the single place that translates the LOG_LEVEL / LOG_FORMAT /
// LOG_FILENAME configuration (validated in the config package) into a
// concrete *logrus.Logger. By default the logger writes structured text to
// stdout (12-factor / L01); LOG_FORMAT=json switches to JSON output, and a
// non-empty LOG_FILENAME redirects output to a file (L03).
package logging

import (
	"fmt"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
)

// ParseLevel maps a string to a logrus.Level. It accepts the levels declared
// in the SPEC: debug, info, warn, warning, error, fatal, panic. An unknown
// value returns an error so invalid configuration fails fast at startup.
func ParseLevel(s string) (logrus.Level, error) {
	switch strings.ToLower(s) {
	case "debug":
		return logrus.DebugLevel, nil
	case "info":
		return logrus.InfoLevel, nil
	case "warn", "warning":
		return logrus.WarnLevel, nil
	case "error":
		return logrus.ErrorLevel, nil
	case "fatal":
		return logrus.FatalLevel, nil
	case "panic":
		return logrus.PanicLevel, nil
	default:
		return 0, fmt.Errorf("invalid log level %q (want debug, info, warn, warning, error, fatal, panic)", s)
	}
}

// ParseFormat maps a string to a logrus.Formatter. It accepts "text" (the
// default) and "json". An unknown value returns an error so invalid
// configuration fails fast at startup.
func ParseFormat(s string) (logrus.Formatter, error) {
	switch strings.ToLower(s) {
	case "text", "":
		return &logrus.TextFormatter{}, nil
	case "json":
		return &logrus.JSONFormatter{}, nil
	default:
		return nil, fmt.Errorf("invalid log format %q (want text or json)", s)
	}
}

// New builds a *logrus.Logger from the given level, format and (optional)
// filename. When filename is non-empty the logger appends to that file
// (created with 0600 permissions) instead of writing to stdout.
func New(level, format, filename string) (*logrus.Logger, error) {
	lvl, err := ParseLevel(level)
	if err != nil {
		return nil, err
	}

	formatter, err := ParseFormat(format)
	if err != nil {
		return nil, err
	}

	out := os.Stdout
	if filename != "" {
		// #nosec G304 -- filename comes from operator-controlled config (LOG_FILENAME), not user input.
		f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return nil, fmt.Errorf("open log file: %w", err)
		}
		out = f
	}

	logger := logrus.New()
	logger.SetOutput(out)
	logger.SetFormatter(formatter)
	logger.SetLevel(lvl)
	return logger, nil
}
