# AI Agent Instructions for Go Backend (Resource Planning System)

## 1. Role & Mindset
You are a Senior Go Software Architect. You write idiomatic, readable, and highly maintainable Go code. You favor simplicity and explicit control over "magic" and heavy frameworks. You strictly follow Hexagonal Architecture (Ports and Adapters).

Security-relevant code (authentication, authorization, audit) is held to a stricter standard than the rest: it needs an explicit negative test for every rule it enforces. A security rule without a failing-path test does not count as implemented.

## 2. Architecture Constraints (Strict!)
The project uses a strict Hexagonal Architecture. Do not violate these boundaries:

*   **`internal/domain`**: The absolute core. Contains only pure Go structs, state enums, and pure business logic (state machines).
    *   **NEVER** import external packages here.
    *   **NEVER** use database tags (e.g., `db:"..."`) or HTTP specific logic here.
    *   **NEVER** call `time.Now()`. Time enters through the `Clock` port. Session expiry, token lifetimes and lockout windows must be testable without sleeping.
    *   **NEVER** import `crypto/*` for hashing or random generation. The domain knows *that* a password is verified and *that* a secret is generated, not *how*.
*   **`internal/application`**: Contains Use Cases and Ports (Interfaces).
    *   **`ports`**: Define interfaces for Repositories, Unit of Work, EventBus, `PasswordHasher`, `SecretGenerator`, `Clock`, `PermissionResolver`, etc.
    *   **`usecases`**: Implement the orchestration logic. Depends ONLY on the domain and ports.
*   **`internal/adapters`**: Implementations of the ports.
    *   **`postgres`**: Implements repositories and the Unit of Work. All repos share the internal `querier` interface (`Exec`/`Query`/`QueryRow`) defined in `transaction.go`, so they work with both `pgx.Tx` and `pgxpool.Pool`.
    *   **`http`** (package `httpadapter`): Implements REST handlers. Each use case the handler depends on is declared as a **local narrow interface** inside `handler.go` (e.g., `CreateRequestUseCase`, `GetRequestUseCase`). When adding a new endpoint, define a new interface there, add a field to `handler`, and wire it in `NewHandler()`.
    *   **`auth`**: Authenticators and credential handling. All cryptography lives here — Argon2id hashing, `crypto/rand` secret generation, token encoding, SHA-256 token hashing, constant-time comparison. Nothing outside this package touches a hash or a raw secret.
    *   **`sse`**: In-memory event broker.
    *   **RULE**: Adapters depend on the application layer. The application layer NEVER depends on adapters.

## 3. Tech Stack & Dependencies
*   **Go Version**: Go 1.26 (see `go.mod`). Use the standard `net/http` package (specifically the Go 1.22 `ServeMux` method-pattern routing, e.g., `"POST /api/v1/requests"`).
*   **BANNED HTTP Frameworks**: Do NOT use Gin, Echo, Fiber, or Mux.
*   **Database**: PostgreSQL.
*   **Driver**: Use `github.com/jackc/pgx/v5`.
*   **BANNED ORMs**: Strictly NO ORMs. Do NOT use GORM, ent, or similar. Write raw SQL, use `sqlc`, or a lightweight query builder like `squirrel`.
*   **Crypto**: `golang.org/x/crypto/argon2` for password hashing (BSD-3-Clause, compatible). `crypto/rand`, `crypto/sha256` and `crypto/subtle` from the standard library. Do NOT add a general-purpose auth framework or a JWT library without an explicit decision — see D-1 in `systemdesign.md`.
*   **Licensing Constraint**: The project is Apache 2.0. Do NOT introduce any GPL or AGPL dependencies.
*   **JSON struct tags**: Use `omitzero` (Go 1.24+) for optional pointer fields (e.g., `json:"wish_from,omitzero"`). Do NOT use `omitempty` for pointer/time fields.

### BANNED Crypto & Secret Practices (Strict!)
*   NO `math/rand` for anything security-relevant — only `crypto/rand`.
*   NO `==`, `bytes.Equal` or `strings.Compare` on secrets, tokens or hashes — only `crypto/subtle.ConstantTimeCompare`.
*   NO SHA-256, MD5, or unsalted hashes for **passwords**. Argon2id only. (SHA-256 *is* correct for hashing high-entropy random tokens — the distinction is entropy of the input.)
*   NO reversible encryption of passwords, ever.
*   NO plaintext tokens in the database, in logs, in URLs, in error messages, or in audit payloads.
*   NO hand-rolled token signing or hand-rolled JWT parsing.
*   NO secret material in DTOs. Response structs for user administration must never contain `password_hash`, `token_hash`, or raw secrets. Define separate response DTOs; never serialize a domain entity that carries credentials.

