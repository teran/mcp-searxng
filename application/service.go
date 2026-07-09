package application

import (
	"context"
	"fmt"

	"github.com/teran/mcp-searxng/domain"
)

// SearchService implements search-related use cases.
type SearchService struct {
	repo domain.SearchRepository
}

// NewSearchService creates a new SearchService.
func NewSearchService(repo domain.SearchRepository) *SearchService {
	return &SearchService{repo: repo}
}

// Search performs a search against SearXNG with the given parameters.
func (s *SearchService) Search(ctx context.Context, params domain.SearchParams) (*domain.SearchResponse, error) {
	result, err := s.repo.Search(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	return result, nil
}
