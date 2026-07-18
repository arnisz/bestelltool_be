CREATE INDEX idx_resources_valid_until ON resources (valid_until);
CREATE INDEX idx_resources_dispatch_lookup ON resources (status, resource_class_id, valid_until);

CREATE INDEX idx_requests_status ON requests (status);
CREATE INDEX idx_requests_technician_id ON requests (technician_id);
CREATE INDEX idx_requests_context_ref ON requests (context_ref);
CREATE INDEX idx_requests_status_updated_at ON requests (status, updated_at);
CREATE INDEX idx_request_resource_classes_resource_class_id ON request_resource_classes (resource_class_id);

CREATE INDEX idx_allocations_request_id ON allocations (request_id);
CREATE INDEX idx_allocations_resource_id ON allocations (resource_id);
CREATE INDEX idx_allocations_status ON allocations (status);
CREATE INDEX idx_allocations_planned_until ON allocations (planned_until);
CREATE INDEX idx_allocations_dispatch_window ON allocations (resource_id, status, planned_from, planned_until);

CREATE UNIQUE INDEX uq_allocations_single_active_resource
    ON allocations (resource_id)
    WHERE status IN ('allocated', 'shipped', 'with_technician', 'return_requested', 'shipped_back', 'inspection');

CREATE INDEX idx_audit_events_entity_timeline ON audit_events (entity_type, entity_id, server_recorded_at);
CREATE INDEX idx_audit_events_actor_timeline ON audit_events (actor_id, server_recorded_at);
CREATE INDEX idx_audit_events_server_recorded_at ON audit_events (server_recorded_at);

CREATE INDEX idx_idempotency_outcomes_actor_seq ON idempotency_outcomes (actor_id, client_seq);