## 4. Database, Transactions & Unit of Work (Crucial)
*   **Unit of Work (UoW)**: You MUST use the UoW pattern for any state-changing operation. Repositories must not open their own transactions. The UoW injects the transaction context (e.g., `pgx.Tx`) into the repositories.
*   **Strict Audit Trail**: Every status change MUST generate an `AuditEvent` within the EXACT SAME database transaction.
    *   An audit event requires two timestamps: `ClientOccurredAt` (nullable, from offline client) and `ServerRecordedAt` (authoritative, database time).
    *   Use `newAuditEvent(meta, entityType, entityID, action, fromStatus, toStatus)` from `usecases/common.go`.
    *   Always call `validateAuditMeta(meta)` before use — it enforces non-empty `ActorID` and `ActorRole`.
    *   `ActorRole` is the session's `active_role`, never the union of the user's roles.
*   **Audit is append-only.** Never write an `UPDATE` or `DELETE` against `audit_events`, not even in a migration or a repair script. The application DB role has `INSERT`/`SELECT` only and a trigger blocks the rest. If a value is wrong, write a corrective event.
*   **Row-Level Locking**: When the Dispatcher allocates a resource, use explicit row-locks (`SELECT ... FOR UPDATE`) to prevent concurrent assignment conflicts.
*   **Optimistic Locking**: Offline-sync actions use a `version` field for optimistic concurrency control ("Dispatcher wins" policy). `users` carries a `version` column and follows the same pattern. `sessions` and `refresh_tokens` are not versioned — they are state-transitioned under a row lock instead.

### Error Sentinels
Sentinel errors from two packages must be mapped in `mapHTTPError` (`internal/adapters/http/handler.go`):

| Package | Sentinel | HTTP Status |
|---------|----------|-------------|
| `ports` | `ErrUnauthenticated` | 401 |
| `ports` | `ErrCredentialsInvalid` | 401 (generic message only) |
| `ports` | `ErrForbidden` | 403 |
| `ports` | `ErrNotFound` | 404 |
| `ports` | `ErrConflict` | 409 |
| `domain` | `ErrAlreadyCompleted` | 409 |
| `ports` | `ErrValidation` | 422 |
| `domain` | `ErrRequiredField`, `ErrInvalidState`, `ErrInvalidTimeRange`, `ErrReasonRequired`, `ErrInvalidTransition` | 422 |
| `ports` | `ErrThrottled` | 429 (with `Retry-After`) |

`ErrCredentialsInvalid` must produce one single, indistinguishable response for unknown user, wrong password, and disabled account (SEC-03). Never differentiate in the message, the error code, or the HTTP status.

### Lock Order (Strict — prevents deadlocks)
For any use case that touches multiple aggregates, acquire row-level locks in this fixed global order:

1. **User**
2. **UserRole**
3. **Session** / **RefreshToken**
4. **Request** (`GetForUpdate`)
5. **Allocation** (`GetForUpdate`)
6. **Resource** (`GetForUpdate`)

Never acquire locks in a different order. Single-aggregate use cases are exempt. A transaction should not normally span the identity aggregates (1–3) and the operational aggregates (4–6); if a future use case must, the order above applies.

### Last-Admin Guard (Strict — prevents lockout)
`DisableUser`, `RevokeRole` and any `UpdateUser` path that can clear `is_active` must guarantee that at least one active admin remains. The check is only race-free if it locks the **entire** set of admin assignments, not just the target row:

```sql
SELECT ur.user_id
  FROM user_roles ur
  JOIN users u ON u.id = ur.user_id
 WHERE ur.role_code = 'admin' AND u.is_active
   FOR UPDATE OF u, ur;
```

Locking only the target row lets two concurrent removals each count two admins and both succeed. A transaction-scoped advisory lock (`pg_advisory_xact_lock`) over a constant key is an acceptable alternative. A plain `READ COMMITTED` transaction without either is **not** sufficient. This rule requires a concurrency integration test with two parallel transactions.

