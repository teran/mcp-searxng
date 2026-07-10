package domain

import (
	"context"
	"testing"
)

// mockRepo implements SearchRepository for testing.
type mockRepo struct {
	searchFunc  func(ctx context.Context, params SearchParams) (*SearchResponse, error)
	enginesFunc func(ctx context.Context) ([]EngineInfo, error)
}

func (m *mockRepo) Search(ctx context.Context, params SearchParams) (*SearchResponse, error) {
	return m.searchFunc(ctx, params)
}

func (m *mockRepo) GetEngines(ctx context.Context) ([]EngineInfo, error) {
	return m.enginesFunc(ctx)
}

func TestSearchRepositoryInterface(t *testing.T) {
	t.Parallel()

	t.Run("mock repository returns expected response", func(t *testing.T) {
		repo := &mockRepo{
			searchFunc: func(_ context.Context, params SearchParams) (*SearchResponse, error) {
				return &SearchResponse{
					Query:   params.Query,
					Results: []SearchResult{},
				}, nil
			},
		}

		resp, err := repo.Search(context.Background(), SearchParams{
			Query: "test",
		})
		if err != nil {
			t.Fatalf("Search() returned error: %v", err)
		}
		if resp.Query != "test" {
			t.Errorf("Query = %q, want %q", resp.Query, "test")
		}
	})
}
