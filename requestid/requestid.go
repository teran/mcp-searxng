// Package requestid provides context-scoped request correlation ids for the
// access log and outbound HTTP calls (L09/L04GO). A generated id is carried in
// the context, surfaced on every access-log line, and forwarded upstream as
// X-Request-ID.
package requestid

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

type key struct{}

// requestIDBytes is the number of random bytes in a generated request id (16
// bytes → 32 hex characters).
const requestIDBytes = 16

// With returns a copy of ctx carrying the given request id.
func With(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, key{}, id)
}

// From returns the request id carried in ctx, or an empty string if none.
func From(ctx context.Context) string {
	if id, ok := ctx.Value(key{}).(string); ok {
		return id
	}

	return ""
}

// New returns a random 32-character hex request id.
func New() string {
	b := make([]byte, requestIDBytes)
	_, _ = rand.Read(b) // crypto/rand.Read never fails

	return hex.EncodeToString(b)
}
