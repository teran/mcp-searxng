package handlers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/teran/mcp-searxng/application"
	"github.com/teran/mcp-searxng/domain"
)

// Sentinel errors returned to the MCP client with user-friendly messages.
// Detailed internal errors are logged server-side via log.Printf.
var ErrSearchFailed = errors.New("search failed")

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

type InfoboxItem struct {
	ID      string `json:"id,omitempty"`
	URL     string `json:"url,omitempty"`
	Content string `json:"content,omitempty"`
}

type SearchOutput struct {
	Query           string             `json:"query"`
	Results         []SearchResultItem `json:"results"`
	Answers         []string           `json:"answers,omitempty"`
	Suggestions     []string           `json:"suggestions,omitempty"`
	Infoboxes       []InfoboxItem      `json:"infoboxes,omitempty"`
	NumberOfResults int                `json:"number_of_results"`
}

// FormatDate converts an ISO 8601 date string to a readable format (e.g. "10 Jul 2026").
// Returns empty string if the input is nil.
func FormatDate(d *string) string {
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

// ============================================================
// Tool handler factories
// ============================================================

// NewSearchHandler creates a handler for search.
func NewSearchHandler(svc *application.SearchService) mcp.ToolHandlerFor[SearchInput, SearchOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input SearchInput) (*mcp.CallToolResult, SearchOutput, error) {
		page := input.Page
		if page <= 0 {
			page = 1
		}

		result, err := svc.Search(ctx, domain.SearchParams{
			Query:      input.Query,
			Categories: input.Categories,
			Language:   input.Language,
			Page:       page,
			TimeRange:  input.TimeRange,
			SafeSearch: input.SafeSearch,
			Format:     "json",
		})
		if err != nil {
			log.Printf("ERROR search: %s", SanitizeLog(err.Error()))
			return nil, SearchOutput{}, fmt.Errorf("search: %w", ErrSearchFailed)
		}

		items := make([]SearchResultItem, 0, len(result.Results))
		for _, r := range result.Results {
			items = append(items, SearchResultItem{
				Title:         r.Title,
				URL:           r.URL,
				Content:       r.Content,
				Engine:        r.Engine,
				Category:      r.Category,
				PublishedDate: r.PublishedDate,
				FormattedDate: FormatDate(r.PublishedDate),
				ImgSrc:        r.ImgSrc,
				Source:        r.Source,
			})
		}

		answers := make([]string, 0, len(result.Answers))
		for _, a := range result.Answers {
			answers = append(answers, string(a))
		}

		suggestions := make([]string, 0, len(result.Suggestions))
		for _, s := range result.Suggestions {
			suggestions = append(suggestions, string(s))
		}

		infoboxes := make([]InfoboxItem, 0, len(result.Infoboxes))
		for _, ib := range result.Infoboxes {
			infoboxes = append(infoboxes, InfoboxItem{
				ID:      ib.ID,
				URL:     ib.URL,
				Content: ib.Content,
			})
		}

		return nil, SearchOutput{
			Query:           result.Query,
			Results:         items,
			Answers:         answers,
			Suggestions:     suggestions,
			Infoboxes:       infoboxes,
			NumberOfResults: result.NumberOfResults,
		}, nil
	}
}
