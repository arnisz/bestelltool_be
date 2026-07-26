package usecases

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
	"time"

	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/domain"
)

type fakeTokenEncryptor struct{}

func (fakeTokenEncryptor) Encrypt(plaintext []byte) ([]byte, error) {
	return append([]byte("encrypted:"), plaintext...), nil
}

func (fakeTokenEncryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	plain, ok := strings.CutPrefix(string(ciphertext), "encrypted:")
	if !ok {
		return nil, errors.New("invalid ciphertext")
	}
	return []byte(plain), nil
}

func newRefreshTestUseCase(t *testing.T, now time.Time) (*RefreshSessionUseCase, *fakeClock, *fakeTransaction, string) {
	t.Helper()
	originalToken := "rp_rt_original.original-secret"
	hash := sha256.Sum256([]byte("original-secret"))
	session := &ports.Session{
		ID:         "session-1",
		UserID:     "user-1",
		ActiveRole: domain.ActorRoleTechnician,
		CreatedAt:  now.Add(-time.Hour),
		ExpiresAt:  now.Add(time.Hour),
	}
	presented := &ports.RefreshToken{
		ID:        "original",
		SessionID: session.ID,
		TokenHash: hash[:],
		FamilyID:  "family-1",
		CreatedAt: now.Add(-time.Minute),
		ExpiresAt: now.Add(time.Hour),
	}
	tx := &fakeTransaction{
		sessions:      &fakeSessionRepository{sessions: map[string]*ports.Session{session.ID: session}},
		refreshTokens: &fakeRefreshTokenRepository{tokens: map[string]*ports.RefreshToken{presented.ID: presented}},
	}
	clock := &fakeClock{now: now}
	uc := NewRefreshSessionUseCaseWithConfig(
		&fakeUnitOfWork{tx: tx},
		&fakeSecretGenerator{tokens: []string{"rotated-access", "rotated-refresh", "retry-access"}},
		fakeTokenEncryptor{},
		clock,
		15*time.Minute,
		7*24*time.Hour,
		30*time.Second,
	)
	return uc, clock, tx, originalToken
}

func TestRefreshSessionGraceReturnsOriginalSuccessor_D2(t *testing.T) {
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	uc, clock, tx, originalToken := newRefreshTestUseCase(t, now)

	first, err := uc.Execute(t.Context(), RefreshSessionInput{RefreshToken: originalToken})
	if err != nil {
		t.Fatalf("first Execute() error = %v", err)
	}
	clock.now = now.Add(5 * time.Second)
	retry, err := uc.Execute(t.Context(), RefreshSessionInput{RefreshToken: originalToken})
	if err != nil {
		t.Fatalf("grace retry Execute() error = %v", err)
	}
	if retry.RefreshToken != first.RefreshToken {
		t.Fatalf("grace refresh token = %q, want existing successor %q", retry.RefreshToken, first.RefreshToken)
	}
	if retry.AccessToken == first.AccessToken {
		t.Fatal("grace retry returned the original access token")
	}
	if tx.sessions.sessions["session-1"].RevokedAt != nil {
		t.Fatal("session was revoked during grace retry")
	}
	if len(tx.refreshTokens.tokens) != 2 {
		t.Fatalf("refresh token count = %d, want 2 without a fork", len(tx.refreshTokens.tokens))
	}
}

func TestRefreshSessionReplayOutsideGraceRevokesFamily_SEC08(t *testing.T) {
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	uc, clock, tx, originalToken := newRefreshTestUseCase(t, now)
	if _, err := uc.Execute(t.Context(), RefreshSessionInput{RefreshToken: originalToken}); err != nil {
		t.Fatalf("first Execute() error = %v", err)
	}
	clock.now = now.Add(31 * time.Second)
	_, err := uc.Execute(context.Background(), RefreshSessionInput{RefreshToken: originalToken})
	if !errors.Is(err, ports.ErrTokenInvalid) {
		t.Fatalf("replay error = %v, want ErrTokenInvalid", err)
	}
	if tx.sessions.sessions["session-1"].RevokedAt == nil {
		t.Fatal("session was not revoked")
	}
	for id, token := range tx.refreshTokens.tokens {
		if token.RevokedAt == nil {
			t.Fatalf("refresh token %q was not revoked", id)
		}
	}
	if tx.audits == nil || len(tx.audits.events) != 2 {
		t.Fatalf("audit event count = %d, want rotation and replay events", len(tx.audits.events))
	}
	if got := tx.audits.events[1].Action; got != string(domain.ActionSessionReplayDetected) {
		t.Fatalf("replay audit action = %q, want %q", got, domain.ActionSessionReplayDetected)
	}
}
