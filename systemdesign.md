# Resource Management & Dispatch Planning – System Design

**Status:** Draft / Shared Understanding
**Goal:** Generic Open-Source System for planning and managing rentable resources.

> Sections 1–4 describe the operational domain. Sections 5–9 describe identity,
> authorization, audit and data protection. Section 10 describes the hexagonal
> structure, Section 11 the migration phases.
> Security requirements are numbered (`SEC-xx`) so that they can be referenced
> from `status.md`, from code comments and from tests.

---

## 1. Purpose & Core Idea

A generic system where resources are planned, requested, allocated, and returned.

* **Users (Technicians)** request resources of a specific class for a job/context,
  use them, and return them.
* **A central Dispatch Control (Dispatcher)** manages the resource pool, assigns
  concrete instances, and maintains an overview of all open requests.
* **Administrators** maintain master data, users and roles. They do **not**
  operate the dispatching workflow (see SEC-15).
* The specific domain (e.g. calibration reference standards) is deliberately
  *not* part of the core. Domain-specific aspects (calibration validity, special
  validation rules) will be added later as optional plugins/extensions.
* **Non-Goal:** History/Certificate evaluation (which resource was last used for
  what). This is a separate, standalone system.
* **Non-Goal:** Being an identity provider. The system manages *application*
  users, roles and sessions; it can delegate identity proof to an external
  OpenID Connect provider (Section 7.1).

---

## 2. Roles

| Role | Count (Sizing) | Clients | Workflow |
| :--- | :--- | :--- | :--- |
| **Technician** | ~150 | Mobile device + Laptop, offline-capable | Requests resources, uses them, requests return |
| **Dispatcher** | 3–4 concurrently | Online only | Allocates concrete resources, approves direct transfers |
| **Admin** | Few | Online only | Master data, resource classes, users, roles |
| **System** | – | – | Internal background jobs; never assignable to a person (SEC-17) |

*All human roles are held by multiple people → every status change must be
uniquely traceable to a specific individual acting in a specific role
(audit trail, Section 8).*

Roles are **not** hard-coded permission checks scattered across the HTTP layer.
A role is a centrally maintained **bundle of permissions** (Section 6).

---

## 3. Resource Lifecycle Rules

### Direct Transfer (Site-to-Site)

A resource does **not** have to return to the warehouse between two deployments.
If it is not reported defective/blocked, it may be transferred directly from one
deployment site to the next (direct transfer).

* A full return cycle with inspection is **desired but not required**.
* Direct transfer is only permitted if the resource has **no active block**
  (not defective, not under mandatory inspection).
* At any point in time a resource has **at most one active allocation**
  (enforced by a unique partial index). On direct transfer, the previous
  allocation must be completed in the same transaction in which the next
  allocation becomes active — there is no gap and no overlap.
* A direct transfer produces audit events for both allocations (completion of
  the old, activation of the new).
* **Final decision:** direct transfers and operative resource allocations are
  performed **exclusively by the dispatcher**. Technician-to-technician transfer
  is excluded. This is an operational decision, not a technical limitation, and
  is enforced by the permission `resource.transfer_direct` (Section 6).

### Return to Warehouse & Inspection

* When a resource physically returns to the warehouse (`shipped_back` →
  received), it **should** be inspected before becoming `available` again.
  Inspection is the default path.
* There is **no automatic transition** from `shipped_back` to `available`;
  making a resource available again is always an explicit dispatcher action
  (with audit trail).
* `shipped_back` counts as an **active** state for the single-active-allocation
  constraint: a resource in transit back cannot be allocated to a new request.
  Direct transfer is only possible *before* a return shipment is initiated.

### External Procurement

*This subsection is derived directly from `internal/domain/resource.go` and its
test coverage; it documents what the code currently does, not a designed
business process. Open questions are listed in Section 12.*

* `Resource.MarkExternallyProcured()` transitions a resource from `available`
  to `externally_procured`. No other precondition is checked (no block-reason
  guard, no holder requirement) and no reason or note is recorded — the method
  takes no parameters beyond the receiver.
* No code path transitions a resource **out of** `externally_procured` again;
  as implemented today it is a terminal state in the domain model.
