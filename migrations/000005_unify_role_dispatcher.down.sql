-- No role-value change to revert; re-establishes the same constraints.
ALTER TABLE users
    DROP CONSTRAINT IF EXISTS chk_users_role_valid;

ALTER TABLE users
    ADD CONSTRAINT chk_users_role_valid CHECK (
        role IN ('technician', 'dispatcher', 'admin')
    );

ALTER TABLE audit_events DISABLE TRIGGER USER;

ALTER TABLE audit_events
    DROP CONSTRAINT IF EXISTS chk_audit_events_actor_role_valid;

ALTER TABLE audit_events
    ADD CONSTRAINT chk_audit_events_actor_role_valid CHECK (
        actor_role IN ('technician', 'dispatcher', 'system')
    );

ALTER TABLE audit_events ENABLE TRIGGER USER;
