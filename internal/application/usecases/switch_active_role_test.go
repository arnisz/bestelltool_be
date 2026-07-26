package usecases

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/domain"
)

type rollbackUnitOfWork struct {
	tx        *fakeTransaction
	commits   int
	rollbacks int
}

func (uow *rollbackUnitOfWork) WithinTransaction(ctx context.Context, fn func(ctx context.Context, tx ports.Transaction) error) error {
	snapshot := cloneFakeTransaction(uow.tx)
	err := fn(ctx, uow.tx)
	if err != nil {
		uow.rollbacks++
		*uow.tx = *snapshot
		return err
	}
	uow.commits++
	return nil
}

func cloneFakeTransaction(src *fakeTransaction) *fakeTransaction {
	if src == nil {
		return nil
	}
	return &fakeTransaction{
		users:         src.users,
		userRoles:     cloneFakeUserRoleRepository(src.userRoles),
		authIds:       src.authIds,
		sessions:      cloneFakeSessionRepository(src.sessions),
		refreshTokens: cloneFakeRefreshTokenRepository(src.refreshTokens),
		audits:        cloneFakeAuditWriter(src.audits),
	}
}

func cloneFakeUserRoleRepository(src *fakeUserRoleRepository) *fakeUserRoleRepository {
	if src == nil {
		return nil
	}
	clone := &fakeUserRoleRepository{
		roles: append([]domain.ActorRole(nil), src.roles...),
		err:   src.err,
	}
	if src.hasRoleForUpdate != nil {
		clone.hasRoleForUpdate = new(*src.hasRoleForUpdate)
	}
	return clone
}

func cloneFakeSessionRepository(src *fakeSessionRepository) *fakeSessionRepository {
	if src == nil {
		return nil
	}
	clonedSessions := make(map[string]*ports.Session, len(src.sessions))
	for id, session := range src.sessions {
		clonedSessions[id] = cloneSession(session)
	}
	return &fakeSessionRepository{
		sessions:     clonedSessions,
		err:          src.err,
		saveErr:      src.saveErr,
		updateErr:    src.updateErr,
		getErr:       src.getErr,
		revokeErr:    src.revokeErr,
		revokeAllErr: src.revokeAllErr,
	}
}

func cloneSession(src *ports.Session) *ports.Session {
	if src == nil {
		return nil
	}
	clone := *src
	clone.TokenHash = bytes.Clone(src.TokenHash)
	if src.RevokedAt != nil {
		clone.RevokedAt = new(*src.RevokedAt)
	}
	return &clone
}

func cloneFakeRefreshTokenRepository(src *fakeRefreshTokenRepository) *fakeRefreshTokenRepository {
	if src == nil {
		return nil
	}
	clonedTokens := make(map[string]*ports.RefreshToken, len(src.tokens))
	for id, token := range src.tokens {
		clonedTokens[id] = cloneRefreshToken(token)
	}
	return &fakeRefreshTokenRepository{tokens: clonedTokens, err: src.err}
}

func cloneRefreshToken(src *ports.RefreshToken) *ports.RefreshToken {
	if src == nil {
		return nil
	}
	clone := *src
	clone.TokenHash = bytes.Clone(src.TokenHash)
	clone.EncryptedSuccessor = bytes.Clone(src.EncryptedSuccessor)
	if src.SuccessorTokenID != nil {
		clone.SuccessorTokenID = new(*src.SuccessorTokenID)
	}
	if src.RevokedAt != nil {
		clone.RevokedAt = new(*src.RevokedAt)
	}
	return &clone
}

func cloneFakeAuditWriter(src *fakeAuditWriter) *fakeAuditWriter {
	if src == nil {
		return nil
	}
	return &fakeAuditWriter{events: append([]domain.AuditEvent(nil), src.events...), txIDs: append([]int(nil), src.txIDs...), failOnWrite: src.failOnWrite}
}

