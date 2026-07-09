package application

import (
	"context"
	"errors"
	"testing"

	"github.com/teran/mcp-searxng/domain"
)

type mockSearchRepo struct {
	searchFunc func(ctx context.Context, params domain.SearchParams) (*domain.SearchResponse, error)
}

func (m *mockSearchRepo) Search(ctx context.Context, params domain.SearchParams) (*domain.SearchResponse, error) {
	return m.searchFunc(ctx, params)
}

func TestSearchService_Search(t *testing.T) {
	t.Parallel()

	t.Run("successful search", func(t *testing.T) {
		repo := &mockSearchRepo{
			searchFunc: func(_ context.Context, params domain.SearchParams) (*domain.SearchResponse, error) {
				return &domain.SearchResponse{
					Query:           params.Query,
					NumberOfResults: 42,
					Results: []domain.SearchResult{
						{Title: "Result 1", URL: "https://example.com/1", Content: "Content 1", Engine: "google"},
					},
				}, nil
			},
		}

		svc := NewSearchService(repo)
		resp, err := svc.Search(context.Background(), domain.SearchParams{
			Query: "test query",
		})
		if err != nil {
			t.Fatalf("Search() returned error: %v", err)
		}
		if resp.Query != "test query" {
			t.Errorf("Query = %q, want %q", resp.Query, "test query")
		}
		if resp.NumberOfResults != 42 {
			t.Errorf("NumberOfResults = %d, want 42", resp.NumberOfResults)
		}
		if len(resp.Results) != 1 {
			t.Errorf("len(Results) = %d, want 1", len(resp.Results))
		}
	})

	t.Run("repository error is propagated", func(t *testing.T) {
		expectedErr := errors.New("connection refused")
		repo := &mockSearchRepo{
			searchFunc: func(_ context.Context, _ domain.SearchParams) (*domain.SearchResponse, error) {
				return nil, expectedErr
			},
		}

		svc := NewSearchService(repo)
		_, err := svc.Search(context.Background(), domain.SearchParams{
			Query: "test",
		})
		if err == nil {
			t.Fatal("Search() expected error, got nil")
		}
	})
}
