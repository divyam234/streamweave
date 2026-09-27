DATABASE_URL ?= postgres://media_engine:media_engine@127.0.0.1:5432/media_engine?sslmode=disable
PROD_ENV ?= .env.production

.PHONY: generate test build embed-ui web-build check db-up db-down db-migrate db-status prod-config prod-up prod-down prod-logs

generate:
	bun run api:generate
	go tool ogen --config ogen.yaml --target internal/api/gen -package gen --clean api/openapi/openapi.yaml
	go tool sqlc generate
	bun run web:types

test:
	go test ./...

embed-ui:
	cd web && bun run build
	rm -rf internal/webui/dist
	mkdir -p internal/webui/dist
	cp -R web/dist/. internal/webui/dist/

build: embed-ui
	go build -tags ui ./cmd/server
	go build ./cmd/migrate

web-build: embed-ui

check: generate embed-ui
	go test ./...
	go test -tags ui ./internal/webui ./internal/httpserver
	go vet ./...
	go build ./...
	go build -tags ui ./cmd/server
	cd web && bun run lint
	podman compose config >/dev/null
	POSTGRES_PASSWORD=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
	DATABASE_URL='postgres://media_engine:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa@postgres:5432/media_engine?sslmode=disable' \
	MASTER_KEY=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb \
	ADMIN_TOKEN=cccccccccccccccccccccccccccccccccccccccccccccccc \
	podman compose -f compose.prod.yaml config >/dev/null
	test ! -e internal/api/gen/oas_unimplemented_gen.go

db-up:
	podman compose up -d postgres

db-down:
	podman compose down

db-migrate:
	go tool goose -dir db/migrations postgres "$(DATABASE_URL)" up

db-status:
	go tool goose -dir db/migrations postgres "$(DATABASE_URL)" status

prod-config:
	podman compose --env-file "$(PROD_ENV)" -f compose.prod.yaml config

prod-up:
	podman compose --env-file "$(PROD_ENV)" -f compose.prod.yaml up -d --build

prod-down:
	podman compose --env-file "$(PROD_ENV)" -f compose.prod.yaml down

prod-logs:
	podman compose --env-file "$(PROD_ENV)" -f compose.prod.yaml logs -f app