func TestSwitchActiveRoleCreatesNewSessionAndRevokesCurrentSession(t *testing.T) {
	now := time.Date(2026, time.July, 26, 16, 0, 0, 0, time.UTC)
	oldSession := &ports.Session{ID: "old-session", UserID: "user-1", ActiveRole: domain.ActorRoleTechnician, ExpiresAt: now.Add(time.Hour)}
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

func TestSwitchActiveRoleUserRolesErrorRollsBack(t *testing.T) {
	now := time.Date(2026, time.July, 26, 16, 0, 0, 0, time.UTC)
	oldSession := &ports.Session{ID: "old-session", UserID: "user-1", ActiveRole: domain.ActorRoleTechnician, ExpiresAt: now.Add(time.Hour)}
	tx := &fakeTransaction{
		sessions:      &fakeSessionRepository{sessions: map[string]*ports.Session{oldSession.ID: oldSession}},
		refreshTokens: &fakeRefreshTokenRepository{tokens: map[string]*ports.RefreshToken{}},
		userRoles:     &fakeUserRoleRepository{err: errors.New("role lock failed")},
		audits:        &fakeAuditWriter{},
	}
	uow := &rollbackUnitOfWork{tx: tx}
	uc := NewSwitchActiveRoleUseCase(uow, &fakeSecretGenerator{tokens: []string{"access-secret", "refresh-secret"}}, &fakeClock{now: now})

	_, err := uc.Execute(t.Context(), SwitchActiveRoleInput{UserID: oldSession.UserID, CurrentSessionID: oldSession.ID, RequestedRole: domain.ActorRoleDispatcher})
	if err == nil {
		t.Fatal("Execute() error = nil, want role lock failure")
	}
	if tx.sessions.sessions[oldSession.ID].RevokedAt != nil {
		t.Fatal("old session must not be revoked on role lock failure")
	}
	if len(tx.sessions.sessions) != 1 || len(tx.refreshTokens.tokens) != 0 || len(tx.audits.events) != 0 {
		t.Fatalf("rollback failed: sessions=%d refresh=%d audits=%d", len(tx.sessions.sessions), len(tx.refreshTokens.tokens), len(tx.audits.events))
	}
	if uow.rollbacks != 1 || uow.commits != 0 {
		t.Fatalf("uow state: rollbacks=%d commits=%d, want 1/0", uow.rollbacks, uow.commits)
	}
}

func TestSwitchActiveRoleSessionSaveErrorRollsBack(t *testing.T) {
	now := time.Date(2026, time.July, 26, 16, 0, 0, 0, time.UTC)
	oldSession := &ports.Session{ID: "old-session", UserID: "user-1", ActiveRole: domain.ActorRoleTechnician, ExpiresAt: now.Add(time.Hour)}
	tx := &fakeTransaction{
		sessions:      &fakeSessionRepository{sessions: map[string]*ports.Session{oldSession.ID: oldSession}, saveErr: errors.New("save failed")},
		refreshTokens: &fakeRefreshTokenRepository{tokens: map[string]*ports.RefreshToken{}},
		userRoles:     &fakeUserRoleRepository{roles: []domain.ActorRole{domain.ActorRoleTechnician, domain.ActorRoleDispatcher}},
		audits:        &fakeAuditWriter{},
	}
	uow := &rollbackUnitOfWork{tx: tx}
	uc := NewSwitchActiveRoleUseCase(uow, &fakeSecretGenerator{tokens: []string{"access-secret", "refresh-secret"}}, &fakeClock{now: now})

	_, err := uc.Execute(t.Context(), SwitchActiveRoleInput{UserID: oldSession.UserID, CurrentSessionID: oldSession.ID, RequestedRole: domain.ActorRoleDispatcher})
	if err == nil {
		t.Fatal("Execute() error = nil, want session save failure")
	}
	if tx.sessions.sessions[oldSession.ID].RevokedAt != nil {
		t.Fatal("old session must be rolled back to active on save failure")
	}
	if len(tx.sessions.sessions) != 1 || len(tx.refreshTokens.tokens) != 0 || len(tx.audits.events) != 0 {
		t.Fatalf("rollback failed: sessions=%d refresh=%d audits=%d", len(tx.sessions.sessions), len(tx.refreshTokens.tokens), len(tx.audits.events))
	}
	if uow.rollbacks != 1 || uow.commits != 0 {
		t.Fatalf("uow state: rollbacks=%d commits=%d, want 1/0", uow.rollbacks, uow.commits)
	}
}

func TestSwitchActiveRoleAuditWriteErrorRollsBack(t *testing.T) {
	now := time.Date(2026, time.July, 26, 16, 0, 0, 0, time.UTC)
	oldSession := &ports.Session{ID: "old-session", UserID: "user-1", ActiveRole: domain.ActorRoleTechnician, ExpiresAt: now.Add(time.Hour)}
	tx := &fakeTransaction{
		sessions:      &fakeSessionRepository{sessions: map[string]*ports.Session{oldSession.ID: oldSession}},
		refreshTokens: &fakeRefreshTokenRepository{tokens: map[string]*ports.RefreshToken{}},
		userRoles:     &fakeUserRoleRepository{roles: []domain.ActorRole{domain.ActorRoleTechnician, domain.ActorRoleDispatcher}},
		audits:        &fakeAuditWriter{failOnWrite: true},
	}
	uow := &rollbackUnitOfWork{tx: tx}
	uc := NewSwitchActiveRoleUseCase(uow, &fakeSecretGenerator{tokens: []string{"access-secret", "refresh-secret"}}, &fakeClock{now: now})

	_, err := uc.Execute(t.Context(), SwitchActiveRoleInput{UserID: oldSession.UserID, CurrentSessionID: oldSession.ID, RequestedRole: domain.ActorRoleDispatcher})
	if !errors.Is(err, errAuditFailed) {
		t.Fatalf("Execute() error = %v, want errAuditFailed", err)
	}
	if tx.sessions.sessions[oldSession.ID].RevokedAt != nil {
		t.Fatal("old session must be rolled back to active on audit failure")
	}
	if len(tx.sessions.sessions) != 1 || len(tx.refreshTokens.tokens) != 0 || len(tx.audits.events) != 0 {
		t.Fatalf("rollback failed: sessions=%d refresh=%d audits=%d", len(tx.sessions.sessions), len(tx.refreshTokens.tokens), len(tx.audits.events))
	}
	if uow.rollbacks != 1 || uow.commits != 0 {
		t.Fatalf("uow state: rollbacks=%d commits=%d, want 1/0", uow.rollbacks, uow.commits)
	}
}
