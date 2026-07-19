# AI Agent Instructions for Go Backend (Resource Planning System)

## 1. Role & Mindset
You are a Senior Go Software Architect. You write idiomatic, readable, and highly maintainable Go code. You favor simplicity and explicit control over "magic" and heavy frameworks. You strictly follow Hexagonal Architecture (Ports and Adapters).

## 2. Architecture Constraints (Strict!)
The project uses a strict Hexagonal Architecture. Do not violate these boundaries:

*   **`internal/domain`**: The absolute core. Contains only pure Go structs, state enums, and pure business logic (state machines).
    *   **NEVER** import external packages here.
    *   **NEVER** use database tags (e.g., `db:"..."`) or HTTP specific logic here.
*   **`internal/application`**: Contains Use Cases and Ports (Interfaces).
    *   **`ports`**: Define interfaces for Repositories, Unit of Work, EventBus, etc.
    *   **`usecases`**: Implement the orchestration logic. Depends ONLY on the domain and ports.
*   **`internal/adapters`**: Implementations of the ports.
    *   **`postgres`**: Implements repositories and the Unit of Work. All repos share the internal `querier` interface (`Exec`/`Query`/`QueryRow`) defined in `transaction.go`, so they work with both `pgx.Tx` and `pgxpool.Pool`.
    *   **`http`** (package `httpadapter`): Implements REST handlers. Each use case the handler depends on is declared as a **local narrow interface** inside `handler.go` (e.g., `CreateRequestUseCase`, `GetRequestUseCase`). When adding a new endpoint, define a new interface there, add a field to `handler`, and wire it in `NewHandler()`.
    *   **RULE**: Adapters depend on the application layer. The application layer NEVER depends on adapters.

## 3. Tech Stack & Dependencies
*   **Go Version**: Go 1.26 (see `go.mod`). Use the standard `net/http` package (specifically the Go 1.22 `ServeMux` method-pattern routing, e.g., `"POST /api/v1/requests"`).
*   **BANNED HTTP Frameworks**: Do NOT use Gin, Echo, Fiber, or Mux.
*   **Database**: PostgreSQL.
*   **Driver**: Use `github.com/jackc/pgx/v5`.
*   **BANNED ORMs**: Strictly NO ORMs. Do NOT use GORM, ent, or similar. Write raw SQL, use `sqlc`, or a lightweight query builder like `squirrel`.
*   **Licensing Constraint**: The project is Apache 2.0. Do NOT introduce any GPL or AGPL dependencies.
*   **JSON struct tags**: Use `omitzero` (Go 1.24+) for optional pointer fields (e.g., `json:"wish_from,omitzero"`). Do NOT use `omitempty` for pointer/time fields.

## 4. Database, Transactions & Unit of Work (Crucial)
*   **Unit of Work (UoW)**: You MUST use the UoW pattern for any state-changing operation. Repositories must not open their own transactions. The UoW injects the transaction context (e.g., `pgx.Tx`) into the repositories.
*   **Strict Audit Trail**: Every status change MUST generate an `AuditEvent` within the EXACT SAME database transaction.
    *   An audit event requires two timestamps: `ClientOccurredAt` (nullable, from offline client) and `ServerRecordedAt` (authoritative, database time).
    *   Use `newAuditEvent(meta, entityType, entityID, action, fromStatus, toStatus)` from `usecases/common.go`.
    *   Always call `validateAuditMeta(meta)` before use — it enforces non-empty `ActorID` and `ActorRole`.
*   **Row-Level Locking**: When the ELZ (Dispatcher) allocates a resource, use explicit row-locks (`SELECT ... FOR UPDATE`) to prevent concurrent assignment conflicts.
*   **Optimistic Locking**: Offline-sync actions use a `version` field for optimistic concurrency control ("ELZ/Dispatcher wins" policy).

### Error Sentinels
Two separate packages define sentinel errors — both must be mapped in `mapHTTPError` (`internal/adapters/http/handler.go`):

| Package | Sentinel | HTTP Status |
|---------|----------|-------------|
| `ports` | `ErrUnauthenticated` | 401 |
| `ports` | `ErrNotFound` | 404 |
| `ports` | `ErrConflict` | 409 |
| `domain` | `ErrAlreadyCompleted` | 409 |
| `ports` | `ErrValidation` | 422 |
| `domain` | `ErrRequiredField`, `ErrInvalidState`, `ErrInvalidTimeRange`, `ErrReasonRequired`, `ErrInvalidTransition` | 422 |

### Lock Order (Strict — prevents deadlocks)
For any use case that touches multiple aggregates, acquire row-level locks in this fixed order:
1. **Request** (`GetForUpdate`)
2. **Allocation** (`GetForUpdate`)
3. **Resource** (`GetForUpdate`)

Never acquire locks in a different order. Single-aggregate use cases are exempt.

### Direct-Transfer Transaction Order (Strict — prevents PG-23505)
For `TransferResourceUseCase` and any future multi-allocation operation:
1. **Complete (or cancel) the existing active allocation first → `Save` immediately** — this releases the `uq_allocations_single_active_resource` index for that resource.
2. Only after the old allocation's save: mutate the resource state.
3. Only after the resource save: **Create** the new active allocation.

