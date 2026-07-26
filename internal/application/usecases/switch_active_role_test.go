package usecases

import (
	"errors"
	"testing"
	"time"

	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/domain"
)

func TestSwitchActiveRoleCreatesNewSessionAndRevokesCurrentSession(t *testing.T) {
	now := time.Date(2026, time.July, 26, 16, 0, 0, 0, time.UTC)
	oldSession := &ports.Session{ID: "old-session", UserID: "user-1", ActiveRole: domain.ActorRoleTechnician}
	tx := &fakeTransaction{
		sessions:      &fakeSessionRepository{sessions: map[string]*ports.Session{oldSession.ID: oldSession}},
		refreshTokens: &fakeRefreshTokenRepository{tokens: map[string]*ports.RefreshToken{}},
		userRoles:     &fakeUserRoleRepository{roles: []domain.ActorRole{domain.ActorRoleTechnician, domain.ActorRoleDispatcher}},
		audits:        &fakeAuditWriter{},
	}
	uc := NewSwitchActiveRoleUseCase(
		&fakeUnitOfWork{tx: tx},
		&fakeSecretGenerator{tokens: []string{"access-secret", "refresh-secret"}},
		&fakeClock{now: now},
	)

	out, err := uc.Execute(t.Context(), SwitchActiveRoleInput{
		UserID:           oldSession.UserID,
		CurrentSessionID: oldSession.ID,
		RequestedRole:    domain.ActorRoleDispatcher,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if out.ActiveRole != domain.ActorRoleDispatcher {
		t.Fatalf("active role = %q, want %q", out.ActiveRole, domain.ActorRoleDispatcher)
	}
	if oldSession.RevokedAt == nil {
		t.Fatal("old session was not revoked")
	}
	if len(tx.sessions.sessions) != 2 {
		t.Fatalf("sessions = %d, want 2", len(tx.sessions.sessions))
	}
	if len(tx.audits.events) != 2 {
		t.Fatalf("audit events = %d, want 2", len(tx.audits.events))
	}
	if tx.audits.events[0].Action != string(domain.ActionSessionRevoke) || tx.audits.events[0].Note != "role_switch" {
		t.Fatalf("old-session audit = %+v, want session.revoke with role_switch", tx.audits.events[0])
	}
	if tx.audits.events[1].Action != string(domain.ActionSessionCreate) || tx.audits.events[1].Note != "role_switch" {
		t.Fatalf("new-session audit = %+v, want session.create with role_switch", tx.audits.events[1])
	}
}

func TestSwitchActiveRoleRejectsRoleNotHeld(t *testing.T) {
	tx := &fakeTransaction{
		sessions:      &fakeSessionRepository{sessions: map[string]*ports.Session{}},
		refreshTokens: &fakeRefreshTokenRepository{tokens: map[string]*ports.RefreshToken{}},
		userRoles:     &fakeUserRoleRepository{roles: []domain.ActorRole{domain.ActorRoleTechnician}},
		audits:        &fakeAuditWriter{},
	}
	uc := NewSwitchActiveRoleUseCase(
		&fakeUnitOfWork{tx: tx},
		&fakeSecretGenerator{tokens: []string{"must-not-be-used"}},
		&fakeClock{},
	)

	_, err := uc.Execute(t.Context(), SwitchActiveRoleInput{
		UserID:           "user-1",
		CurrentSessionID: "old-session",
		RequestedRole:    domain.ActorRoleAdmin,
	})
	if !errors.Is(err, ports.ErrForbidden) {
		t.Fatalf("Execute() error = %v, want ErrForbidden", err)
	}
	if len(tx.sessions.sessions) != 0 || len(tx.refreshTokens.tokens) != 0 || len(tx.audits.events) != 0 {
		t.Fatalf("forbidden switch changed state: sessions=%d refresh=%d audits=%d", len(tx.sessions.sessions), len(tx.refreshTokens.tokens), len(tx.audits.events))
	}
}
