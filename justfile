# Default recipe
default:
    @just --list

# Run Go server (dev mode)
dev:
    go run ./cmd/server

# Run Vite dev server with API proxy
dev-ui:
    cd web && npm run dev

# Build Vue SPA then Go binary
build:
    cd web && npm run build
    mkdir -p internal/server/static
    cp -r web/dist/* internal/server/static/
    go build -o mailgun-mock-api ./cmd/server

# Build and run
run: build
    ./mailgun-mock-api

# Lint Go and frontend code
lint:
    go vet ./...
    cd web && npm run lint

# Run Go tests (unit + integration)
test:
    go test ./...

# Run integration tests only, optionally filter by section name (e.g. just integration Credentials)
integration section="":
    @if [ -z "{{section}}" ]; then \
        go test ./tests/integration/ -v; \
    else \
        go test ./tests/integration/ -run "Test{{section}}" -v; \
    fi

# Run Playwright e2e tests (builds first, starts server automatically)
test-e2e:
    cd web && npm run test:e2e

# Remove build artifacts
clean:
    rm -rf mailgun-mock-api
    rm -rf web/dist
    rm -rf internal/server/static