* The transition is not yet reachable through any use case or HTTP endpoint —
  no caller exists outside of the domain package itself — and it is the only
  resource state transition with **no unit test**. `externally_procured` is
  already a valid value in the `chk_resources_status_valid` constraint
  (migration `000001`), so the column can hold it, but nothing sets it there
  today.

### Allocation Cancellation

*This subsection is derived directly from `internal/domain/allocation.go` and
its test coverage; it documents what the code currently does, not a designed
business process. Open questions are listed in Section 12.*

* `Allocation.Cancel()` only succeeds from `allocated`, i.e. strictly before
  `MarkShipped`. From `completed` it returns `ErrAlreadyCompleted`; from any
  other status (`shipped`, `with_technician`, `return_requested`,
  `shipped_back`, `inspection`, already `cancelled`) it returns
  `ErrInvalidTransition`.
* No reason or note is required or recorded — unlike blocking a resource
  (`CompleteInspectionBlocked`, which mandates a `BlockReason`), `Cancel` takes
  only a timestamp.
* `Request` has a separate, structurally identical `Cancel()` method
  (`open`/`in_progress` → `cancelled`). The two `Cancel` methods are
  independent in code; nothing links cancelling a request to cancelling its
  allocations, or vice versa.
* Neither `Allocation.Cancel()` nor `Request.Cancel()` is called by any use
  case in `internal/application/usecases`, and neither has an HTTP route. No
  permission constant exists for either (compare the catalogue in Section 6.2).
* `Resource.Reserve()` (the transition that would put a resource into
  `reserved` for an allocation) is likewise not called by any production use
  case today — only `TransferResourceUseCase` creates a new allocation, and
  only for a resource that is already `in_use`. There is currently no
  observable end-to-end path where an `allocated`-status allocation exists
  together with a `reserved`-status resource, so the interaction between
  cancelling such an allocation and freeing its resource cannot be verified
  against running code.

---

## 4. Offline Operation (Technicians)

Technicians work offline-capable. This has consequences for authentication:

* Queued client-side mutations are replayed on reconnect and de-duplicated via
  the existing **idempotency store**. Replay must never create duplicate
  allocations or duplicate audit events.
* `X-Client-Occurred-At` remains the client-reported time; the server always
  records its own `server_recorded_at`. Client time is never trusted for
  ordering or authorization.
* An access token will usually be expired after an offline period. The client
  refreshes first (Section 7.3), then replays the queue.
* If the session was revoked while the client was offline, the replay must fail
  with `401` and the client must surface the queued, unsent actions to the user.
  Silently dropping them is not acceptable.
* Refresh token lifetime must therefore exceed the realistic maximum offline
  period (default 30 days), while access token lifetime stays short.

---

## 5. Identity & User Management

### 5.1 Placement

User, role and session management belongs into the **Go backend**, not into the
.NET admin tool and not into PostgreSQL role management:

```text
.NET admin client / Web client / Mobile client
        │  HTTPS
        ▼
Go backend
  ├─ Authentication      (login, refresh, logout, role switch)
  ├─ Authorization       (permissions, deny by default)
  ├─ User management     (create, update, disable, reactivate)
  ├─ Role management     (assign, revoke)
  └─ Auditing            (append-only)
        │
        ▼
PostgreSQL  (single least-privilege application account)
```

PostgreSQL logins and application roles stay strictly separate. No PostgreSQL
login is created per technician. `technician`, `dispatcher`, `admin` and
`system` are **application** roles.

### 5.2 Data Model

The current `users(id, role, display_name, is_active)` shape is sufficient for
static test tokens but too narrow for real user management. Separate concerns:

| Table | Responsibility |
| :--- | :--- |
| `users` | The business person |
| `auth_identities` | How that person authenticates (local password, OIDC subject) |
| `roles` | Role catalogue |
| `permissions` | Individual permissions |
| `role_permissions` | Permissions granted by a role |
| `user_roles` | Role assignments |
| `sessions` | Active login sessions |
| `refresh_tokens` | Rotation lineage of refresh tokens |

```sql
CREATE TABLE users (
    id            text PRIMARY KEY,
    username      text        NOT NULL,
    display_name  text        NOT NULL,
    email         text,
    is_active     boolean     NOT NULL DEFAULT true,
    version       bigint      NOT NULL DEFAULT 1,
    created_at    timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at    timestamptz NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT uq_users_username UNIQUE (username)
);
```

