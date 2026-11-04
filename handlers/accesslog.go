// Package handlers contains the MCP tool handlers, HTTP middleware and
// transport-agnostic access-log / metrics wrapping for the server.
package handlers

import (
	"context"
	"encoding/json"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sirupsen/logrus"

	"github.com/teran/mcp-searxng/requestid"
)

// accessLogMaxArgs bounds the number of characters of the sanitized arguments
// JSON written to the access log so a single log line stays bounded even for
// large tool inputs.
const accessLogMaxArgs = 512

// redactArgs serializes the tool input to JSON and sanitizes it for safe
// logging, bounding its length. Serialization failures yield a fixed marker
// rather than leaking internals.
func redactArgs(input any) string {
	b, err := json.Marshal(RedactStruct(input))
	if err != nil {
		return "<unserializable>"
	}
	s := RedactURL(SanitizeLog(string(b)))
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
func WrapAccessLog[I, O any](
	logger *logrus.Logger, source, toolName string, handler mcp.ToolHandlerFor[I, O],
) mcp.ToolHandlerFor[I, O] {
	return func(ctx context.Context, req *mcp.CallToolRequest, input I) (*mcp.CallToolResult, O, error) {
		start := time.Now()

		// Generate a per-call request id, thread it through the context (so the
		// outbound client forwards it as X-Request-ID) and surface it on the
		// access-log line (L09/L04GO).
		requestID := requestid.New()
		ctx = requestid.With(ctx, requestID)

		// The L08 `source` is the gateway-provided client IP (X-Real-IP /
		// X-Forwarded-For, injected into the context by ClientIPMiddleware)
		// when a gateway populated them; otherwise it falls back to the
		// transport label passed at registration.
		sourceForLog := source
		if ip := ClientIPFrom(ctx); ip != "" {
			sourceForLog = ip
		}

		result, output, err := handler(ctx, req, input)
		duration := time.Since(start)

		if logger != nil && logger.IsLevelEnabled(logrus.InfoLevel) {
			fields := logrus.Fields{
				"tool":       toolName,
				"args":       redactArgs(input),
				"source":     sourceForLog,
				"request_id": requestID,
				"duration":   duration,
				"outcome":    "ok",
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
