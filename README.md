# Resource Planning System

A generic open-source system for planning, requesting, allocating, and returning rentable or shared resources.

The project is intentionally domain-neutral. It can be used for tools, measuring equipment, calibration standards, devices, or other shared resources. Domain-specific rules are intended to be added later through extensions or additional adapters.

## Project Status

The project is in an early (alpha) development stage. The domain layer, the PostgreSQL persistence adapters, and a first slice of the HTTP API are implemented and tested; identity/authorization, offline sync, and the client applications are not.

### Implemented and tested

- Go module and hexagonal project structure
- Pure domain layer: entities, state machines (including the direct-transfer path), version fields for optimistic concurrency, domain-specific errors, `AuditEvent` as a pure data structure
- Application layer: ports (`UnitOfWork`, repositories, `AuditWriter`, `IdempotencyStore`, `EventPublisher`) and use cases — `CreateRequest`, `GetRequest`, `RequestReturn`, `TransferResource` are wired to HTTP; `MarkAllocationShippedBack`, `UpdateExecutionState`, and `ReactivateResource` are implemented and unit-tested but not yet exposed through an endpoint
- PostgreSQL adapters: connection pool, Unit of Work, transactional repositories for requests/resources/allocations, `AuditWriter`, `IdempotencyStore`, and an embedded migration runner (`migrations/`, five migrations, optionally run at startup via `RUN_MIGRATIONS`)
- HTTP API on Go's standard `net/http` `ServeMux`: `POST /api/v1/requests`, `GET /api/v1/requests/{id}`, `POST /api/v1/allocations/{id}/return-request`, `POST /api/v1/resources/{id}/transfer`, and `GET /api/v1/events`, plus the public `GET /healthz` liveness endpoint
- Server-Sent Events: an in-memory broker (`internal/adapters/sse`) publishes typed events from the write use cases; `GET /api/v1/events` streams them with a dispatcher/technician role filter
- Bearer-token authentication middleware and per-route role authorization (`requireRoles`)
- Test suite: domain unit tests, use-case unit tests with in-memory fakes, PostgreSQL integration tests, HTTP handler tests, and an end-to-end test through the real HTTP → use case → repository → PostgreSQL stack

### Implemented as a transitional solution

- **Authentication currently uses `StaticTokenAuthenticator`** (`internal/adapters/auth`), configured via the `AUTH_STATIC_TOKENS` environment variable. This stands in for the session/token-based authentication described in `systemdesign.md` §7 and is accepted only when `APP_ENV=dev`; any other value is a fatal startup error. See the tech-debt list in `status.md` for the replacement plan.

### Ports without a connected endpoint

- `IdempotencyStore` has a port and a PostgreSQL adapter, but no use case calls it yet — idempotent outcome replay for offline sync is not enforced.
- `ResourceClass` exists as a domain type with a database table, but has no repository port or adapter — resource classes cannot yet be created or queried through the application.

### Not yet implemented

- Session-based authentication (login/refresh/logout), Argon2id password hashing, the roles-and-permissions model, and admin user-management endpoints (see `systemdesign.md` §5–§8 and §10.1)
- Offline synchronization with idempotent outcome replay
- Web dashboard
- Technician client

## Deployment Strategy

For binding rules on automatic database migrations at server startup (`RUN_MIGRATIONS`), see `docs/deployment.md`.

In production the backend must not be exposed directly: it binds to an internal interface only, behind a TLS-terminating reverse proxy (SEC-27, see `docs/deployment.md`). The local Docker Compose setup in this repository does not include such a proxy — it publishes the backend directly on `127.0.0.1:8080` for development convenience only.

## Core Features

The planned system includes:

- Resource classes and concrete resource instances
- Resource requests created by technicians
- Allocation of concrete resources by dispatch
- Individual planning periods for each resource allocation
- Direct site-to-site transfer of resources without a warehouse return, restricted to the dispatcher
- Explicit return and recall workflows
- Inspection of returned resources
- Blocking defective or unavailable resources
- Complete audit trail for status changes
- Offline-capable technician clients
- Conflict resolution based on the "dispatcher wins" policy
- Idempotent synchronization with outcome replay
- Live updates for the dispatch dashboard using SSE

## Architecture

The backend follows a strict hexagonal architecture:

```text
cmd/server/
internal/
  domain/
  application/
    ports/
    usecases/
  adapters/
    postgres/
    http/
    auth/
    sse/
migrations/
docs/
scripts/
```

### Domain Layer

`internal/domain` contains only:

- Pure Go data types
- Status values
- State machines
- Business validations
- Domain-specific errors

The domain layer has no dependencies on:

- PostgreSQL
- HTTP
- JSON DTOs
- Frameworks
- ORM libraries
- External packages

### Application Layer

