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
	"io"
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

// Options configures the logger built by New. It is transport-agnostic so the
// logging package does not need to import config (avoiding an import cycle):
//
//   - Mode     — "http" or "stdio". Determines whether logging is enabled by
//     default and where output is routed (see L02/L01 in the SPEC).
//   - Level    — optional explicit level; nil means LOG_LEVEL was unset.
//   - Format   — "text" (default) or "json".
//   - Filename — optional log file path; when non-empty the logger appends to
//     it (created with 0600 permissions) instead of stdout.
type Options struct {
	Mode     string
	Level    *string
	Format   string
	Filename string
}

// New builds a *logrus.Logger from the given Options.
//
// Logging enablement (L02): a logger is enabled when the mode is "http" (the
// default assumption is that HTTP deployments want logs) or an explicit level
// was provided. A stdio deployment with LOG_LEVEL unset is disabled: the
// returned logger writes to io.Discard at PanicLevel so it remains a valid
// logger for wiring into slog/the MCP SDK but produces no output.
//
// Output channel (L01): when Filename is set the logger appends to that file;
// an http logger otherwise writes to stdout; an enabled stdio logger with no
// filename writes to io.Discard (never stdout, N04GO).
func New(opts Options) (*logrus.Logger, error) {
	formatter, err := ParseFormat(opts.Format)
	if err != nil {
		return nil, err
	}

	enabled := opts.Mode == "http" || opts.Level != nil
	if !enabled {
		// Disabled (stdio + LOG_LEVEL unset): a valid but silent logger. The
		// SDK still needs a non-nil logger, so we return one that drops all
		// output rather than a nil pointer.
		logger := logrus.New()
		logger.SetOutput(io.Discard)
		logger.SetLevel(logrus.PanicLevel)
		return logger, nil
	}

	// Resolve level: http defaults to InfoLevel unless an explicit level is set.
	level := logrus.InfoLevel
	if opts.Level != nil {
		lvl, err := ParseLevel(*opts.Level)
		if err != nil {
			return nil, err
		}
		level = lvl
	}

	var out io.Writer
	switch {
	case opts.Filename != "":
		// #nosec G304 -- filename comes from operator-controlled config (LOG_FILENAME), not user input.
		f, err := os.OpenFile(opts.Filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return nil, fmt.Errorf("open log file: %w", err)
		}
		out = f
	case opts.Mode == "http":
		out = os.Stdout
	default:
		// Enabled stdio with no filename: never write to stdout (N04GO).
		out = io.Discard
	}

	logger := logrus.New()
	logger.SetOutput(out)
	logger.SetFormatter(formatter)
	logger.SetLevel(level)
	return logger, nil
}

// NewString is a thin compatibility wrapper around the Options-based New. It
// keeps the previous string-based constructor available for existing callers
// and tests, treating the logger as an http logger (always enabled) with the
// given level, format and optional filename.
func NewString(level, format, filename string) (*logrus.Logger, error) {
	return New(Options{Mode: "http", Level: &level, Format: format, Filename: filename})
}
