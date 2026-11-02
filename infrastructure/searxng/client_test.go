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

	"resty.dev/v3"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/teran/mcp-searxng/domain"
	infra "github.com/teran/mcp-searxng/infrastructure/searxng"
	"github.com/teran/mcp-searxng/requestid"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *infra.Client) {
	t.Helper()
	srv := httptest.NewServer(handler)
	client := infra.NewClient(srv.URL, resty.New())

	return srv, client
}

// TestClient_ForwardsRequestID verifies L09/L04GO: when the context carries a
// request_id, the outbound request to SearXNG includes it as X-Request-ID so
// upstream logs can be correlated with the server's access log.
func TestClient_ForwardsRequestID(t *testing.T) {
	t.Parallel()

	var gotHeader string
	srv, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Request-ID")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"query":"x","results":[],"number_of_results":0}`))
	})
	defer srv.Close()

	ctx := requestid.With(context.Background(), "req-abc123")
	if _, err := client.Search(ctx, domain.SearchParams{Query: "x"}); err != nil {
		t.Fatalf("Search: %v", err)
	}

	if gotHeader != "req-abc123" {
		t.Errorf("X-Request-ID = %q, want req-abc123", gotHeader)
	}
}

// TestClient_NoRequestID_OmitsHeader verifies no X-Request-ID is sent when the
// context carries none.
func TestClient_NoRequestID_OmitsHeader(t *testing.T) {
	t.Parallel()

	var gotHeader string
	srv, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Request-ID")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"query":"x","results":[],"number_of_results":0}`))
	})
	defer srv.Close()

	if _, err := client.Search(context.Background(), domain.SearchParams{Query: "x"}); err != nil {
		t.Fatalf("Search: %v", err)
	}

	if gotHeader != "" {
		t.Errorf("X-Request-ID = %q, want empty", gotHeader)
	}
}