### Direct-Transfer Transaction Order (Strict — prevents PG-23505)
For `TransferResourceUseCase` and any future multi-allocation operation:
1. **Complete (or cancel) the existing active allocation first → `Save` immediately** — this releases the `uq_allocations_single_active_resource` index for that resource.
2. Only after the old allocation's save: mutate the resource state.
3. Only after the resource save: **Create** the new active allocation.

Reversing step 1 and 3 causes PostgreSQL error `23505` on `uq_allocations_single_active_resource` mid-transaction, even though the final state would be valid.

### Migration Rules
*   Migrations are numbered, embedded via `go:embed`, and paired (up/down). Down migrations are a manual operation — see `docs/deployment.md`.
*   **Expand/contract**: add a column or table in one release, backfill, switch the code, drop the old column in a *later* release. `users.role` must not be dropped in the same release that introduces `user_roles`, because rolling deployments run both code versions at once.
*   `audit_events.actor_role` stays a `text` column with a `CHECK` list and **no foreign key** to `roles(code)`. The audit trail must remain readable and insertable independently of mutable master data. A new role therefore requires a migration — that is intentional.
*   Adding a new `actor_role` or `entity_type` value requires the audit migration **before** any code path can emit it. A use case that writes an unmapped audit value is a release blocker, not a warning.
*   Privilege grants (`GRANT`/`REVOKE` per DB role) are versioned like migrations, not applied by hand.

## 5. Offline-Sync & Idempotency Rules
*   **Outcome-Replay**: The technician client operates offline and sends batches of actions (`client_action_id`, `client_seq`).
*   Before processing any action, you MUST check the `IdempotencyStore`.
*   If an action was already processed, return the stored result (Outcome-Replay). Do NOT evaluate the business logic again.
*   **Batch Semantics**: A sync batch can partially fail. If one action fails, dependent actions are marked as `skipped`, independent actions continue. Never blindly reject the entire batch.
*   **Authentication endpoints are excluded from the idempotency mechanism.** `login`, `refresh`, `logout` and `switch-role` must never be served from the `IdempotencyStore`: replaying a stored outcome would hand out the same token twice and defeat single-use refresh semantics. Refresh retries are handled exclusively by `REFRESH_REPLAY_GRACE` / `successor_token_id` (see Section 7).
*   A replay arriving on a revoked session must fail with `401` and must not be silently discarded — the client has to be able to surface unsent work to the technician.

## 6. Coding Style & Go Idioms
*   **Context**: Pass `context.Context` as the first parameter to all repository and use case methods. Use it for cancellation and timeouts.
*   **HTTP Error Contract**: REST handlers MUST return errors in the JSON envelope `{"error":{"code":"...","message":"..."}}` and map errors centrally via `errors.Is`/`errors.As` (e.g., not found → 404, conflict → 409, validation → 422, unknown → 500 without internal details).
*   **JSON decode**: Always use `decodeJSONBody` (from `handler.go`), which calls `DisallowUnknownFields()` and rejects trailing data. Malformed JSON → `400 bad_request`.
*   **Errors**:
    *   Never swallow errors.
    *   Use custom error types or `errors.Is`/`errors.As` for domain-specific errors (e.g., `ErrConflict`, `ErrNotFound`).
    *   Wrap errors with context using `fmt.Errorf("doing xyz: %w", err)`.
    *   Never wrap a credential-verification failure with detail that reaches the client. Log the discriminating detail server-side, return `ErrCredentialsInvalid`.
*   **Naming**: Use short, concise variable names (e.g., `req` instead of `requestData`). Use descriptive function names.
*   **Memory/Pointers**: Return pointers for structs from repositories to allow `nil` checks, but prefer passing structs by value if they are small and immutable.
*   **Logging**: Structured logs. Never log `Authorization` headers, request bodies of auth endpoints, tokens, hashes, or passwords. When in doubt, log the `user_id` and the outcome, not the input.

