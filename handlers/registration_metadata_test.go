package handlers

import (
	"testing"
)

// expectedTools lists the exact set of tool names the server must expose.
var expectedTools = []string{
	"search",
	"search_news",
	"search_images",
	"search_videos",
	"search_music",
}

// TestToolMetadata verifies that every tool carries the MCP metadata required
// by the mcp-server-spec M4 requirement: Annotations (hints) and a Title.
func TestToolMetadata(t *testing.T) {
	t.Parallel()

	defs := ToolDefinitions()

	// Exactly the expected tools, in order.
	if len(defs) != len(expectedTools) {
		t.Fatalf("ToolDefinitions() returned %d tools, want %d", len(defs), len(expectedTools))
	}
	for i, want := range expectedTools {
		if defs[i].Name != want {
			t.Errorf("ToolDefinitions()[%d].Name = %q, want %q", i, defs[i].Name, want)
		}
	}

	for _, tool := range defs {
		t.Run(tool.Name, func(t *testing.T) {
			tool := tool // pin range variable for parallel subtests

			if tool.Title == "" {
				t.Error("tool.Title is empty, want a human-readable title")
			}

			if tool.Description == "" {
				t.Error("tool.Description is empty, want a human-readable description")
			}

			if tool.Annotations == nil {
				t.Fatalf("tool.Annotations is nil, want Annotations metadata")
			}
			ann := tool.Annotations

			// Search is read-only.
			if !ann.ReadOnlyHint {
				t.Error("annotations.ReadOnlyHint = false, want true")
			}

			// Web search interacts with an open world of external entities.
			if ann.OpenWorldHint == nil {
				t.Error("annotations.OpenWorldHint is nil, want non-nil *bool = true")
			} else if !*ann.OpenWorldHint {
				t.Error("annotations.OpenWorldHint = false, want true")
			}

			// None of the tools are destructive.
			if ann.DestructiveHint != nil && *ann.DestructiveHint {
				t.Error("annotations.DestructiveHint = true, want nil or false")
			}
		})
	}
}
