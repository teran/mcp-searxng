package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sirupsen/logrus"
)

func TestWrapAccessLog(t *testing.T) { //nolint:gocognit
	t.Parallel()

	type input struct {
		Query string `json:"query"`
	}

	newJSONLogger := func() (*logrus.Logger, *bytes.Buffer) {
		var buf bytes.Buffer
		logger := logrus.New()
		logger.SetOutput(&buf)
		logger.SetFormatter(&logrus.JSONFormatter{})
		return logger, &buf
	}

	t.Run("successful call logs access line", func(t *testing.T) {
		logger, buf := newJSONLogger()
		handler := WrapAccessLog(logger, "stdio", "search", func(_ context.Context, _ *mcp.CallToolRequest, in input) (*mcp.CallToolResult, any, error) {
			return nil, map[string]any{"query": in.Query}, nil
		})

		result, _, err := handler(context.Background(), &mcp.CallToolRequest{}, input{Query: "hello"})
		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		if result != nil {
			t.Errorf("result = %v, want nil", result)
		}

		var entry map[string]any
		if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
			t.Fatalf("log output is not valid JSON: %v\nraw: %s", err, buf.String())
		}
		if entry["tool"] != "search" {
			t.Errorf("tool = %v, want search", entry["tool"])
		}
		if entry["source"] != "stdio" {
			t.Errorf("source = %v, want stdio", entry["source"])
		}
		if entry["outcome"] != "success" {
			t.Errorf("outcome = %v, want success", entry["outcome"])
		}
		if !strings.Contains(entry["args"].(string), `"query":"hello"`) {
			t.Errorf("args = %v, want query field", entry["args"])
		}
		if entry["duration"] == nil {
			t.Error("duration missing from access log")
		}
	})

	t.Run("error call logs error outcome", func(t *testing.T) {
		logger, buf := newJSONLogger()
		handler := WrapAccessLog(logger, "http", "search", func(_ context.Context, _ *mcp.CallToolRequest, _ input) (*mcp.CallToolResult, any, error) {
			return nil, nil, errors.New("search failed\ninjection")
		})

		_, _, err := handler(context.Background(), &mcp.CallToolRequest{}, input{Query: "q"})
		if err == nil {
			t.Fatal("expected handler error, got nil")
		}

		var entry map[string]any
		if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
			t.Fatalf("log output is not valid JSON: %v\nraw: %s", err, buf.String())
		}
		if entry["outcome"] != "error" {
			t.Errorf("outcome = %v, want error", entry["outcome"])
		}
		// Sanitization strips the newline to prevent log injection.
		if strings.Contains(entry["error"].(string), "\n") {
			t.Errorf("error field not sanitized: %v", entry["error"])
		}
	})

	t.Run("redacts control characters from args", func(t *testing.T) {
		logger, buf := newJSONLogger()
		handler := WrapAccessLog(logger, "http", "search", func(_ context.Context, _ *mcp.CallToolRequest, _ input) (*mcp.CallToolResult, any, error) {
			return nil, nil, nil
		})

		if _, _, err := handler(context.Background(), &mcp.CallToolRequest{}, input{Query: "evil\n\u001b[31mred"}); err != nil {
			t.Fatalf("handler returned error: %v", err)
		}

		// Sanitization must strip control characters (the escape byte) and the
		// literal newline so the log line stays valid, single-line JSON.
		var entry map[string]any
		if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
			t.Fatalf("log output is not valid JSON (sanitization failed): %v\nraw: %s", err, buf.String())
		}
		args, ok := entry["args"].(string)
		if !ok {
			t.Fatalf("args missing from access log: %v", entry)
		}
		if strings.ContainsAny(args, "\n\x1b") {
			t.Errorf("access log args contain raw control characters: %q", args)
		}
	})

	t.Run("disabled logger emits no access log", func(t *testing.T) {
		var buf bytes.Buffer
		logger := logrus.New()
		logger.SetOutput(&buf)
		logger.SetLevel(logrus.PanicLevel) // disabled

		called := false
		handler := WrapAccessLog(logger, "http", "search", func(_ context.Context, _ *mcp.CallToolRequest, _ input) (*mcp.CallToolResult, any, error) {
			called = true
			return nil, nil, nil
		})

		if _, _, err := handler(context.Background(), &mcp.CallToolRequest{}, input{Query: "q"}); err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		if !called {
			t.Error("underlying handler not called")
		}
		if buf.Len() != 0 {
			t.Errorf("disabled logger should emit no output, got %q", buf.String())
		}
	})

	t.Run("nil logger is a no-op and does not panic", func(t *testing.T) {
		called := false
		handler := WrapAccessLog(nil, "http", "search", func(_ context.Context, _ *mcp.CallToolRequest, in input) (*mcp.CallToolResult, any, error) {
			called = true
			return &mcp.CallToolResult{}, map[string]any{"query": in.Query}, nil
		})

		result, output, err := handler(context.Background(), &mcp.CallToolRequest{}, input{Query: "hello"})
		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		if !called {
			t.Error("underlying handler not called")
		}
		if result == nil {
			t.Error("result should be passed through unchanged")
		}
		if got, ok := output.(map[string]any); !ok || got["query"] != "hello" {
			t.Errorf("output not passed through: %#v", output)
		}
	})
}

func TestRedactArgs(t *testing.T) {
	t.Parallel()

	t.Run("serializes struct to JSON", func(t *testing.T) {
		got := redactArgs(map[string]string{"query": "hello"})
		if !strings.Contains(got, `"query":"hello"`) {
			t.Errorf("redactArgs = %q, want query field", got)
		}
	})

	t.Run("sanitizes control characters", func(t *testing.T) {
		got := redactArgs(map[string]string{"query": "a\nb"})
		if strings.Contains(got, "\n") {
			t.Errorf("redactArgs = %q, want newline removed", got)
		}
	})

	t.Run("unserializable input returns marker", func(t *testing.T) {
		got := redactArgs(func() {}) // function is not JSON-marshalable
		if got != "<unserializable>" {
			t.Errorf("redactArgs = %q, want marker", got)
		}
	})

	t.Run("long input is bounded", func(t *testing.T) {
		got := redactArgs(map[string]string{"query": strings.Repeat("x", accessLogMaxArgs*2)})
		if len(got) > accessLogMaxArgs+3 {
			t.Errorf("redactArgs length = %d, want <= %d", len(got), accessLogMaxArgs+3)
		}
	})
}