Users are **deactivated, not deleted**. Requests, allocations and audit events
already reference `users.id`; deletion would destroy traceability. `email` is
mutable and must never be used as an identity key.

```sql
CREATE TABLE roles (
    code          text PRIMARY KEY,
    display_name  text    NOT NULL,
    description   text    NOT NULL DEFAULT '',
    is_assignable boolean NOT NULL DEFAULT true   -- 'system' → false
);

CREATE TABLE permissions (
    code        text PRIMARY KEY,                 -- '<entity>.<action>'
    description text NOT NULL DEFAULT ''
);

CREATE TABLE role_permissions (
    role_code       text NOT NULL REFERENCES roles(code)       ON DELETE RESTRICT,
    permission_code text NOT NULL REFERENCES permissions(code) ON DELETE RESTRICT,
    PRIMARY KEY (role_code, permission_code)
);

CREATE TABLE user_roles (
    user_id     text        NOT NULL REFERENCES users(id)  ON DELETE RESTRICT,
    role_code   text        NOT NULL REFERENCES roles(code) ON DELETE RESTRICT,
    assigned_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    assigned_by text        NOT NULL REFERENCES users(id)  ON DELETE RESTRICT,
    valid_from  timestamptz,
    valid_until timestamptz,
    PRIMARY KEY (user_id, role_code),
    CONSTRAINT ck_user_roles_no_self_assignment CHECK (assigned_by <> user_id)
);
```

A person may hold several roles (e.g. technician *and* dispatcher). The first
alpha UI may still restrict this to one role per user. The database-level
`CHECK` on `assigned_by` is a second line of defence for SEC-16; the primary
enforcement is in the use case.

### 5.3 User Management Use Cases

Provided: `ListUsers`, `GetUser`, `CreateUser`, `UpdateUser`, `DisableUser`,
`ReactivateUser`, `AssignRole`, `RevokeRole`, `RevokeUserSessions`,
`ResetLocalPassword`, `ChangeOwnPassword`, `ListUserAuditEvents`.

Deliberately **not** provided: `DeleteUser`, `EditAuditEvent`,
`AssignSystemRole`, and any operation that grants an admin the full operational
permission set.

Invariants:

* The last active admin can neither be disabled nor stripped of the admin role
  (SEC-18).
* Role changes and user lockouts run in one transaction together with their
  audit event.
* After a role change or lockout, existing sessions are revoked (SEC-10).
* `username` and `(provider, provider_subject)` are unique.
* Locking is the normal case; deletion only for records that were obviously
  never used (`delete_unused`).
* An admin-triggered password reset forces a password change on next login and
  revokes all sessions of that user.

---

## 6. Authorization

### 6.1 Model

`user → roles → permissions`. HTTP routes declare **permissions**, never roles:

```go
protected.Handle(
    "POST /api/v1/admin/users",
    requirePermissions(PermissionUserCreate)(
        http.HandlerFunc(h.handleCreateUser),
    ),
)
```

Role checks are not abolished — roles simply become centrally maintained
permission bundles. The existing separation of authentication (401) and
authorization (403) stays.

**Deny by default (SEC-13):** every registered route must declare at least one
required permission. A route without a declaration must fail closed with `403`,
and a test enumerates all routes of the mux to prove no route is unguarded.

### 6.2 Permission Catalogue

```text
# operational
request.create               request.read
allocation.return_request    allocation.manage
resource.transfer_direct
event.stream.own             event.stream.all

# reference master data
resource_class.read          resource_class.create
resource_class.update        resource_class.deactivate
resource_class.delete_unused
resource.read                resource.create
resource.update_master_data  resource.deactivate
resource.delete_unused

# administration
user.read     user.create   user.update
user.disable  user.reactivate  user.reset_password
role.assign   role.revoke   session.revoke
audit.read
```

### 6.3 Role → Permission Matrix

