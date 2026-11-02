package main

import (
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/teran/mcp-searxng/handlers"
)

// TestServerSelfDescribing verifies M08/M09: the server exposes server-level
// Instructions and a resources capability, and the mcp-searxng://guidance/*
// resource is readable over the protocol while unknown URIs are not-found
// (N37).
func TestServerSelfDescribing(t *testing.T) {
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	metricsLn := mustListen(t)

	cfg := newStdioTestConfig(metricsLn)

	ctx := t.Context()

	errCh := make(chan error, 1)
	go func() {
		errCh <- runStdio(ctx, cfg, testLogger(), metricsLn, serverTransport)
	}()
	waitForServer(t, "http://"+metricsLn.Addr().String()+"/healthz", 3*time.Second)

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v1.0.0"}, nil)
	session := connectStdioClient(t, client, clientTransport)

	// M08: server-level instructions are non-empty and reference the resources.
	if handlers.ServerInstructions == "" {
		t.Error("ServerInstructions is empty, want a usage guide")
	}
	if !strings.Contains(handlers.ServerInstructions, "guidance") {
		t.Error("ServerInstructions does not reference the guidance resources")
	}

	// M09: the guidance overview resource is readable and returns markdown.
	res, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "mcp-searxng://guidance/overview"})
	if err != nil {
		t.Fatalf("ReadResource(overview): %v", err)
	}
	if res == nil || len(res.Contents) == 0 {
		t.Fatal("ReadResource returned no contents")
	}
	content := res.Contents[0]
	if content.MIMEType != "text/markdown" {
		t.Errorf("MIMEType = %q, want text/markdown", content.MIMEType)
	}
	if !strings.Contains(content.Text, "mcp-searxng") {
		t.Error("guidance content missing server name")
	}

	// M09/N37: an unknown resource URI must be not-found (error), not a result.
	if _, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "mcp-searxng://guidance/unknown"}); err == nil {
		t.Error("expected an error for an unknown resource URI, got nil")
	}

	if err := session.Close(); err != nil {
		t.Fatalf("session.Close: %v", err)
	}

	assertRunReturns(t, errCh)
}

// assertRunReturns waits for runStdio to return nil after the client session
// is closed.
func assertRunReturns(t *testing.T, errCh <-chan error) {
	t.Helper()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("runStdio returned unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for runStdio to return after session close")
	}
}
