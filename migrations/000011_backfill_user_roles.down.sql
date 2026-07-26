DELETE FROM user_roles WHERE assigned_by = 'system-bootstrap';
DELETE FROM users WHERE id = 'system-bootstrap';