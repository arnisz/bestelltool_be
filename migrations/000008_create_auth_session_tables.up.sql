-- Migration 000008: Phase 2 auth core, part 2 (systemdesign.md §7, status.md
-- Phase 2 Punkte 6-9). Local-password auth identities, sessions bound to one
-- access token, and refresh-token rotation lineage.
--
-- Deliberately simplified relative to systemdesign.md §7.2/§7.3 for this
-- increment (see systemdesign.md for the reconciled schema):
--   * auth_identities is local-password-only for now (no provider/OIDC
--     columns) - D-5 in systemdesign.md §12 is still open.
--   * sessions carries only what Login/RefreshSession need today
--     (created_at/expires_at/revoked_at); an idle-vs-absolute-timeout split,
--     client_name and revoke_reason can follow in a later migration once
--     those use cases are actually built (YAGNI, not an oversight).
--   * refresh token family revocation (SEC-08) uses a flat `family_id`
--     instead of walking a previous_token_id linked list: revoking a family
--     is `UPDATE refresh_tokens SET revoked_at = now() WHERE family_id = $1`.
--     `successor_token_id IS NOT NULL` marks a token as already consumed -
--     there is no separate `consumed_at` column.
--
-- No user is ever hard-deleted (systemdesign.md §5.2: "deactivated, not
-- deleted"), so every FK to users/sessions/refresh_tokens uses ON DELETE
-- RESTRICT, matching the convention already used everywhere else in this
-- schema (migrations/README.md Löschregeln) rather than introducing the only
-- CASCADE in the whole schema.

CREATE TABLE auth_identities (
    user_id       text NOT NULL PRIMARY KEY REFERENCES users(id) ON DELETE RESTRICT,
    password_hash text NOT NULL
);

CREATE TABLE sessions (
    id          uuid NOT NULL PRIMARY KEY,
    user_id     text NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    active_role text NOT NULL,
    token_hash  bytea NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT clock_timestamp(),
    expires_at  timestamptz NOT NULL,
    revoked_at  timestamptz NULL,
    CONSTRAINT uq_sessions_token_hash UNIQUE (token_hash),
    CONSTRAINT chk_sessions_active_role_valid CHECK (
        active_role IN ('technician', 'dispatcher', 'admin')
    ),
    CONSTRAINT chk_sessions_expires_after_created CHECK (expires_at > created_at)
);

CREATE TABLE refresh_tokens (
    id                 uuid NOT NULL PRIMARY KEY,
    session_id         uuid NOT NULL REFERENCES sessions(id) ON DELETE RESTRICT,
    token_hash         bytea NOT NULL,
    family_id          uuid NOT NULL,
    successor_token_id uuid NULL REFERENCES refresh_tokens(id) ON DELETE RESTRICT,
    created_at         timestamptz NOT NULL DEFAULT clock_timestamp(),
    expires_at         timestamptz NOT NULL,
    revoked_at         timestamptz NULL,
    CONSTRAINT uq_refresh_tokens_token_hash UNIQUE (token_hash),
    CONSTRAINT uq_refresh_tokens_successor UNIQUE (successor_token_id),
    CONSTRAINT chk_refresh_tokens_expires_after_created CHECK (expires_at > created_at)
);

CREATE INDEX idx_sessions_user_id ON sessions (user_id);
CREATE INDEX idx_refresh_tokens_session_id ON refresh_tokens (session_id);
CREATE INDEX idx_refresh_tokens_family_id ON refresh_tokens (family_id);
