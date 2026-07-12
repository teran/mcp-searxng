package handlers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/teran/mcp-searxng/application"
	"github.com/teran/mcp-searxng/domain"
)

// Sentinel errors returned to the MCP client with user-friendly messages.
// Detailed internal errors are logged server-side via log.Printf.
var (
	ErrSearchFailed = errors.New("search failed")
	ErrQueryTooLong = errors.New("query exceeds maximum length")
	ErrPageTooLarge = errors.New("page number exceeds maximum")
	ErrInvalidQuery = errors.New("query contains invalid characters")
)

const (
	// MaxQueryLength is the maximum allowed length for a search query.
	MaxQueryLength = 512
	// MaxPageNumber is the maximum allowed page number for pagination.
	MaxPageNumber = 100
)

// ============================================================
// Input / output types
// ============================================================

// --- search ---

type SearchInput struct {
	Query      string   `json:"query" jsonschema:"the search query,required"`
	Categories []string `json:"categories,omitempty" jsonschema:"comma separated list of active search categories"`
	Language   string   `json:"language,omitempty" jsonschema:"code of the language (e.g. en-US, de-DE)"`
	Page       int      `json:"page,omitempty" jsonschema:"search page number,default=1"`
	TimeRange  string   `json:"time_range,omitempty" jsonschema:"time range of search: day, month, year"`
	SafeSearch int      `json:"safesearch,omitempty" jsonschema:"filter search results: 0=off, 1=moderate, 2=strict"`
}

type SearchResultItem struct {
	Title         string  `json:"title"`
	URL           string  `json:"url"`
	Content       string  `json:"content"`
	Engine        string  `json:"engine"`
	Category      string  `json:"category,omitempty"`
	PublishedDate *string `json:"publishedDate,omitempty"`
	FormattedDate string  `json:"formattedDate,omitempty"`
	ImgSrc        *string `json:"img_src,omitempty"`
	Source        *string `json:"source,omitempty"`
}

type InfoboxAttributeItem struct {
	Key   string `json:"key,omitempty"`
	Value string `json:"value,omitempty"`
}

type InfoboxURLItem struct {
	Title string `json:"title,omitempty"`
	URL   string `json:"url,omitempty"`
}

type InfoboxItem struct {
	ID         string                 `json:"id,omitempty"`
	URL        string                 `json:"url,omitempty"`
	Content    string                 `json:"content,omitempty"`
	Attributes []InfoboxAttributeItem `json:"attributes,omitempty"`
	URLs       []InfoboxURLItem       `json:"urls,omitempty"`
	ImgSrc     *string                `json:"img_src,omitempty"`
	Engine     string                 `json:"engine,omitempty"`
}

type SearchOutput struct {
	Query           string             `json:"query"`
	Results         []SearchResultItem `json:"results"`
	Answers         []string           `json:"answers,omitempty"`
	Suggestions     []string           `json:"suggestions,omitempty"`
	Infoboxes       []InfoboxItem      `json:"infoboxes,omitempty"`
	NumberOfResults int                `json:"number_of_results"`
}

// --- search_news ---

type SearchNewsInput struct {
	Query      string `json:"query" jsonschema:"the search query,required"`
	Language   string `json:"language,omitempty" jsonschema:"code of the language (e.g. en-US, de-DE)"`
	Page       int    `json:"page,omitempty" jsonschema:"search page number,default=1"`
	SafeSearch int    `json:"safesearch,omitempty" jsonschema:"filter search results: 0=off, 1=moderate, 2=strict"`
}

// --- search_images ---

type SearchImagesInput struct {
	Query      string `json:"query" jsonschema:"the search query,required"`
	Page       int    `json:"page,omitempty" jsonschema:"search page number,default=1"`
	SafeSearch int    `json:"safesearch,omitempty" jsonschema:"filter search results: 0=off, 1=moderate, 2=strict"`
	Language   string `json:"language,omitempty" jsonschema:"code of the language (e.g. en-US, de-DE)"`
}

// formatDate converts an ISO 8601 date string to a readable format (e.g. "10 Jul 2026").
// Returns empty string if the input is nil.
func formatDate(d *string) string {
	if d == nil || *d == "" {
		return ""
	}

	t, err := time.Parse(time.RFC3339, *d)
	if err != nil {
		// Try other common ISO 8601 formats.
		t, err = time.Parse("2006-01-02T15:04:05", *d)
		if err != nil {
			t, err = time.Parse("2006-01-02", *d)
			if err != nil {
				return ""
			}
		}
	}

	return t.Format("2 Jan 2006")
}

