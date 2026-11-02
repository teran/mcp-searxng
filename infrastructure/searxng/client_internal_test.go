package searxng

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// TestStatusClass verifies the coarse status-class bucketing used for the
// upstream metrics label (O03), covering every branch.
func TestStatusClass(t *testing.T) {
	t.Parallel()

	cases := []struct {
		code int
		want string
	}{
		{200, "2xx"},
		{299, "2xx"},
		{300, "3xx"},
		{399, "3xx"},
		{400, "4xx"},
		{499, "4xx"},
		{500, "5xx"},
		{599, "5xx"},
	}

	for _, testCase := range cases {
		t.Run(fmt.Sprintf("%d", testCase.code), func(t *testing.T) {
			if got := statusClass(testCase.code); got != testCase.want {
				t.Errorf("statusClass(%d) = %q, want %q", testCase.code, got, testCase.want)
			}
		})
	}
}

func TestExtractErrorDetail(t *testing.T) {
	t.Parallel()

	t.Run("empty body returns status text", testExtractErrorDetailEmptyBody)
	t.Run("JSON detail field is preferred", testExtractErrorDetailJSONDetailPreferred)
	t.Run("JSON message field used when detail absent", testExtractErrorDetailJSONMessage)
	t.Run("invalid JSON falls back to plain text", testExtractErrorDetailInvalidJSON)
	t.Run("plain body truncated at 512 chars", testExtractErrorDetailTruncated)
	t.Run("plain body with newline uses first line only", testExtractErrorDetailNewline)
	t.Run("plain body with carriage return uses first line only", testExtractErrorDetailCRLF)
	t.Run("JSON detail present but empty string falls through", testExtractErrorDetailEmptyDetail)
	t.Run("both detail and message empty falls to plain text", testExtractErrorDetailBothEmpty)
	t.Run("JSON detail valid but detail key missing", testExtractErrorDetailMissingKey)
	t.Run("body exactly 512 chars not truncated", testExtractErrorDetailExact512)
}

func testExtractErrorDetailEmptyBody(t *testing.T) {
	detail := extractErrorDetail([]byte{}, http.StatusNotFound)
	if detail != "Not Found" {
		t.Errorf("detail = %q, want %q", detail, "Not Found")
	}
}

func testExtractErrorDetailJSONDetailPreferred(t *testing.T) {
	detail := extractErrorDetail(
		[]byte(`{"detail":"custom error","message":"fallback"}`),
		http.StatusInternalServerError,
	)
	if detail != "custom error" {
		t.Errorf("detail = %q, want %q", detail, "custom error")
	}
}

func testExtractErrorDetailJSONMessage(t *testing.T) {
	detail := extractErrorDetail(
		[]byte(`{"message":"error message"}`),
		http.StatusInternalServerError,
	)
	if detail != "error message" {
		t.Errorf("detail = %q, want %q", detail, "error message")
	}
}

func testExtractErrorDetailInvalidJSON(t *testing.T) {
	detail := extractErrorDetail(
		[]byte(`not json`),
		http.StatusInternalServerError,
	)
	if detail != "not json" {
		t.Errorf("detail = %q, want %q", detail, "not json")
	}
}

func testExtractErrorDetailTruncated(t *testing.T) {
	longBody := []byte(strings.Repeat("A", 600))
	detail := extractErrorDetail(longBody, http.StatusInternalServerError)
	if len(detail) != 512 {
		t.Errorf("len(detail) = %d, want %d", len(detail), 512)
	}
}

func testExtractErrorDetailNewline(t *testing.T) {
	detail := extractErrorDetail(
		[]byte("first line\nsecond line"),
		http.StatusInternalServerError,
	)
	if detail != "first line" {
		t.Errorf("detail = %q, want %q", detail, "first line")
	}
}

func testExtractErrorDetailCRLF(t *testing.T) {
	detail := extractErrorDetail(
		[]byte("first line\r\nsecond line"),
		http.StatusInternalServerError,
	)
	if detail != "first line" {
		t.Errorf("detail = %q, want %q", detail, "first line")
	}
}

func testExtractErrorDetailEmptyDetail(t *testing.T) {
	detail := extractErrorDetail(
		[]byte(`{"detail":"","message":"real message"}`),
		http.StatusInternalServerError,
	)
	if detail != "real message" {
		t.Errorf("detail = %q, want %q", detail, "real message")
	}
}

func testExtractErrorDetailBothEmpty(t *testing.T) {
	detail := extractErrorDetail(
		[]byte(`{"detail":"","message":""}`),
		http.StatusInternalServerError,
	)
	if detail == "" {
		t.Errorf("detail = %q, want non-empty fallback", detail)
	}
}

func testExtractErrorDetailMissingKey(t *testing.T) {
	// JSON body is valid but has no "detail" or "message" keys.
	detail := extractErrorDetail(
		[]byte(`{"error":"something broke"}`),
		http.StatusInternalServerError,
	)
	// Should fall through both JSON attempts and return the raw body as plain text.
	if detail != `{"error":"something broke"}` {
		t.Errorf("detail = %q, want %q", detail, `{"error":"something broke"}`)
	}
}

func testExtractErrorDetailExact512(t *testing.T) {
	body := []byte(strings.Repeat("B", 512))
	detail := extractErrorDetail(body, http.StatusInternalServerError)
	if len(detail) != 512 {
		t.Errorf("len(detail) = %d, want %d", len(detail), 512)
	}
}
