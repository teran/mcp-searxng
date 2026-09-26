package logging

import (
	"context"
	"log/slog"
	"strings"

	"github.com/sirupsen/logrus"
)

// slogLevelToLogrus maps an slog.Level to the corresponding logrus.Level.
func slogLevelToLogrus(level slog.Level) logrus.Level {
	switch {
	case level < slog.LevelInfo:
		return logrus.DebugLevel
	case level < slog.LevelWarn:
		return logrus.InfoLevel
	case level < slog.LevelError:
		return logrus.WarnLevel
	default:
		return logrus.ErrorLevel
	}
}

// attrEntry binds an attribute to the group path that was active when it was
// added. This matters because slog semantics apply WithGroup only to
// attributes added *after* the group was opened — attributes added before a
// group must not be prefixed by it (e.g. With("server") then WithGroup("req")
// keeps "server" unprefixed while later attrs become "req.*").
type attrEntry struct {
	attr   slog.Attr
	groups []string
}

// logrusHandler is an slog.Handler that forwards every record to a
// *logrus.Logger, preserving the record's level, message and attributes. It
// lets events produced by the MCP go-sdk logger surface in the server's
// logrus output (L03GO/L07).
type logrusHandler struct {
	logger *logrus.Logger
	attrs  []attrEntry
	groups []string
}

// NewSlogHandler builds an slog.Handler that forwards records to logger.
func NewSlogHandler(logger *logrus.Logger) slog.Handler {
	return &logrusHandler{logger: logger}
}

// Slog wraps a *logrus.Logger as an *slog.Logger. It is intended for wiring
// into the MCP go-sdk ServerOptions.Logger so SDK log events reach logrus.
func Slog(logger *logrus.Logger) *slog.Logger {
	return slog.New(NewSlogHandler(logger))
}

// Enabled reports whether records at the given level would be forwarded,
// honouring the logrus logger's configured level.
func (h *logrusHandler) Enabled(_ context.Context, level slog.Level) bool {
	return h.logger.IsLevelEnabled(slogLevelToLogrus(level))
}

// Handle forwards a single record to logrus with its level and attributes.
func (h *logrusHandler) Handle(_ context.Context, r slog.Record) error {
	fields := make(logrus.Fields, len(h.attrs)+r.NumAttrs())
	for _, e := range h.attrs {
		h.appendAttr(fields, e.attr, e.groups)
	}
	r.Attrs(func(a slog.Attr) bool {
		h.appendAttr(fields, a, h.groups)
		return true
	})

	if !r.Time.IsZero() {
		fields["slog_time"] = r.Time
	}
	fields["slog_level"] = r.Level.String()

	h.logger.WithFields(fields).Log(slogLevelToLogrus(r.Level), r.Message)
	return nil
}

// WithAttrs returns a copy of the handler that also forwards attrs, binding
// each new attr to the currently active group path.
func (h *logrusHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	merged := make([]attrEntry, 0, len(h.attrs)+len(attrs))
	merged = append(merged, h.attrs...)
	for _, a := range attrs {
		merged = append(merged, attrEntry{attr: a, groups: h.groups})
	}
	return &logrusHandler{logger: h.logger, attrs: merged, groups: h.groups}
}

// WithGroup returns a copy of the handler that prefixes subsequent attribute
// keys with name (flattened with dots, matching slog's built-in handlers).
func (h *logrusHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	groups := append(append([]string{}, h.groups...), name)
	return &logrusHandler{logger: h.logger, attrs: h.attrs, groups: groups}
}

// appendAttr writes an attribute into fields, applying group prefixes and
// flattening group-valued attributes into dot-separated keys.
func (h *logrusHandler) appendAttr(fields logrus.Fields, a slog.Attr, groups []string) {
	a.Value = a.Value.Resolve()

	if a.Value.Kind() == slog.KindGroup {
		subGroups := append(append([]string{}, groups...), a.Key)
		for _, sub := range a.Value.Group() {
			h.appendAttr(fields, sub, subGroups)
		}
		return
	}

	key := a.Key
	if len(groups) > 0 {
		key = strings.Join(append(append([]string{}, groups...), a.Key), ".")
	}
	fields[key] = a.Value.Any()
}