| Role | Permissions |
| :--- | :--- |
| **Technician** | `request.create`, `request.read`, `allocation.return_request`, `resource_class.read`, `resource.read`, `event.stream.own` |
| **Dispatcher** | `request.read`, `allocation.manage`, `allocation.return_request`, `resource.transfer_direct`, `resource_class.read`, `resource.read`, `event.stream.all` |
| **Admin** | `resource_class.*`, `resource.read`, `resource.create`, `resource.update_master_data`, `resource.deactivate`, `resource.delete_unused`, `user.*`, `role.assign`, `role.revoke`, `session.revoke`, `audit.read` |
| **System** | internal background operations only; no interactive login |

**Admin is not Dispatcher (SEC-15).** An admin maintains master data and users
but must not be able to execute allocations or direct transfers. This preserves
the existing security decision and keeps the four-eyes character of the
operational workflow.

### 6.4 Route-Level vs. Object-Level Checks

A permission answers "may this actor do this *kind* of thing". It does not
answer "may this actor do it to *this* record". Ownership checks stay in the use
case (SEC-19), e.g. a technician may only request the return of an allocation
that is held by them.

`event.stream.own` / `event.stream.all` map the existing SSE filter: dispatchers
receive all operational events, technicians only their own.

---

## 7. Authentication

### 7.1 Preferred Production Solution: External Identity Provider

If Microsoft Entra ID or another OpenID Connect provider exists in the corporate
network, it should be used. The .NET desktop client then uses:

* Authorization Code Flow with **PKCE** (RFC 7636)
* the **system browser**, not an embedded WebView (RFC 8252)

The identity provider proves *identity only*. Business roles and permissions
remain in PostgreSQL:

```text
OIDC subject → auth_identities → internal user → internal roles → permissions
```

The backend exchanges the verified OIDC identity for an **internal session**
(Section 7.2). This keeps exactly one authorization model regardless of how the
user authenticated.

### 7.2 Alternative: Local Accounts

```sql
CREATE TABLE auth_identities (
    id                    text PRIMARY KEY,
    user_id               text        NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    provider              text        NOT NULL,          -- 'local' | 'oidc:<issuer>'
    provider_subject      text,
    password_hash         text,                          -- PHC string, local only
    must_change_password  boolean     NOT NULL DEFAULT false,
    failed_attempts       integer     NOT NULL DEFAULT 0,
    locked_until          timestamptz,
    created_at            timestamptz NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT uq_auth_identity UNIQUE (provider, provider_subject)
);
```

Password rules (SEC-02, SEC-03):

* **Argon2id** with per-password 16-byte salt, encoded as a PHC string
  (`$argon2id$v=19$m=19456,t=2,p=1$…`). Minimum parameters per OWASP:
  `m = 19456 KiB (19 MiB)`, `t = 2`, `p = 1`. Parameters are configurable and
  stored inside the hash, so they can be raised later; a login with outdated
  parameters transparently re-hashes.
* Never plaintext, never reversible encryption, never a fast hash (SHA-256,
  bcrypt-without-cost-review, MD5).
* Policy per NIST SP 800-63B: minimum 12 characters, accept at least 64, no
  composition rules, no forced periodic rotation, optional check against a
  breached-password list.
* Login failures are indistinguishable for *unknown user*, *wrong password* and
  *disabled user*. For an unknown user a dummy Argon2id verification runs so
  that response time does not leak account existence.
* Verification uses constant-time comparison (`crypto/subtle`).
* Throttling per account and per source IP with exponential backoff; response
  `429` with `Retry-After`. Account lockout is time-based (`locked_until`) so an
  attacker cannot permanently deny service to a user; an admin can unlock.

### 7.3 Sessions & Tokens

```sql
CREATE TABLE sessions (
    id                text PRIMARY KEY,
    user_id           text        NOT NULL REFERENCES users(id)  ON DELETE RESTRICT,
    active_role       text        NOT NULL REFERENCES roles(code) ON DELETE RESTRICT,
    client_name       text        NOT NULL DEFAULT '',
    created_at        timestamptz NOT NULL DEFAULT clock_timestamp(),
    last_used_at      timestamptz,
    idle_expires_at   timestamptz NOT NULL,
    absolute_expires_at timestamptz NOT NULL,
    revoked_at        timestamptz,
    revoke_reason     text        NOT NULL DEFAULT ''
);

CREATE TABLE refresh_tokens (
    id                text PRIMARY KEY,
    session_id        text        NOT NULL REFERENCES sessions(id) ON DELETE RESTRICT,
    token_hash        bytea       NOT NULL,
    previous_token_id text        REFERENCES refresh_tokens(id),
    successor_token_id text       REFERENCES refresh_tokens(id),
    issued_at         timestamptz NOT NULL DEFAULT clock_timestamp(),
    expires_at        timestamptz NOT NULL,
    consumed_at       timestamptz,
    revoked_at        timestamptz,
    revoke_reason     text        NOT NULL DEFAULT '',
    CONSTRAINT uq_refresh_token_hash UNIQUE (token_hash)
);
```

