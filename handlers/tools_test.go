package handlers_test

import (
	"context"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/teran/mcp-searxng/application"
	"github.com/teran/mcp-searxng/domain"
	"github.com/teran/mcp-searxng/handlers"
)

type mockSearchRepo struct {
	searchFunc func(ctx context.Context, params domain.SearchParams) (*domain.SearchResponse, error)
}

func (m *mockSearchRepo) Search(ctx context.Context, params domain.SearchParams) (*domain.SearchResponse, error) {
	return m.searchFunc(ctx, params)
}
func newMockService(repo *mockSearchRepo) *application.SearchService {
	return application.NewSearchService(repo)
}

func TestNewSearchHandler(t *testing.T) { //nolint:gocognit
	t.Parallel()

	t.Run("basic search returns results", func(t *testing.T) {
		repo := &mockSearchRepo{
			searchFunc: func(_ context.Context, params domain.SearchParams) (*domain.SearchResponse, error) {
				return &domain.SearchResponse{
					Query: params.Query,
					Results: []domain.SearchResult{
						{
							Title:   "Test Result",
							URL:     "https://example.com",
							Content: "Test content",
							Engine:  "google",
						},
					},
					NumberOfResults: 1,
				}, nil
			},
		}
		svc := newMockService(repo)
		handler := handlers.NewSearchHandler(svc)

		result, output, err := handler(context.Background(), &mcp.CallToolRequest{}, handlers.SearchInput{
			Query: "test query",
		})

		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		if result != nil {
			t.Errorf("expected nil CallToolResult, got %v", result)
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
		if output.Results[0].Engine != "google" {
			t.Errorf("Results[0].Engine = %q, want %q", output.Results[0].Engine, "google")
		}
	})

	t.Run("search with all parameters", func(t *testing.T) {
		repo := &mockSearchRepo{
			searchFunc: func(_ context.Context, params domain.SearchParams) (*domain.SearchResponse, error) {
				if params.Query != "test" {
					t.Errorf("Query = %q, want %q", params.Query, "test")
				}
				if len(params.Categories) != 1 || params.Categories[0] != "news" {
					t.Errorf("Categories = %v, want [news]", params.Categories)
				}
				if params.Language != "en-US" {
					t.Errorf("Language = %q, want %q", params.Language, "en-US")
				}
				if params.Page != 3 {
					t.Errorf("Page = %d, want 3", params.Page)
				}
				if params.TimeRange != "year" {
					t.Errorf("TimeRange = %q, want %q", params.TimeRange, "year")
				}
				if params.SafeSearch != 2 {
					t.Errorf("SafeSearch = %d, want 2", params.SafeSearch)
				}
				return &domain.SearchResponse{
					Query:   params.Query,
					Results: []domain.SearchResult{},
				}, nil
			},
		}
		svc := newMockService(repo)
		handler := handlers.NewSearchHandler(svc)

		_, output, err := handler(context.Background(), &mcp.CallToolRequest{}, handlers.SearchInput{
			Query:      "test",
			Categories: []string{"news"},
			Language:   "en-US",
			Page:       3,
			TimeRange:  "year",
			SafeSearch: 2,
		})

		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		if output.Query != "test" {
			t.Errorf("Query = %q, want %q", output.Query, "test")
		}
	})

	t.Run("page defaults to 1 when not set", func(t *testing.T) {
		repo := &mockSearchRepo{
			searchFunc: func(_ context.Context, params domain.SearchParams) (*domain.SearchResponse, error) {
				if params.Page != 1 {
					t.Errorf("Page = %d, want 1", params.Page)
				}
				return &domain.SearchResponse{
					Query:   params.Query,
					Results: []domain.SearchResult{},
				}, nil
			},
		}
		svc := newMockService(repo)
		handler := handlers.NewSearchHandler(svc)

		_, _, err := handler(context.Background(), &mcp.CallToolRequest{}, handlers.SearchInput{
			Query: "test",
			Page:  0,
		})

		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
	})

	t.Run("repository error propagates", func(t *testing.T) {
		expectedErr := errors.New("search failed")
		repo := &mockSearchRepo{
			searchFunc: func(_ context.Context, _ domain.SearchParams) (*domain.SearchResponse, error) {
				return nil, expectedErr
			},
		}
		svc := newMockService(repo)
		handler := handlers.NewSearchHandler(svc)

		_, _, err := handler(context.Background(), &mcp.CallToolRequest{}, handlers.SearchInput{
			Query: "test",
		})

		if err == nil {
			t.Fatal("handler expected error, got nil")
		}
	})

	t.Run("empty results not nil", func(t *testing.T) {
		repo := &mockSearchRepo{
			searchFunc: func(_ context.Context, params domain.SearchParams) (*domain.SearchResponse, error) {
				return &domain.SearchResponse{
					Query:   params.Query,
					Results: []domain.SearchResult{},
				}, nil
			},
		}
		svc := newMockService(repo)
		handler := handlers.NewSearchHandler(svc)

		_, output, err := handler(context.Background(), &mcp.CallToolRequest{}, handlers.SearchInput{
			Query: "test",
		})

		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		if output.Results == nil {
			t.Errorf("Results = nil, want non-nil empty slice")
		}
		if len(output.Results) != 0 {
			t.Errorf("len(Results) = %d, want 0", len(output.Results))
		}
	})
}

func TestNewSearchNewsHandler(t *testing.T) { //nolint:gocognit
	t.Parallel()

	t.Run("basic news search returns results", func(t *testing.T) {
		repo := &mockSearchRepo{
			searchFunc: func(_ context.Context, params domain.SearchParams) (*domain.SearchResponse, error) {
				return &domain.SearchResponse{
					Query: params.Query,
					Results: []domain.SearchResult{
						{
							Title:   "News Article",
							URL:     "https://news.example.com",
							Content: "News content",
							Engine:  "google",
						},
					},
					NumberOfResults: 1,
				}, nil
			},
		}
		svc := newMockService(repo)
		handler := handlers.NewSearchNewsHandler(svc)

		result, output, err := handler(context.Background(), &mcp.CallToolRequest{}, handlers.SearchNewsInput{
			Query: "test news",
		})

		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		if result != nil {
			t.Errorf("expected nil CallToolResult, got %v", result)
		}
		if output.Query != "test news" {
			t.Errorf("Query = %q, want %q", output.Query, "test news")
		}
		if len(output.Results) != 1 {
			t.Fatalf("len(Results) = %d, want 1", len(output.Results))
		}
		if output.Results[0].Title != "News Article" {
			t.Errorf("Results[0].Title = %q, want %q", output.Results[0].Title, "News Article")
		}
	})

	t.Run("news search passes preset categories and time_range", func(t *testing.T) {
		repo := &mockSearchRepo{
			searchFunc: func(_ context.Context, params domain.SearchParams) (*domain.SearchResponse, error) {
				if len(params.Categories) != 1 || params.Categories[0] != "news" {
					t.Errorf("Categories = %v, want [news]", params.Categories)
				}
				if params.TimeRange != "day" {
					t.Errorf("TimeRange = %q, want %q", params.TimeRange, "day")
				}
				return &domain.SearchResponse{
					Query:   params.Query,
					Results: []domain.SearchResult{},
				}, nil
			},
		}
		svc := newMockService(repo)
		handler := handlers.NewSearchNewsHandler(svc)

		_, _, err := handler(context.Background(), &mcp.CallToolRequest{}, handlers.SearchNewsInput{
			Query: "test",
		})

		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
	})

	t.Run("news search with all parameters", func(t *testing.T) {
		repo := &mockSearchRepo{
			searchFunc: func(_ context.Context, params domain.SearchParams) (*domain.SearchResponse, error) {
				if params.Query != "test" {
					t.Errorf("Query = %q, want %q", params.Query, "test")
				}
				if params.Language != "ru-RU" {
					t.Errorf("Language = %q, want %q", params.Language, "ru-RU")
				}
				if params.Page != 2 {
					t.Errorf("Page = %d, want 2", params.Page)
				}
				if params.SafeSearch != 1 {
					t.Errorf("SafeSearch = %d, want 1", params.SafeSearch)
				}
				return &domain.SearchResponse{
					Query:   params.Query,
					Results: []domain.SearchResult{},
				}, nil
			},
		}
		svc := newMockService(repo)
		handler := handlers.NewSearchNewsHandler(svc)

		_, _, err := handler(context.Background(), &mcp.CallToolRequest{}, handlers.SearchNewsInput{
			Query:      "test",
			Language:   "ru-RU",
			Page:       2,
			SafeSearch: 1,
		})

		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
	})

	t.Run("news search error propagates", func(t *testing.T) {
		repo := &mockSearchRepo{
			searchFunc: func(_ context.Context, _ domain.SearchParams) (*domain.SearchResponse, error) {
				return nil, errors.New("upstream error")
			},
		}
		svc := newMockService(repo)
		handler := handlers.NewSearchNewsHandler(svc)

		_, _, err := handler(context.Background(), &mcp.CallToolRequest{}, handlers.SearchNewsInput{
			Query: "test",
		})

		if err == nil {
			t.Fatal("handler expected error, got nil")
		}
	})
}

func TestNewSearchImagesHandler(t *testing.T) { //nolint:gocognit
	t.Parallel()

	t.Run("basic image search returns results", func(t *testing.T) {
		imgSrc := "https://example.com/image.jpg"
		repo := &mockSearchRepo{
			searchFunc: func(_ context.Context, params domain.SearchParams) (*domain.SearchResponse, error) {
				return &domain.SearchResponse{
					Query: params.Query,
					Results: []domain.SearchResult{
						{
							Title:   "Image Title",
							URL:     "https://example.com",
							Content: "Image content",
							Engine:  "google",
							ImgSrc:  &imgSrc,
						},
					},
					NumberOfResults: 1,
				}, nil
			},
		}
		svc := newMockService(repo)
		handler := handlers.NewSearchImagesHandler(svc)

		result, output, err := handler(context.Background(), &mcp.CallToolRequest{}, handlers.SearchImagesInput{
			Query: "test image",
		})

		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		if result != nil {
			t.Errorf("expected nil CallToolResult, got %v", result)
		}
		if output.Query != "test image" {
			t.Errorf("Query = %q, want %q", output.Query, "test image")
		}
		if len(output.Results) != 1 {
			t.Fatalf("len(Results) = %d, want 1", len(output.Results))
		}
		if output.Results[0].ImgSrc == nil || *output.Results[0].ImgSrc != "https://example.com/image.jpg" {
			t.Errorf("Results[0].ImgSrc = %v, want %v", output.Results[0].ImgSrc, &imgSrc)
		}
	})

	t.Run("image search passes preset categories", func(t *testing.T) {
		repo := &mockSearchRepo{
			searchFunc: func(_ context.Context, params domain.SearchParams) (*domain.SearchResponse, error) {
				if len(params.Categories) != 1 || params.Categories[0] != "images" {
					t.Errorf("Categories = %v, want [images]", params.Categories)
				}
				if params.TimeRange != "" {
					t.Errorf("TimeRange = %q, want empty", params.TimeRange)
				}
				return &domain.SearchResponse{
					Query:   params.Query,
					Results: []domain.SearchResult{},
				}, nil
			},
		}
		svc := newMockService(repo)
		handler := handlers.NewSearchImagesHandler(svc)

		_, _, err := handler(context.Background(), &mcp.CallToolRequest{}, handlers.SearchImagesInput{
			Query: "test",
		})

		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
	})

	t.Run("image search with all parameters", func(t *testing.T) {
		repo := &mockSearchRepo{
			searchFunc: func(_ context.Context, params domain.SearchParams) (*domain.SearchResponse, error) {
				if params.Query != "test" {
					t.Errorf("Query = %q, want %q", params.Query, "test")
				}
				if params.Language != "fr" {
					t.Errorf("Language = %q, want %q", params.Language, "fr")
				}
				if params.Page != 5 {
					t.Errorf("Page = %d, want 5", params.Page)
				}
				if params.SafeSearch != 2 {
					t.Errorf("SafeSearch = %d, want 2", params.SafeSearch)
				}
				return &domain.SearchResponse{
					Query:   params.Query,
					Results: []domain.SearchResult{},
				}, nil
			},
		}
		svc := newMockService(repo)
		handler := handlers.NewSearchImagesHandler(svc)

		_, _, err := handler(context.Background(), &mcp.CallToolRequest{}, handlers.SearchImagesInput{
			Query:      "test",
			Language:   "fr",
			Page:       5,
			SafeSearch: 2,
		})

		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
	})

	t.Run("image search error propagates", func(t *testing.T) {
		repo := &mockSearchRepo{
			searchFunc: func(_ context.Context, _ domain.SearchParams) (*domain.SearchResponse, error) {
				return nil, errors.New("upstream error")
			},
		}
		svc := newMockService(repo)
		handler := handlers.NewSearchImagesHandler(svc)

		_, _, err := handler(context.Background(), &mcp.CallToolRequest{}, handlers.SearchImagesInput{
			Query: "test",
		})

		if err == nil {
			t.Fatal("handler expected error, got nil")
		}
	})
}
