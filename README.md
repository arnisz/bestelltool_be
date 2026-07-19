# Resource Planning System

A generic open-source system for planning, requesting, allocating, and returning rentable or shared resources.

The project is intentionally domain-neutral. It can be used for tools, measuring equipment, calibration standards, devices, or other shared resources. Domain-specific rules are intended to be added later through extensions or additional adapters.

## Project Status

The project is currently in an early development stage.

Already implemented:

- Go module and basic project structure
- Hexagonal architecture
- Pure domain layer
- Entities for resources, requests, and allocations
- State machines for `Resource`, `Request`, and `Allocation`
- Version fields for optimistic concurrency control
- Domain-specific errors
- `AuditEvent` as a pure data structure
- Unit tests for valid and invalid state transitions

Not yet implemented:

- PostgreSQL integration
- Unit of Work and repository adapters
- HTTP API
- Authentication
- Offline synchronization
- Idempotency store
- Server-Sent Events
- Web dashboard
- Technician client

The current version therefore does not yet provide a production-ready server.

## Deployment Strategy

For verbindliche Regeln zu automatischen Datenbankmigrationen beim Serverstart (`RUN_MIGRATIONS`) siehe `docs/deployment.md`.

## Core Features

The planned system includes:

- Resource classes and concrete resource instances
- Resource requests created by technicians
- Allocation of concrete resources by dispatch
- Individual planning periods for each resource allocation
- Explicit return and recall workflows
- Inspection of returned resources
- Blocking defective or unavailable resources
- Complete audit trail for status changes
- Offline-capable technician clients
- Conflict resolution based on the “dispatcher wins” policy
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
migrations/
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

The application layer will contain use cases and ports. State-changing operations will be executed through a Unit of Work so that the state change and its corresponding audit event are stored in the same database transaction.

### Adapters

Planned adapters include:

- PostgreSQL using `pgx/v5`
- REST API using Go's standard `net/http` package
- Server-Sent Events
- Optional future database or integration adapters

## Technology Stack

- Go 1.22 or newer
- Go standard library
- PostgreSQL
- `github.com/jackc/pgx/v5`
- Docker and Docker Compose
- Caddy as reverse proxy
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

For the current development stage, the following tools are required:

- Go 1.22 or newer
- Git
- PowerShell, Bash, or another terminal
- GoLand is optional

Check the installed versions:

```powershell
go version
git --version
```

## Installation

### 1. Clone the Repository

If the repository is already hosted on GitHub:

```powershell
git clone <REPOSITORY-URL>
cd bestelltool_be
```

For an existing local project, change directly to the project directory:

```powershell
cd C:\Users\arnol\source\repos\bestelltool_be
```

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

The current domain layer only uses the Go standard library. The following command may still be used to clean and verify module dependencies:

```powershell
go mod tidy
```

### 4. Verify the Project

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

Messages such as the following are not errors:

```text
?       bestelltool_be             [no test files]
?       bestelltool_be/cmd/server  [no test files]
```

They only indicate that these packages currently contain no test files.

## Running the Project

The current `cmd/server` package contains only a minimal compilable placeholder. A functional HTTP server has not yet been implemented.

Compilation can be verified with:

```powershell
go build ./...
```

Once the server adapter has been implemented, the application is expected to be started with:

```powershell
go run ./cmd/server
```

At the current development stage, this command does not yet expose a production API.

## Development with GoLand

1. Open the project directory in GoLand.
2. Ensure that a Go SDK version 1.22 or newer is configured.
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
```

A return is always an explicit action. Reaching the planned end date never causes an automatic state transition.

### Resource

```text
available
  → reserved
  → issued
  → in_use
  → shipped_back
  → inspection
```

After inspection:

```text
inspection → available
inspection → blocked
```

A blocked resource can only be reactivated through an explicit action.

### Request

```text
open
  → in_progress
  → partially_allocated
  → allocated
  → active
  → completed
```

A request may also be cancelled while it is still in one of the permitted early states.

## Tests

The test suite covers, among other things:

- Valid construction
- Missing required fields
- Allowed state transitions
- Forbidden state transitions
- Unchanged state after failed operations
- Unchanged version after failed operations
- Version increments after successful operations
- Invalid time ranges
- Required reasons and notes
- Explicit reactivation of blocked resources
- No automatic state changes based on dates

## Next Development Steps

1. Define application ports
2. Define the Unit of Work contract
3. Define repository contracts
4. Define `AuditWriter` and `IdempotencyStore`
5. Implement the first transactional use cases
6. Design the PostgreSQL schema and migrations
7. Implement PostgreSQL adapters using `pgx/v5`
8. Add the HTTP API and SSE support

## Project Rules

Before making changes, read:

- `agents.md`
- `status.md`
- `systemdesign.md`

After completing significant work, update `status.md`.

## License

This project is intended to be released under the Apache License 2.0.

GPL or AGPL dependencies must not be introduced.
