package handlers

import (
	"strings"
	"testing"
)

// secretFixture is a struct used to exercise the annotation-driven redaction
// helper (S02): fields that may hold sensitive data are annotated secret:"true",
// and the helper must mask exactly those fields.
type secretFixture struct {
	Name    string
	APIKey  string `secret:"true"`
	URL     string `secret:"true"`
	Nested  nestedFixture
	Visible int
}

type nestedFixture struct {
	Token string `secret:"true"`
	Label string
}

func TestRedactValue(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   any
		want any
	}{
		{
			// #nosec G101 -- test fixture deliberately uses fake credentials.
			name: "user:pass URL keeps host, hides userinfo",
			in:   "http://user:pass@host/search?q=x",
			want: "http://host/search?q=x",
		},
		{
			// #nosec G101 -- test fixture deliberately uses a fake token.
			name: "token URL keeps host, hides token",
			in:   "https://token@host/x",
			want: "https://host/x",
		},
		{
			name: "URL without userinfo is kept",
			in:   "https://searxng.example.com:8888",
			want: "https://searxng.example.com:8888",
		},
		{
			name: "plain secret string is masked",
			in:   "super-secret-token",
			want: redactedMarker,
		},
		{
			name: "non-string value is masked",
			in:   42,
			want: redactedMarker,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := RedactValue(tt.in); got != tt.want {
				t.Errorf("RedactValue(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestRedactStructMasksSecrets(t *testing.T) {
	t.Parallel()

	// #nosec G101 -- test fixture deliberately uses fake credentials.
	in := secretFixture{
		Name:    "visible",
		APIKey:  "hunter2",
		URL:     "https://user:pass@searxng.example.com",
		Nested:  nestedFixture{Token: "tok", Label: "label"},
		Visible: 7,
	}

	got, _ := RedactStruct(in).(secretFixture)

	if got.Name != "visible" {
		t.Errorf("Name = %q, want unchanged", got.Name)
	}
	if got.APIKey != redactedMarker {
		t.Errorf("APIKey = %q, want %q", got.APIKey, redactedMarker)
	}
	if got.URL != "https://searxng.example.com" {
		t.Errorf("URL = %q, want host-visible redacted URL", got.URL)
	}
	if got.Nested.Token != redactedMarker {
		t.Errorf("Nested.Token = %q, want %q", got.Nested.Token, redactedMarker)
	}
	if got.Nested.Label != "label" {
		t.Errorf("Nested.Label = %q, want unchanged", got.Nested.Label)
	}
	if got.Visible != 7 {
		t.Errorf("Visible = %d, want unchanged", got.Visible)
	}
}

func TestRedactStructNoSecretFields(t *testing.T) {
	t.Parallel()

	in := struct {
		Query string
		Page  int
	}{Query: "hello", Page: 2}

	got, _ := RedactStruct(in).(struct {
		Query string
		Page  int
	})

	if got != in {
		t.Errorf("RedactStruct = %+v, want unchanged %+v", got, in)
	}
}

func TestRedactStructNonStructValue(t *testing.T) {
	t.Parallel()

	if got := RedactStruct(map[string]string{"query": "x"}); got == nil {
		t.Fatal("RedactStruct on a map returned nil")
	}
	if got := RedactStruct("plain"); got != "plain" {
		t.Errorf("RedactStruct on a string = %v, want unchanged", got)
	}
}

func TestRedactStructNonStringSecret(t *testing.T) {
	t.Parallel()

	in := struct {
		Pin int `secret:"true"`
	}{Pin: 1234}

	got, _ := RedactStruct(in).(struct {
		Pin int `secret:"true"`
	})

	if got.Pin != 0 {
		t.Errorf("Pin = %d, want zeroed to avoid leaking", got.Pin)
	}
}

func TestRedactArgsAnnotationDriven(t *testing.T) {
	t.Parallel()

	// #nosec G101 -- test fixture deliberately uses fake credentials.
	got := redactArgs(secretFixture{
		Name:   "visible",
		APIKey: "hunter2",
		URL:    "https://user:pass@searxng.example.com",
		Nested: nestedFixture{Token: "secret-token"},
	})

	// Annotated secret fields must never reach the access-log args.
	for _, leak := range []string{"hunter2", "user:pass", "secret-token"} {
		if strings.Contains(got, leak) {
			t.Errorf("redactArgs leaked %q: %s", leak, got)
		}
	}
	// Non-secret fields are preserved.
	if !strings.Contains(got, "visible") {
		t.Errorf("redactArgs dropped a non-secret field: %s", got)
	}
	// For a URL secret field the host stays visible (userinfo hidden), matching
	// the startup-banner contract.
	if !strings.Contains(got, "searxng.example.com") {
		t.Errorf("redactArgs should keep the URL host visible: %s", got)
	}
}
