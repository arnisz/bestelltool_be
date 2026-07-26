package domain

// AuditAction identifies a canonical audit_events.action value for the
// identity/administration taxonomy described in systemdesign.md §8.1.
//
// AuditEvent.Action itself stays a plain string: existing operational use
// cases already populate it with their own descriptive strings (e.g.
// "complete_direct_transfer"), and changing that field's type is out of
// scope here. These constants exist so that the Phase 2-4 use cases that
// will emit administrative audit events (login, user/role management,
// session handling) reference a named value instead of a free-form string
// literal - convert with string(domain.ActionUserCreate) when building an
// AuditEvent.
type AuditAction string

const (
	// ActionUserCreate records that a user account was created.
	ActionUserCreate AuditAction = "user.create"
	// ActionUserUpdate records that a user's master data was changed.
	ActionUserUpdate AuditAction = "user.update"
	// ActionUserDisable records that a user was deactivated.
	ActionUserDisable AuditAction = "user.disable"
	// ActionUserReactivate records that a disabled user was reactivated.
	ActionUserReactivate AuditAction = "user.reactivate"
	// ActionUserPasswordReset records an admin-triggered password reset.
	ActionUserPasswordReset AuditAction = "user.password_reset"
	// ActionAuthPasswordChanged records a successful own-password change.
	ActionAuthPasswordChanged AuditAction = "auth.password_changed"
	// ActionAuthPasswordChangeFailed records a rejected own-password change.
	ActionAuthPasswordChangeFailed AuditAction = "auth.password_change_failed"

	// ActionRoleAssign records that a role was assigned to a user.
	ActionRoleAssign AuditAction = "role.assign"
	// ActionRoleRevoke records that a role was revoked from a user.
	ActionRoleRevoke AuditAction = "role.revoke"

	// ActionSessionCreate records that a session was created (login).
	ActionSessionCreate AuditAction = "session.create"
	// ActionSessionRefresh records a successful refresh-token rotation.
	ActionSessionRefresh AuditAction = "session.refresh"
	// ActionSessionRevoke records that a session was revoked.
	ActionSessionRevoke AuditAction = "session.revoke"
	// ActionSessionReplayDetected records detected refresh-token replay
	// (SEC-08): the entire token family and its session were revoked.
	ActionSessionReplayDetected AuditAction = "session.replay_detected"

	// ActionAuthLoginFailed records a failed login attempt (SEC-03).
	ActionAuthLoginFailed AuditAction = "auth.login_failed"

	// ActionResourceClassCreate records creation of a resource class.
	ActionResourceClassCreate AuditAction = "resource_class.create"
	// ActionResourceClassUpdate records an update to a resource class.
	ActionResourceClassUpdate AuditAction = "resource_class.update"
	// ActionResourceClassDeactivate records deactivation of a resource class.
	ActionResourceClassDeactivate AuditAction = "resource_class.deactivate"

	// ActionResourceCreate records creation of a resource.
	ActionResourceCreate AuditAction = "resource.create"
	// ActionResourceUpdate records an update to a resource's master data.
	ActionResourceUpdate AuditAction = "resource.update"
)
