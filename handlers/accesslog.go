package handlers

import (
	"context"
	"encoding/json"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sirupsen/logrus"
)

// accessLogMaxArgs bounds the number of characters of the sanitized arguments
// JSON written to the access log so a single log line stays bounded even for
// large tool inputs.
const accessLogMaxArgs = 512

// redactArgs serializes the tool input to JSON and sanitizes it for safe
// logging, bounding its length. Serialization failures yield a fixed marker
// rather than leaking internals.
func redactArgs(input any) string {
	b, err := json.Marshal(input)
	if err != nil {
		return "<unserializable>"
	}
	s := SanitizeLog(string(b))
	if len(s) > accessLogMaxArgs {
		s = s[:accessLogMaxArgs] + "..."
	}
	return s
}

// WrapAccessLog wraps an MCP tool handler so that every tools/call emits one
// INFO-level access log line carrying the tool name, redacted (sanitized and
// bounded) arguments, the source, the duration and the outcome. It is
// transport-agnostic and is composed at registration time so it fires for both
// HTTP and stdio transports.
//
// When the logger is not enabled at InfoLevel the wrapper is a no-op beyond a
// cheap IsLevelEnabled check, so a disabled logger costs nothing.
func WrapAccessLog[I, O any](logger *logrus.Logger, source, toolName string, handler mcp.ToolHandlerFor[I, O]) mcp.ToolHandlerFor[I, O] {
	return func(ctx context.Context, req *mcp.CallToolRequest, input I) (*mcp.CallToolResult, O, error) {
		start := time.Now()
		result, output, err := handler(ctx, req, input)
		duration := time.Since(start)

		if logger != nil && logger.IsLevelEnabled(logrus.InfoLevel) {
			fields := logrus.Fields{
				"tool":     toolName,
				"args":     redactArgs(input),
				"source":   source,
				"duration": duration,
				"outcome":  "success",
			}
			if err != nil {
				fields["outcome"] = "error"
				fields["error"] = SanitizeLog(err.Error())
			}
			logger.WithFields(fields).Info("mcp_tool")
		}
		return result, output, err
	}
}