// sanitizeOutput strips control characters and Unicode bidi formatting characters
// from strings returned to the LLM. This prevents indirect prompt injection via
// malicious web content embedded in search results.
//
// Preserved: tabs (\t), newlines (\n), carriage returns (\r) — basic formatting.
// Stripped: control chars (0x00-0x08, 0x0b-0x1f, 0x7f), bidi override chars.
func sanitizeOutput(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' {
			return r
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		// Remove Unicode directionality formatting characters to prevent
		// bidi spoofing and hidden prompt injection payloads.
		if r >= 0x200e && r <= 0x200f { // LRM, RLM
			return -1
		}
		if r >= 0x202a && r <= 0x202e { // LRE, RLE, PDF, LRO, RLO
			return -1
		}
		if r >= 0x2066 && r <= 0x2069 { // LRI, RLI, FSI, PDI
			return -1
		}
		return r
	}, s)
}

// sanitizeURL validates and sanitizes a URL from search results.
// Only http and https schemes are allowed. Returns the sanitized URL
// or an empty string if the URL is invalid or uses a disallowed scheme.
func sanitizeURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}

	return u.String()
}

// sanitizeOutputPtr applies sanitizeOutput to a *string, returning nil on empty.
func sanitizeOutputPtr(s *string) *string {
	if s == nil {
		return nil
	}
	sanitized := sanitizeOutput(*s)
	return &sanitized
}

// sanitizeImgSrc validates an image source URL, allowing only http/https/data schemes
// (data URIs are typically generated by search engines for thumbnails).
func sanitizeImgSrc(s *string) *string {
	if s == nil {
		return nil
	}

	u, err := url.Parse(*s)
	if err != nil {
		return nil
	}

	if u.Scheme == "" || u.Scheme == "http" || u.Scheme == "https" || u.Scheme == "data" {
		return s
	}

	return nil
}

// searchHelper performs a search and converts the result to SearchOutput.
func searchHelper(ctx context.Context, svc *application.SearchService, params domain.SearchParams) (SearchOutput, error) {
	page := params.Page
	if page <= 0 {
		page = 1
	}
	params.Page = page

	result, err := svc.Search(ctx, params)
	if err != nil {
		return SearchOutput{}, err
	}

	items := make([]SearchResultItem, 0, len(result.Results))
	for _, r := range result.Results {
		items = append(items, SearchResultItem{
			Title:         sanitizeOutput(r.Title),
			URL:           sanitizeURL(r.URL),
			Content:       sanitizeOutput(r.Content),
			Engine:        sanitizeOutput(r.Engine),
			Category:      sanitizeOutput(r.Category),
			PublishedDate: r.PublishedDate,
			FormattedDate: formatDate(r.PublishedDate),
			ImgSrc:        sanitizeImgSrc(r.ImgSrc),
			Source:        sanitizeOutputPtr(r.Source),
		})
	}

	answers := make([]string, 0, len(result.Answers))
	for _, a := range result.Answers {
		answers = append(answers, sanitizeOutput(string(a)))
	}

	suggestions := make([]string, 0, len(result.Suggestions))
	for _, s := range result.Suggestions {
		suggestions = append(suggestions, sanitizeOutput(string(s)))
	}

	infoboxes := make([]InfoboxItem, 0, len(result.Infoboxes))
	for _, ib := range result.Infoboxes {
		attrs := make([]InfoboxAttributeItem, 0, len(ib.Attributes))
		for _, a := range ib.Attributes {
			attrs = append(attrs, InfoboxAttributeItem{
				Key:   sanitizeOutput(a.Key),
				Value: sanitizeOutput(a.Value),
			})
		}

		urls := make([]InfoboxURLItem, 0, len(ib.URLs))
		for _, u := range ib.URLs {
			urls = append(urls, InfoboxURLItem{
				Title: sanitizeOutput(u.Title),
				URL:   sanitizeURL(u.URL),
			})
		}

		infoboxes = append(infoboxes, InfoboxItem{
			ID:         sanitizeOutput(ib.ID),
			URL:        sanitizeURL(ib.URL),
			Content:    sanitizeOutput(ib.Content),
			Attributes: attrs,
			URLs:       urls,
			ImgSrc:     sanitizeImgSrc(ib.ImgSrc),
			Engine:     sanitizeOutput(ib.Engine),
		})
	}

	return SearchOutput{
		Query:           sanitizeOutput(result.Query),
		Results:         items,
		Answers:         answers,
		Suggestions:     suggestions,
		Infoboxes:       infoboxes,
		NumberOfResults: result.NumberOfResults,
	}, nil
}

