package searxng

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/teran/mcp-searxng/domain"
)

var ErrAPIClient = errors.New("API error")

// Client is the SearXNG HTTP client implementing domain.SearchRepository.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new SearXNG API client with the given HTTP client.
// The caller should provide an *http.Client with CheckRedirect set to prevent
// credential forwarding, and a shared Transport for connection reuse.
func NewClient(baseURL string, httpClient *http.Client) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: httpClient,
	}
}

// Search implements domain.SearchRepository.
func (c *Client) Search(ctx context.Context, params domain.SearchParams) (*domain.SearchResponse, error) {
	q := buildSearchQuery(params)

	body, err := c.doRequest(ctx, "/search", q)
	if err != nil {
		return nil, err
	}

	var raw rawSearchResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal search response: %w", err)
	}

	result := raw.toDomain()
	return &result, nil
}

// GetEngines implements domain.SearchRepository.
func (c *Client) GetEngines(ctx context.Context) ([]domain.EngineInfo, error) {
	// Try /stats first, fall back to /api/stats.
	body, err := c.doRequest(ctx, "/stats", url.Values{"format": {"json"}})
	if err != nil {
		body, err = c.doRequest(ctx, "/api/stats", url.Values{"format": {"json"}})
		if err != nil {
			return nil, fmt.Errorf("get engines: %w", err)
		}
	}

	var raw rawStatsResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal stats response: %w", err)
	}

	return raw.toDomain(), nil
}

// -- private HTTP helpers --

func (c *Client) doRequest(ctx context.Context, path string, query url.Values) ([]byte, error) {
	u, err := url.JoinPath(c.baseURL, path)
	if err != nil {
		return nil, fmt.Errorf("build URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if query != nil {
		req.URL.RawQuery = query.Encode()
	}

	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	// Limit response body to 10 MB to prevent memory exhaustion.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail := extractErrorDetail(body, resp.StatusCode)
		return nil, fmt.Errorf("API status=%d: %s: %w", resp.StatusCode, detail, ErrAPIClient)
	}

	return body, nil
}

// extractErrorDetail extracts a human-readable detail from an error response body.
func extractErrorDetail(body []byte, statusCode int) string {
	if len(body) == 0 {
		return http.StatusText(statusCode)
	}

	// Try JSON {"detail":"..."} first.
	var errResp struct {
		Detail string `json:"detail"`
	}
	if json.Unmarshal(body, &errResp) == nil && errResp.Detail != "" {
		return errResp.Detail
	}

	// Try message field.
	var msgResp struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &msgResp) == nil && msgResp.Message != "" {
		return msgResp.Message
	}

	// Fallback: take first line, sanitize.
	detail := string(body)
	if idx := strings.IndexAny(detail, "\n\r"); idx >= 0 {
		detail = detail[:idx]
	}
	if len(detail) > 512 {
		detail = detail[:512] + "..."
	}
	return detail
}

func buildSearchQuery(params domain.SearchParams) url.Values {
	q := url.Values{}
	if params.Query != "" {
		q.Set("q", params.Query)
	}
	for _, cat := range params.Categories {
		q.Add("categories", cat)
	}
	if params.Language != "" {
		q.Set("language", params.Language)
	}
	if params.Page > 0 {
		q.Set("pageno", strconv.Itoa(params.Page))
	}
	if params.TimeRange != "" {
		q.Set("time_range", params.TimeRange)
	}
	if params.SafeSearch > 0 {
		q.Set("safesearch", strconv.Itoa(params.SafeSearch))
	}
	q.Set("format", "json")

	return q
}
