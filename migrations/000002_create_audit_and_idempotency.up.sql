CREATE TABLE audit_events (
    id text PRIMARY KEY,
    client_occurred_at timestamptz NULL,
    client_seq bigint NULL,
    server_recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    actor_id text NOT NULL,
    actor_role text NOT NULL,
    entity_type text NOT NULL,
    entity_id text NOT NULL,
    action text NOT NULL,
    from_status text NOT NULL DEFAULT '',
    to_status text NOT NULL DEFAULT '',
    note text NOT NULL DEFAULT '',
    CONSTRAINT fk_audit_events_actor
        FOREIGN KEY (actor_id) REFERENCES users(id) ON DELETE RESTRICT,
    CONSTRAINT chk_audit_events_client_seq_non_negative CHECK (client_seq IS NULL OR client_seq >= 0),
    CONSTRAINT chk_audit_events_actor_role_valid CHECK (
        actor_role IN ('technician', 'dispatcher', 'system')
    ),
    CONSTRAINT chk_audit_events_entity_type_valid CHECK (
        entity_type IN ('request', 'allocation', 'resource')
    )
);

CREATE TABLE idempotency_outcomes (
    client_action_id text PRIMARY KEY,
    status_code integer NOT NULL,
    payload bytea NOT NULL DEFAULT ''::bytea,
    error_text text NOT NULL DEFAULT '',
    processed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    actor_id text NULL,
    client_seq bigint NULL,
    action_type text NULL,
    entity_type text NULL,
    entity_id text NULL,
    CONSTRAINT fk_idempotency_outcomes_actor
        FOREIGN KEY (actor_id) REFERENCES users(id) ON DELETE RESTRICT,
    CONSTRAINT chk_idempotency_outcomes_client_seq_non_negative CHECK (client_seq IS NULL OR client_seq >= 0),
    CONSTRAINT chk_idempotency_outcomes_entity_type_valid CHECK (
        entity_type IS NULL OR entity_type IN ('request', 'allocation', 'resource')
    )
);

CREATE FUNCTION reject_audit_events_mutation()
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