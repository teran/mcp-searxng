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
	searchFunc func(ctx context.Context, params domain.SearchParams) (*domain.SearchResponse, error)
}

func (m *mockSearchRepo) Search(ctx context.Context, params domain.SearchParams) (*domain.SearchResponse, error) {
	return m.searchFunc(ctx, params)
}
func newMockService(repo *mockSearchRepo) *application.SearchService {
	return application.NewSearchService(repo)
}

func TestRegisterTools(t *testing.T) {
	t.Parallel()

	t.Run("registers search tool without metrics", func(t *testing.T) {
		srv := mcp.NewServer(&mcp.Implementation{Name: "test"}, nil)

		// Should not panic.
		RegisterTools(srv, nil, nil)
	})

	t.Run("registers search tool with metrics", func(t *testing.T) {
		reg := prometheus.NewRegistry()
		m := NewMetrics(reg)
		srv := mcp.NewServer(&mcp.Implementation{Name: "test"}, nil)

		// Should not panic.
		RegisterTools(srv, m, nil)
	})

	t.Run("handler works with explicit service", func(t *testing.T) {
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

		// Pass the service directly to the handler — no context lookup needed.
		handler := NewSearchHandler(svc)

		_, output, err := handler(context.Background(), &mcp.CallToolRequest{}, SearchInput{
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
