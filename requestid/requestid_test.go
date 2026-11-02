// Package requestid provides context-scoped request correlation ids for the
// access log and outbound HTTP calls (L09/L04GO).
package requestid

import (
	"context"
	"testing"
)

func TestNew(t *testing.T) {
	t.Parallel()

	a := New()
	b := New()

	if a == "" {
		t.Error("New() returned an empty id")
	}
	if len(a) != 32 {
		t.Errorf("New() length = %d, want 32", len(a))
	}
	if a == b {
		t.Error("two New() calls returned the same id")
	}
}

func TestWithFrom(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	if got := From(ctx); got != "" {
		t.Errorf("From(background) = %q, want empty", got)
	}

	ctx = With(ctx, "abc123")
	if got := From(ctx); got != "abc123" {
		t.Errorf("From = %q, want abc123", got)
	}
}
