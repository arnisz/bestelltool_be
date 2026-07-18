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
    *   **`postgres`**: Implements repositories and the Unit of Work.
    *   **`http`**: Implements REST handlers and SSE.
    *   **RULE**: Adapters depend on the application layer. The application layer NEVER depends on adapters.

## 3. Tech Stack & Dependencies
*   **Go Version**: Go 1.22+. Use the standard `net/http` package (specifically the new Go 1.22 `ServeMux`).
*   **BANNED HTTP Frameworks**: Do NOT use Gin, Echo, Fiber, or Mux.
*   **Database**: PostgreSQL.
*   **Driver**: Use `github.com/jackc/pgx/v5`.
*   **BANNED ORMs**: Strictly NO ORMs. Do NOT use GORM, ent, or similar. Write raw SQL, use `sqlc`, or a lightweight query builder like `squirrel`.
*   **Licensing Constraint**: The project is Apache 2.0. Do NOT introduce any GPL or AGPL dependencies.

## 4. Database, Transactions & Unit of Work (Crucial)
*   **Unit of Work (UoW)**: You MUST use the UoW pattern for any state-changing operation. Repositories must not open their own transactions. The UoW injects the transaction context (e.g., `pgx.Tx`) into the repositories.
*   **Strict Audit Trail**: Every status change MUST generate an `AuditEvent` within the EXACT SAME database transaction.
    *   An audit event requires two timestamps: `ClientOccurredAt` (nullable, from offline client) and `ServerRecordedAt` (authoritative, database time).
*   **Row-Level Locking**: When the ELZ (Dispatcher) allocates a resource, use explicit row-locks (`SELECT ... FOR UPDATE`) to prevent concurrent assignment conflicts.
*   **Optimistic Locking**: Offline-sync actions use a `version` field for optimistic concurrency control ("ELZ/Dispatcher wins" policy).

## 5. Offline-Sync & Idempotency Rules
*   **Outcome-Replay**: The technician client operates offline and sends batches of actions (`client_action_id`, `client_seq`).
*   Before processing any action, you MUST check the `IdempotencyStore`.
*   If an action was already processed, return the stored result (Outcome-Replay). Do NOT evaluate the business logic again.
*   **Batch Semantics**: A sync batch can partially fail. If one action fails, dependent actions are marked as `skipped`, independent actions continue. Never blindly reject the entire batch.

## 6. Coding Style & Go Idioms
*   **Context**: Pass `context.Context` as the first parameter to all repository and use case methods. Use it for cancellation and timeouts.
*   **Errors**:
    *   Never swallow errors.
    *   Use custom error types or `errors.Is`/`errors.As` for domain-specific errors (e.g., `ErrConflict`, `ErrNotFound`).
    *   Wrap errors with context using `fmt.Errorf("doing xyz: %w", err)`.
*   **Naming**: Use short, concise variable names (e.g., `req` instead of `requestData`). Use descriptive function names.
*   **Memory/Pointers**: Return pointers for structs from repositories to allow `nil` checks, but prefer passing structs by value if they are small and immutable.

## 7. Working Process
1. Before writing code, review `systemdesign.md` for architectural context.
2. Review `status.md` to understand the current progress and dependencies.
3. Upon completing a significant task, update always `status.md` to reflect the new state.
4. Update always 'agents.md' if necessary to reflect any changes in the AI Agent rules or architectural constraints.