// TestClient_UpstreamMetrics verifies O03: a successful upstream call records
// the request counter under the 2xx status class.
func TestClient_UpstreamMetrics(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	srv, client := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"query":"x","results":[],"number_of_results":0}`))
	})
	defer srv.Close()

	client.SetMetrics(infra.NewUpstreamMetrics(reg))
	if _, err := client.Search(context.Background(), domain.SearchParams{Query: "x"}); err != nil {
		t.Fatalf("Search: %v", err)
	}

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	seen := false
	for _, mf := range families {
		if mf.GetName() != "mcp_searxng_upstream_requests_total" {
			continue
		}
		for _, m := range mf.GetMetric() {
			class := m.GetLabel()[0].GetValue()
			if class == "2xx" && m.GetCounter().GetValue() == 1 {
				seen = true
			}
		}
	}
	if !seen {
		t.Error("expected mcp_searxng_upstream_requests_total{status_class=2xx} == 1")
	}
}

// TestClient_UpstreamMetricsServerError verifies the upstream counter records a
// 5xx status class when SearXNG returns a server error.
func TestClient_UpstreamMetricsServerError(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	srv, client := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	defer srv.Close()

	client.SetMetrics(infra.NewUpstreamMetrics(reg))
	if _, err := client.Search(context.Background(), domain.SearchParams{Query: "x"}); err == nil {
		t.Fatal("expected an error from a 500 upstream response")
	}

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	seen := false
	for _, mf := range families {
		if mf.GetName() != "mcp_searxng_upstream_requests_total" {
			continue
		}
		for _, m := range mf.GetMetric() {
			if m.GetLabel()[0].GetValue() == "5xx" && m.GetCounter().GetValue() == 1 {
				seen = true
			}
		}
	}
	if !seen {
		t.Error("expected mcp_searxng_upstream_requests_total{status_class=5xx} == 1")
	}
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

	t.Run("empty response returns empty result", testClientSearchHappyEmpty)
	t.Run("search with answers and suggestions", testClientSearchHappyAnswersSuggestions)
	t.Run("search with infoboxes", testClientSearchHappyInfoboxes)
	t.Run("search with empty params uses default format", testClientSearchHappyEmptyParams)
}

func testClientSearchHappyEmpty(t *testing.T) {
	srv, client := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
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
}

func testClientSearchHappyAnswersSuggestions(t *testing.T) {
	srv, client := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
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
}

func testClientSearchHappyInfoboxes(t *testing.T) {
	srv, client := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
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
}

func testClientSearchHappyEmptyParams(t *testing.T) {
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
}

// -- HTTP error path - basic errors -----------------------------------------

func TestClient_Search_HTTPErrors_Basic(t *testing.T) {
	t.Parallel()

	t.Run("HTTP 403 returns API error", func(t *testing.T) {
		srv, client := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
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
		srv, client := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
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
		srv, client := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
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

	t.Run("HTTP 500 with plain text body", testClientSearchErr500PlainText)
	t.Run("HTTP 404 with empty body", testClientSearchErr404Empty)
	t.Run("HTTP 500 with plain text body exceeding 512 chars", testClientSearchErr500LongBody)
	t.Run("HTTP 500 with body containing newlines", testClientSearchErr500Newlines)
}

func testClientSearchErr500PlainText(t *testing.T) {
	srv, client := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
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
}

func testClientSearchErr404Empty(t *testing.T) {
	srv, client := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
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
}

func testClientSearchErr500LongBody(t *testing.T) {
	srv, client := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
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
}

func testClientSearchErr500Newlines(t *testing.T) {
	srv, client := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
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
}

// -- network & IO error path tests ------------------------------------------

func TestClient_Search_NetworkErrors(t *testing.T) {
	t.Parallel()

	t.Run("network error / timeout", func(t *testing.T) {
		expectedErr := errors.New("connection refused")
		brokenClient := infra.NewClient("http://127.0.0.1:1", resty.NewWithClient(&http.Client{
			Transport: errTransport{err: expectedErr},
		}))

		_, err := brokenClient.Search(context.Background(), domain.SearchParams{Query: "test"})
		if err == nil {
			t.Fatal("Search() expected error for network failure, got nil")
		}
		if !strings.Contains(err.Error(), "execute request") {
			t.Errorf("error = %q, want substring %q", err.Error(), "execute request")
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		srv, client := newTestServer(t, func(_ http.ResponseWriter, r *http.Request) {
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
		srv, client := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
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
		srv, client := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
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

	srv, client := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
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
	infobox := result.Infoboxes[0]

	if len(infobox.URLs) != 2 {
		t.Fatalf("len(URLs) = %d, want 2", len(infobox.URLs))
	}
	if infobox.URLs[0].Title != "Link 1" || infobox.URLs[0].URL != "https://example.com/1" {
		t.Errorf("URLs[0] = %+v, want {Link 1 https://example.com/1}", infobox.URLs[0])
	}
	if infobox.URLs[1].Title != "Link 2" || infobox.URLs[1].URL != "https://example.com/2" {
		t.Errorf("URLs[1] = %+v, want {Link 2 https://example.com/2}", infobox.URLs[1])
	}
}

func TestToDomain_InfoboxAttributes(t *testing.T) {
	t.Parallel()

	srv, client := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
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
	infobox := result.Infoboxes[0]

	if len(infobox.Attributes) != 2 {
		t.Fatalf("len(Attributes) = %d, want 2", len(infobox.Attributes))
	}
	if infobox.Attributes[0].Key != "Population" || infobox.Attributes[0].Value != "8M" {
		t.Errorf("Attributes[0] = %+v, want {Population 8M}", infobox.Attributes[0])
	}
	if infobox.Attributes[1].Key != "Area" || infobox.Attributes[1].Value != "2,000 km²" {
		t.Errorf("Attributes[1] = %+v, want {Area 2,000 km²}", infobox.Attributes[1])
	}
}

func TestToDomain_InfoboxDetails(t *testing.T) {
	t.Parallel()

	srv, client := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
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
	infobox := result.Infoboxes[0]

	if len(infobox.Infoboxes) != 1 {
		t.Fatalf("len(Infoboxes[0].Infoboxes) = %d, want 1", len(infobox.Infoboxes))
	}
	det := infobox.Infoboxes[0]
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
