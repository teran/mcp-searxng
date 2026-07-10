package searxng

import (
	"net/http"
	"strings"
	"testing"
)

func TestExtractErrorDetail(t *testing.T) {
	t.Parallel()

	t.Run("empty body returns status text", func(t *testing.T) {
		detail := extractErrorDetail([]byte{}, http.StatusNotFound)
		if detail != "Not Found" {
			t.Errorf("detail = %q, want %q", detail, "Not Found")
		}
	})

	t.Run("JSON detail field is preferred", func(t *testing.T) {
		detail := extractErrorDetail(
			[]byte(`{"detail":"custom error","message":"fallback"}`),
			http.StatusInternalServerError,
		)
		if detail != "custom error" {
			t.Errorf("detail = %q, want %q", detail, "custom error")
		}
	})

	t.Run("JSON message field used when detail absent", func(t *testing.T) {
		detail := extractErrorDetail(
			[]byte(`{"message":"error message"}`),
			http.StatusInternalServerError,
		)
		if detail != "error message" {
			t.Errorf("detail = %q, want %q", detail, "error message")
		}
	})

	t.Run("invalid JSON falls back to plain text", func(t *testing.T) {
		detail := extractErrorDetail(
			[]byte(`not json`),
			http.StatusInternalServerError,
		)
		if detail != "not json" {
			t.Errorf("detail = %q, want %q", detail, "not json")
		}
	})

	t.Run("plain body truncated at 512 chars", func(t *testing.T) {
		longBody := []byte(strings.Repeat("A", 600))
		detail := extractErrorDetail(longBody, http.StatusInternalServerError)
		if len(detail) != 512 {
			t.Errorf("len(detail) = %d, want %d", len(detail), 512)
		}
	})

	t.Run("plain body with newline uses first line only", func(t *testing.T) {
		detail := extractErrorDetail(
			[]byte("first line\nsecond line"),
			http.StatusInternalServerError,
		)
		if detail != "first line" {
			t.Errorf("detail = %q, want %q", detail, "first line")
		}
	})

	t.Run("plain body with carriage return uses first line only", func(t *testing.T) {
		detail := extractErrorDetail(
			[]byte("first line\r\nsecond line"),
			http.StatusInternalServerError,
		)
		if detail != "first line" {
			t.Errorf("detail = %q, want %q", detail, "first line")
		}
	})

	t.Run("JSON detail present but empty string falls through", func(t *testing.T) {
		detail := extractErrorDetail(
			[]byte(`{"detail":"","message":"real message"}`),
			http.StatusInternalServerError,
		)
		if detail != "real message" {
			t.Errorf("detail = %q, want %q", detail, "real message")
		}
	})

	t.Run("both detail and message empty falls to plain text", func(t *testing.T) {
		detail := extractErrorDetail(
			[]byte(`{"detail":"","message":""}`),
			http.StatusInternalServerError,
		)
		if detail == "" {
			t.Errorf("detail = %q, want non-empty fallback", detail)
		}
	})

	t.Run("JSON detail valid but detail key missing", func(t *testing.T) {
		// JSON body is valid but has no "detail" or "message" keys.
		detail := extractErrorDetail(
			[]byte(`{"error":"something broke"}`),
			http.StatusInternalServerError,
		)
		// Should fall through both JSON attempts and return the raw body as plain text.
		if detail != `{"error":"something broke"}` {
			t.Errorf("detail = %q, want %q", detail, `{"error":"something broke"}`)
		}
	})

	t.Run("body exactly 512 chars not truncated", func(t *testing.T) {
		body := []byte(strings.Repeat("B", 512))
		detail := extractErrorDetail(body, http.StatusInternalServerError)
		if len(detail) != 512 {
			t.Errorf("len(detail) = %d, want %d", len(detail), 512)
		}
	})
}