### Security: Actor Identity (Strict!)
*   **Actor-Identität für AuditEvents stammt ausschließlich aus dem authentifizierten Principal im Request-Context.** Handler dürfen niemals Actor-Angaben aus Body oder Headern akzeptieren.
*   The `auditPayload` struct MUST NOT contain `actor_id` or `actor_role` fields. `decodeJSONBody` (with `DisallowUnknownFields`) ensures that any client attempting to send these fields receives a `400 bad_request`.
*   Use `buildAuditMeta(r, payload)` in every write handler — it reads `ActorID` and `ActorRole` exclusively from `PrincipalFromContext(r.Context())`.
*   A missing Principal in the context is a **programming error** (middleware was bypassed). `buildAuditMeta` returns an untyped error → `writeMappedError` maps it to **500**, NOT 401. This makes the bug immediately visible.
*   `X-Client-Occurred-At` (offline client timestamp) is the only header that remains legitimate and non-identity-relevant. It is handled inside `buildAuditMeta`.

## 7. Authentication & Session Handling (Strict!)
Target state per `systemdesign.md` §7. Until Phase 2 is complete, `StaticTokenAuthenticator` remains in use, but no new code may depend on it.

*   **Passwords**: Argon2id via the `PasswordHasher` port. Minimum parameters `m = 19456 KiB`, `t = 2`, `p = 1`, 16-byte random salt, PHC-encoded string stored in `auth_identities.password_hash`. Parameters live in the hash so they can be raised later; a login with outdated parameters re-hashes transparently.
*   **Unknown user**: run a dummy Argon2id verification so response time does not reveal account existence (SEC-03).
*   **Tokens**: format `rp_at_<id>.<secret>` (access) and `rp_rt_<id>.<secret>` (refresh). The `<id>` part is public and indexed; only `<secret>` is compared, and only its SHA-256 hash is stored. The fixed prefix exists so secret scanners can detect leaks.
*   **Access tokens are opaque and session-bound** (decision D-1). Do not introduce JWTs without revisiting D-1 — a JWT cannot be revoked before expiry, and revocation is a requirement here.
*   **Principal caching** is allowed with a hard TTL (`PRINCIPAL_CACHE_TTL`, default 30 s) and must be bounded in size. An unbounded or non-expiring cache breaks SEC-11 and is a review blocker.
*   **Refresh rotation**: every refresh consumes the presented token and issues a successor. Presenting an already-consumed token outside `REFRESH_REPLAY_GRACE` revokes the **entire token family and the session** and writes a `session.replay_detected` audit event. Revoking only the presented token is wrong.
*   **Grace window**: within `REFRESH_REPLAY_GRACE`, re-presenting a consumed token returns the already-issued successor (`successor_token_id`) rather than minting a new one. Never mint a second successor for the same predecessor — that would fork the family and defeat replay detection.
*   **Revocation triggers**: user disabled, role assigned or revoked, password changed or reset, explicit `session.revoke`, detected replay. Each of these must revoke sessions inside the same transaction as the change.
*   **`active_role`**: a session carries exactly one active role. Switching roles creates a *new* session and revokes the old one; it never mutates the existing session. `active_role` is re-validated against `user_roles` on every refresh — a revoked role must not survive in a live session.
*   **Throttling**: `login` and `refresh` are rate-limited per account and per source IP. Client IP comes from proxy headers only when `TRUST_PROXY_HEADERS=true`; otherwise use the socket peer address, or the limit is trivially bypassed.
*   **Lockout is time-based** (`locked_until`), never permanent, so an attacker cannot deny service to a user by failing logins.
*   **`AUTH_STATIC_TOKENS` is accepted only when `APP_ENV=dev`.** Any other value must be a fatal startup error (SEC-26).

