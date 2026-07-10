package domain

import "context"

// SearchParams holds parameters for a SearXNG search.
type SearchParams struct {
	Query      string
	Categories []string
	Language   string
	Page       int
	TimeRange  string
	SafeSearch int
}

// SearchRepository defines the interface for SearXNG search operations.
type SearchRepository interface {
	Search(ctx context.Context, params SearchParams) (*SearchResponse, error)
}