**Access token — decision D-1: opaque, session-bound, not a JWT.**
A JWT cannot be revoked before it expires; with 150 technicians and 3–4
dispatchers the request volume does not justify that trade-off. The access token
is a random secret (≥256 bit) stored only as a SHA-256 hash. A short in-process
cache of the resolved principal (default 30 s) keeps the per-request cost low
while bounding revocation lag (SEC-11). This also works with several backend
instances, since the shared source of truth is PostgreSQL. If JWTs become
necessary for scaling, a per-user `security_epoch` must be added and checked, or
revocation becomes impossible.

Token format: `rp_at_<id>.<secret>` / `rp_rt_<id>.<secret>`. The public `id`
allows an indexed lookup; only the `secret` part is hashed and compared in
constant time. The fixed prefix makes the tokens detectable by secret scanners.

Lifetimes:

* Access token: 15 minutes (default).
* Refresh token: 30 days (default) — must cover the offline period, Section 4.
* Session idle timeout and absolute maximum lifetime (default 90 days) are both
  enforced server-side.

**Rotation and replay detection (SEC-08):** every refresh consumes the presented
token and issues a new one. If a token that is already `consumed_at` is
presented, the **entire token family and its session are revoked** and an audit
event is written. Revoking only the presented token would leave a stolen token
usable. RFC 9700 requires protection against refresh token replay for public
clients; rotation with reuse detection is that protection.

**Retry grace window (decision D-2):** on a flaky mobile connection the client
may not receive the response to a successful refresh. To avoid forcing a
re-login, presenting a consumed token *again* within `REFRESH_REPLAY_GRACE`
(default 30 s) returns the already-issued successor token instead of creating a
new one; this is why `successor_token_id` exists. Any presentation after the
grace window is treated as replay. Setting the value to `0` disables the grace
window at the cost of occasional forced re-logins. Clients must additionally
single-flight their refresh calls.

Revocation triggers (SEC-10): user disabled, role assigned or revoked, password
reset or changed, explicit `session.revoke`, detected replay.

### 7.4 Multiple Roles and `active_role`

If a user holds several roles, every action must remain unambiguously attributed
to one role:

* A user may hold several roles.
* A session carries exactly one `active_role`.
* Switching roles creates a **new session** (and thus new tokens) rather than
  mutating the existing one; the old session is revoked and both events are
  audited.
* `active_role` is validated against the *current* `user_roles` on every
  refresh, not only at login. A revoked role cannot survive in a session.
* The audit trail stores `actor_id` and this `actor_role`.

```json
{ "sub": "user-4711", "active_role": "dispatcher",
  "roles": ["technician", "dispatcher"], "session_id": "…" }
```

The actor identity is resolved **exclusively** from the authenticated principal
and never from the request body (SEC-01) — this is already implemented and must
not regress.

---

## 8. Audit Trail

### 8.1 Required Schema Extension

The current schema allows `actor_role ∈ {technician, dispatcher, system}` and
`entity_type ∈ {request, allocation, resource}`. Administrative actions
therefore **cannot be recorded at all** today. This is the single most important
immediate change: without it an admin could change users or master data without
a valid audit record.

```sql
actor_role IN ('technician', 'dispatcher', 'admin', 'system')
```

Additional entity types: `user`, `role`, `user_role`, `resource_class`,
`resource_class_membership`, `session`, `auth_identity`.

`audit_events.actor_role` deliberately stays a plain text column with a `CHECK`
constraint and **no foreign key** to `roles(code)`: audit records must stay
readable and insertable independently of mutable master data. The cost is one
migration per new role, which is acceptable and explicit.

