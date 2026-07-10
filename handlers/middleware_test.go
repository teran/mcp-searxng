package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSanitizeLog(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
	}{
		{"normal string", "normal string"},
		{"tab\there", "tab\there"},
		{"newline\nhere", "newlinehere"},
		{"carriage\rreturn", "carriagereturn"},
		{"null\x00byte", "nullbyte"},
		{"\x1b[31mred\x1b[0m", "[31mred[0m"},
		// Unicode bidi formatting characters (should be removed)
		{"bidi\u200Eleft", "bidileft"},
		{"bidi\u200Fright", "bidiright"},
		{"\u202Aembed\u202C", "embed"},
		{"\u202Doverride\u202C", "override"},
		{"\u202Ereversed", "reversed"},
		{"\u2066isolate\u2069", "isolate"},
		{"\u2067rli\u2069", "rli"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("sanitize(%q)", tt.input), func(t *testing.T) {
			got := SanitizeLog(tt.input)
			if got != tt.expected {
				t.Errorf("SanitizeLog(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestRecoveryMiddleware(t *testing.T) {
	t.Parallel()

	t.Run("panicking handler recovers and returns 500", func(t *testing.T) {
		handler := RecoveryMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic("test panic")
		}))

		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusInternalServerError {
			t.Errorf("expected 500, got %d", rr.Code)
		}
	})

	t.Run("normal handler passes through", func(t *testing.T) {
		handler := RecoveryMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
	})
}

func TestBodyLimitMiddleware(t *testing.T) {
	t.Parallel()

	t.Run("body within limit passes through", func(t *testing.T) {
		handler := BodyLimitMiddleware(1024)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("ReadAll: %v", err)
			}
			if len(body) != 5 {
				t.Errorf("body length = %d, want 5", len(body))
			}
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", strings.NewReader("hello"))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
	})

	t.Run("body exceeds limit returns error", func(t *testing.T) {
		handler := BodyLimitMiddleware(5)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// MaxBytesReader will return an error when reading.
			_, err := io.ReadAll(r.Body)
			if err == nil {
				t.Error("expected MaxBytesReader error, got nil")
			}
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", strings.NewReader("hello world"))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
	})
}

func TestMaxBytesError(t *testing.T) {
	t.Parallel()

	t.Run("nil error returns false", func(t *testing.T) {
		if MaxBytesError(nil) {
			t.Error("MaxBytesError(nil) = true, want false")
		}
	})

	t.Run("http.MaxBytesError returns true", func(t *testing.T) {
		err := http.MaxBytesError{}
		if !MaxBytesError(&err) {
			t.Error("MaxBytesError = false, want true")
		}
	})

	t.Run("other error returns false", func(t *testing.T) {
		if MaxBytesError(fmt.Errorf("some error")) {
			t.Error("MaxBytesError = true, want false")
		}
	})
}

func TestMCPRequestMethod(t *testing.T) {
	t.Parallel()

	t.Run("empty body", func(t *testing.T) {
		if got := mcpRequestMethod(nil); got != "empty_body" {
			t.Errorf("got %q, want %q", got, "empty_body")
		}
	})

	t.Run("tools/call with tool name", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{
			"method": "tools/call",
			"params": map[string]any{
				"name": "search",
			},
		})
		if got := mcpRequestMethod(body); got != "search" {
			t.Errorf("got %q, want %q", got, "search")
		}
	})

	t.Run("non-tool method", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{
			"method": "ping",
		})
		if got := mcpRequestMethod(body); got != "ping" {
			t.Errorf("got %q, want %q", got, "ping")
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		if got := mcpRequestMethod([]byte("not json")); got != "parse_error" {
			t.Errorf("got %q, want %q", got, "parse_error")
		}
	})

	t.Run("no method field", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{
			"id": 1,
		})
		if got := mcpRequestMethod(body); got != "no_method" {
			t.Errorf("got %q, want %q", got, "no_method")
		}
	})
}

func TestLoggingMiddleware(t *testing.T) {
	t.Parallel()

	t.Run("logs request and passes through", func(t *testing.T) {
		handler := LoggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0"}`))
		}))

		body := strings.NewReader(`{"method":"ping"}`)
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/mcp", body)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
	})

	t.Run("rejects batch request exceeding max size", func(t *testing.T) {
		batch := make([]map[string]any, MaxBatchSize+1)
		for i := range batch {
			batch[i] = map[string]any{"method": "ping"}
		}
		body, _ := json.Marshal(batch)

		handler := LoggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("next handler should not be called for oversized batch")
		}))

		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/mcp", strings.NewReader(string(body)))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rr.Code)
		}
	})
}

func TestCheckBatchSize(t *testing.T) {
	t.Parallel()

	t.Run("empty body passes", func(t *testing.T) {
		if err := checkBatchSize(nil); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("single object is not a batch", func(t *testing.T) {
		body := []byte(`{}`)
		if err := checkBatchSize(body); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("batch within limit passes", func(t *testing.T) {
		batch := make([]map[string]any, 5)
		for i := range batch {
			batch[i] = map[string]any{"method": "ping"}
		}
		body, _ := json.Marshal(batch)
		if err := checkBatchSize(body); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("batch exceeding limit fails", func(t *testing.T) {
		batch := make([]map[string]any, MaxBatchSize+1)
		for i := range batch {
			batch[i] = map[string]any{"method": "ping"}
		}
		body, _ := json.Marshal(batch)
		if err := checkBatchSize(body); err == nil {
			t.Error("expected error, got nil")
		}
	})

	t.Run("malformed JSON does not error", func(t *testing.T) {
		if err := checkBatchSize([]byte("[invalid")); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestLoggingResponseWriter(t *testing.T) {
	t.Parallel()

	t.Run("captures status code and body size", func(t *testing.T) {
		rr := httptest.NewRecorder()
		lrw := &loggingResponseWriter{
			ResponseWriter: rr,
			statusCode:     http.StatusOK,
		}

		lrw.WriteHeader(http.StatusNotFound)
		n, err := lrw.Write([]byte("test"))
		if err != nil {
			t.Fatalf("Write: %v", err)
		}

		if lrw.statusCode != http.StatusNotFound {
			t.Errorf("statusCode = %d, want %d", lrw.statusCode, http.StatusNotFound)
		}
		if lrw.bodySize != 4 {
			t.Errorf("bodySize = %d, want 4", lrw.bodySize)
		}
		if n != 4 {
			t.Errorf("Write returned %d, want 4", n)
		}
	})
}
