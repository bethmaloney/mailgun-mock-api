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

# Remove build artifacts
clean:
    rm -rf mailgun-mock-api
    rm -rf web/dist
    rm -rf internal/server/static
