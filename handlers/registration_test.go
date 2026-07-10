package handlers

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/teran/mcp-searxng/application"
	"github.com/teran/mcp-searxng/domain"
)

// mockSearchRepo implements domain.SearchRepository for use in registration tests.
type mockSearchRepo struct {
	searchFunc  func(ctx context.Context, params domain.SearchParams) (*domain.SearchResponse, error)
	enginesFunc func(ctx context.Context) ([]domain.EngineInfo, error)
}

func (m *mockSearchRepo) Search(ctx context.Context, params domain.SearchParams) (*domain.SearchResponse, error) {
	return m.searchFunc(ctx, params)
}

func (m *mockSearchRepo) GetEngines(ctx context.Context) ([]domain.EngineInfo, error) {
	return m.enginesFunc(ctx)
}

func newMockService(repo *mockSearchRepo) *application.SearchService {
	return application.NewSearchService(repo)
}

func TestContextWithServices(t *testing.T) {
	t.Parallel()

	t.Run("SearchService stored and retrieved", func(t *testing.T) {
		ctx := ContextWithServices(context.Background(), nil)
		svc := SearchServiceFromContext(ctx)
		if svc != nil {
			t.Errorf("SearchServiceFromContext = %v, want nil", svc)
		}
	})

	t.Run("SearchServiceFromContext returns nil for empty context", func(t *testing.T) {
		svc := SearchServiceFromContext(context.Background())
		if svc != nil {
			t.Errorf("SearchServiceFromContext = %v, want nil", svc)
		}
	})

	t.Run("real service round-trips correctly", func(t *testing.T) {
		repo := &mockSearchRepo{
			searchFunc: func(_ context.Context, params domain.SearchParams) (*domain.SearchResponse, error) {
				return &domain.SearchResponse{
					Query:   params.Query,
					Results: []domain.SearchResult{},
				}, nil
			},
		}
		svc := newMockService(repo)
		ctx := ContextWithServices(context.Background(), svc)

		got := SearchServiceFromContext(ctx)
		if got == nil {
			t.Fatal("SearchServiceFromContext returned nil for stored service")
		}
		if got != svc {
			t.Error("SearchServiceFromContext returned different pointer than stored")
		}

		// Verify the retrieved service is functional.
		result, err := got.Search(context.Background(), domain.SearchParams{Query: "test"})
		if err != nil {
			t.Errorf("service returned error: %v", err)
		}
		if result.Query != "test" {
			t.Errorf("result.Query = %q, want %q", result.Query, "test")
		}
	})
}

func TestRegisterTools(t *testing.T) {
	t.Parallel()

	t.Run("registers search tool without metrics", func(t *testing.T) {
		srv := mcp.NewServer(&mcp.Implementation{Name: "test"}, nil)

		// Should not panic.
		RegisterTools(srv, nil)
	})

	t.Run("registers search tool with metrics", func(t *testing.T) {
		reg := prometheus.NewRegistry()
		m := NewMetrics(reg)
		srv := mcp.NewServer(&mcp.Implementation{Name: "test"}, nil)

		// Should not panic.
		RegisterTools(srv, m)
	})

	t.Run("handler uses service from context", func(t *testing.T) {
		repo := &mockSearchRepo{
			searchFunc: func(_ context.Context, params domain.SearchParams) (*domain.SearchResponse, error) {
				return &domain.SearchResponse{
					Query:           params.Query,
					Results:         []domain.SearchResult{{Title: "Test Result"}},
					NumberOfResults: 1,
				}, nil
			},
		}
		svc := newMockService(repo)
		ctx := ContextWithServices(context.Background(), svc)

		// This is the same composition used in RegisterTools:
		// NewSearchHandler(SearchServiceFromContext(ctx))
		handler := NewSearchHandler(SearchServiceFromContext(ctx))

		_, output, err := handler(ctx, &mcp.CallToolRequest{}, SearchInput{
			Query: "test query",
		})
		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		if output.Query != "test query" {
			t.Errorf("Query = %q, want %q", output.Query, "test query")
		}
		if len(output.Results) != 1 {
			t.Fatalf("len(Results) = %d, want 1", len(output.Results))
		}
		if output.Results[0].Title != "Test Result" {
			t.Errorf("Results[0].Title = %q, want %q", output.Results[0].Title, "Test Result")
		}
	})
}