## 8. Authorization Rules (Strict!)
*   **Every protected route must carry an explicit permission (target) or role (current) allowlist** at the point of registration. No route may be registered on the protected mux without a wrapper. Failing to do so leaves the endpoint open to any authenticated role.
*   **Middleware order is strictly: Authentication → authorization wrapper → Handler.** Never swap this order.
*   **Deny by default**: a test enumerates every route registered on the protected mux and fails if any route lacks an authorization wrapper (SEC-13). Adding a route without updating that test is not possible by design — the test discovers routes, it does not list them manually.
*   **Explizite öffentliche Ausnahme:** `GET /healthz` bleibt absichtlich auf dem äußeren (öffentlichen) Mux ohne Authentifizierung/Autorisierung für Liveness-Probes. Diese Route muss in Deny-by-Default-Tests als erlaubte Ausnahme geführt werden und darf keine fachlichen Daten, DB-Checks, Versions- oder Buildinformationen liefern.
*   Missing or invalid token → **401**. A disallowed role/permission → **403**. A missing Principal inside a handler → **500** (programming error, not 401 or 403). Rate limited → **429**.
*   The authorization check reads exclusively from `PrincipalFromContext`; it never inspects body fields, query parameters, or headers.
*   Permissions are resolved server-side from current database state via `PermissionResolver`. Never trust a permission or role list supplied by the client, even inside a token payload (SEC-14).
*   **Route-level permissions are not sufficient alone.** Ownership and scope checks belong in the use case (e.g., a technician may only request the return of an allocation they hold) — SEC-19.
*   **Administrative permissions never imply operational ones.** `admin` must not be able to allocate resources or perform a direct transfer (SEC-15). Any change that widens the admin role needs an explicit decision recorded in `systemdesign.md`.
*   **No self-escalation**: a user may never assign a role to themselves. Enforced in the use case and by `CHECK (assigned_by <> user_id)` (SEC-16).
*   **`system` is not assignable** to a person (`is_assignable = false`) and has no interactive login (SEC-17).
*   Canonical role constants: `domain.ActorRoleTechnician`, `domain.ActorRoleDispatcher`, `domain.ActorRoleAdmin`, `domain.ActorRoleSystem`. Permission codes are constants too (`PermissionUserCreate`, …). No raw string literals for roles, permissions, audit actions or entity types.

### Transition: `requireRoles` → `requirePermissions`
*   **Current state (until Phase 3):** register with `requireRoles(domain.ActorRoleXxx)`.
*   **Target state:** register with `requirePermissions(PermissionXxx)`.
*   During the transition both wrappers exist and both satisfy the deny-by-default test. Do not mix them on one route. When a route is migrated, its authorization matrix test must be migrated with it.
*   Once every route uses `requirePermissions`, delete `requireRoles` — do not leave it as a convenience.

## 9. SSE / Event Rules
*   Operational events keep their existing role filter: dispatcher receives all, technician only their own (`event.stream.all` / `event.stream.own`).
*   **Identity and administrative events are not published on the operational stream.** User creation, role changes, session revocation and login failures belong in the audit trail, not in a stream that technicians subscribe to.
*   Event payloads follow the same secret rules as logs: no tokens, no hashes, no credentials.
*   The stream handler flushes the SSE headers **before** `Subscribe` returns, so clients detect the connection immediately even if the broker blocks briefly. The regression test `TestHandleEventsFlushesBeforeSubscribeReturns` guards this.

