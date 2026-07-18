DROP INDEX IF EXISTS idx_idempotency_outcomes_actor_seq;

DROP INDEX IF EXISTS idx_audit_events_server_recorded_at;
DROP INDEX IF EXISTS idx_audit_events_actor_timeline;
DROP INDEX IF EXISTS idx_audit_events_entity_timeline;

DROP INDEX IF EXISTS uq_allocations_single_active_resource;
DROP INDEX IF EXISTS idx_allocations_dispatch_window;
DROP INDEX IF EXISTS idx_allocations_planned_until;
DROP INDEX IF EXISTS idx_allocations_status;
DROP INDEX IF EXISTS idx_allocations_resource_id;
DROP INDEX IF EXISTS idx_allocations_request_id;

DROP INDEX IF EXISTS idx_request_resource_classes_resource_class_id;
DROP INDEX IF EXISTS idx_requests_status_updated_at;
DROP INDEX IF EXISTS idx_requests_context_ref;
DROP INDEX IF EXISTS idx_requests_technician_id;
DROP INDEX IF EXISTS idx_requests_status;

DROP INDEX IF EXISTS idx_resources_dispatch_lookup;
DROP INDEX IF EXISTS idx_resources_valid_until;