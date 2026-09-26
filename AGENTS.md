# AGENTS.md — Agent Documentation

## Overview

This document describes the agents/assistants involved in the development and operation of `mcp-searxng`. Each agent has a specific role, scope of responsibility, and set of tools available.

## Agent Roles

### User Agent
- **Role**: End-user interacting via an MCP-compatible AI assistant (e.g., Claude, Copilot).
- **Scope**: Sends natural-language queries that get translated into MCP tool calls.
- **No direct access** to SearXNG API.

### MCP Server (`mcp-searxng`)
- **Role**: Mediator between the AI assistant and SearXNG.
- **Scope**: Translates MCP tool invocations into SearXNG Search API calls.
- **Responsible for**: Request routing, response formatting, rate limiting, metrics.

### SearXNG
- **Role**: Meta search engine backend.
- **Scope**: Aggregates search results from multiple engines.
- **API**: HTTP Search API under `/search`.

## Package Layout

| Package / File                              | Purpose                                         |
|---------------------------------------------|-------------------------------------------------|
| `cmd/server/main.go`                        | Entrypoint; launch-mode selection (`-mode`/`MODE`); binds HTTP + stdio servers (`runHTTP`/`runStdio`) and the internal observability listener |
| `config/config.go`                          | Configuration loading (`envconfig` + ozzo-validation) — `MODE`, `INTERNAL_ADDR`, `LOG_LEVEL` (nil when unset) |
| `logging/logging.go`                        | Mode-aware logrus logger construction (L02 enablement + L01 output channel per mode) |
| `logging/slog.go`                           | `slog` adapter that forwards MCP SDK log events into logrus |
| `handlers/middleware.go`                    | Body limit, logging, batch validation middleware (HTTP mode) |
| `handlers/ratelimit.go`                     | Rate limiting middleware (global + per-client) — HTTP mode only |
| `handlers/metrics.go`                       | Prometheus metrics collectors + middleware + `WrapToolHandler` |
| `handlers/accesslog.go`                     | `WrapAccessLog` — transport-agnostic per-tool access log (L08) |
| `handlers/tools.go`                         | MCP tool handler factories + I/O types          |
| `handlers/registration.go`                  | Tool registration via `RegisterToolsWithSource()` |
| `application/service.go`                    | Business logic / use case layer                 |
| `domain/`                                   | Domain models + repository interfaces (ports)   |
| `infrastructure/searxng/client.go`          | SearXNG HTTP API client (adapters)             |
| `infrastructure/searxng/models.go`          | JSON wire models + `toDomain()` conversion      |

## Launch Modes & Logging Contract (L02)

`mcp-searxng` is a **Hybrid** MCP server. One binary selects its transport at startup via the **`-mode http|stdio`** CLI flag or the **`MODE`** env var (flag wins); the default is **`stdio`**.

- **`-mode http`** — Streamable HTTP transport on `LISTEN_ADDR` (default `:8080`); full middleware chain (recovery → metrics → rate limit → body limit → logging) + `GET /healthz`. Logging is **always enabled** at default level `info`, written to **stdout** (12-factor). Rate limiting and body limits apply here.
- **`-mode stdio`** (default) — `mcp.StdioTransport`; **no** HTTP middleware and **no** rate limiting (local trusted process). Logging is enabled **only when `LOG_LEVEL` is set** and is written to the `LOG_FILENAME` file (chmod `0600`), **never** stdout — protecting the JSON-RPC stdio channel. If stdio logging is enabled but `LOG_FILENAME` is empty, output is discarded.

Config env-var changes to keep in mind: **`MODE`** selects the launch mode; **`INTERNAL_ADDR`** (default `:8081`) replaced the deprecated **`PROMETHEUS_METRICS_ADDR`** (honored only when `INTERNAL_ADDR` is unset); **`LOG_LEVEL`** is now `nil` when unset (no longer defaults to `info`).

## Tool-to-Agent Mapping

| Endpoint / Tool | Agent Role | SearXNG Endpoint    |
|-----------------|------------|---------------------|
| `search` (MCP)  | MCP Server | `GET /search`       |
| `search_news` (MCP) | MCP Server | `GET /search` (preset categories=news) |
| `search_images` (MCP) | MCP Server | `GET /search` (preset categories=images) |
| `GET /healthz`  | Devops     | —                   |
| `GET /metrics`  | Devops     | —                   |

## Metrics

The server exposes Prometheus metrics on an internal observability listener (**`INTERNAL_ADDR`**, default `:8081`, bound in both modes; legacy `PROMETHEUS_METRICS_ADDR` honored only when `INTERNAL_ADDR` is unset):

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `mcp_tool_requests_total` | Counter | `{tool, status_class}` | Per-tool request count (tool names are hardcoded at registration) |
| `mcp_tool_duration_seconds` | Histogram | `{tool}` | Per-tool request duration (DefBuckets: .005–10s) |
| `mcp_active_requests` | Gauge | — | Current in-flight MCP requests |
| `go_*` (goroutines, memstats, GC, etc.) | Various | — | Go runtime metrics via `collectors.NewGoCollector()` |

## CI Pipeline

Every commit on any branch is checked by:

1. **golangci-lint** — static analysis with `gosec` enabled.
2. **govulncheck** — vulnerability scan of the dependency graph; any finding fails the build and must be fixed (see the S5 policy in Security Considerations).
3. **go test** — unit tests with coverage profile (uploaded as artifact).
4. **Coverage gate** — total test coverage must be at least **95%** (checked via `go tool cover` after tests).
5. **gremlins unleash** — mutation testing on packages with highest coverage (`handlers`, `application`, `infrastructure/searxng`, `config`). Runs as a **hard gate** — the build fails if mutation efficacy or mutant coverage drop below **80%** (C02/C08GO).

Workflow files:
- `.github/workflows/ci.yml` — lint + govulncheck + test + coverage upload + coverage gate
- `.github/workflows/gremlins.yml` — mutation testing
- `.github/workflows/master.yml` — snapshot Docker image on push to master
- `.github/workflows/release.yml` — goreleaser + multi-arch Docker build

## Development Agents

| Agent       | Responsible For                                    |
|-------------|----------------------------------------------------|
| `architect` | High-level design decisions, system boundaries     |
| `developer` | Writing Go code, implementing tools and client     |
| `qa`        | Writing tests, verifying correctness               |
| `security`  | Reviewing rate limiting, log sanitization          |
| `code-review` | Reviewing merge requests before deployment      |
| `devops`    | CI/CD pipelines, Docker image, deployment, mutation testing |
| `techwriter` | Writing and maintaining technical documentation |

## Conflict Resolution

If multiple agents provide contradictory recommendations:

1. **Security first** — any recommendation that weakens the security boundary is rejected.
2. **SPEC compliance** — the choice that best matches SPEC.md wins.
3. **Simplicity** — prefer the solution with fewer moving parts.
4. **Go idioms** — prefer standard library over external dependencies.

The final decision is recorded in the project TODO list by the orchestrating agent.
