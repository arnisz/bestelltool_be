-- Align audit_events.actor_role to use 'dispatcher' as the canonical value.
ALTER TABLE audit_events DISABLE TRIGGER USER;

ALTER TABLE audit_events
    DROP CONSTRAINT IF EXISTS chk_audit_events_actor_role_valid;

ALTER TABLE audit_events
    ADD CONSTRAINT chk_audit_events_actor_role_valid CHECK (
        actor_role IN ('technician', 'dispatcher', 'system')
    );

ALTER TABLE audit_events ENABLE TRIGGER USER;