`internal/application/ports` defines the interfaces (`UnitOfWork`, `RequestRepository`, `ResourceRepository`, `AllocationRepository`, `AuditWriter`, `IdempotencyStore`, `EventPublisher`, `Authenticator`). `internal/application/usecases` implements the orchestration logic against those ports only. State-changing use cases run through the `UnitOfWork` so that the state change and its `AuditEvent` are written in the same database transaction.

### Adapters

Implemented adapters:

- `postgres`: connection pool, Unit of Work, and transactional repositories using `pgx/v5`
- `http` (package `httpadapter`): REST handlers and the SSE stream endpoint on Go's standard `net/http`
- `auth`: `StaticTokenAuthenticator`, a transitional bearer-token authenticator (see Project Status)
- `sse`: in-memory event broker

## Technology Stack

- Go 1.26 or newer (see `go.mod`)
- Go standard library
- PostgreSQL
- `github.com/jackc/pgx/v5`
- Docker and Docker Compose
- Apache License 2.0

Not allowed:

- Gin
- Echo
- Fiber
- Gorilla Mux
- GORM
- ent
- Other ORMs
- GPL or AGPL dependencies

## Requirements

- Go 1.26 or newer
- Git
- Docker Desktop with Docker Compose (to run PostgreSQL and the server; not required to build or run unit tests)
- PowerShell, Bash, or another terminal
- GoLand is optional

Check the installed versions:

```powershell
go version
git --version
docker compose version
```

## Installation

### 1. Clone the Repository

If the repository is already hosted on GitHub:

```powershell
git clone <REPOSITORY-URL>
cd bestelltool_be
```

For an existing local project, change directly to the project directory.

### 2. Verify the Go Module

```powershell
go env GOMOD
```

The output must point to the `go.mod` file in the project directory.

Example:

```text
C:\Users\arnol\source\repos\bestelltool_be\go.mod
```

### 3. Resolve Dependencies

The project depends on `github.com/jackc/pgx/v5` (PostgreSQL driver) and its transitive dependencies, pinned in `go.sum`. Download them into the local module cache:

```powershell
go mod download
```

### 4. Configure the Environment

Copy the example environment file and adjust it for local use:

```powershell
copy .env.example .env
```

`docker compose` reads `.env` automatically. The example includes `AUTH_STATIC_TOKENS` for the transitional static-token mode (format `token:user-id:role,...`). This variable is accepted exclusively when `APP_ENV=dev`; setting it in `staging` or `prod` causes a fatal startup error (SEC-26). See `agents.md` for the complete environment-variable reference.

## Running the Project

The server requires `APP_ENV`, `DATABASE_URL`, and `ENCRYPTION_KEY` at startup (`cmd/server/config.go`); it fails fast if any is missing or invalid. `AUTH_STATIC_TOKENS` is additionally required only for `AUTH_MODE=static`, which is restricted to `APP_ENV=dev` (SEC-26). The quickest way to run it locally is via Docker Compose.

### Start the development database

```powershell
docker compose up -d --wait db
```

(Equivalent to running `persistent.bat`.) This starts a persistent PostgreSQL instance on `127.0.0.1:5432`.

### Build and start the backend

```powershell
docker compose up -d --build backend
```

This builds the multi-stage `Dockerfile` (distroless, non-root user `65532:65532`), starts the container with `RUN_MIGRATIONS=true`, and applies the embedded migrations from `migrations/` on startup. The server listens on `127.0.0.1:8080`.

### Verify it is running

```powershell
curl http://127.0.0.1:8080/healthz
```

A successful response is `{"status":"ok"}` with HTTP status `200`. `GET /healthz` is intentionally unauthenticated and returns no business, database, version, or build information (see `agents.md`).

### Seed data (optional)

`cmd/seed` is a small dev-only tool that creates dummy users (technician, dispatcher, admin), one resource class and two resources (available / in-use) for manual API testing. It goes through the same `UnitOfWork` and repositories as the server, so seeded rows are exactly as valid as ones the application would create itself — no hand-written SQL. It is not run automatically; run it against the `db` container's `resource` database:

```powershell
$env:APP_ENV = "dev"
$env:DATABASE_URL = "postgres://dev:dev@127.0.0.1:5432/resource?sslmode=disable"
go run ./cmd/seed
```

`APP_ENV` must be `dev` — the tool refuses to run otherwise. It is safe to run more than once against the same database: an already-seeded record is logged and skipped instead of failing.

### Stop / reset

`docker compose down` stops the containers and keeps the `resource_pgdata` volume. `resetdb.bat` deletes that volume for a clean slate.

## Verify the Project

Format the source code:

```powershell
go fmt ./...
```

Run static analysis:

```powershell
go vet ./...
```

Run all tests without using cached test results:

