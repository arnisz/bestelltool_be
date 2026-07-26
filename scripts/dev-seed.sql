INSERT INTO users (id, role, display_name)
VALUES
    ('dev-dispatcher', 'dispatcher', 'Dev Dispatcher'),
    ('dev-technician', 'technician', 'Dev Technician')
ON CONFLICT (id) DO NOTHING;