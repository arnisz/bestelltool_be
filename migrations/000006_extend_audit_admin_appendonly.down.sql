-- Down-migration for 000006: revert audit_events to its pre-000006 shape.
--
-- WARNING: this fails by design (CHECK constraint violation) if any row
-- already has actor_role = 'admin' or one of the new entity_type values
-- ('user', 'role', 'user_role', 'resource_class',
-- 'resource_class_membership', 'session', 'auth_identity'). Applying this
-- down-migration therefore requires those rows to be removed or migrated
-- first - see migrations/README.md.

-- Revert the append-only triggers/function to the pre-000006 shape: no
-- TRUNCATE guard, no explicit SQLSTATE on the raised exception.
DROP TRIGGER IF EXISTS trg_audit_events_no_truncate ON audit_events;
DROP TRIGGER IF EXISTS trg_audit_events_no_delete ON audit_events;
DROP TRIGGER IF EXISTS trg_audit_events_no_update ON audit_events;

CREATE OR REPLACE FUNCTION reject_audit_events_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'audit_events is append-only';
END;
$$;

CREATE TRIGGER trg_audit_events_no_update
BEFORE UPDATE ON audit_events
FOR EACH ROW
EXECUTE FUNCTION reject_audit_events_mutation();

CREATE TRIGGER trg_audit_events_no_delete
BEFORE DELETE ON audit_events
FOR EACH ROW
EXECUTE FUNCTION reject_audit_events_mutation();

-- Restore the pre-000006 CHECK constraints.
ALTER TABLE audit_events
    DROP CONSTRAINT IF EXISTS chk_audit_events_actor_role_valid;

ALTER TABLE audit_events
    ADD CONSTRAINT chk_audit_events_actor_role_valid CHECK (
        actor_role IN ('technician', 'dispatcher', 'system')
    );

ALTER TABLE audit_events
    DROP CONSTRAINT IF EXISTS chk_audit_events_entity_type_valid;

ALTER TABLE audit_events
    ADD CONSTRAINT chk_audit_events_entity_type_valid CHECK (
        entity_type IN ('request', 'allocation', 'resource')
    );

-- The REVOKE statements from 000006.up.sql are intentionally NOT reverted
-- here: they only revoke privileges from PUBLIC, which has none on this
-- table by default (the owning role is unaffected), so there is nothing to
-- restore - and re-granting to PUBLIC would be a regression, not a rollback.