```powershell
go test -count=1 -v ./...
```

Run only the domain layer tests:

```powershell
go test -count=1 -v ./internal/domain
```

A successful test run ends with output similar to:

```text
PASS
ok      bestelltool_be/internal/domain
```

`?       bestelltool_be/internal/application/ports  [no test files]` and the same line for `bestelltool_be/migrations` are not errors — those packages currently contain no test files.

### Integration Tests

PostgreSQL integration tests in `internal/adapters/postgres/` (and the end-to-end test in `internal/adapters/http/`) are skipped automatically when `TEST_DATABASE_URL` is not set — running `go test ./...` without it still reports `ok`, just without exercising those cases. To run them for real:

```powershell
docker compose --profile test up -d --force-recreate --wait db-test
```

(Equivalent to running `emptydb.bat`.) This starts an ephemeral, tmpfs-backed PostgreSQL instance on port `5433` for a reproducible, empty test database. The `db-test` port mapping deliberately has no loopback host address (`"5433:5432"`): Docker therefore binds it to `0.0.0.0`, which allows WSL2 to reach the Windows-hosted test database. Do not change this mapping to `127.0.0.1:5433:5432`; WSL2 race tests would no longer be able to connect.

```powershell
$env:TEST_DATABASE_URL = "postgres://dev:dev@127.0.0.1:5433/resource_test?sslmode=disable"
go test -count=1 -p 1 ./...
```

Use `-p 1` here because the postgres and http packages otherwise access the same test database in parallel.

### Race Tests from WSL2

Run the ephemeral `db-test` container from Windows as above. In WSL2, use the Windows host address rather than `localhost`, then run the complete suite with the race detector:

```bash
WIN_IP=$(ip route show default | awk '{print $3}')
TEST_DATABASE_URL="postgres://dev:dev@${WIN_IP}:5433/resource_test?sslmode=disable" \
  go test -count=1 -p 1 -race ./...
```

Before running this command, verify that the URL still targets only `resource_test` on port `5433`. The integration-test cleanup drops the `public` schema, so it must never point at the development database on port `5432`. This WSL2 setup is the supported local path for the mandatory race run when the Windows Go toolchain has no suitable C compiler.

## Development with GoLand

1. Open the project directory in GoLand.
2. Ensure that a Go SDK version 1.26 or newer is configured.
3. Open the integrated terminal with `Alt+F12`.
4. Run the following commands from the project root:

```powershell
go fmt ./...
go vet ./...
go test -count=1 ./...
```

Individual tests can be started using the green icon next to a test function or from the command line.

Example:

```powershell
go test -count=1 -v ./internal/domain -run '^TestAllocationAllowedTransitions$'
```

## State Models

### Allocation

```text
allocated
  → shipped
  → with_technician
  → return_requested
  → shipped_back
  → inspection
  → completed

allocated → cancelled
with_technician → completed   (direct transfer, see below)
```

A return is always an explicit action. Reaching the planned end date never causes an automatic state transition. A direct transfer completes the allocation straight from `with_technician`, bypassing `return_requested` / `shipped_back` / `inspection`; a pending return request does not block it.

### Resource

```text
available
  → reserved
  → issued
  → in_use
  → shipped_back
  → inspection

available → externally_procured
in_use → reserved   (direct transfer to a new holder)
```

After inspection:

```text
inspection → available
inspection → blocked
```

A blocked resource can only be reactivated through an explicit action. A direct transfer (`in_use → reserved` for the new holder) is only permitted while the resource has no active block; per `systemdesign.md` §3 it is restricted to the dispatcher.

### Request

```text
open
  → in_progress
  → partially_allocated
  → allocated
  → active
  → completed
```

A request may also be cancelled while it is still `open` or `in_progress`.

## Tests

The test suite covers, among other things:

- Valid construction and missing required fields
- Allowed and forbidden state transitions, including direct transfer
- Unchanged state and version after failed operations; version increments after successful ones
- Invalid time ranges, required reasons and notes
- Explicit reactivation of blocked resources; no automatic state changes based on dates
- Use cases against in-memory fakes and against a real PostgreSQL database (row locking, optimistic-locking conflicts, the single-active-allocation constraint)
- HTTP handlers, including authentication/authorization negative cases and JSON error mapping
- One end-to-end request-to-return-to-transfer lifecycle through the real HTTP → PostgreSQL stack

## Next Development Steps

The roadmap is tracked in `status.md` ("Next Steps") and the migration phases in `systemdesign.md` §13, not duplicated here.

## Project Rules

Before making changes, read:

- `agents.md`
- `status.md`
- `systemdesign.md`

After completing significant work, update `status.md`.

## License

This project is released under the Apache License 2.0 — see `LICENSE`.

GPL or AGPL dependencies must not be introduced.