// ============================================================
// Tool handler factories
// ============================================================

// validateQuery validates the search query parameter.
// Returns an error if the query is empty, too long, or contains invalid characters.
func validateQuery(q string) error {
	if q == "" {
		return ErrInvalidQuery
	}
	if len(q) > MaxQueryLength {
		return ErrQueryTooLong
	}
	return nil
}

// validatePage validates the page number parameter.
func validatePage(page int) error {
	if page > MaxPageNumber {
		return ErrPageTooLarge
	}
	return nil
}

// NewSearchHandler creates a handler for search.
func NewSearchHandler(svc *application.SearchService) mcp.ToolHandlerFor[SearchInput, SearchOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input SearchInput) (*mcp.CallToolResult, SearchOutput, error) {
		if err := validateQuery(input.Query); err != nil {
			log.Printf("ERROR search validation: %s", SanitizeLog(err.Error()))
			return nil, SearchOutput{}, fmt.Errorf("search: %w", err)
		}
		if err := validatePage(input.Page); err != nil {
			log.Printf("ERROR search validation: %s", SanitizeLog(err.Error()))
			return nil, SearchOutput{}, fmt.Errorf("search: %w", err)
		}

		output, err := searchHelper(ctx, svc, domain.SearchParams{
			Query:      input.Query,
			Categories: input.Categories,
			Language:   input.Language,
			Page:       input.Page,
			TimeRange:  input.TimeRange,
			SafeSearch: input.SafeSearch,
		})
		if err != nil {
			log.Printf("ERROR search: %s", SanitizeLog(err.Error()))
			return nil, SearchOutput{}, fmt.Errorf("search: %w", ErrSearchFailed)
		}
		return nil, output, nil
	}
}

// NewSearchNewsHandler creates a handler for search_news with presets: categories=["news"], time_range="day".
func NewSearchNewsHandler(svc *application.SearchService) mcp.ToolHandlerFor[SearchNewsInput, SearchOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input SearchNewsInput) (*mcp.CallToolResult, SearchOutput, error) {
		if err := validateQuery(input.Query); err != nil {
			log.Printf("ERROR search_news validation: %s", SanitizeLog(err.Error()))
			return nil, SearchOutput{}, fmt.Errorf("search_news: %w", err)
		}
		if err := validatePage(input.Page); err != nil {
			log.Printf("ERROR search_news validation: %s", SanitizeLog(err.Error()))
			return nil, SearchOutput{}, fmt.Errorf("search_news: %w", err)
		}

		output, err := searchHelper(ctx, svc, domain.SearchParams{
			Query:      input.Query,
			Categories: []string{"news"},
			Language:   input.Language,
			Page:       input.Page,
			TimeRange:  "day",
			SafeSearch: input.SafeSearch,
		})
		if err != nil {
			log.Printf("ERROR search_news: %s", SanitizeLog(err.Error()))
			return nil, SearchOutput{}, fmt.Errorf("search_news: %w", ErrSearchFailed)
		}
		return nil, output, nil
	}
}

// NewSearchImagesHandler creates a handler for search_images with presets: categories=["images"].
func NewSearchImagesHandler(svc *application.SearchService) mcp.ToolHandlerFor[SearchImagesInput, SearchOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input SearchImagesInput) (*mcp.CallToolResult, SearchOutput, error) {
		if err := validateQuery(input.Query); err != nil {
			log.Printf("ERROR search_images validation: %s", SanitizeLog(err.Error()))
			return nil, SearchOutput{}, fmt.Errorf("search_images: %w", err)
		}
		if err := validatePage(input.Page); err != nil {
			log.Printf("ERROR search_images validation: %s", SanitizeLog(err.Error()))
			return nil, SearchOutput{}, fmt.Errorf("search_images: %w", err)
		}

		output, err := searchHelper(ctx, svc, domain.SearchParams{
			Query:      input.Query,
			Categories: []string{"images"},
			Language:   input.Language,
			Page:       input.Page,
			SafeSearch: input.SafeSearch,
		})
		if err != nil {
			log.Printf("ERROR search_images: %s", SanitizeLog(err.Error()))
			return nil, SearchOutput{}, fmt.Errorf("search_images: %w", ErrSearchFailed)
		}
		return nil, output, nil
	}
}
