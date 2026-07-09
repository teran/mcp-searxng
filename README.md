# mcp-searxng

Remote MCP (Model Context Protocol) server for [SearXNG](https://docs.searxng.org/).

This server exposes SearXNG search functionality through the MCP protocol using **Streamable HTTP** transport, allowing AI assistants to perform web searches via a SearXNG instance.

## Features

- **Web search** — search the web via SearXNG with full parameter support (categories, language, time range, safe search, pagination)
- **Rich results** — returns search results, direct answers, suggestions, and infoboxes
- **Prometheus metrics** — request counters, duration histograms, and Go runtime metrics on a separate port
- **Rate limiting** — two-tier token-bucket rate limiting (global + per-client IP)
- **Graceful shutdown** — cleanly drains connections on SIGTERM/SIGINT
- **Distroless container** — minimal scratch-based Docker image

## Documentation

- **[SPEC.md](SPEC.md)** — full specification: architecture, middleware chain, error handling, security considerations, development guide
- **[AGENTS.md](AGENTS.md)** — agent roles, package layout, tool-to-agent mapping, conflict resolution

## Quick Start

### 1. Run SearXNG

```bash
docker run -d --name searxng -p 8888:8080 searxng/searxng
```

### 2. Run mcp-searxng

```bash
export SEARXNG_URL=http://localhost:8888
go run ./cmd/server
```

### 3. Configure your MCP client

```json
{
  "mcpServers": {
    "searxng": {
      "url": "http://localhost:8080",
      "type": "streamable-http"
    }
  }
}
```

## Configuration

| Variable               | Required | Default | Description                          |
|------------------------|----------|---------|--------------------------------------|
| `SEARXNG_URL`          | Yes      | —       | Base URL of the SearXNG instance     |
| `LISTEN_ADDR`          | No       | `:8080` | MCP server listen address            |
| `PROMETHEUS_METRICS_ADDR` | No    | `:8081` | Prometheus metrics endpoint address  |
| `RATE_LIMIT_GLOBAL`    | No       | `100`   | Global rate limit (requests/second)  |
| `RATE_LIMIT_PER_CLIENT`| No       | `10`    | Per-client rate limit (requests/sec) |
| `WRITE_TIMEOUT`        | No       | `300s`  | HTTP write timeout                   |

## MCP Tools

### `search`

Search the web using SearXNG.

**Parameters:**
- `query` (required) — the search query
- `categories` (optional) — search categories (general, news, images, etc.)
- `language` (optional) — language code (e.g. en-US, de-DE)
- `page` (optional) — page number (default: 1)
- `time_range` (optional) — day, month, or year
- `safesearch` (optional) — 0=off, 1=moderate, 2=strict

## Build & Release

```bash
# Build
go build -o mcp-searxng ./cmd/server

# Test
go test -count=1 ./...

# Docker
docker build -t ghcr.io/teran/mcp-searxng:latest .

# Release (via goreleaser)
goreleaser release --clean
```

## Docker

```bash
docker pull ghcr.io/teran/mcp-searxng:latest
docker run -e SEARXNG_URL=http://searxng:8888 -p 8080:8080 ghcr.io/teran/mcp-searxng:latest
```

## License

[Apache 2.0](LICENSE)
