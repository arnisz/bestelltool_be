package usecases

import (
	"errors"
	"testing"
	"time"

	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/domain"
)

func TestLogoutRevokesSessionAndAudits(t *testing.T) {
	now := time.Date(2026, time.July, 26, 14, 0, 0, 0, time.UTC)
	session := &ports.Session{ID: "session-1", UserID: "user-1", ActiveRole: domain.ActorRoleTechnician, ExpiresAt: now.Add(time.Hour)}
	tx := &fakeTransaction{sessions: &fakeSessionRepository{sessions: map[string]*ports.Session{session.ID: session}}}
	uc := NewLogoutUseCase(&fakeUnitOfWork{tx: tx}, &fakeClock{now: now})

	err := uc.Execute(t.Context(), LogoutInput{SessionID: session.ID, ActorID: session.UserID, ActorRole: session.ActiveRole})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if session.RevokedAt == nil || !session.RevokedAt.Equal(now) {
		t.Fatalf("session revoked at = %v, want %v", session.RevokedAt, now)
	}
	events := tx.audits.events
	if len(events) != 1 || events[0].Action != string(domain.ActionSessionRevoke) {
		t.Fatalf("audit events = %#v, want one session.revoke event", events)
	}
}

func TestLogoutRejectsMismatchedPrincipal_SEC11(t *testing.T) {
	now := time.Date(2026, time.July, 26, 14, 0, 0, 0, time.UTC)
	session := &ports.Session{ID: "session-1", UserID: "user-1", ActiveRole: domain.ActorRoleTechnician, ExpiresAt: now.Add(time.Hour)}
	tx := &fakeTransaction{sessions: &fakeSessionRepository{sessions: map[string]*ports.Session{session.ID: session}}}
	uc := NewLogoutUseCase(&fakeUnitOfWork{tx: tx}, &fakeClock{now: now})

	err := uc.Execute(t.Context(), LogoutInput{SessionID: session.ID, ActorID: "other-user", ActorRole: session.ActiveRole})
	if !errors.Is(err, ports.ErrUnauthenticated) {
		t.Fatalf("Execute() error = %v, want ErrUnauthenticated", err)
	}
	if session.RevokedAt != nil {
		t.Fatal("session was revoked for a mismatched principal")
	}
}
