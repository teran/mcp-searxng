package handlers

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sirupsen/logrus"

	"github.com/teran/mcp-searxng/application"
)

// TestRegisterToolsLogsToolSurface verifies the L10/L05GO startup tool-surface
// log: when logging is enabled, every registered tool (the set returned by
// tools/list) is logged by name/title with an `always_present` field (L11).
// All search tools are static, so always_present must be true for each.
func TestRegisterToolsLogsToolSurface(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := logrus.New()
	logger.SetOutput(&buf)
	logger.SetFormatter(&logrus.JSONFormatter{})
	logger.SetLevel(logrus.InfoLevel)

	srv := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	RegisterToolsWithSource(srv, nil, logger, &application.SearchService{}, "stdio")

	var found []map[string]any
	for line := range strings.SplitSeq(buf.String(), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if json.Unmarshal([]byte(line), &rec) != nil {
			continue
		}
		if rec["msg"] == "mcp_tool_surface" {
			found = append(found, rec)
		}
	}

	if len(found) != len(expectedTools) {
		t.Fatalf("got %d tool-surface log lines, want %d", len(found), len(expectedTools))
	}

	seen := make(map[string]bool, len(found))
	for _, rec := range found {
		tool, _ := rec["tool"].(string)
		seen[tool] = true

		if title, _ := rec["title"].(string); title == "" {
			t.Errorf("tool %q: title missing in tool-surface log", tool)
		}
		// All search tools are static, so always_present must be true (L11).
		if ap, _ := rec["always_present"].(bool); !ap {
			t.Errorf("tool %q: always_present = %v, want true", tool, rec["always_present"])
		}
	}

	for _, want := range expectedTools {
		if !seen[want] {
			t.Errorf("tool %q missing from tool-surface log", want)
		}
	}
}

// TestToolSurfaceLogDisabled verifies the tool-surface log is suppressed when
// logging is disabled (stdio mode with no LOG_LEVEL — the logger sits at
// PanicLevel), so nothing is written for an operator who disabled logging.
func TestToolSurfaceLogDisabled(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := logrus.New()
	logger.SetOutput(&buf)
	logger.SetFormatter(&logrus.JSONFormatter{})
	logger.SetLevel(logrus.PanicLevel) // disabled (stdio, LOG_LEVEL unset)

	srv := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	RegisterToolsWithSource(srv, nil, logger, &application.SearchService{}, "stdio")

	if buf.Len() != 0 {
		t.Errorf("expected no log output when logging disabled, got %q", buf.String())
	}
}
