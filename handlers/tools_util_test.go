package handlers

import (
	"testing"
)

func TestFormatDate(t *testing.T) {
	t.Parallel()

	t.Run("nil input returns empty", func(t *testing.T) {
		if got := formatDate(nil); got != "" {
			t.Errorf("formatDate(nil) = %q, want empty", got)
		}
	})

	t.Run("empty string returns empty", func(t *testing.T) {
		s := ""
		if got := formatDate(&s); got != "" {
			t.Errorf("formatDate(&\"\") = %q, want empty", got)
		}
	})

	t.Run("RFC3339 format", func(t *testing.T) {
		s := "2026-07-10T14:30:00Z"
		if got := formatDate(&s); got != "10 Jul 2026" {
			t.Errorf("formatDate(%q) = %q, want %q", s, got, "10 Jul 2026")
		}
	})

	t.Run("ISO 8601 without timezone", func(t *testing.T) {
		s := "2026-01-15T10:00:00"
		if got := formatDate(&s); got != "15 Jan 2026" {
			t.Errorf("formatDate(%q) = %q, want %q", s, got, "15 Jan 2026")
		}
	})

	t.Run("date only format", func(t *testing.T) {
		s := "2025-12-25"
		if got := formatDate(&s); got != "25 Dec 2025" {
			t.Errorf("formatDate(%q) = %q, want %q", s, got, "25 Dec 2025")
		}
	})

	t.Run("invalid date returns empty", func(t *testing.T) {
		s := "not-a-date"
		if got := formatDate(&s); got != "" {
			t.Errorf("formatDate(%q) = %q, want empty", s, got)
		}
	})
}

func TestSanitizeURL(t *testing.T) {
	t.Parallel()

	t.Run("empty string returns empty", func(t *testing.T) {
		if got := sanitizeURL(""); got != "" {
			t.Errorf("sanitizeURL(\"\") = %q, want empty", got)
		}
	})

	t.Run("valid https URL preserved", func(t *testing.T) {
		url := "https://example.com/path?q=1"
		if got := sanitizeURL(url); got != url {
			t.Errorf("sanitizeURL(%q) = %q, want %q", url, got, url)
		}
	})

	t.Run("valid http URL preserved", func(t *testing.T) {
		url := "http://example.com"
		if got := sanitizeURL(url); got != url {
			t.Errorf("sanitizeURL(%q) = %q, want %q", url, got, url)
		}
	})

	t.Run("invalid URL returns empty", func(t *testing.T) {
		if got := sanitizeURL("://invalid"); got != "" {
			t.Errorf("sanitizeURL(\"://invalid\") = %q, want empty", got)
		}
	})

	t.Run("non-http scheme returns empty", func(t *testing.T) {
		if got := sanitizeURL("ftp://example.com"); got != "" {
			t.Errorf("sanitizeURL(\"ftp://example.com\") = %q, want empty", got)
		}
	})

	t.Run("javascript URL returns empty", func(t *testing.T) {
		if got := sanitizeURL("javascript:alert(1)"); got != "" {
			t.Errorf("sanitizeURL(\"javascript:alert(1)\") = %q, want empty", got)
		}
	})
}

func TestSanitizeOutputPtr(t *testing.T) {
	t.Parallel()

	t.Run("nil returns nil", func(t *testing.T) {
		if got := sanitizeOutputPtr(nil); got != nil {
			t.Errorf("sanitizeOutputPtr(nil) = %v, want nil", got)
		}
	})

	t.Run("non-nil returns sanitized pointer", func(t *testing.T) {
		s := "hello\x00world"
		got := sanitizeOutputPtr(&s)
		if got == nil {
			t.Fatal("sanitizeOutputPtr(&s) = nil, want non-nil")
		}
		if *got != "helloworld" {
			t.Errorf("sanitizeOutputPtr = %q, want %q", *got, "helloworld")
		}
	})
}

func TestSanitizeImgSrc(t *testing.T) {
	t.Parallel()

	t.Run("nil returns nil", func(t *testing.T) {
		if got := sanitizeImgSrc(nil); got != nil {
			t.Errorf("sanitizeImgSrc(nil) = %v, want nil", got)
		}
	})

	t.Run("valid https URL preserved", func(t *testing.T) {
		s := "https://example.com/image.jpg"
		got := sanitizeImgSrc(&s)
		if got == nil {
			t.Fatal("sanitizeImgSrc = nil, want non-nil")
		}
		if *got != s {
			t.Errorf("sanitizeImgSrc = %q, want %q", *got, s)
		}
	})

	t.Run("data URI preserved", func(t *testing.T) {
		s := "data:image/png;base64,abc123"
		got := sanitizeImgSrc(&s)
		if got == nil {
			t.Fatal("sanitizeImgSrc = nil, want non-nil")
		}
		if *got != s {
			t.Errorf("sanitizeImgSrc = %q, want %q", *got, s)
		}
	})

	t.Run("invalid URL returns nil", func(t *testing.T) {
		s := "://invalid"
		if got := sanitizeImgSrc(&s); got != nil {
			t.Errorf("sanitizeImgSrc(%q) = %v, want nil", s, got)
		}
	})

	t.Run("non-http scheme returns nil", func(t *testing.T) {
		s := "ftp://example.com/image.jpg"
		if got := sanitizeImgSrc(&s); got != nil {
			t.Errorf("sanitizeImgSrc(%q) = %v, want nil", s, got)
		}
	})

	t.Run("javascript URL returns nil", func(t *testing.T) {
		s := "javascript:alert(1)"
		if got := sanitizeImgSrc(&s); got != nil {
			t.Errorf("sanitizeImgSrc(%q) = %v, want nil", s, got)
		}
	})
}

