# Stage 1: Build Vue SPA
FROM docker.io/node:22-alpine AS frontend
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Stage 2: Build Go binary
FROM docker.io/golang:1.26-alpine AS backend
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/web/dist internal/server/static/
RUN CGO_ENABLED=0 go build -o mailgun-mock-api ./cmd/server

# Stage 3: Final image
FROM docker.io/alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=backend /app/mailgun-mock-api /usr/local/bin/mailgun-mock-api
EXPOSE 8025
HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
  CMD wget -qO- http://localhost:8025/mock/health || exit 1
ENTRYPOINT ["mailgun-mock-api"]