## 10. Working Process
1. Before writing code, review `systemdesign.md` for architectural context. Security requirements are numbered `SEC-01`–`SEC-27` in §11 and open decisions `D-1`–`D-5` in §12.
2. Review `status.md` to understand the current progress, phase, and dependencies.
3. Reference the relevant `SEC-xx` in the commit message and in the test name for every security-relevant change.
4. Upon completing a significant task, always update `status.md` to reflect the new state.
5. Always update `agents.md` if necessary to reflect any changes in the AI Agent rules or architectural constraints.
6. **No administrative endpoint ships before the audit migration (Phase 1) is complete.** Without `admin` in `actor_role` and the administrative entity types, an admin action cannot be recorded at all.
7. If an open decision (`D-1`–`D-5`) blocks implementation, stop and ask instead of picking a default silently.

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
TEST_DATABASE_URL="postgres://user:pass@host/dbname" go test -count=1 -p 1 ./internal/adapters/postgres/...
```

Use `-p 1` here because the postgres and http packages otherwise access the same test database in parallel.

Open question (do not implement in this step): should integration tests use schema isolation per package so they can safely run in parallel again?

At the end of every phase, run the integration tests against the real test database. Green skips are not sufficient once the environment is available.

### Required Negative Tests (do not remove)
| Rule | Test must prove |
|------|-----------------|
| SEC-01 | `actor_id`/`actor_role` in the body → `400`; missing Principal → `500` |
| SEC-03 | unknown user, wrong password, disabled user → byte-identical response |
| SEC-08 | consumed refresh token outside grace → family + session revoked |
| SEC-10 | access token stops working after disable/role change within `PRINCIPAL_CACHE_TTL` |
| SEC-13 | route enumeration finds no unguarded protected route |
| SEC-15 | admin attempting a direct transfer → `403` |
| SEC-16 | self role assignment → rejected by use case *and* by DB `CHECK` |
| SEC-18 | two parallel last-admin removals → exactly one succeeds |
| SEC-20 | `UPDATE`/`DELETE` on `audit_events` → error |
| SEC-25 | `refdata_tool` DB role cannot read identity/session/audit tables |

### Environment Variables (Server)
| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | ✅ | — | PostgreSQL connection string |
| `APP_ENV` | ✅ | — | `dev` / `staging` / `prod`; gates `AUTH_STATIC_TOKENS` |
| `AUTH_MODE` | ❌ | `session` | `session` or `static` (dev only) |
| `AUTH_STATIC_TOKENS` | dev only | — | Bearer token config (`token:user-id:role,...`). **Transitional** — see Tech Debt in `status.md`. Parsed at startup; invalid config is a fatal error; present outside `dev` is a fatal error. |
| `RUN_MIGRATIONS` | ❌ | `false` | Run embedded up-migrations at startup |
| `ACCESS_TOKEN_TTL` | ❌ | `15m` | Access token lifetime |
| `REFRESH_TOKEN_TTL` | ❌ | `720h` | Refresh token lifetime (must cover technician offline periods) |
| `SESSION_IDLE_TTL` | ❌ | `720h` | Session idle timeout |
| `SESSION_MAX_LIFETIME` | ❌ | `2160h` | Absolute session lifetime |
| `REFRESH_REPLAY_GRACE` | ❌ | `30s` | Retry window for a consumed refresh token; `0` disables |
| `PRINCIPAL_CACHE_TTL` | ❌ | `30s` | Upper bound on revocation lag |
| `ARGON2_MEMORY_KIB` | ❌ | `19456` | Argon2id memory (OWASP minimum) |
| `ARGON2_TIME` | ❌ | `2` | Argon2id iterations |
| `ARGON2_PARALLELISM` | ❌ | `1` | Argon2id parallelism |
| `LOGIN_MAX_ATTEMPTS` | ❌ | `10` | Failed logins before time-based lock |
| `LOGIN_LOCKOUT_WINDOW` | ❌ | `15m` | Lock duration |
| `TRUST_PROXY_HEADERS` | ❌ | `false` | Only enable behind a trusted reverse proxy |
| `HTTP_ADDR` | ❌ | `:8080` | Listen address |
| `HTTP_READ_TIMEOUT` | ❌ | `15s` | Server read timeout (Go duration string) |
| `HTTP_WRITE_TIMEOUT` | ❌ | `15s` | Server write timeout |
| `HTTP_IDLE_TIMEOUT` | ❌ | `60s` | Server idle timeout |
| `HTTP_SHUTDOWN_TIMEOUT` | ❌ | `10s` | Graceful shutdown timeout |

Secrets are never baked into images or committed. Startup fails fast on invalid configuration — never fall back to an insecure default.

### Adding a New Use Case + Endpoint (Checklist)
1. Define use case input/output structs and implement logic in `internal/application/usecases/`.
2. Add a local narrow interface in `internal/adapters/http/handler.go` (e.g., `type MyUseCase interface { Execute(...) }`).
3. Add a field to the `handler` struct and update `NewHandler()` / `NewHandlerWithClock()`.
4. Register the route on the **`protected` inner mux** in `NewHandlerWithClock()` with an **explicit authorization wrapper**:
   ```go
   // current
   protected.Handle("METHOD /api/v1/...", requireRoles(domain.ActorRoleXxx)(http.HandlerFunc(h.handleX)))
   // target (Phase 3+)
   protected.Handle("METHOD /api/v1/...", requirePermissions(PermissionXxx)(http.HandlerFunc(h.handleX)))
   ```
   No route may be registered on the protected mux without a wrapper.
5. If the route mutates state: audit action and entity type must already exist as constants **and** be permitted by the `audit_events` CHECK constraints. If not, write the migration first.
6. Add ownership/scope checks inside the use case where the permission alone is too coarse.
7. Wire the concrete use case into `cmd/server/main.go` (composition root).
8. Add handler tests using `httptest.NewRecorder` and in-package fake structs (see `handler_test.go`). All test requests for protected routes must include `Authorization: Bearer <token>` and a `fakeAuthenticator`. Cover at minimum: success, `400` malformed JSON, `401` missing token, `403` wrong role/permission, and the relevant `404`/`409`/`422` mappings.
9. Update `status.md`.