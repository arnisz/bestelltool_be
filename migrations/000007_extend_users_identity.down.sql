-- Down-migration for 000007: revert `users` to its pre-000007 shape.
-- Destructive by nature (any real username/email/version data is lost) - this
-- is expected for a column-drop down-migration, not a "fails by design" case.

DROP INDEX IF EXISTS uq_users_username;

ALTER TABLE users
    DROP COLUMN IF EXISTS version,
    DROP COLUMN IF EXISTS email,
    DROP COLUMN IF EXISTS username;
