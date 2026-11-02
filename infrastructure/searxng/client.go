// Package searxng implements the SearXNG HTTP API client (infrastructure
// adapter for the domain.SearchRepository port).
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

	"resty.dev/v3"

	"github.com/teran/mcp-searxng/domain"
	"github.com/teran/mcp-searxng/requestid"
)

// maxResponseBodySize caps the size of a SearXNG response body read by the
// client to prevent memory exhaustion.
const maxResponseBodySize = 10 << 20 // 10 MB

// maxErrorDetailSize caps the size of an error body excerpt used for detail.
const maxErrorDetailSize = 512

// ErrAPIClient is returned when SearXNG responds with a non-2xx status.
var ErrAPIClient = errors.New("API error")

// Client is the SearXNG HTTP client implementing domain.SearchRepository.
type Client struct {
	baseURL string
	client  *resty.Client
}

// NewClient creates a new SearXNG API client with the given resty client.
// The caller should configure the resty client (base URL, timeout, redirect
// policy, transport) before passing it in. Resty performs no retries by
// default, so outbound errors are returned to the model rather than silently
// retried.
func NewClient(baseURL string, client *resty.Client) *Client {
	return &Client{
		baseURL: baseURL,
		client:  client,
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

// -- private HTTP helpers --

func (c *Client) doRequest(ctx context.Context, path string, query url.Values) ([]byte, error) {
	fullURL, err := url.JoinPath(c.baseURL, path)
	if err != nil {
		return nil, fmt.Errorf("build URL: %w", err)
	}

	req := c.client.R().
		SetContext(ctx).
		SetHeader("Accept", "application/json").
		// Do not let resty buffer the whole response; we read the raw stream
		// below under a 10 MB cap to prevent memory exhaustion.
		SetResponseDoNotParse(true)

	// Forward the correlation request_id as X-Request-ID (L09/L04GO) so the
	// upstream SearXNG logs can be correlated with this server's access log.
	if id := requestid.From(ctx); id != "" {
		req = req.SetHeader("X-Request-ID", id)
	}

	if query != nil {
		req = req.SetQueryParamsFromValues(query)
	}

	resp, err := req.Get(fullURL)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	// Limit response body to 10 MB to prevent memory exhaustion.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		detail := extractErrorDetail(body, resp.StatusCode())

		return nil, fmt.Errorf("API status=%d: %s: %w", resp.StatusCode(), detail, ErrAPIClient)
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

	// Fallback: take first line. Truncate before string conversion to avoid
	// double-allocation of large error bodies (up to 10 MB).
	if len(body) > maxErrorDetailSize {
		body = body[:maxErrorDetailSize]
	}
	detail := string(body)
	if idx := strings.IndexAny(detail, "\n\r"); idx >= 0 {
		detail = detail[:idx]
	}

	return detail
}

func buildSearchQuery(params domain.SearchParams) url.Values {
	query := url.Values{}
	if params.Query != "" {
		query.Set("q", params.Query)
	}
	for _, cat := range params.Categories {
		query.Add("categories", cat)
	}
	if params.Language != "" {
		query.Set("language", params.Language)
	}
	if params.Page > 0 {
		query.Set("pageno", strconv.Itoa(params.Page))
	}
	if params.TimeRange != "" {
		query.Set("time_range", params.TimeRange)
	}
	if params.SafeSearch > 0 {
		query.Set("safesearch", strconv.Itoa(params.SafeSearch))
	}
	query.Set("format", "json")

	return query
}
