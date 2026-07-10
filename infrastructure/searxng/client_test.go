package searxng_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

// errTransport is a round tripper that always returns the given error.
type errTransport struct {
	err error
}

func (t errTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, t.err
}

// -- happy path - basic -----------------------------------------------------

func TestClient_Search_BasicSearch(t *testing.T) {
	t.Parallel()

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
}

func TestClient_Search_AllParameters(t *testing.T) {
	t.Parallel()

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
}

func TestClient_Search_Categories(t *testing.T) {
	t.Parallel()

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
}

// -- happy path - edge cases ------------------------------------------------

func TestClient_Search_HappyPath_EdgeCases(t *testing.T) {
	t.Parallel()

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

	t.Run("search with empty params uses default format", func(t *testing.T) {
		srv, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			if got := r.URL.Query().Get("format"); got != "json" {
				t.Errorf("format = %q, want %q", got, "json")
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"query": "", "results": []any{}})
		})
		defer srv.Close()

		_, err := client.Search(context.Background(), domain.SearchParams{})
		if err != nil {
			t.Fatalf("Search() returned error: %v", err)
		}
	})
}

// -- HTTP error path - basic errors -----------------------------------------

func TestClient_Search_HTTPErrors_Basic(t *testing.T) {
	t.Parallel()

	t.Run("HTTP 403 returns API error", func(t *testing.T) {
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

	t.Run("HTTP 500 with JSON detail field", func(t *testing.T) {
		srv, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"detail":"internal server error detail"}`))
		})
		defer srv.Close()

		_, err := client.Search(context.Background(), domain.SearchParams{Query: "test"})
		if err == nil {
			t.Fatal("Search() expected error for HTTP 500, got nil")
		}
		if !errors.Is(err, infra.ErrAPIClient) {
			t.Errorf("error should wrap ErrAPIClient")
		}
		if !strings.Contains(err.Error(), "internal server error detail") {
			t.Errorf("error = %q, want substring %q", err.Error(), "internal server error detail")
		}
	})

	t.Run("HTTP 500 with JSON message field", func(t *testing.T) {
		srv, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"message":"something went wrong"}`))
		})
		defer srv.Close()

		_, err := client.Search(context.Background(), domain.SearchParams{Query: "test"})
		if err == nil {
			t.Fatal("Search() expected error for HTTP 500, got nil")
		}
		if !errors.Is(err, infra.ErrAPIClient) {
			t.Errorf("error should wrap ErrAPIClient")
		}
		if !strings.Contains(err.Error(), "something went wrong") {
			t.Errorf("error = %q, want substring %q", err.Error(), "something went wrong")
		}
	})
}

// -- HTTP error path - extended errors --------------------------------------

func TestClient_Search_HTTPErrors_Extended(t *testing.T) {
	t.Parallel()

	t.Run("HTTP 500 with plain text body", func(t *testing.T) {
		srv, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("Internal Server Error"))
		})
		defer srv.Close()

		_, err := client.Search(context.Background(), domain.SearchParams{Query: "test"})
		if err == nil {
			t.Fatal("Search() expected error for HTTP 500, got nil")
		}
		if !errors.Is(err, infra.ErrAPIClient) {
			t.Errorf("error should wrap ErrAPIClient")
		}
		if !strings.Contains(err.Error(), "Internal Server Error") {
			t.Errorf("error = %q, want substring %q", err.Error(), "Internal Server Error")
		}
	})

	t.Run("HTTP 404 with empty body", func(t *testing.T) {
		srv, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})
		defer srv.Close()

		_, err := client.Search(context.Background(), domain.SearchParams{Query: "test"})
		if err == nil {
			t.Fatal("Search() expected error for HTTP 404, got nil")
		}
		if !errors.Is(err, infra.ErrAPIClient) {
			t.Errorf("error should wrap ErrAPIClient")
		}
		if !strings.Contains(err.Error(), "Not Found") {
			t.Errorf("error = %q, want substring %q", err.Error(), "Not Found")
		}
	})

	t.Run("HTTP 500 with plain text body exceeding 512 chars", func(t *testing.T) {
		srv, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Header().Set("Content-Type", "text/plain")
			longBody := strings.Repeat("A", 600)
			_, _ = w.Write([]byte(longBody))
		})
		defer srv.Close()

		_, err := client.Search(context.Background(), domain.SearchParams{Query: "test"})
		if err == nil {
			t.Fatal("Search() expected error for long body, got nil")
		}
		// Body should be truncated to 512 chars (error wraps body with ~40 chars of prefix/suffix).
		if len(err.Error()) > 570 {
			t.Errorf("error length = %d, want <= 570 (truncated)", len(err.Error()))
		}
	})

	t.Run("HTTP 500 with body containing newlines", func(t *testing.T) {
		srv, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("line1\nline2\nline3"))
		})
		defer srv.Close()

		_, err := client.Search(context.Background(), domain.SearchParams{Query: "test"})
		if err == nil {
			t.Fatal("Search() expected error, got nil")
		}
		if strings.Contains(err.Error(), "line2") || strings.Contains(err.Error(), "line3") {
			t.Errorf("error should contain only first line, got = %q", err.Error())
		}
		if !strings.Contains(err.Error(), "line1") {
			t.Errorf("error should contain first line, got = %q", err.Error())
		}
	})
}

