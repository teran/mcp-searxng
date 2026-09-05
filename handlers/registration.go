package handlers

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/teran/mcp-searxng/application"
)

// trueVal is a package-level *bool used for the OpenWorldHint annotation,
// since the SDK represents it as a *bool.
var trueVal = true

// ToolDefinitions returns the metadata for every tool exposed by the server.
// Each tool carries its MCP metadata (Name, Description, Title and
// Annotations) so it is inspectable in tests. RegisterTools uses this list to
// wire each tool to its handler.
func ToolDefinitions() []*mcp.Tool {
	return []*mcp.Tool{
		{
			Name:        "search",
			Title:       "Search the web",
			Description: "Search the web using SearXNG. Returns search results, answers, suggestions, and infoboxes.",
			Annotations: toolAnnotations(),
		},
		{
			Name:        "search_news",
			Title:       "Search news",
			Description: "Search news using SearXNG. Convenience wrapper around search with categories=[news], time_range=day.",
			Annotations: toolAnnotations(),
		},
		{
			Name:        "search_images",
			Title:       "Search images",
			Description: "Search images using SearXNG. Convenience wrapper around search with categories=[images].",
			Annotations: toolAnnotations(),
		},
		{
			Name:        "search_videos",
			Title:       "Search videos",
			Description: "Search videos using SearXNG. Convenience wrapper around search with categories=[videos].",
			Annotations: toolAnnotations(),
		},
		{
			Name:        "search_music",
			Title:       "Search music",
			Description: "Search music using SearXNG. Convenience wrapper around search with categories=[music].",
			Annotations: toolAnnotations(),
		},
	}
}

// toolAnnotations returns the shared MCP annotations applied to every search
// tool. All searches are read-only, idempotent and interact with an open world
// of external entities; none of them are destructive.
func toolAnnotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint:    true,
		IdempotentHint:  true,
		OpenWorldHint:   &trueVal,
		DestructiveHint: nil,
	}
}

// RegisterTools registers all MCP tools on the server with the given
// SearchService. If metrics is non-nil, each tool handler is wrapped with
// WrapToolHandler for per-tool Prometheus metrics (request count and duration).
func RegisterTools(s *mcp.Server, metrics *Metrics, svc *application.SearchService) {
	for _, tool := range ToolDefinitions() {
		switch tool.Name {
		case "search":
			mcp.AddTool(s, tool, WrapToolHandler(metrics, tool.Name, func(ctx context.Context, _ *mcp.CallToolRequest, in SearchInput) (*mcp.CallToolResult, SearchOutput, error) {
				return NewSearchHandler(svc)(ctx, nil, in)
			}))
		case "search_news":
			mcp.AddTool(s, tool, WrapToolHandler(metrics, tool.Name, func(ctx context.Context, _ *mcp.CallToolRequest, in SearchNewsInput) (*mcp.CallToolResult, SearchOutput, error) {
				return NewSearchNewsHandler(svc)(ctx, nil, in)
			}))
		case "search_images":
			mcp.AddTool(s, tool, WrapToolHandler(metrics, tool.Name, func(ctx context.Context, _ *mcp.CallToolRequest, in SearchImagesInput) (*mcp.CallToolResult, SearchOutput, error) {
				return NewSearchImagesHandler(svc)(ctx, nil, in)
			}))
		case "search_videos":
			mcp.AddTool(s, tool, WrapToolHandler(metrics, tool.Name, func(ctx context.Context, _ *mcp.CallToolRequest, in SearchVideosInput) (*mcp.CallToolResult, SearchOutput, error) {
				return NewSearchVideosHandler(svc)(ctx, nil, in)
			}))
		case "search_music":
			mcp.AddTool(s, tool, WrapToolHandler(metrics, tool.Name, func(ctx context.Context, _ *mcp.CallToolRequest, in SearchMusicInput) (*mcp.CallToolResult, SearchOutput, error) {
				return NewSearchMusicHandler(svc)(ctx, nil, in)
			}))
		default:
			panic("unhandled tool in RegisterTools: " + tool.Name)
		}
	}
}
