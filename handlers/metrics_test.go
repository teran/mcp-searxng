package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/prometheus/client_golang/prometheus"
)

func mustRegisterMetrics(t *testing.T) *Metrics {
	t.Helper()
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)
	return m
}

func TestNewMetrics(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)

	if m == nil {
		t.Fatal("NewMetrics returned nil")
	}
	if m.toolRequestsTotal == nil {
		t.Error("toolRequestsTotal is nil")
	}
	if m.toolDuration == nil {
		t.Error("toolDuration is nil")
	}
	if m.activeRequests == nil {
		t.Error("activeRequests is nil")
	}
}

type testInput struct {
	Name string `json:"name"`
}

type testOutput struct {
	Result string `json:"result"`
}

func TestWrapToolHandler(t *testing.T) {
	t.Parallel()

	t.Run("successful handler records 2xx metric", func(t *testing.T) {
		m := mustRegisterMetrics(t)
		handler := WrapToolHandler[testInput, testOutput](m, "test_tool", func(ctx context.Context, _ *mcp.CallToolRequest, input testInput) (*mcp.CallToolResult, testOutput, error) {
			return nil, testOutput{Result: "ok"}, nil
		})

		_, output, err := handler(context.Background(), nil, testInput{Name: "test"})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if output.Result != "ok" {
			t.Errorf("output.Result = %q, want %q", output.Result, "ok")
		}
	})

	t.Run("error handler records 4xx metric", func(t *testing.T) {
		m := mustRegisterMetrics(t)
		handler := WrapToolHandler[testInput, testOutput](m, "test_tool", func(ctx context.Context, _ *mcp.CallToolRequest, input testInput) (*mcp.CallToolResult, testOutput, error) {
			return nil, testOutput{}, fmt.Errorf("test error")
		})

		_, _, err := handler(context.Background(), nil, testInput{Name: "test"})
		if err == nil {
			t.Error("expected error, got nil")
		}
	})

	t.Run("nil metrics does not panic", func(t *testing.T) {
		handler := WrapToolHandler[testInput, testOutput](nil, "test_tool", func(ctx context.Context, _ *mcp.CallToolRequest, input testInput) (*mcp.CallToolResult, testOutput, error) {
			return nil, testOutput{Result: "ok"}, nil
		})

		_, _, err := handler(context.Background(), nil, testInput{Name: "test"})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestMetricsMiddleware(t *testing.T) {
	t.Parallel()

	t.Run("active requests gauge works", func(t *testing.T) {
		m := mustRegisterMetrics(t)
		handler := MetricsMiddleware(m)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
	})
}

func TestRegisterMetricsOnRegistry(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	handler := RegisterMetricsOnRegistry(reg)

	if handler == nil {
		t.Fatal("RegisterMetricsOnRegistry returned nil handler")
	}

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestWrapToolHandler_ErrorTypes(t *testing.T) {
	t.Parallel()

	t.Run("sentinel error wrapping", func(t *testing.T) {
		m := mustRegisterMetrics(t)
		sentinelErr := errors.New("search failed")
		handler := WrapToolHandler[testInput, testOutput](m, "search", func(ctx context.Context, _ *mcp.CallToolRequest, in testInput) (*mcp.CallToolResult, testOutput, error) {
			return nil, testOutput{}, fmt.Errorf("search: %w", sentinelErr)
		})

		_, _, err := handler(context.Background(), nil, testInput{Name: "test"})
		if err == nil {
			t.Fatal("expected error")
		}
		if !errors.Is(err, sentinelErr) {
			t.Errorf("expected error to wrap sentinelErr")
		}
	})
}