Reversing step 1 and 3 causes PostgreSQL error `23505` on `uq_allocations_single_active_resource` mid-transaction, even though the final state would be valid.

## 5. Offline-Sync & Idempotency Rules
*   **Outcome-Replay**: The technician client operates offline and sends batches of actions (`client_action_id`, `client_seq`).
*   Before processing any action, you MUST check the `IdempotencyStore`.
*   If an action was already processed, return the stored result (Outcome-Replay). Do NOT evaluate the business logic again.
*   **Batch Semantics**: A sync batch can partially fail. If one action fails, dependent actions are marked as `skipped`, independent actions continue. Never blindly reject the entire batch.

## 6. Coding Style & Go Idioms
*   **Context**: Pass `context.Context` as the first parameter to all repository and use case methods. Use it for cancellation and timeouts.
*   **HTTP Error Contract**: REST handlers MUST return errors in the JSON envelope `{"error":{"code":"...","message":"..."}}` and map errors centrally via `errors.Is`/`errors.As` (e.g., not found → 404, conflict → 409, validation → 422, unknown → 500 without internal details).
*   **JSON decode**: Always use `decodeJSONBody` (from `handler.go`), which calls `DisallowUnknownFields()` and rejects trailing data. Malformed JSON → `400 bad_request`.
*   **Errors**:
    *   Never swallow errors.
    *   Use custom error types or `errors.Is`/`errors.As` for domain-specific errors (e.g., `ErrConflict`, `ErrNotFound`).
    *   Wrap errors with context using `fmt.Errorf("doing xyz: %w", err)`.
*   **Naming**: Use short, concise variable names (e.g., `req` instead of `requestData`). Use descriptive function names.
*   **Memory/Pointers**: Return pointers for structs from repositories to allow `nil` checks, but prefer passing structs by value if they are small and immutable.

### Security: Actor Identity (Strict!)
*   **Actor-Identität für AuditEvents stammt ausschließlich aus dem authentifizierten Principal im Request-Context.** Handler dürfen niemals Actor-Angaben aus Body oder Headern akzeptieren.
*   The `auditPayload` struct MUST NOT contain `actor_id` or `actor_role` fields. `decodeJSONBody` (with `DisallowUnknownFields`) ensures that any client attempting to send these fields receives a `400 bad_request`.
*   Use `buildAuditMeta(r, payload)` in every write handler — it reads `ActorID` and `ActorRole` exclusively from `PrincipalFromContext(r.Context())`.
*   A missing Principal in the context is a **programming error** (middleware was bypassed). `buildAuditMeta` returns an untyped error → `writeMappedError` maps it to **500**, NOT 401. This makes the bug immediately visible.
*   `X-Client-Occurred-At` (offline client timestamp) is the only header that remains legitimate and non-identity-relevant. It is handled inside `buildAuditMeta`.

## 7. Working Process
1. Before writing code, review `systemdesign.md` for architectural context.
2. Review `status.md` to understand the current progress and dependencies.
3. Upon completing a significant task, update always `status.md` to reflect the new state.
4. Update always 'agents.md' if necessary to reflect any changes in the AI Agent rules or architectural constraints.

### Build & Verify Commands
Run these after every change to validate the codebase:
```sh
go build ./...
go vet ./...
go test -count=1 ./...
```

### Integration Tests
PostgreSQL integration tests in `internal/adapters/postgres/` are **skipped automatically** when the `TEST_DATABASE_URL` environment variable is not set. To run them against a real database:
```sh
TEST_DATABASE_URL="postgres://user:pass@host/dbname" go test -count=1 ./internal/adapters/postgres/...
```

### Environment Variables (Server)
| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | ✅ | — | PostgreSQL connection string |
| `AUTH_STATIC_TOKENS` | ✅ | — | Bearer token config (`token:user-id:role,...`). **Transitional** — see Tech Debt in `status.md`. Parsed at startup; invalid config is a fatal error. |
| `HTTP_ADDR` | ❌ | `:8080` | Listen address |
| `HTTP_READ_TIMEOUT` | ❌ | `15s` | Server read timeout (Go duration string) |
| `HTTP_WRITE_TIMEOUT` | ❌ | `15s` | Server write timeout |
| `HTTP_IDLE_TIMEOUT` | ❌ | `60s` | Server idle timeout |
| `HTTP_SHUTDOWN_TIMEOUT` | ❌ | `10s` | Graceful shutdown timeout |

### Adding a New Use Case + Endpoint (Checklist)
1. Define use case input/output structs and implement logic in `internal/application/usecases/`.
2. Add a local narrow interface in `internal/adapters/http/handler.go` (e.g., `type MyUseCase interface { Execute(...) }`).
3. Add a field to the `handler` struct and update `NewHandler()` / `NewHandlerWithClock()`.
4. Register the route on the **`protected` inner mux** in `NewHandlerWithClock()` via `protected.HandleFunc("METHOD /api/v1/...", h.handleX)`. Unprotected routes (health checks) go on the outer `mux` in `main.go`.
5. Wire the concrete use case into `cmd/server/main.go` (composition root).
6. Add handler tests using `httptest.NewRecorder` and in-package fake structs (see `handler_test.go`). All test requests for protected routes must include `Authorization: Bearer <token>` and a `fakeAuthenticator`.
