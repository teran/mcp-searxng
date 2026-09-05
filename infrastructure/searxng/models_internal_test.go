package searxng

import (
	"encoding/json"
	"testing"

	"github.com/teran/mcp-searxng/domain"
)

func strPtr(s string) *string {
	return &s
}

func TestRawSearchResult_ToDomain_PubDateFallback(t *testing.T) {
	t.Parallel()

	t.Run("PubDate used when PublishedDate is nil", func(t *testing.T) {
		r := rawSearchResult{
			PublishedDate: nil,
			PubDate:       strPtr("2024-01-01"),
		}

		got := r.toDomain()
		if got.PublishedDate == nil || *got.PublishedDate != "2024-01-01" {
			t.Errorf("PublishedDate = %v, want %q", got.PublishedDate, "2024-01-01")
		}
	})

	t.Run("PublishedDate takes precedence over PubDate", func(t *testing.T) {
		r := rawSearchResult{
			PublishedDate: strPtr("2024-02-02"),
			PubDate:       strPtr("2024-01-01"),
		}

		got := r.toDomain()
		if got.PublishedDate == nil || *got.PublishedDate != "2024-02-02" {
			t.Errorf("PublishedDate = %v, want %q", got.PublishedDate, "2024-02-02")
		}
	})
}

func TestRawAnswerToString(t *testing.T) {
	t.Parallel()

	t.Run("plain string passes through", func(t *testing.T) {
		got := rawAnswerToString(json.RawMessage(`"42 is the answer"`))
		if string(got) != "42 is the answer" {
			t.Errorf("rawAnswerToString = %q, want %q", got, "42 is the answer")
		}
	})

	t.Run("object is marshaled back to JSON string", func(t *testing.T) {
		got := rawAnswerToString(json.RawMessage(`{"a":1}`))
		if string(got) != `{"a":1}` {
			t.Errorf("rawAnswerToString = %q, want %q", got, `{"a":1}`)
		}
	})

	t.Run("invalid raw message falls back to raw bytes", func(t *testing.T) {
		got := rawAnswerToString(json.RawMessage(`invalid`))
		if string(got) != "invalid" {
			t.Errorf("rawAnswerToString = %q, want %q", got, "invalid")
		}
	})

	t.Run("object answer flows through full toDomain conversion", func(t *testing.T) {
		resp := rawSearchResponse{
			Answers: []json.RawMessage{
				json.RawMessage(`{"a":1}`),
			},
		}
		got := resp.toDomain()
		if len(got.Answers) != 1 {
			t.Fatalf("len(Answers) = %d, want 1", len(got.Answers))
		}
		if string(got.Answers[0]) != `{"a":1}` {
			t.Errorf("Answers[0] = %q, want %q", got.Answers[0], `{"a":1}`)
		}
		if got.Answers[0] != domain.AnswerResult(`{"a":1}`) {
			t.Errorf("Answers[0] type/value = %q, want domain.AnswerResult %q", got.Answers[0], `{"a":1}`)
		}
	})
}
