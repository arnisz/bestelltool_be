-- Down-migration for 000008: drop the auth/session tables in dependency
-- order (refresh_tokens references sessions; both reference users).
DROP TABLE IF EXISTS refresh_tokens;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS auth_identities;