Typical actions: `user.create`, `user.update`, `user.disable`,
`user.reactivate`, `user.password_reset`, `role.assign`, `role.revoke`,
`session.create`, `session.revoke`, `session.replay_detected`,
`auth.login_failed`, `resource_class.create`, `resource_class.update`,
`resource_class.deactivate`, `resource.create`, `resource.update`.

Every role change records: `actor_id`, `actor_role`, `target_user_id`,
`old_roles`, `new_roles`, `server_recorded_at`, `reason`. The reason is
mandatory for security-relevant changes.

### 8.2 Immutability Must Be Enforced, Not Assumed (SEC-20)

Calling the audit trail immutable is only true if the database enforces it:

* The backend's DB role holds `INSERT` and `SELECT` on `audit_events` only;
  `UPDATE` and `DELETE` are revoked.
* Additionally a `BEFORE UPDATE OR DELETE` trigger raises an exception, so even
  a mistakenly over-privileged role cannot silently rewrite history.
* Audit payloads must never contain credentials, token values or hashes
  (SEC-23).
* Optional hardening: a `prev_hash` chain per stream for tamper evidence.

---

## 9. Data Protection & Retention (Deployment Concern)

Not legal advice, but the design must make compliant operation possible:

* Audit data and session metadata identify individual employees and are
  therefore personal data. Retention periods must be configurable, documented
  and enforced by a scheduled job — no unbounded storage.
* Store the minimum: `client_name` is sufficient for session recognition.
  If IP address or user agent are recorded at all, they get a short retention
  period and are not part of the operational audit stream.
* For deployments in Germany, a technical system capable of monitoring employee
  behaviour typically triggers works-council co-determination
  (BetrVG § 87 (1) no. 6). The audit trail is required for the domain, so
  document its purpose limitation: dispatch traceability, not performance
  measurement.
* Deleting audit records is not possible by design (Section 8.2); erasure
  requests must therefore be handled by pseudonymising `users` rows, which is
  why the audit references `users.id` and not names.

---

## 10. Architecture (Hexagonal / Clean Architecture)

```text
┌───────────────────────────────────────────────────────────────┐
│  Adapters (interchangeable)                                   │
│  - DB: PostgreSQL (default), SQLite ready                     │
│  - HTTP API (REST + SSE), auth middleware, permission gate    │
│  - Auth: Argon2idHasher, CryptoRandSecrets, OIDC (later)      │
│  - Web dashboard, .NET admin client                           │
├───────────────────────────────────────────────────────────────┤
│  Ports (interfaces)                                           │
│  - ResourceRepository, RequestRepository, AllocationRepository │
│  - UserRepository, AuthIdentityRepository, RoleRepository      │
│  - SessionRepository, RefreshTokenRepository                   │
│  - PermissionResolver, PasswordHasher, SecretGenerator, Clock  │
│  - Authenticator, EventPublisher, AuditWriter, IdempotencyStore│
│  - UnitOfWork                                                  │
├───────────────────────────────────────────────────────────────┤
│  Application Layer (use cases)                                │
│  - CreateRequest, AllocateResource, ReturnResource,            │
│    TransferResource, SyncOutbox …                              │
│  - Login, RefreshSession, Logout, SwitchActiveRole,            │
│    ChangeOwnPassword                                           │
│  - CreateUser, UpdateUser, DisableUser, ReactivateUser,        │
│    AssignRole, RevokeRole, RevokeUserSessions,                 │
│    ResetLocalPassword, ListUsers, ListUserAuditEvents          │
├───────────────────────────────────────────────────────────────┤
│  Domain Core (pure model, no external dependencies)           │
│  - Entities, state machines, invariants                        │
│  - User activation, role-assignment invariants, session        │
│    validity, permission sets                                   │
└───────────────────────────────────────────────────────────────┘
```

Architectural rules that the new area must respect:

* Argon2id, `crypto/rand` and token encoding live in **adapters**; the domain
  only sees `PasswordHasher`, `SecretGenerator` and `Clock` ports. Hashes never
  leave the auth adapter as plaintext-adjacent values.
* Every mutating security operation runs through the existing `UnitOfWork` and
  writes its `AuditEvent` in the same transaction (SEC-21).
