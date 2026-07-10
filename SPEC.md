# MCP SearXNG — Specification

## Overview

An MCP (Model Context Protocol) server for [SearXNG](https://docs.searxng.org/).  
This server exposes SearXNG search functionality through the MCP protocol using **Streamable HTTP** transport (remote mode), allowing AI assistants to perform web searches via a SearXNG instance.

## Key Differentiators

- **Remote (HTTP) transport** — uses MCP Streamable HTTP protocol; not stdio-bound.
- **No authentication** — the server operates as a public MCP server without per-request authentication. SearXNG API credentials are handled at the network level (VPN, firewall, or reverse proxy).
- **Full query parameter support** — exposes SearXNG search parameters including categories, language, time range, safe search, and pagination.

## Architecture

```
┌──────────────────┐      MCP (Streamable HTTP)      ┌───────────────────────┐
│  MCP Client      │  ◄──────────────────────────►   │  mcp-searxng          │
│  (AI Assistant)  │                                  │  (Go server)          │
└──────────────────┘                                   └──────┬────────────────
                                                               │ HTTP (no auth)
                                                               ▼
                                                    ┌───────────────────────┐
                                                    │  SearXNG Instance     │
                                                    │  Search API           │
                                                    └───────────────────────┘
```

## Technology Stack

| Component         | Choice                                                          |
|-------------------|-----------------------------------------------------------------|
| Language          | Go                                                              |
| MCP SDK           | `github.com/modelcontextprotocol/go-sdk`                        |
| Transport         | Streamable HTTP (MCP spec 2025-03-26+, remote-capable)          |
| HTTP Router       | `net/http` standard library + middleware pattern                |
| Tool Registration | `handlers/registration.go` — `RegisterTools(srv, metrics, svc)` |
| DI Pattern        | Explicit constructor injection (no context-based lookups)       |
| Metrics           | Prometheus (Go runtime + custom MCP metrics) on port 8081       |

## Configuration (Environment Variables)

| Variable               | Required | Default | Description                          |
|------------------------|----------|---------|--------------------------------------|
| `SEARXNG_URL`          | Yes      | —       | Base URL of the SearXNG instance (e.g. `http://searxng:8888`) |
| `LISTEN_ADDR`          | No       | `:8080` | TCP address to listen on             |
| `PROMETHEUS_METRICS_ADDR` | No    | `:8081` | TCP address for the Prometheus `/metrics` endpoint (separate HTTP server, no auth) |
| `RATE_LIMIT_GLOBAL`    | No       | `100`   | Global rate limit (requests/second)  |
| `RATE_LIMIT_PER_CLIENT`| No       | `10`    | Per-client IP rate limit (requests/second) |
| `WRITE_TIMEOUT`        | No       | `300s`  | HTTP write timeout (Go duration, e.g. `300s`, `5m`) |

The MCP server listens on the `/` HTTP path via the Streamable HTTP handler.

## MCP Tools

### 1. `search`

Search the web using SearXNG. Returns search results, answers, suggestions, and infoboxes.

**Input**:

| Parameter    | Type     | Required | Description                                        |
|--------------|----------|----------|----------------------------------------------------|
| `query`      | string   | yes      | The search query                                   |
| `categories` | []string | no       | Active search categories (e.g. general, news, images, videos, files, social, music) |
| `language`   | string   | no       | Code of the language (e.g. en-US, de-DE, ru-RU)   |
| `page`       | int      | no       | Search page number (default: 1)                    |
| `time_range` | string   | no       | Time range: `day`, `month`, `year`                 |
| `safesearch` | int      | no       | Safe search filter: 0=off, 1=moderate, 2=strict    |

**Output**:

| Field            | Type       | Description                                  |
|------------------|------------|----------------------------------------------|
| `query`          | string     | Original search query                        |
| `results`        | []object   | Search result items                          |
| `results[].title` | string    | Result title                                 |
| `results[].url`   | string    | Result URL                                   |
| `results[].content` | string  | Result snippet/content                       |
| `results[].engine` | string   | Search engine that provided this result      |
| `results[].category` | string | Result category                              |
| `results[].publishedDate` | string | Publication date (ISO 8601)           |
| `results[].formattedDate` | string  | Formatted date (e.g. "10 Jul 2026")          |
| `results[].img_src` | string  | Image source URL (for image results)         |
| `results[].source` | string   | Source domain                                |
| `answers`        | []string   | Direct answers (e.g. calculations)           |
| `suggestions`    | []string   | Search suggestions                           |
| `infoboxes`      | []object   | Infobox data                                 |
| `infoboxes[].id` | string    | Infobox identifier                           |
| `infoboxes[].url` | string   | Infobox source URL                           |
| `infoboxes[].content` | string | Infobox content                          |
| `infoboxes[].img_src` | string | Image source URL                         |
| `infoboxes[].engine` | string  | Engine that provided the infobox             |
| `infoboxes[].attributes` | []object | Infobox attributes (key/value pairs)    |
| `infoboxes[].attributes[].key` | string | Attribute name                       |
| `infoboxes[].attributes[].value` | string | Attribute value                     |
| `infoboxes[].urls` | []object   | Related URLs                                |
| `infoboxes[].urls[].title` | string | Link title                             |
| `infoboxes[].urls[].url` | string   | Link URL                               |
| `number_of_results` | int    | Total number of results                      |

### 2. `search_news`

Search for news. Convenience wrapper around `search` with presets: `categories=["news"]`, `time_range="day"`.

**Input**:

| Parameter    | Type     | Required | Description                                        |
|--------------|----------|----------|----------------------------------------------------|
| `query`      | string   | yes      | The search query                                   |
| `language`   | string   | no       | Code of the language (e.g. en-US, de-DE, ru-RU)   |
| `page`       | int      | no       | Search page number (default: 1)                    |
| `safesearch` | int      | no       | Safe search filter: 0=off, 1=moderate, 2=strict    |

**Output**: Same as `search`.

---

### 3. `search_images`

Search for images. Convenience wrapper around `search` with presets: `categories=["images"]`.

**Input**:

| Parameter    | Type     | Required | Description                                        |
|--------------|----------|----------|----------------------------------------------------|
| `query`      | string   | yes      | The search query                                   |
| `language`   | string   | no       | Code of the language (e.g. en-US, de-DE, ru-RU)   |
| `page`       | int      | no       | Search page number (default: 1)                    |
| `safesearch` | int      | no       | Safe search filter: 0=off, 1=moderate, 2=strict    |

**Output**: Same as `search`.

---

## Middleware Chain

The server applies five middleware layers to every HTTP request before reaching the MCP Streamable HTTP handler, executed in this order (outermost first). The search service is injected directly into tool handlers at registration time (via `RegisterTools`), so no service-injection middleware is needed.

### 1. RecoveryMiddleware (`handlers/middleware.go`)

Catches panics in any downstream handler via `defer recover()`, logs the panic, and returns **500 Internal Server Error**.

### 2. MetricsMiddleware (`handlers/metrics.go`)

Tracks the number of in-flight requests using the `mcp_active_requests` Prometheus gauge.

Custom Prometheus metrics exposed on the metrics server:

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `mcp_tool_requests_total` | Counter | `{tool, status_class}` | Per-tool request count |
| `mcp_tool_duration_seconds` | Histogram | `{tool}` | Per-tool request duration (DefBuckets: .005–10s) |
| `mcp_active_requests` | Gauge | — | Current in-flight requests |

### 3. RateLimitMiddleware (`handlers/ratelimit.go`)

Implements two-tier token-bucket rate limiting using `golang.org/x/time/rate`:
- **Global limit** (default 100 rps) — prevents overall request flooding.
- **Per-client limit** (default 10 rps) — per-IP limiting.
- **Burst** — each limiter has a burst capacity of 2× its rate limit, allowing short traffic spikes.
- **Background eviction** — stale per-client limiters are evicted every 10 minutes (TTL: 30 min). The eviction goroutine is stopped during graceful shutdown via the returned stop function.

Returns **429 Too Many Requests** when the limit is exceeded.

### 4. BodyLimitMiddleware (`handlers/middleware.go`)

Limits the request body size to 1 MB using `http.MaxBytesReader`.

### 5. LoggingMiddleware (`handlers/middleware.go`)

Reads and buffers the request body to parse the JSON-RPC method name, validates batch size (max 100), and logs a single line at INFO level with the `mcp_log` prefix.

## Error Handling

### HTTP Level (Middleware)

| Status | Cause | Source |
|--------|-------|--------|
| **429 Too Many Requests** | Request frequency exceeds rate limit | `RateLimitMiddleware` |
| **400 Bad Request** | Batch JSON-RPC request exceeds maximum size (100) | `LoggingMiddleware` |

### MCP Level (Tool Handlers)

| Scenario | MCP Error | Cause |
|----------|-----------|-------|
| Search failed | `isError: true` | SearXNG returns an error or is unavailable |
| Invalid parameters | `isError: true` | Missing required `query` parameter |

## Security Considerations

- The server has **no authentication** — it should be deployed behind TLS and network-level access controls in production.
- Global (100 rps) and per-client (10 rps) rate limiting via `RateLimitMiddleware`.
- Batch JSON-RPC requests are limited to 100 items per batch to prevent amplification attacks.
- All log strings are sanitized — control characters are stripped to prevent log injection.
- Credentials in the SearXNG URL are redacted before logging via `url.Redacted()`.
- HTTP redirects are disabled (`CheckRedirect: http.ErrUseLastResponse`).
- Response bodies from SearXNG are limited to 10 MB via `io.LimitReader`.
- **Prometheus metrics** are exposed on a separate HTTP server (default `:8081`) with no built-in authentication.

## Development

### Prerequisites

- Go 1.26+
- golangci-lint (for linting)
- goreleaser (for building/releasing)
- gremlins (for mutation testing, optional)

### Building

```bash
# Quick build
go build -o mcp-searxng ./cmd/server

# Release build using goreleaser
goreleaser build --snapshot --clean

# Build and push Docker image
docker buildx build --platform linux/amd64,linux/arm64 \
  -t ghcr.io/teran/mcp-searxng:latest --push .
```

### Quality gates (CI pipeline)

Every commit on any branch is checked by:

1. **golangci-lint** — static analysis with `gosec` enabled.
2. **go test** — unit tests with coverage profile.
3. **Coverage gate** — total test coverage must be at least **85%** (checked via `go tool cover` after tests).
4. **gremlins unleash** — mutation testing (informational, does not block).

### Linting

```bash
golangci-lint run ./...
```

### Running tests

Run tests with the race detector enabled:

```bash
go test -race -count=1 ./...
```

### Test coverage

The CI enforces a minimum **85% total coverage** gate. To check coverage locally:

```bash
go test -race -coverprofile=coverage.out -count=1 ./...
go tool cover -func=coverage.out
go tool cover -func=coverage.out | grep '^total:' | awk '{print $3}'  # coverage percentage
```

### Mutation testing (gremlins)

```bash
go install github.com/go-gremlins/gremlins/cmd/gremlins@latest
gremlins unleash handlers application infrastructure/searxng config
```

### Adding a new tool

1. Define input/output types in `handlers/tools.go`
2. Write the handler factory function in `handlers/tools.go`
3. Register the tool via `RegisterTools()` in `handlers/registration.go` (pass the search service explicitly as a parameter)
4. If a new domain entity is needed, define it in `domain/` and add a repository interface
5. If a new service or dependency is needed, wire it in `Run()` (`cmd/server/main.go`) and pass it to `RegisterTools`

### Dependency Management

Dependencies are updated automatically via [Dependabot](https://docs.github.com/code-security/dependabot) (`.github/dependabot.yml`):
- Go module dependencies — checked weekly
- Docker base image (`golang:1.26-alpine`) — checked weekly
- GitHub Actions — checked weekly
