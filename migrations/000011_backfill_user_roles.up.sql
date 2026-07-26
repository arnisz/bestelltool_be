-- Bootstrap-Actor: existiert ausschließlich als FK-Ziel für assigned_by,
-- niemals einloggbar (keine auth_identities-Zeile), niemals aktiv.
-- role='admin', weil chk_users_role_valid kein 'system' zulässt (bewusst,
-- SEC-17: system ist kein zuweisbarer/interaktiver Benutzer) und diese
-- Legacy-Spalte ohnehin mit Phase 7 (systemdesign.md §13) entfällt.
INSERT INTO users (id, username, display_name, role, is_active)
VALUES ('system-bootstrap', 'system-bootstrap', 'System (Migration Bootstrap)', 'admin', false)
    ON CONFLICT (id) DO NOTHING;

-- Backfill aus users.role, Bootstrap-Actor bewusst ausgeschlossen
-- (sonst Selbstzuweisung, Verstoß gegen ck_user_roles_no_self_assignment).
INSERT INTO user_roles (user_id, role_code, assigned_by, assigned_at)
SELECT id, role, 'system-bootstrap', created_at
FROM users
WHERE role IS NOT NULL
  AND id <> 'system-bootstrap'
    ON CONFLICT (user_id, role_code) DO NOTHING;