* Time comes from the `Clock` port so that session expiry is testable.
* HTTP error mapping is extended by `429` (throttling) alongside the existing
  `400/401/403/404/409/422/500`.

### 10.1 Endpoints

```text
POST   /api/v1/auth/login              public, throttled
POST   /api/v1/auth/refresh            public, throttled
POST   /api/v1/auth/logout             authenticated
POST   /api/v1/auth/switch-role        authenticated
GET    /api/v1/auth/me                 authenticated
POST   /api/v1/me/password             authenticated

GET    /api/v1/admin/users             user.read
POST   /api/v1/admin/users             user.create
GET    /api/v1/admin/users/{id}        user.read
PATCH  /api/v1/admin/users/{id}        user.update
POST   /api/v1/admin/users/{id}/disable      user.disable
POST   /api/v1/admin/users/{id}/reactivate   user.reactivate
POST   /api/v1/admin/users/{id}/roles        role.assign
DELETE /api/v1/admin/users/{id}/roles/{role} role.revoke
POST   /api/v1/admin/users/{id}/password-reset  user.reset_password
DELETE /api/v1/admin/users/{id}/sessions        session.revoke
GET    /api/v1/admin/audit-events               audit.read
```

### 10.2 Boundary of the .NET Admin Tool

For the alpha test a mixed architecture is acceptable, but the boundary must be
**enforced by database privileges, not by convention** (SEC-25):

| Direct PostgreSQL access | Allowed |
| :--- | :--- |
| Reference groups (resource classes) | ✅ |
| References (resources) | ✅ |
| Group memberships | ✅ |
| Recalibration data | ✅ |
| `users`, `auth_identities`, `roles`, `user_roles`, `role_permissions` | ❌ |
| `sessions`, `refresh_tokens` | ❌ |
| `audit_events` | ❌ |

A desktop client can be copied and its connection string extracted, so its DB
credentials must be treated as **already compromised**. It therefore gets its
own PostgreSQL role with `GRANT` only on the reference tables and explicit
`REVOKE` on everything else. User management appears in the .NET tool only once
the admin endpoints exist, and then goes through the API.

Medium term the reference maintenance should also move to the API so that all
writes are audited through one path.

---

## 11. Security Requirements (testable)

### Identity & credentials
* **SEC-01** Actor identity is resolved solely from the authenticated principal, never from the request body. A missing principal on a protected route is a programming error (`500`).
* **SEC-02** Passwords are hashed with Argon2id (`m ≥ 19456 KiB`, `t ≥ 2`, `p = 1`), unique 16-byte salt, PHC-encoded, re-hashed on login when parameters change.
* **SEC-03** Login responses and response times do not distinguish unknown user, wrong password and disabled user.
* **SEC-04** All secret comparisons are constant-time.
* **SEC-05** Login and refresh are throttled per account and per source IP; `429` with `Retry-After`. Client IP is taken from proxy headers only if `TRUST_PROXY_HEADERS=true`.
* **SEC-06** Secrets, tokens and hashes never appear in logs, URLs, error messages or audit payloads.

### Tokens & sessions
* **SEC-07** Access tokens are opaque, ≥256 bit entropy, stored only as SHA-256 hash, TTL ≤ 15 min by default.
* **SEC-08** Refresh tokens are hashed at rest, single use and rotated. Presenting a consumed token outside the grace window revokes the whole token family and the session, and is audited.
* **SEC-09** Sessions have both an idle timeout and an absolute maximum lifetime, enforced server-side.
* **SEC-10** Disabling a user, changing roles, resetting or changing a password, and detected replay revoke all affected sessions.
* **SEC-11** Revocation takes effect within `PRINCIPAL_CACHE_TTL` (default 30 s). Principal caching is bounded; there is no unbounded cache.
* **SEC-12** Tokens travel over TLS only, never in a query string. Clients store refresh tokens in the OS keystore.

