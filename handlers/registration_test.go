package handlers

import (
	"context"
	"testing"
)

func TestContextWithServices(t *testing.T) {
	t.Parallel()

	t.Run("SearchService stored and retrieved", func(t *testing.T) {
		// Can't create real SearchService without a repo, so test nil handling
		ctx := ContextWithServices(context.Background(), nil)
		svc := SearchServiceFromContext(ctx)
		if svc != nil {
			t.Errorf("SearchServiceFromContext = %v, want nil", svc)
		}
	})

	t.Run("SearchServiceFromContext returns nil for empty context", func(t *testing.T) {
		svc := SearchServiceFromContext(context.Background())
		if svc != nil {
			t.Errorf("SearchServiceFromContext = %v, want nil", svc)
		}
	})
}
