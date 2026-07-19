CREATE TABLE users (
    id text PRIMARY KEY,
    role text NOT NULL,
    display_name text NOT NULL,
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT chk_users_role_valid CHECK (role IN ('technician', 'dispatcher', 'admin'))
);

CREATE TABLE resource_classes (
    id text PRIMARY KEY,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);

CREATE TABLE resources (
    id text PRIMARY KEY,
    resource_class_id text NOT NULL,
    serial_number text NOT NULL,
    status text NOT NULL,
    block_reason text NULL,
    block_note text NOT NULL DEFAULT '',
    holder_id text NULL,
    location text NOT NULL DEFAULT '',
    valid_until timestamptz NULL,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    version bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT fk_resources_resource_class
        FOREIGN KEY (resource_class_id) REFERENCES resource_classes(id) ON DELETE RESTRICT,
    CONSTRAINT fk_resources_holder
        FOREIGN KEY (holder_id) REFERENCES users(id) ON DELETE RESTRICT,
    CONSTRAINT uq_resources_serial_number UNIQUE (serial_number),
    CONSTRAINT chk_resources_status_valid CHECK (
        status IN (
            'available',
            'reserved',
            'issued',
            'in_use',
            'shipped_back',
            'inspection',
            'blocked',
            'externally_procured'
        )
    ),
    CONSTRAINT chk_resources_block_reason_valid CHECK (
        block_reason IS NULL OR block_reason IN ('defective', 'maintenance', 'inspection_due')
    ),
    CONSTRAINT chk_resources_block_reason_consistency CHECK (
        (status = 'blocked' AND block_reason IS NOT NULL)
        OR
        (status <> 'blocked' AND block_reason IS NULL)
    ),
    CONSTRAINT chk_resources_version_positive CHECK (version >= 1)
);

CREATE TABLE requests (
    id text PRIMARY KEY,
    technician_id text NOT NULL,
    status text NOT NULL,
    execution_state text NOT NULL,
    execution_note text NOT NULL DEFAULT '',
    context_ref text NOT NULL DEFAULT '',
    context_label text NOT NULL DEFAULT '',
    wish_from timestamptz NULL,
    wish_until timestamptz NULL,
    note text NOT NULL DEFAULT '',
    version bigint NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT fk_requests_technician
        FOREIGN KEY (technician_id) REFERENCES users(id) ON DELETE RESTRICT,
    CONSTRAINT chk_requests_status_valid CHECK (
        status IN (
            'open',
            'in_progress',
            'partially_allocated',
            'allocated',
            'active',
            'completed',
            'cancelled'
        )
    ),
    CONSTRAINT chk_requests_execution_state_valid CHECK (
        execution_state IN ('executable', 'partially_blocked', 'blocked')
    ),
    CONSTRAINT chk_requests_wish_range CHECK (
        wish_from IS NULL OR wish_until IS NULL OR wish_until > wish_from
    ),
    CONSTRAINT chk_requests_execution_note_required CHECK (
        (
            execution_state IN ('blocked', 'partially_blocked')
            AND btrim(execution_note) <> ''
        )
        OR
        (
            execution_state = 'executable'
            AND btrim(execution_note) = ''
        )
    ),
    CONSTRAINT chk_requests_version_positive CHECK (version >= 1)
);

CREATE TABLE request_resource_classes (
    request_id text NOT NULL,
    position bigint NOT NULL,
    resource_class_id text NOT NULL,
    CONSTRAINT pk_request_resource_classes PRIMARY KEY (request_id, position),
    CONSTRAINT fk_request_resource_classes_request
        FOREIGN KEY (request_id) REFERENCES requests(id) ON DELETE CASCADE,
    CONSTRAINT fk_request_resource_classes_resource_class
        FOREIGN KEY (resource_class_id) REFERENCES resource_classes(id) ON DELETE RESTRICT,
    CONSTRAINT chk_request_resource_classes_position_non_negative CHECK (position >= 0)
);

CREATE TABLE allocations (
    id text PRIMARY KEY,
    request_id text NOT NULL,
    resource_id text NOT NULL,
    status text NOT NULL,
    planned_from timestamptz NOT NULL,
    planned_until timestamptz NOT NULL,
    return_requested_at timestamptz NULL,
    shipped_at timestamptz NULL,
    received_at timestamptz NULL,
    version bigint NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT fk_allocations_request
        FOREIGN KEY (request_id) REFERENCES requests(id) ON DELETE RESTRICT,
    CONSTRAINT fk_allocations_resource
        FOREIGN KEY (resource_id) REFERENCES resources(id) ON DELETE RESTRICT,
    CONSTRAINT chk_allocations_status_valid CHECK (
        status IN (
            'allocated',
            'shipped',
            'with_technician',
            'return_requested',
            'shipped_back',
            'inspection',
            'completed',
            'cancelled'
        )
    ),
    CONSTRAINT chk_allocations_planned_range CHECK (planned_until > planned_from),
    CONSTRAINT chk_allocations_version_positive CHECK (version >= 1)
);