func TestSanitizeOutput(t *testing.T) {
	t.Parallel()

	t.Run("normal string preserved", func(t *testing.T) {
		if got := sanitizeOutput("hello world"); got != "hello world" {
			t.Errorf("sanitizeOutput = %q, want %q", got, "hello world")
		}
	})

	t.Run("tabs preserved", func(t *testing.T) {
		if got := sanitizeOutput("tab\there"); got != "tab\there" {
			t.Errorf("sanitizeOutput = %q, want %q", got, "tab\there")
		}
	})

	t.Run("newlines preserved", func(t *testing.T) {
		if got := sanitizeOutput("line1\nline2"); got != "line1\nline2" {
			t.Errorf("sanitizeOutput = %q, want %q", got, "line1\nline2")
		}
	})

	t.Run("carriage returns preserved", func(t *testing.T) {
		if got := sanitizeOutput("a\rb"); got != "a\rb" {
			t.Errorf("sanitizeOutput = %q, want %q", got, "a\rb")
		}
	})

	t.Run("control chars stripped", func(t *testing.T) {
		if got := sanitizeOutput("a\x00b\x01c"); got != "abc" {
			t.Errorf("sanitizeOutput = %q, want %q", got, "abc")
		}
	})

	t.Run("DEL char stripped", func(t *testing.T) {
		if got := sanitizeOutput("a\x7fb"); got != "ab" {
			t.Errorf("sanitizeOutput = %q, want %q", got, "ab")
		}
	})

	t.Run("bidi LRM RLM stripped", func(t *testing.T) {
		if got := sanitizeOutput("a\u200eb\u200fc"); got != "abc" {
			t.Errorf("sanitizeOutput = %q, want %q", got, "abc")
		}
	})

	t.Run("bidi LRE RLE PDF LRO RLO stripped", func(t *testing.T) {
		if got := sanitizeOutput("a\u202ab\u202bc\u202cd\u202de\u202ef"); got != "abcdef" {
			t.Errorf("sanitizeOutput = %q, want %q", got, "abcdef")
		}
	})

	t.Run("bidi LRI RLI FSI PDI stripped", func(t *testing.T) {
		if got := sanitizeOutput("a\u2066b\u2067c\u2068d\u2069e"); got != "abcde" {
			t.Errorf("sanitizeOutput = %q, want %q", got, "abcde")
		}
	})
}

func TestValidateQuery(t *testing.T) {
	t.Parallel()

	t.Run("valid query returns nil", func(t *testing.T) {
		if err := validateQuery("test query"); err != nil {
			t.Errorf("validateQuery = %v, want nil", err)
		}
	})

	t.Run("empty query returns error", func(t *testing.T) {
		if err := validateQuery(""); err != ErrInvalidQuery {
			t.Errorf("validateQuery(\"\") = %v, want %v", err, ErrInvalidQuery)
		}
	})

	t.Run("query too long returns error", func(t *testing.T) {
		long := make([]byte, MaxQueryLength+1)
		for i := range long {
			long[i] = 'a'
		}
		if err := validateQuery(string(long)); err != ErrQueryTooLong {
			t.Errorf("validateQuery(too long) = %v, want %v", err, ErrQueryTooLong)
		}
	})
}

func TestValidatePage(t *testing.T) {
	t.Parallel()

	t.Run("valid page returns nil", func(t *testing.T) {
		if err := validatePage(1); err != nil {
			t.Errorf("validatePage(1) = %v, want nil", err)
		}
	})

	t.Run("max page number returns nil", func(t *testing.T) {
		if err := validatePage(MaxPageNumber); err != nil {
			t.Errorf("validatePage(%d) = %v, want nil", MaxPageNumber, err)
		}
	})

	t.Run("page too large returns error", func(t *testing.T) {
		if err := validatePage(MaxPageNumber + 1); err != ErrPageTooLarge {
			t.Errorf("validatePage(%d) = %v, want %v", MaxPageNumber+1, err, ErrPageTooLarge)
		}
	})
}
