package usecases

import (
	"errors"
	"testing"
	"time"

	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/domain"
)

func TestChangeOwnPasswordSuccessRevokesOtherSessionsAndAudits_SEC10(t *testing.T) {
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	user, err := domain.NewUser("user-123", "alice", domain.ActorRoleTechnician, "Alice", nil, now)
	if err != nil {
		t.Fatalf("NewUser() error = %v", err)
	}
	identity := &ports.AuthIdentity{UserID: user.ID, PasswordHash: "old-hash"}
	current := &ports.Session{ID: "session-current", UserID: user.ID, ActiveRole: domain.ActorRoleTechnician}
	other := &ports.Session{ID: "session-other", UserID: user.ID, ActiveRole: domain.ActorRoleTechnician}
	tx := &fakeTransaction{
		users:         &fakeUserRepository{users: map[domain.UserID]*domain.User{user.ID: user}, byUsername: map[string]*domain.User{user.Username: user}},
		authIds:       &fakeAuthIdentityRepository{identities: map[domain.UserID]*ports.AuthIdentity{user.ID: identity}},
		sessions:      &fakeSessionRepository{sessions: map[string]*ports.Session{current.ID: current, other.ID: other}},
		refreshTokens: &fakeRefreshTokenRepository{tokens: make(map[string]*ports.RefreshToken)},
	}
	uow := &fakeUnitOfWork{tx: tx}
	uc := NewChangeOwnPasswordUseCase(uow, &fakePasswordHasher{correctPassword: "old-password", correctHash: "old-hash"}, &fakeClock{now: now})

	err = uc.Execute(t.Context(), ChangeOwnPasswordInput{
		UserID:           user.ID,
		ActorRole:        domain.ActorRoleTechnician,
		CurrentSessionID: current.ID,
		OldPassword:      "old-password",
		NewPassword:      "new-password",
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if uow.transactionCalls != 1 {
		t.Fatalf("transaction calls = %d, want 1", uow.transactionCalls)
	}
	if identity.PasswordHash != "fake-hash-of-new-password" {
		t.Fatalf("password hash = %q", identity.PasswordHash)
	}
	if current.RevokedAt != nil {
		t.Fatal("current session was revoked")
	}
	if other.RevokedAt == nil {
		t.Fatal("other active session was not revoked")
	}
	if len(tx.audits.events) != 1 || tx.audits.events[0].Action != string(domain.ActionAuthPasswordChanged) {
		t.Fatalf("audit events = %#v, want password_changed event", tx.audits.events)
	}
}

func TestChangeOwnPasswordWrongOldPasswordAuditsFailure_SEC21(t *testing.T) {
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	user, err := domain.NewUser("user-123", "alice", domain.ActorRoleTechnician, "Alice", nil, now)
	if err != nil {
		t.Fatalf("NewUser() error = %v", err)
	}
	identity := &ports.AuthIdentity{UserID: user.ID, PasswordHash: "old-hash"}
	tx := &fakeTransaction{
		users:         &fakeUserRepository{users: map[domain.UserID]*domain.User{user.ID: user}, byUsername: map[string]*domain.User{user.Username: user}},
		authIds:       &fakeAuthIdentityRepository{identities: map[domain.UserID]*ports.AuthIdentity{user.ID: identity}},
		sessions:      &fakeSessionRepository{sessions: make(map[string]*ports.Session)},
		refreshTokens: &fakeRefreshTokenRepository{tokens: make(map[string]*ports.RefreshToken)},
	}
	uc := NewChangeOwnPasswordUseCase(&fakeUnitOfWork{tx: tx}, &fakePasswordHasher{correctPassword: "old-password", correctHash: "old-hash"}, &fakeClock{now: now})

	err = uc.Execute(t.Context(), ChangeOwnPasswordInput{
		UserID:           user.ID,
		ActorRole:        domain.ActorRoleTechnician,
		CurrentSessionID: "session-current",
		OldPassword:      "wrong-password",
		NewPassword:      "new-password",
	})
	if !errors.Is(err, ports.ErrCredentialsInvalid) {
		t.Fatalf("Execute() error = %v, want ErrCredentialsInvalid", err)
	}
	if identity.PasswordHash != "old-hash" {
		t.Fatalf("password hash = %q, want unchanged old hash", identity.PasswordHash)
	}
	if len(tx.audits.events) != 1 || tx.audits.events[0].Action != string(domain.ActionAuthPasswordChangeFailed) {
		t.Fatalf("audit events = %#v, want password_change_failed event", tx.audits.events)
	}
}

func TestChangeOwnPasswordRejectsMissingPasswords(t *testing.T) {
	uc := NewChangeOwnPasswordUseCase(nil, nil, nil)
	err := uc.Execute(t.Context(), ChangeOwnPasswordInput{
		UserID:           "user-123",
		ActorRole:        domain.ActorRoleTechnician,
		CurrentSessionID: "session-current",
	})
	if !errors.Is(err, domain.ErrRequiredField) {
		t.Fatalf("Execute() error = %v, want ErrRequiredField", err)
	}
}
