FROM oven/bun:1.4.2-alpine AS web-builder

WORKDIR /src/web
COPY web/package.json web/bun.lock ./
RUN bun install --frozen-lockfile
COPY web/ ./
RUN bun run build

FROM golang:1.26-alpine AS builder

WORKDIR /src
RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
COPY . .
COPY --from=web-builder /src/web/dist /src/internal/webui/dist
RUN CGO_ENABLED=0 go build -tags ui -trimpath -ldflags="-s -w" -o /out/streamweave ./cmd/server
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/streamweave-migrate ./cmd/migrate

FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata wget \
    && addgroup -S streamweave \
    && adduser -S -G streamweave -h /app streamweave

WORKDIR /app
COPY --from=builder /out/streamweave /usr/local/bin/streamweave
COPY --from=builder /out/streamweave-migrate /usr/local/bin/streamweave-migrate
COPY db/migrations /app/db/migrations

USER streamweave
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/streamweave"]
