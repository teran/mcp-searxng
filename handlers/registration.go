package handlers

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/teran/mcp-searxng/application"
)

// RegisterTools registers all MCP tools on the server with the given
// SearchService. If metrics is non-nil, each tool handler is wrapped with
// WrapToolHandler for per-tool Prometheus metrics (request count and duration).
func RegisterTools(s *mcp.Server, metrics *Metrics, svc *application.SearchService) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "search",
		Description: "Search the web using SearXNG. Returns search results, answers, suggestions, and infoboxes.",
	}, WrapToolHandler(metrics, "search", func(ctx context.Context, _ *mcp.CallToolRequest, in SearchInput) (*mcp.CallToolResult, SearchOutput, error) {
		return NewSearchHandler(svc)(ctx, nil, in)
	}))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "search_news",
		Description: "Search news using SearXNG. Convenience wrapper around search with categories=[news], time_range=day.",
	}, WrapToolHandler(metrics, "search_news", func(ctx context.Context, _ *mcp.CallToolRequest, in SearchNewsInput) (*mcp.CallToolResult, SearchOutput, error) {
		return NewSearchNewsHandler(svc)(ctx, nil, in)
	}))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "search_images",
		Description: "Search images using SearXNG. Convenience wrapper around search with categories=[images].",
	}, WrapToolHandler(metrics, "search_images", func(ctx context.Context, _ *mcp.CallToolRequest, in SearchImagesInput) (*mcp.CallToolResult, SearchOutput, error) {
		return NewSearchImagesHandler(svc)(ctx, nil, in)
	}))

}
