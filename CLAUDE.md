# Mailgun Mock API

A mock Mailgun service for dev/testing. Accepts real Mailgun API calls, stores data for inspection, and simulates events — without sending real email. Intended as a drop-in replacement that developers can point their Mailgun client at during local development and CI.

See `implementation_plan/overview.md` for the full breakdown of feature areas.

## Tech Stack

- **Backend:** Go 1.26 (chi router, GORM with SQLite/Postgres)
- **Frontend:** Vue 3 + TypeScript + Vite (in `web/`)
- **Task runner:** [just](https://github.com/casey/just) (see `justfile`)

## Commands

All commands use `just`. Run `just` with no args to list available recipes.

| Task | Command |
|---|---|
| Run Go server (dev) | `just dev` |
| Run Vite dev server with API proxy | `just dev-ui` |
| Build (Vue SPA + Go binary) | `just build` |
| Build and run | `just run` |
| Lint (Go + frontend) | `just lint` |
| Run Go tests (unit + integration) | `just test` |
| Run integration tests only (with filter) | `just integration [Section]` |
| Run Playwright e2e tests | `just test-e2e` |
| Clean build artifacts | `just clean` |

## API Authority: SDK wins over OpenAPI spec

This repo contains `mailgun.yaml` (the upstream Mailgun OpenAPI spec), but the spec is **not authoritative**. For a drop-in mock, the goal is SDK compatibility — real clients use the official `mailgun-go` SDK (and other language SDKs), so the mock must match whatever those SDKs actually accept, even when the published spec disagrees.

**When the spec and an SDK disagree, the SDK is correct.** Treat `mailgun.yaml` as a starting reference only.

**How to verify behavior:**
1. Check the upstream Go SDK in `github.com/mailgun/mailgun-go/v5` — look at what status codes / response shapes it expects (e.g. `ExpectedOneOf=[]int{200, 202, 204}` in `UnexpectedResponseError` means the endpoint returns one of those in production).
2. Cross-check against `mailgun.yaml` for request/response schema hints.
3. If they conflict, match the SDK and leave a comment on the handler explaining why it diverges from the spec.

**Concrete example:** `POST /v3/lists` — the spec says 201, but `mailgun-go/v5` only accepts 200/202/204. The handler returns 200 (see `internal/mailinglist/handlers.go` `CreateList`) with a comment noting the divergence.
