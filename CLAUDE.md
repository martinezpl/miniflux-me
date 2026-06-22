# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
make miniflux          # Build binary for current platform
make test              # Unit tests with race detection and coverage
make lint              # go vet + gofmt + golangci-lint
make run               # Run locally with debug + auto-migration (requires PostgreSQL)
make integration-test  # Full integration tests (requires a running Miniflux instance)
```

Single test: `go test ./internal/some/package/... -run TestFunctionName`

Local PostgreSQL for development:
```bash
docker run --rm --name miniflux-db -p 5432:5432 \
  -e POSTGRES_DB=miniflux2 -e POSTGRES_USER=postgres -e POSTGRES_PASSWORD=postgres \
  postgres
# make run uses DATABASE_URL=postgres://postgres:postgres@localhost/miniflux2?sslmode=disable
```

## Architecture

**Entry point:** `main.go` → `internal/cli.Parse()` → either a one-shot CLI command or `startDaemon()`.

**Daemon** (`internal/cli/daemon.go`) spins up three concurrent subsystems:
1. HTTP server (`internal/http/server`) — routes requests to four handler layers
2. Worker pool (`internal/worker`) — goroutine pool that drains `chan model.Job` to refresh feeds
3. Scheduler (`internal/cli/scheduler.go`) — tickers that enqueue refresh jobs and run cleanup

**HTTP layers** (all registered in `internal/http/server/routes.go`):
- REST API v1 — `internal/api/`
- Fever API — `internal/fever/`
- Google Reader API — `internal/googlereader/`
- Web UI (server-rendered HTML) — `internal/ui/`

**Storage** (`internal/storage/`) is a single `Storage` struct wrapping `*sql.DB`. No ORM — raw SQL only. Every domain area has its own file (`entry.go`, `feed.go`, `user.go`, …). The storage object is created once in `cli` and injected into every layer.

**Feed reading pipeline** (`internal/reader/`):
- `handler/` — orchestrates fetching, parsing, scraping, deduplication
- `fetcher/` — HTTP fetching with ETag/Last-Modified caching
- `parser/` — Atom/RSS/JSON Feed/RDF parsing
- `scraper/` — full-content extraction
- `rewrite/` — content rewriting rules

**Integrations** (`internal/integration/`) — one sub-package per third-party service (Pocket, Pinboard, Slack, Notion, etc.), invoked via `internal/integration/integration.go`.

## Database Migrations

Migrations live in `internal/database/migrations.go` as an ordered slice of `func(tx *sql.Tx) error`. The current schema version equals `len(migrations)` and is stored in the `schema_version` table. To add a migration, append a new function — never modify existing entries.

## Key Conventions

- **PostgreSQL only** — no abstraction layer for other databases.
- **Go `embed`** bundles all static assets (templates, CSS, JS) at compile time; files live in `internal/template/` and `internal/ui/static/`.
- Every source file requires an SPDX license header (`// SPDX-License-Identifier: Apache-2.0`) — the linter enforces this.
- Logging uses `log/slog` with structured key/value pairs; avoid `fmt.Print*` in non-test code.
- The `client/` directory is a public Go SDK for the REST API — its API surface is stable and versioned separately.
- Contributing philosophy: **improving existing features takes priority over adding new ones**. Minimalism is a hard constraint, not a guideline.
