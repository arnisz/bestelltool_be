CREATE TABLE roles (
                       code          text PRIMARY KEY,
                       display_name  text    NOT NULL,
                       description   text    NOT NULL DEFAULT '',
                       is_assignable boolean NOT NULL DEFAULT true
);

CREATE TABLE permissions (
                             code        text PRIMARY KEY,
                             description text NOT NULL DEFAULT ''
);

CREATE TABLE role_permissions (
                                  role_code       text NOT NULL REFERENCES roles(code)       ON DELETE RESTRICT,
                                  permission_code text NOT NULL REFERENCES permissions(code) ON DELETE RESTRICT,
                                  PRIMARY KEY (role_code, permission_code)
);

CREATE TABLE user_roles (
                            user_id     text        NOT NULL REFERENCES users(id)  ON DELETE RESTRICT,
                            role_code   text        NOT NULL REFERENCES roles(code) ON DELETE RESTRICT,
                            assigned_at timestamptz NOT NULL DEFAULT clock_timestamp(),
                            assigned_by text        NOT NULL REFERENCES users(id)  ON DELETE RESTRICT,
                            valid_from  timestamptz,
                            valid_until timestamptz,
                            PRIMARY KEY (user_id, role_code),
                            CONSTRAINT ck_user_roles_no_self_assignment CHECK (assigned_by <> user_id)
);

CREATE INDEX idx_user_roles_role_code ON user_roles(role_code);

INSERT INTO roles (code, display_name, description, is_assignable) VALUES
                                                                       ('technician', 'Technician', 'Requests and uses resources in the field', true),
                                                                       ('dispatcher', 'Dispatcher', 'Allocates resources and manages the operational pool', true),
                                                                       ('admin',      'Admin',      'Maintains master data, users and roles', true),
                                                                       ('system',     'System',     'Internal background operations, never assignable to a person', false);

INSERT INTO permissions (code, description) VALUES
                                                ('request.create',             'Create a resource request'),
                                                ('request.read',               'Read resource requests'),
                                                ('allocation.return_request',  'Request the return of an allocation'),
                                                ('allocation.manage',          'Create, approve and manage allocations'),
                                                ('resource.transfer_direct',   'Directly transfer a resource between sites'),
                                                ('event.stream.own',           'Receive own operational events via SSE'),
                                                ('event.stream.all',           'Receive all operational events via SSE'),
                                                ('resource_class.read',          'Read resource classes'),
                                                ('resource_class.create',        'Create resource classes'),
                                                ('resource_class.update',        'Update resource classes'),
                                                ('resource_class.deactivate',    'Deactivate resource classes'),
                                                ('resource_class.delete_unused', 'Delete unused resource classes'),
                                                ('resource.read',                'Read resources'),
                                                ('resource.create',              'Create resources'),
                                                ('resource.update_master_data',  'Update resource master data'),
                                                ('resource.deactivate',          'Deactivate resources'),
                                                ('resource.delete_unused',       'Delete unused resources'),
                                                ('user.read',           'Read users'),
                                                ('user.create',         'Create users'),
                                                ('user.update',         'Update users'),
                                                ('user.disable',        'Disable users'),
                                                ('user.reactivate',     'Reactivate users'),
                                                ('user.reset_password', 'Reset a user''s local password'),
                                                ('role.assign',         'Assign a role to a user'),
                                                ('role.revoke',         'Revoke a role from a user'),
                                                ('session.revoke',      'Revoke a user''s sessions'),
                                                ('audit.read',          'Read audit events');

INSERT INTO role_permissions (role_code, permission_code) VALUES
                                                              ('technician', 'request.create'),
                                                              ('technician', 'request.read'),
                                                              ('technician', 'allocation.return_request'),
                                                              ('technician', 'resource_class.read'),
                                                              ('technician', 'resource.read'),
                                                              ('technician', 'event.stream.own'),

                                                              ('dispatcher', 'request.read'),
                                                              ('dispatcher', 'allocation.manage'),
                                                              ('dispatcher', 'allocation.return_request'),
                                                              ('dispatcher', 'resource.transfer_direct'),
                                                              ('dispatcher', 'resource_class.read'),
                                                              ('dispatcher', 'resource.read'),
                                                              ('dispatcher', 'event.stream.all'),

                                                              ('admin', 'resource_class.read'),
                                                              ('admin', 'resource_class.create'),
                                                              ('admin', 'resource_class.update'),
                                                              ('admin', 'resource_class.deactivate'),
                                                              ('admin', 'resource_class.delete_unused'),
                                                              ('admin', 'resource.read'),
                                                              ('admin', 'resource.create'),
                                                              ('admin', 'resource.update_master_data'),
                                                              ('admin', 'resource.deactivate'),
                                                              ('admin', 'resource.delete_unused'),
                                                              ('admin', 'user.read'),
                                                              ('admin', 'user.create'),
                                                              ('admin', 'user.update'),
                                                              ('admin', 'user.disable'),
                                                              ('admin', 'user.reactivate'),
                                                              ('admin', 'user.reset_password'),
                                                              ('admin', 'role.assign'),
                                                              ('admin', 'role.revoke'),
                                                              ('admin', 'session.revoke'),
                                                              ('admin', 'audit.read');