### Authorization
* **SEC-13** Deny by default: every route declares required permissions; an undeclared route fails closed. A test enumerates the router to prove it.
* **SEC-14** Permissions are resolved server-side from current database state, never from client-supplied claims.
* **SEC-15** Administrative permissions never imply operational ones. Admin ≠ Dispatcher.
* **SEC-16** No user may assign a role to themselves (`assigned_by <> user_id`), enforced in the use case and by a `CHECK` constraint.
* **SEC-17** The `system` role is not assignable to a person (`is_assignable = false`) and has no interactive login.
* **SEC-18** The last active admin cannot be disabled or stripped of the admin role. The check must be race-free — a plain `READ COMMITTED` transaction is *not* sufficient; use `SELECT … FOR UPDATE` over the admin assignments, a transaction-scoped advisory lock, or `SERIALIZABLE` with retry.
* **SEC-19** Object-level ownership is checked in the use case, not only at route level.

### Audit
* **SEC-20** `audit_events` is append-only: `UPDATE`/`DELETE` are revoked for the application role and additionally blocked by a trigger.
* **SEC-21** Every security-relevant change writes its audit event in the same transaction as the change; rollback of one rolls back the other.
* **SEC-22** Role changes record actor, target, old and new roles, and a reason (mandatory for security-relevant changes).
* **SEC-23** Audit payloads contain no credentials, token values or hashes.

### Database & deployment
* **SEC-24** The backend uses a least-privilege DB role, no superuser. DDL runs only under a separate migration role.
* **SEC-25** The .NET tool's DB role has no access to identity, session or audit tables, enforced by `GRANT`/`REVOKE`. Its credentials are assumed compromised.
* **SEC-26** `AUTH_STATIC_TOKENS` is accepted only when `APP_ENV=dev`; otherwise startup fails.
* **SEC-27** TLS is terminated by the reverse proxy; in production the backend binds to an internal interface only.

---

## 12. Open Decisions

| ID | Decision | Status |
| :--- | :--- | :--- |
| **D-1** | Access token opaque + session lookup instead of JWT | proposed, default in this document |
| **D-2** | `REFRESH_REPLAY_GRACE` default 30 s for offline mobile clients | proposed, needs confirmation |
| **D-3** | Technicians hold `request.read` for *all* requests (current behaviour, deliberate and fully audited). Scoping to `request.read.own` is the recommended hardening once mobile clients are in the field. | open |
| **D-4** | Whether reference master data maintenance moves from direct DB access to the API before beta | open |
| **D-5** | Whether Entra ID / OIDC is available in the target network at all | open, decides Section 7.1 vs 7.2 |
| **D-6** | External procurement (Section 3, "External Procurement"): what does `externally_procured` mean operationally (write-off vs. temporary substitute), who may trigger it, is a reason mandatory as for blocking, can a resource ever leave this state, and must it be disallowed while an active allocation exists? None of this is decidable from the current code, which has no caller and no test for the transition. | open |
| **D-7** | Allocation cancellation (Section 3, "Allocation Cancellation"): what triggers `Allocation.Cancel()` in practice — a dispatcher decision, a cascading `Request.Cancel()`, both? Is a reason mandatory? Who may cancel? Does cancelling free the resource (no such resource-side transition exists today)? Undecided pending the use case and endpoint that would call it. | open |

---

## 13. Migration Phases

1. **Audit foundation** — extend `actor_role` by `admin` and add administrative
   entity types; enforce append-only. Without this, no administrative action can
   be recorded lawfully, so this is the first step of the whole area.
2. **Auth core** — extend `users`, add `auth_identities`, `sessions`,
   `refresh_tokens`; Argon2id adapter; login/refresh/logout; replace
   `AUTH_STATIC_TOKENS` (kept as dev-only, SEC-26). `users.role` stays for now.
3. **Roles & permissions** — add `roles`, `permissions`, `role_permissions`,
   `user_roles`; backfill from `users.role`; switch routes from `requireRoles`
   to `requirePermissions`; `active_role` and role switching.
4. **Admin endpoints** — user and role use cases with the invariants of
   Section 5.3, session revocation, audit reading.
5. **Privilege hardening** — separate DB roles for backend, migrations and the
   .NET tool; `GRANT`/`REVOKE` scripts as part of deployment.
6. **Optional OIDC** — Entra ID or another provider, mapped onto the internal
   session model.
7. **Contract migration** — drop `users.role` once no code path reads it
   (expand/contract; must not happen in the same release as step 3 if rolling
   deployments are used).