// -- network & IO error path tests ------------------------------------------

func TestClient_Search_NetworkErrors(t *testing.T) {
	t.Parallel()

	t.Run("network error / timeout", func(t *testing.T) {
		expectedErr := errors.New("connection refused")
		brokenClient := infra.NewClient("http://127.0.0.1:1", &http.Client{
			Transport: errTransport{err: expectedErr},
		})

		_, err := brokenClient.Search(context.Background(), domain.SearchParams{Query: "test"})
		if err == nil {
			t.Fatal("Search() expected error for network failure, got nil")
		}
		if !strings.Contains(err.Error(), "execute request") {
			t.Errorf("error = %q, want substring %q", err.Error(), "execute request")
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		srv, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		})
		defer srv.Close()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := client.Search(ctx, domain.SearchParams{Query: "test"})
		if err == nil {
			t.Fatal("Search() expected error for cancelled context, got nil")
		}
	})
}

// -- response parsing error paths -------------------------------------------

func TestClient_Search_ParseErrors(t *testing.T) {
	t.Parallel()

	t.Run("malformed JSON response", func(t *testing.T) {
		srv, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{broken`))
		})
		defer srv.Close()

		_, err := client.Search(context.Background(), domain.SearchParams{Query: "test"})
		if err == nil {
			t.Fatal("Search() expected error for malformed JSON, got nil")
		}
		if !strings.Contains(err.Error(), "unmarshal search response") {
			t.Errorf("error = %q, want substring %q", err.Error(), "unmarshal search response")
		}
	})

	t.Run("response body larger than 10MB", func(t *testing.T) {
		srv, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			largeStr := string(bytes.Repeat([]byte("x"), 12<<20)) // 12 MB
			_, _ = fmt.Fprintf(w, `{"query":"test","results":[],"padding":%q}`, largeStr)
		})
		defer srv.Close()

		_, err := client.Search(context.Background(), domain.SearchParams{Query: "test"})
		if err == nil {
			t.Fatal("Search() expected error for truncated JSON, got nil")
		}
		if !strings.Contains(err.Error(), "unmarshal search response") {
			t.Errorf("error = %q, want substring %q", err.Error(), "unmarshal search response")
		}
	})
}

// -- toDomain infobox sub-type tests ----------------------------------------

func TestToDomain_InfoboxURLs(t *testing.T) {
	t.Parallel()

	srv, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"query":   "test",
			"results": []any{},
			"infoboxes": []map[string]any{
				{
					"id": "ib1",
					"urls": []map[string]any{
						{"title": "Link 1", "url": "https://example.com/1"},
						{"title": "Link 2", "url": "https://example.com/2"},
					},
				},
			},
		})
	})
	defer srv.Close()

	result, err := client.Search(context.Background(), domain.SearchParams{Query: "test"})
	if err != nil {
		t.Fatalf("Search() returned error: %v", err)
	}
	if len(result.Infoboxes) != 1 {
		t.Fatalf("len(Infoboxes) = %d, want 1", len(result.Infoboxes))
	}
	ib := result.Infoboxes[0]

	if len(ib.URLs) != 2 {
		t.Fatalf("len(URLs) = %d, want 2", len(ib.URLs))
	}
	if ib.URLs[0].Title != "Link 1" || ib.URLs[0].URL != "https://example.com/1" {
		t.Errorf("URLs[0] = %+v, want {Link 1 https://example.com/1}", ib.URLs[0])
	}
	if ib.URLs[1].Title != "Link 2" || ib.URLs[1].URL != "https://example.com/2" {
		t.Errorf("URLs[1] = %+v, want {Link 2 https://example.com/2}", ib.URLs[1])
	}
}

func TestToDomain_InfoboxAttributes(t *testing.T) {
	t.Parallel()

	srv, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"query":   "test",
			"results": []any{},
			"infoboxes": []map[string]any{
				{
					"id": "ib1",
					"attributes": []map[string]any{
						{"key": "Population", "value": "8M"},
						{"key": "Area", "value": "2,000 km²"},
					},
				},
			},
		})
	})
	defer srv.Close()

	result, err := client.Search(context.Background(), domain.SearchParams{Query: "test"})
	if err != nil {
		t.Fatalf("Search() returned error: %v", err)
	}
	if len(result.Infoboxes) != 1 {
		t.Fatalf("len(Infoboxes) = %d, want 1", len(result.Infoboxes))
	}
	ib := result.Infoboxes[0]

	if len(ib.Attributes) != 2 {
		t.Fatalf("len(Attributes) = %d, want 2", len(ib.Attributes))
	}
	if ib.Attributes[0].Key != "Population" || ib.Attributes[0].Value != "8M" {
		t.Errorf("Attributes[0] = %+v, want {Population 8M}", ib.Attributes[0])
	}
	if ib.Attributes[1].Key != "Area" || ib.Attributes[1].Value != "2,000 km²" {
		t.Errorf("Attributes[1] = %+v, want {Area 2,000 km²}", ib.Attributes[1])
	}
}

func TestToDomain_InfoboxDetails(t *testing.T) {
	t.Parallel()

	srv, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"query":   "test",
			"results": []any{},
			"infoboxes": []map[string]any{
				{
					"id": "ib1",
					"infoboxes": []map[string]any{
						{
							"id":      "detail_1",
							"content": "Detail content",
							"urls": []map[string]any{
								{"title": "Detail Link", "url": "https://example.com/detail"},
							},
							"attributes": []map[string]any{
								{"key": "Color", "value": "Blue"},
							},
						},
					},
				},
			},
		})
	})
	defer srv.Close()

	result, err := client.Search(context.Background(), domain.SearchParams{Query: "test"})
	if err != nil {
		t.Fatalf("Search() returned error: %v", err)
	}
	if len(result.Infoboxes) != 1 {
		t.Fatalf("len(Infoboxes) = %d, want 1", len(result.Infoboxes))
	}
	ib := result.Infoboxes[0]

	if len(ib.Infoboxes) != 1 {
		t.Fatalf("len(Infoboxes[0].Infoboxes) = %d, want 1", len(ib.Infoboxes))
	}
	det := ib.Infoboxes[0]
	if det.ID != "detail_1" {
		t.Errorf("InfoboxDetail.ID = %q, want %q", det.ID, "detail_1")
	}
	if det.Content != "Detail content" {
		t.Errorf("InfoboxDetail.Content = %q, want %q", det.Content, "Detail content")
	}
	if len(det.URLs) != 1 || det.URLs[0].Title != "Detail Link" {
		t.Errorf("InfoboxDetail.URLs = %+v, want [{Detail Link https://example.com/detail}]", det.URLs)
	}
	if len(det.Attributes) != 1 || det.Attributes[0].Key != "Color" {
		t.Errorf("InfoboxDetail.Attributes = %+v, want [{Color Blue}]", det.Attributes)
	}
}
