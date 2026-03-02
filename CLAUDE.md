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
| Run Go tests | `go test ./...` |
| Clean build artifacts | `just clean` |
