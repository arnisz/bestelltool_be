-- Migration 000007: Phase 2 auth core, part 1 (systemdesign.md §5.2,
-- status.md Phase 2 Punkt 5). Expands `users` towards real identity
-- management: a stable, unique `username` distinct from the internal `id`,
-- an optional `email`, and a `version` column for future optimistic locking.
--
-- NOTE: `created_at`/`updated_at` are NOT added here - they already exist on
-- `users` since migration 000001 (`timestamptz NOT NULL DEFAULT
-- clock_timestamp()`). Re-adding them would fail with "column already
-- exists"; this migration only adds what is actually missing.
--
-- `users.role` is deliberately left untouched - Phase 3 (roles/permissions)
-- handles role separation later (agents.md Migration Rules, Expand/Contract).

-- 1. Expand: add columns without NOT NULL/UNIQUE first. Dummy rows created by
-- earlier seeding (e.g. cmd/seed) have no username yet, so it cannot be
-- NOT NULL until backfilled.
ALTER TABLE users
    ADD COLUMN username text,
    ADD COLUMN email text,
    ADD COLUMN version integer NOT NULL DEFAULT 1;

-- 2. Backfill: give every existing row a username derived from its stable
-- `id`. This only has to be "good enough" for pre-existing dummy/seed data;
-- real users get a real, chosen username through the future CreateUser use
-- case (Phase 4).
UPDATE users SET username = id WHERE username IS NULL;

-- 3. Contract: username is now populated for every row, so it can be
-- required going forward.
ALTER TABLE users
    ALTER COLUMN username SET NOT NULL;

-- 4. Enforce uniqueness.
CREATE UNIQUE INDEX uq_users_username ON users (username);
