// Package e2e contains end-to-end tests (C09GO/T02) that exercise the full
// path from the MCP tool handler to a real SearXNG instance running in Docker,
// using the go-docker-testsuite harness. The tests are build-tagged
// (//go:build e2e) so they are excluded from the default fast unit pass; run
// them explicitly with `make e2e` (requires a running Docker daemon).
package e2e
