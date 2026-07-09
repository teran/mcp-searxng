package handlers

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/teran/mcp-searxng/application"
)

// Context keys for dependency injection.
type (
	searchServiceCtxKey struct{}
)

// ContextWithServices stores application services in context for retrieval
// by tool handlers at runtime.
func ContextWithServices(ctx context.Context, searchSvc *application.SearchService) context.Context {
	ctx = context.WithValue(ctx, searchServiceCtxKey{}, searchSvc)
	return ctx
}

// SearchServiceFromContext retrieves the SearchService from context.
func SearchServiceFromContext(ctx context.Context) *application.SearchService {
	v, _ := ctx.Value(searchServiceCtxKey{}).(*application.SearchService)
	return v
}

// RegisterTools registers all MCP tools on the server.
// If metrics is non-nil, each tool handler is wrapped with WrapToolHandler for
// per-tool Prometheus metrics (request count and duration).
func RegisterTools(s *mcp.Server, metrics *Metrics) {
	mcp.AddTool(s, &mcp.Tool{ //nolint:exhaustruct
		Name:        "search",
		Description: "Search the web using SearXNG. Returns search results, answers, suggestions, and infoboxes.",
	}, WrapToolHandler(metrics, "search", func(ctx context.Context, _ *mcp.CallToolRequest, in SearchInput) (*mcp.CallToolResult, SearchOutput, error) {
		return NewSearchHandler(SearchServiceFromContext(ctx))(ctx, nil, in)
	}))
}
