DROP TRIGGER IF EXISTS trg_audit_events_no_delete ON audit_events;
DROP TRIGGER IF EXISTS trg_audit_events_no_update ON audit_events;
DROP FUNCTION IF EXISTS reject_audit_events_mutation();

DROP TABLE IF EXISTS idempotency_outcomes;
DROP TABLE IF EXISTS audit_events;