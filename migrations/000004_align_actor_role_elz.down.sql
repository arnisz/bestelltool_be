ALTER TABLE audit_events DISABLE TRIGGER USER;

ALTER TABLE audit_events
DROP CONSTRAINT IF EXISTS chk_audit_events_actor_role_valid;

UPDATE audit_events
SET actor_role = 'dispatcher'
WHERE actor_role = 'elz';

ALTER TABLE audit_events
ADD CONSTRAINT chk_audit_events_actor_role_valid CHECK (
    actor_role IN ('technician', 'dispatcher', 'system')
);

ALTER TABLE audit_events ENABLE TRIGGER USER;
