package searxng_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/teran/mcp-searxng/domain"
	infra "github.com/teran/mcp-searxng/infrastructure/searxng"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *infra.Client) {
	t.Helper()
	srv := httptest.NewServer(handler)
	client := infra.NewClient(srv.URL, &http.Client{})
	return srv, client
}

func TestClient_Search(t *testing.T) { //nolint:gocognit,maintidx
	t.Parallel()

	t.Run("successful search returns parsed response", func(t *testing.T) {
		srv, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("q") != "test query" {
				t.Errorf("q = %q, want %q", r.URL.Query().Get("q"), "test query")
			}
			if r.URL.Query().Get("format") != "json" {
				t.Errorf("format = %q, want %q", r.URL.Query().Get("format"), "json")
			}

			w.Header().Set("Content-Type", "application/json")
			resp := map[string]any{
				"query": "test query",
				"results": []map[string]any{
					{
						"title":   "Result 1",
						"url":     "https://example.com/1",
						"content": "Content 1",
						"engine":  "google",
					},
				},
				"number_of_results": 42,
			}
			_ = json.NewEncoder(w).Encode(resp)
		})
		defer srv.Close()

		result, err := client.Search(context.Background(), domain.SearchParams{
			Query: "test query",
		})
		if err != nil {
			t.Fatalf("Search() returned error: %v", err)
		}

		if result.Query != "test query" {
			t.Errorf("Query = %q, want %q", result.Query, "test query")
		}
		if result.NumberOfResults != 42 {
			t.Errorf("NumberOfResults = %d, want 42", result.NumberOfResults)
		}
		if len(result.Results) != 1 {
			t.Fatalf("len(Results) = %d, want 1", len(result.Results))
		}
		if result.Results[0].Title != "Result 1" {
			t.Errorf("Results[0].Title = %q, want %q", result.Results[0].Title, "Result 1")
		}
	})

	t.Run("search with all parameters", func(t *testing.T) {
		srv, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			if got := r.URL.Query().Get("q"); got != "test" {
				t.Errorf("q = %q, want %q", got, "test")
			}
			if got := r.URL.Query().Get("language"); got != "en-US" {
				t.Errorf("language = %q, want %q", got, "en-US")
			}
			if got := r.URL.Query().Get("pageno"); got != "2" {
				t.Errorf("pageno = %q, want %q", got, "2")
			}
			if got := r.URL.Query().Get("time_range"); got != "month" {
				t.Errorf("time_range = %q, want %q", got, "month")
			}
			if got := r.URL.Query().Get("safesearch"); got != "1" {
				t.Errorf("safesearch = %q, want %q", got, "1")
			}
			if got := r.URL.Query().Get("format"); got != "json" {
				t.Errorf("format = %q, want %q", got, "json")
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"query":   "test",
				"results": []any{},
			})
		})
		defer srv.Close()

		_, err := client.Search(context.Background(), domain.SearchParams{
			Query:      "test",
			Language:   "en-US",
			Page:       2,
			TimeRange:  "month",
			SafeSearch: 1,
		})
		if err != nil {
			t.Fatalf("Search() returned error: %v", err)
		}
	})

	t.Run("search with categories", func(t *testing.T) {
		srv, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			cats := r.URL.Query()["categories"]
			if len(cats) != 2 || cats[0] != "general" || cats[1] != "news" {
				t.Errorf("categories = %v, want [general news]", cats)
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"query":   "test",
				"results": []any{},
			})
		})
		defer srv.Close()

		_, err := client.Search(context.Background(), domain.SearchParams{
			Query:      "test",
			Categories: []string{"general", "news"},
		})
		if err != nil {
			t.Fatalf("Search() returned error: %v", err)
		}
	})

	t.Run("HTTP error returns API error", func(t *testing.T) {
		srv, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Forbidden", http.StatusForbidden)
		})
		defer srv.Close()

		_, err := client.Search(context.Background(), domain.SearchParams{
			Query: "test",
		})
		if err == nil {
			t.Fatal("Search() expected error, got nil")
		}
	})

	t.Run("empty response returns empty result", func(t *testing.T) {
		srv, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"query":             "test",
				"results":           []any{},
				"number_of_results": 0,
			})
		})
		defer srv.Close()

		result, err := client.Search(context.Background(), domain.SearchParams{
			Query: "test",
		})
		if err != nil {
			t.Fatalf("Search() returned error: %v", err)
		}
		if len(result.Results) != 0 {
			t.Errorf("len(Results) = %d, want 0", len(result.Results))
		}
	})

	t.Run("search with answers and suggestions", func(t *testing.T) {
		srv, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"query":       "test",
				"results":     []any{},
				"answers":     []string{"42 is the answer"},
				"suggestions": []string{"did you mean test2"},
			})
		})
		defer srv.Close()

		result, err := client.Search(context.Background(), domain.SearchParams{
			Query: "test",
		})
		if err != nil {
			t.Fatalf("Search() returned error: %v", err)
		}
		if len(result.Answers) != 1 || string(result.Answers[0]) != "42 is the answer" {
			t.Errorf("Answers = %v, want [42 is the answer]", result.Answers)
		}
		if len(result.Suggestions) != 1 || string(result.Suggestions[0]) != "did you mean test2" {
			t.Errorf("Suggestions = %v, want [did you mean test2]", result.Suggestions)
		}
	})

	t.Run("search with infoboxes", func(t *testing.T) {
		srv, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"query":   "test",
				"results": []any{},
				"infoboxes": []map[string]any{
					{
						"id":      "infobox_1",
						"content": "Infobox content",
					},
				},
			})
		})
		defer srv.Close()

		result, err := client.Search(context.Background(), domain.SearchParams{
			Query: "test",
		})
		if err != nil {
			t.Fatalf("Search() returned error: %v", err)
		}
		if len(result.Infoboxes) != 1 {
			t.Fatalf("len(Infoboxes) = %d, want 1", len(result.Infoboxes))
		}
		if result.Infoboxes[0].ID != "infobox_1" {
			t.Errorf("Infoboxes[0].ID = %q, want %q", result.Infoboxes[0].ID, "infobox_1")
		}
	})
}
