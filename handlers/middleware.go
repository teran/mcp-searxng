package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode"
)

// checkBatchSize validates JSON-RPC batch request size.
// If the body starts with '[', it is a batch request — the function parses
// it as an array and returns an error if len(array) > MaxBatchSize.
// Non-batch requests (starting with '{' or empty) are always accepted.
func checkBatchSize(body []byte) error {
	trimmed := bytes.TrimLeftFunc(body, unicode.IsSpace)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return nil // not a batch request
	}

	var batch []json.RawMessage
	if err := json.Unmarshal(trimmed, &batch); err != nil {
		return nil // malformed JSON will be caught downstream
	}
	if len(batch) > MaxBatchSize {
		return fmt.Errorf("batch size %d exceeds maximum of %d", len(batch), MaxBatchSize)
	}
	return nil
}

// DefaultMaxRequestBodySize is the maximum allowed size for a request body (1 MB).
const DefaultMaxRequestBodySize = 1 << 20

// MaxBatchSize is the maximum number of JSON-RPC requests allowed in a
// single batch. Batches larger than this are rejected to prevent
// amplification attacks where a single HTTP request triggers many
// upstream API calls.
const MaxBatchSize = 100

// BodyLimitMiddleware limits the request body size to maxBytes.
// The size limit is enforced before the body reaches downstream handlers,
// preventing resource exhaustion from large requests.
func BodyLimitMiddleware(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}

// MaxBytesError returns true if the error is an http.MaxBytesError.
func MaxBytesError(err error) bool {
	var maxBytesErr *http.MaxBytesError
	return err != nil && errors.As(err, &maxBytesErr)
}

// loggingResponseWriter wraps http.ResponseWriter to capture
// the HTTP status code and response body size.
type loggingResponseWriter struct {
	http.ResponseWriter

	statusCode int
	bodySize   int
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

func (lrw *loggingResponseWriter) Write(b []byte) (int, error) {
	n, err := lrw.ResponseWriter.Write(b)
	lrw.bodySize += n
	return n, err
}

// mcpRequestMethod attempts to extract the MCP method name from a JSON-RPC
// request body. For "tools/call" it additionally extracts the tool name from
// params.name. Returns the extracted name or one of the following sentinel
// values when parsing fails:
//   - "empty_body"    — the body is nil or zero-length
//   - "parse_error"   — the body is not valid JSON
//   - "no_method"     — JSON is valid but the "method" field is empty or missing
func mcpRequestMethod(body []byte) string {
	if len(body) == 0 {
		return "empty_body"
	}
	var req struct {
		Method string `json:"method"`
		Params struct {
			Name string `json:"name"`
		} `json:"params"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return "parse_error"
	}
	if req.Method == "tools/call" && req.Params.Name != "" {
		return req.Params.Name
	}
	if req.Method != "" {
		return req.Method
	}
	return "no_method"
}

// SanitizeLog strips control characters from strings before logging
// to prevent log injection attacks (e.g. newlines or ANSI escape codes
// injected via JSON fields). Only printable characters, horizontal tab,
// and basic Latin characters are preserved. Control characters (0x00-0x08,
// 0x0b-0x1f, 0x7f) and Unicode bidi formatting characters (LRM/RLM,
// LRE/RLE/PDF/LRO/RLO, LRI/RLI/FSI/PDI) are removed.
func SanitizeLog(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' {
			return r
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		// Remove Unicode directionality formatting characters to prevent
		// bidi spoofing in log output.
		if r >= 0x200e && r <= 0x200f { // LRM, RLM
			return -1
		}
		if r >= 0x202a && r <= 0x202e { // LRE, RLE, PDF, LRO, RLO
			return -1
		}
		if r >= 0x2066 && r <= 0x2069 { // LRI, RLI, FSI, PDI
			return -1
		}
		return r
	}, s)
}

// RecoveryMiddleware recovers from panics in downstream handlers, logs the
// panic with a stack trace, and returns 500 Internal Server Error. Without
// this middleware any panic in an HTTP goroutine would crash the entire server.
func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("ERROR panic recovered: %s", SanitizeLog(fmt.Sprintf("%v", rec)))
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// LoggingMiddleware logs MCP request details at INFO level.
// Records: timestamp, MCP method name, HTTP method, request path, request
// duration, request body size, and response body size.
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Read and buffer request body for method extraction.
		// r.Body is already wrapped with http.MaxBytesReader by BodyLimitMiddleware (outermost),
		// so reading is bounded to 1 MB.
		body, err := io.ReadAll(r.Body)
		if err != nil {
			log.Printf("INFO mcp_log request_error=read_body error=%v", err)
			http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))

		reqSize := len(body)
		mcpMethod := SanitizeLog(mcpRequestMethod(body))

		// Reject batch requests that exceed MaxBatchSize to prevent
		// amplification attacks.
		if err := checkBatchSize(body); err != nil {
			log.Printf("INFO mcp_log http_method=%s path=%s method=%s duration=%v req_size=%d resp_size=%d status=%d", //nolint:gosec
				SanitizeLog(r.Method), SanitizeLog(r.URL.Path), mcpMethod, time.Since(start), reqSize, 0, http.StatusBadRequest)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Wrap ResponseWriter to capture response size.
		lrw := &loggingResponseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		next.ServeHTTP(lrw, r)

		duration := time.Since(start)
		log.Printf("INFO mcp_log http_method=%s path=%s method=%s duration=%v req_size=%d resp_size=%d status=%d", //nolint:gosec
			SanitizeLog(r.Method), SanitizeLog(r.URL.Path), mcpMethod, duration, reqSize, lrw.bodySize, lrw.statusCode)
	})
}
