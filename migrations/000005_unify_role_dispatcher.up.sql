-- Migration 000005: Confirm 'dispatcher' as the single canonical role value
-- in users.role and audit_events.actor_role across all layers.

-- 1. users table
ALTER TABLE users
    DROP CONSTRAINT IF EXISTS chk_users_role_valid;

ALTER TABLE users
    ADD CONSTRAINT chk_users_role_valid CHECK (
        role IN ('technician', 'dispatcher', 'admin')
    );

-- 2. audit_events table
ALTER TABLE audit_events DISABLE TRIGGER USER;

ALTER TABLE audit_events
    DROP CONSTRAINT IF EXISTS chk_audit_events_actor_role_valid;

ALTER TABLE audit_events
    ADD CONSTRAINT chk_audit_events_actor_role_valid CHECK (
        actor_role IN ('technician', 'dispatcher', 'system')
    );

ALTER TABLE audit_events ENABLE TRIGGER USER;
