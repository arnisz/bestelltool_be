-- Migration 000006: Phase 1 audit foundation (systemdesign.md §8, §11 SEC-20).
-- Extends audit_events for administrative actions and hardens append-only
-- enforcement with a stable SQLSTATE and a TRUNCATE guard.

-- 1. Append-only trigger function.
-- Idempotent: CREATE OR REPLACE so re-running this migration is safe.
-- Uses ERRCODE 42501 (insufficient_privilege) so callers can distinguish this
-- from any other error by SQLSTATE instead of matching error text (E3).
CREATE OR REPLACE FUNCTION reject_audit_events_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'audit_events is append-only' USING ERRCODE = '42501';
END;
$$;

-- 2. (Re-)install the append-only triggers. DROP IF EXISTS before CREATE keeps
-- this idempotent. UPDATE/DELETE are row-level (existing behaviour since
-- migration 000002); TRUNCATE is statement-level, since PostgreSQL does not
-- support FOR EACH ROW triggers on TRUNCATE.
DROP TRIGGER IF EXISTS trg_audit_events_no_update ON audit_events;
DROP TRIGGER IF EXISTS trg_audit_events_no_delete ON audit_events;
DROP TRIGGER IF EXISTS trg_audit_events_no_truncate ON audit_events;

CREATE TRIGGER trg_audit_events_no_update
BEFORE UPDATE ON audit_events
FOR EACH ROW
EXECUTE FUNCTION reject_audit_events_mutation();

CREATE TRIGGER trg_audit_events_no_delete
BEFORE DELETE ON audit_events
FOR EACH ROW
EXECUTE FUNCTION reject_audit_events_mutation();

CREATE TRIGGER trg_audit_events_no_truncate
BEFORE TRUNCATE ON audit_events
FOR EACH STATEMENT
EXECUTE FUNCTION reject_audit_events_mutation();

-- 3. actor_role: add 'admin' (systemdesign.md §8.1). No foreign key to
-- roles(code) - the audit trail must stay readable and insertable
-- independently of mutable master data (SEC-20 rationale).
ALTER TABLE audit_events
    DROP CONSTRAINT IF EXISTS chk_audit_events_actor_role_valid;

ALTER TABLE audit_events
    ADD CONSTRAINT chk_audit_events_actor_role_valid CHECK (
        actor_role IN ('technician', 'dispatcher', 'admin', 'system')
    );

-- 4. entity_type: add administrative entity types (systemdesign.md §8.1).
ALTER TABLE audit_events
    DROP CONSTRAINT IF EXISTS chk_audit_events_entity_type_valid;

ALTER TABLE audit_events
    ADD CONSTRAINT chk_audit_events_entity_type_valid CHECK (
        entity_type IN (
            'request', 'allocation', 'resource',
            'user', 'role', 'user_role', 'resource_class',
            'resource_class_membership', 'session', 'auth_identity'
        )
    );

-- 5. Privilege hardening groundwork (SEC-20, SEC-24, SEC-25 preparation only).
-- No dedicated least-privilege application role exists yet (that is Phase 5,
-- systemdesign.md §13). Today the application connects with the same role
-- that owns these tables, and table owners are unaffected by REVOKE ... FROM
-- PUBLIC. This statement is therefore preparatory only - it does not yet
-- provide real enforcement against the application role, which still gets
-- its append-only guarantee exclusively from the trigger above. Real
-- enforcement follows once Phase 5 introduces a dedicated, non-owner
-- application role - see docs/deployment.md.
REVOKE UPDATE, DELETE ON audit_events FROM PUBLIC;
