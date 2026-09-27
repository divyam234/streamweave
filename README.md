# Media Engine

A greenfield media aggregation platform with a shared typed engine and client adapters for Stremio and Nuvio.

## Stack

- Go 1.26, `net/http`, Chi
- TypeSpec -> OpenAPI 3.1 -> ogen
- PostgreSQL -> pgx/v5 -> sqlc
- Goose SQL migrations
- Bun for JavaScript package management and scripts
- React 19, TypeScript, Vite 8, TanStack Query/Router
- shadcn/ui with Tailwind CSS 4
- Podman Compose for local services

## Code generation

The API contract lives in `api/typespec`.

```sh
make generate
```

Generation performs:

1. TypeSpec -> OpenAPI 3.1
2. OpenAPI -> Go server/types with ogen
3. SQL -> Go query code with sqlc
4. OpenAPI -> browser TypeScript definitions through isolated `bunx openapi-typescript@latest`

`ogen.yaml` disables `ogen/unimplemented`, so every generated handler operation must be implemented explicitly at compile time.

## Install dependencies

```sh
bun install
cd web && bun install
```

Bun lockfiles are the source of truth for JavaScript dependencies.

## Local PostgreSQL with Podman

Start PostgreSQL:

```sh
make db-up
```

Apply migrations:

```sh
make db-migrate
```

The default local DSN is:

```text
postgres://media_engine:media_engine@127.0.0.1:5432/media_engine?sslmode=disable
```

Override it with `DATABASE_URL` when needed.

## Run the backend

```sh
DATABASE_URL='postgres://media_engine:media_engine@127.0.0.1:5432/media_engine?sslmode=disable' \
  go run ./cmd/server
```

For trusted local development only, private/LAN remote-addon endpoints can be enabled with:

```sh
ALLOW_PRIVATE_PROVIDER_ENDPOINTS=true
```

Private endpoints are rejected by default.

The server can run without `DATABASE_URL`; persistence-backed control endpoints return empty lists and status reports the database as disabled.

## Run the UI

```sh
bun run web:dev
```

Vite proxies `/api` and `/addon` to the Go server during development.

## Protocol endpoints

Initial Stremio-compatible endpoints:

```text
/addon/{installationID}/manifest.json
/addon/{installationID}/stream/{type}/{id}.json
```

Nuvio is a first-class client mode over the Stremio addon protocol. Nuvio manifests advertise `tt` and `tmdb` ID prefixes plus P2P support according to the installation resolution mode. Use `client` resolution mode when Nuvio itself should resolve raw torrent hashes through its native TorBox/Premiumize integration; use `server` when Media Engine must return resolved HTTP links only; `hybrid` resolves what it can server-side while retaining raw torrent fallback. Each installation has a distinct manifest ID so multiple Media Engine configurations can coexist in Nuvio.

## Remote addon presets

Remote Stremio addons remain ordinary `remote-addon` providers internally. The control API and UI provide convenience presets for:

- Torrentio — `https://torrentio.strem.fun`
- Comet — `https://comet.elfhosted.com`
- MediaFusion — `https://mediafusion.elfhosted.com`

A preset supplies the public default endpoint when no endpoint is provided. A custom/configured addon URL always wins over the preset.

Configured manifest URLs can be pasted directly, for example:

```text
https://example-addon/config-token/manifest.json
```

The backend validates the URL and stores the normalized addon base path:

```text
https://example-addon/config-token
```

Stream discovery then uses the standard Stremio `/stream/{type}/{id}.json` route and feeds returned torrents/direct streams through the same normalization, dedupe, ranking, cache, and resolver pipeline as native providers.

## Aggregation pipeline

Provider results pass through a deterministic engine pipeline before protocol adaptation:

1. bounded concurrent provider discovery
2. 30-second bounded in-memory discovery cache with singleflight request coalescing
3. release metadata enrichment from titles and filenames (resolution, codec, size)
4. torrent/direct URL deduplication with metadata merging
5. deterministic scoring and ordering
6. optional resolver execution
7. a second ranking pass after resolver metadata is known

The default ranking strongly prefers cached and directly playable results, then higher resolution, efficient codecs, and seed availability. Ties are broken by stable source and candidate IDs rather than provider response timing.

## AllDebrid resolution

AllDebrid is available as a resolver account. Resolver API keys are never returned by the control API and are encrypted before persistence using AES-256-GCM.

Set a stable 32-byte master key encoded as 64 hex characters:

```sh
MASTER_KEY='<64 hex characters>'
```

Do not rotate or lose this key without re-encrypting stored resolver credentials.

Create an AllDebrid account from the Resolvers tab, then assign it to an installation. Resolution modes are:

- `client`: return original torrent/direct candidates without server-side debrid resolution.
- `server`: return only candidates that resolve to playable server-side links.
- `hybrid`: prefer resolved links while retaining original torrent candidates when resolution is unavailable.

The resolver supports ready magnets, nested magnet file trees, explicit torrent file indexes, largest-video fallback selection, link unlocking, and detection of delayed-link responses. Hybrid mode preserves the original torrent candidate when immediate resolution is unavailable.

For integration tests only, `ALLDEBRID_BASE_URL` can point the client at a local mock API. Production should use the default HTTPS AllDebrid API endpoint.

## Production deployment

The supported production deployment is `compose.prod.yaml`. It runs PostgreSQL on an internal network and a non-root/read-only Go application container with the Vite UI embedded in the server binary. Media Engine serves its own HTTP interface and has no domain, TLS-terminator, or reverse-proxy configuration.

Production mode is intentionally fail-closed. `PRODUCTION=true` requires PostgreSQL, a valid master encryption key, and an admin token of at least 32 characters. Private/LAN provider endpoints cannot be enabled in production.

Create the production environment file:

```sh
cp .env.production.example .env.production
openssl rand -hex 32       # use for POSTGRES_PASSWORD
openssl rand -hex 32       # use for MASTER_KEY
openssl rand -base64 48    # use for ADMIN_TOKEN
```

Set `DATABASE_URL` in `.env.production` explicitly. If you choose a password with URL-special characters instead of the recommended hex password, URL-encode it before embedding it in the PostgreSQL DSN.

The production Compose stack publishes the embedded Go application on `APP_BIND_ADDR:APP_PORT` (`127.0.0.1:8080` by default). Change those values based only on how you want the application listener exposed. Media Engine does not inspect or trust `X-Forwarded-For`, `X-Real-IP`, or `X-Forwarded-Proto`; rate limiting uses the direct TCP peer it sees.

If `10.199.17.0/24` conflicts with another Podman allocation on the host, change `BACKEND_SUBNET`, `POSTGRES_IP`, `MIGRATE_IP`, and `APP_IP` together in `.env.production`.

The Vite production bundle is embedded into the Go server binary with `go:embed`. The Go process serves the UI, API, auth endpoints, addon protocol, and health endpoints from the same HTTP listener.

Validate and deploy:

```sh
make prod-config
make prod-up
```

The migration container runs Goose before the application starts. Health endpoints are public and deliberately minimal:

```text
/healthz
/readyz
```

The browser control plane exchanges `ADMIN_TOKEN` at `POST /auth/login` for an HttpOnly, SameSite=Strict session cookie. The admin token is not stored in `sessionStorage` or `localStorage`. State-changing control requests additionally require the CSRF protection header emitted by the UI client.

Installations use opaque 48-character public tokens instead of database UUIDs. The addon URL shown in the UI has the form:

```text
<scheme>://<host>/addon/<installation-token>/manifest.json
```

Unknown or disabled installation tokens return 404. Treat an installation URL as a capability URL: anyone who has it can use that installation until it is disabled or rotated.

Public request hardening includes bounded request bodies, per-IP fixed-window rate limits, 8-second provider timeouts, 18-second resolver timeouts, a maximum of 100 returned candidates, and no more than 20 torrent/NZB resolver targets per lookup. Outbound HTTP uses DNS-aware SSRF protection and rejects private, loopback, link-local, CGNAT, benchmark, documentation, and other non-public address ranges as well as HTTPS-to-HTTP redirect downgrades.

Before upgrades, back up the PostgreSQL volume and retain the exact `MASTER_KEY`; losing that key makes encrypted provider/resolver credentials unrecoverable.


Back up PostgreSQL before upgrades and test restores regularly:

```sh
podman compose --env-file .env.production -f compose.prod.yaml \
  exec -T postgres pg_dump -U media_engine -Fc media_engine > media-engine.dump
```

Keep database backups and the matching `MASTER_KEY` in separate protected locations. A database backup without the key cannot recover encrypted provider/resolver credentials.

This Compose topology is intended for a single application replica. The discovery cache and IP rate limiter are in-process. Before running multiple application replicas, move those controls to a shared service or enforce equivalent limits at the load balancer/WAF; otherwise each replica maintains independent limits and cache state.

For a public service, set an abuse/contact policy appropriate for your jurisdiction and only expose providers/content sources you are authorized to operate. The application does not make legal or licensing determinations for indexed content.

## Validation

```sh
make check
```

This regenerates contracts/code, runs Go tests/vet/build, lints and builds the web app, and validates the Podman Compose configuration.
