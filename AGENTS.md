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
| `cmd/server/main.go`                        | Entrypoint, HTTP server, middleware wiring      |
| `config/config.go`                          | Configuration loading (`envconfig` + ozzo-validation) |
| `handlers/middleware.go`                    | Body limit, logging, batch validation middleware |
| `handlers/ratelimit.go`                     | Rate limiting middleware (global + per-client)  |
| `handlers/metrics.go`                       | Prometheus metrics collectors + middleware + `WrapToolHandler` |
| `handlers/tools.go`                         | MCP tool handler factories + I/O types          |
| `handlers/registration.go`                  | Tool registration via `RegisterTools()`         |
| `application/service.go`                    | Business logic / use case layer                 |
| `domain/`                                   | Domain models + repository interfaces (ports)   |
| `infrastructure/searxng/client.go`          | SearXNG HTTP API client (adapters)             |
| `infrastructure/searxng/models.go`          | JSON wire models + `toDomain()` conversion      |

## Tool-to-Agent Mapping

| Endpoint / Tool | Agent Role | SearXNG Endpoint    |
|-----------------|------------|---------------------|
| `search` (MCP)  | MCP Server | `GET /search`       |
| `search_news` (MCP) | MCP Server | `GET /search` (preset categories=news) |
| `search_images` (MCP) | MCP Server | `GET /search` (preset categories=images) |
| `GET /healthz`  | Devops     | —                   |
| `GET /metrics`  | Devops     | —                   |

## Metrics

The server exposes Prometheus metrics on a separate HTTP server (default port `:8081`, configurable via `PROMETHEUS_METRICS_ADDR`):

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `mcp_tool_requests_total` | Counter | `{tool, status_class}` | Per-tool request count (tool names are hardcoded at registration) |
| `mcp_tool_duration_seconds` | Histogram | `{tool}` | Per-tool request duration (DefBuckets: .005–10s) |
| `mcp_active_requests` | Gauge | — | Current in-flight MCP requests |
| `go_*` (goroutines, memstats, GC, etc.) | Various | — | Go runtime metrics via `collectors.NewGoCollector()` |

## CI Pipeline

Every commit on any branch is checked by:

1. **golangci-lint** — static analysis with `gosec` enabled.
2. **go test** — unit tests with coverage profile (uploaded as artifact).
3. **Coverage gate** — total test coverage must be at least **85%** (checked via `go tool cover` after tests).
4. **gremlins unleash** — mutation testing on packages with highest coverage (`handlers`, `application`, `infrastructure/searxng`, `config`). Runs as `continue-on-error` — informational only, does not block the PR.

Workflow files:
- `.github/workflows/ci.yml` — lint + test + coverage upload + coverage gate
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
