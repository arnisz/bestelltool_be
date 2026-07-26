package usecases

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/domain"
)

// Mock implementations for testing

type fakeUserRepository struct {
	users      map[domain.UserID]*domain.User
	byUsername map[string]*domain.User
	err        error
}

func (r *fakeUserRepository) GetByID(ctx context.Context, id domain.UserID) (*domain.User, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.users[id], nil
}

func (r *fakeUserRepository) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	if r.err != nil {
		return nil, r.err
	}
	if user, ok := r.byUsername[username]; ok {
		return user, nil
	}
	return nil, nil // Return nil user (which signals "not found" per SEC-03)
}

func (r *fakeUserRepository) Create(ctx context.Context, u *domain.User) error {
	if r.err != nil {
		return r.err
	}
	r.users[u.ID] = u
	r.byUsername[u.Username] = u
	return nil
}

type fakeAuthIdentityRepository struct {
	identities map[domain.UserID]*ports.AuthIdentity
	err        error
}

func (r *fakeAuthIdentityRepository) GetByUserID(ctx context.Context, userID domain.UserID) (*ports.AuthIdentity, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.identities[userID], nil
}

func (r *fakeAuthIdentityRepository) Save(ctx context.Context, identity *ports.AuthIdentity) error {
	if r.err != nil {
		return r.err
	}
	r.identities[identity.UserID] = identity
	return nil
}

type fakeSessionRepository struct {
	sessions map[string]*ports.Session
	err      error
}

func (r *fakeSessionRepository) Save(ctx context.Context, session *ports.Session) error {
	if r.err != nil {
		return r.err
	}
	r.sessions[session.ID] = session
	return nil
}

func (r *fakeSessionRepository) Update(ctx context.Context, session *ports.Session) error {
	return r.Save(ctx, session)
}

func (r *fakeSessionRepository) GetByID(ctx context.Context, id string) (*ports.Session, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.sessions[id], nil
}

func (r *fakeSessionRepository) GetByTokenHash(ctx context.Context, tokenHash []byte) (*ports.Session, error) {
	for _, session := range r.sessions {
		if string(session.TokenHash) == string(tokenHash) {
			return session, nil
		}
	}
	return nil, ports.ErrNotFound
}

func (r *fakeSessionRepository) GetForUpdate(ctx context.Context, id string) (*ports.Session, error) {
	return r.GetByID(ctx, id)
}

func (r *fakeSessionRepository) Revoke(ctx context.Context, id string, at time.Time) error {
	if r.err != nil {
		return r.err
	}
	if session, ok := r.sessions[id]; ok {
		session.RevokedAt = &at
	}
	return nil
}

func (r *fakeSessionRepository) RevokeAllForUserExcept(ctx context.Context, userID domain.UserID, exceptSessionID string, at time.Time) error {
	if r.err != nil {
		return r.err
	}
	for id, session := range r.sessions {
		if id != exceptSessionID && session.UserID == userID && session.RevokedAt == nil {
			session.RevokedAt = &at
		}
	}
	return nil
}

type fakeRefreshTokenRepository struct {
	tokens map[string]*ports.RefreshToken
	err    error
}

func (r *fakeRefreshTokenRepository) Save(ctx context.Context, token *ports.RefreshToken) error {
	if r.err != nil {
		return r.err
	}
	r.tokens[token.ID] = token
	return nil
}

func (r *fakeRefreshTokenRepository) GetByID(ctx context.Context, id string) (*ports.RefreshToken, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.tokens[id], nil
}

func (r *fakeRefreshTokenRepository) GetForUpdate(ctx context.Context, id string) (*ports.RefreshToken, error) {
	return r.GetByID(ctx, id)
}

func (r *fakeRefreshTokenRepository) Update(ctx context.Context, token *ports.RefreshToken) error {
	if r.err != nil {
		return r.err
	}
	r.tokens[token.ID] = token
	return nil
}

func (r *fakeRefreshTokenRepository) GetFamily(ctx context.Context, familyID string) ([]*ports.RefreshToken, error) {
	if r.err != nil {
		return nil, r.err
	}
	var result []*ports.RefreshToken
	for _, token := range r.tokens {
		if token.FamilyID == familyID {
			result = append(result, token)
		}
	}
	return result, nil
}

func (r *fakeRefreshTokenRepository) GetFamilyForUpdate(ctx context.Context, familyID string) ([]*ports.RefreshToken, error) {
	return r.GetFamily(ctx, familyID)
}

type fakePasswordHasher struct {
	correctPassword string
	correctHash     string
	needsRehash     bool
	err             error
	verifyCalls     int
	lastVerifyHash  string
	lastVerifyPwd   string
}

func (h *fakePasswordHasher) Hash(password string) (string, error) {
	if h.err != nil {
		return "", h.err
	}
	return "fake-hash-of-" + password, nil
}

func (h *fakePasswordHasher) Verify(password, hash string) (ok bool, needsRehash bool, err error) {
	h.verifyCalls++
	h.lastVerifyPwd = password
	h.lastVerifyHash = hash
	if h.err != nil {
		return false, false, h.err
	}
	if password == h.correctPassword && hash == h.correctHash {
		return true, h.needsRehash, nil
	}
	return false, false, nil
}

type fakeSecretGenerator struct {
	tokens []string
	index  int
}

func (g *fakeSecretGenerator) GenerateToken() (string, error) {
	if g.index >= len(g.tokens) {
		return "", errors.New("ran out of tokens")
	}
	token := g.tokens[g.index]
	g.index++
	return token, nil
}

type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	return c.now
}

type fakeTransaction struct {
	users         *fakeUserRepository
	authIds       *fakeAuthIdentityRepository
	sessions      *fakeSessionRepository
	refreshTokens *fakeRefreshTokenRepository
	audits        *fakeAuditWriter
}

func (tx *fakeTransaction) Users() ports.UserRepository {
	return tx.users
}

func (tx *fakeTransaction) AuthIdentities() ports.AuthIdentityRepository {
	return tx.authIds
}

func (tx *fakeTransaction) Sessions() ports.SessionRepository {
	return tx.sessions
}

func (tx *fakeTransaction) RefreshTokens() ports.RefreshTokenRepository {
	return tx.refreshTokens
}

func (tx *fakeTransaction) ResourceClasses() ports.ResourceClassRepository {
	panic("not implemented in test")
}

func (tx *fakeTransaction) Requests() ports.RequestRepository {
	panic("not implemented in test")
}

func (tx *fakeTransaction) Resources() ports.ResourceRepository {
	panic("not implemented in test")
}

func (tx *fakeTransaction) Allocations() ports.AllocationRepository {
	panic("not implemented in test")
}

func (tx *fakeTransaction) Audits() ports.AuditWriter {
	if tx.audits == nil {
		tx.audits = &fakeAuditWriter{}
	}
	return tx.audits
}

func (tx *fakeTransaction) AuditEvents() ports.AuditRepository {
	return tx.Audits().(*fakeAuditWriter)
}

func (tx *fakeTransaction) Idempotency() ports.IdempotencyStore {
	panic("not implemented in test")
}

type fakeUnitOfWork struct {
	tx               *fakeTransaction
	transactionCalls int
}

func (uow *fakeUnitOfWork) WithinTransaction(ctx context.Context, fn func(ctx context.Context, tx ports.Transaction) error) error {
	uow.transactionCalls++
	return fn(ctx, uow.tx)
}

// Test cases

func TestLoginSuccess(t *testing.T) {
	now := time.Now()
	clock := &fakeClock{now: now}

	// Create a test user
	user, _ := domain.NewUser(
		"user-123",
		"alice",
		domain.ActorRoleTechnician,
		"Alice",
		nil,
		now,
	)

	// Create auth identity with known hash
	authId := &ports.AuthIdentity{
		UserID:       user.ID,
		PasswordHash: "fake-hash-of-correct-password",
	}

	tx := &fakeTransaction{
		users: &fakeUserRepository{
			users:      map[domain.UserID]*domain.User{user.ID: user},
			byUsername: map[string]*domain.User{"alice": user},
		},
		authIds: &fakeAuthIdentityRepository{
			identities: map[domain.UserID]*ports.AuthIdentity{user.ID: authId},
		},
		sessions: &fakeSessionRepository{
			sessions: make(map[string]*ports.Session),
		},
		refreshTokens: &fakeRefreshTokenRepository{
			tokens: make(map[string]*ports.RefreshToken),
		},
	}

	uow := &fakeUnitOfWork{tx: tx}
	passwordHasher := &fakePasswordHasher{
		correctPassword: "correct-password",
		correctHash:     authId.PasswordHash,
		needsRehash:     false,
	}
	secretGen := &fakeSecretGenerator{
		tokens: []string{"access-secret", "refresh-secret"},
	}

	uc := NewLoginUseCaseWithTTLs(uow, passwordHasher, secretGen, clock, 15*time.Minute, 7*24*time.Hour)

	output, err := uc.Execute(context.Background(), LoginInput{
		Username: "alice",
		Password: "correct-password",
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if output == nil {
		t.Fatal("output is nil")
	}

	if !startsWith(output.AccessToken, "rp_at_") {
		t.Errorf("AccessToken missing prefix, got %q", output.AccessToken)
	}

	if !startsWith(output.RefreshToken, "rp_rt_") {
		t.Errorf("RefreshToken missing prefix, got %q", output.RefreshToken)
	}

	if output.ExpiresIn != 900 { // 15 minutes in seconds
		t.Errorf("ExpiresIn mismatch: got %d, want 900", output.ExpiresIn)
	}

	// Verify session was created
	if len(tx.sessions.sessions) != 1 {
		t.Errorf("Expected 1 session, got %d", len(tx.sessions.sessions))
	}

	// Verify refresh token was created
	if len(tx.refreshTokens.tokens) != 1 {
		t.Errorf("Expected 1 refresh token, got %d", len(tx.refreshTokens.tokens))
	}

	// Verify that tokens are valid family members
	for _, token := range tx.refreshTokens.tokens {
		if token.FamilyID == "" {
			t.Error("RefreshToken has empty FamilyID")
		}
		if token.SessionID == "" {
			t.Error("RefreshToken has empty SessionID")
		}
	}
	if tx.audits == nil || len(tx.audits.events) != 1 {
		t.Fatalf("login audit event count = %d, want 1", len(tx.audits.events))
	}
	if got := tx.audits.events[0].Action; got != string(domain.ActionSessionCreate) {
		t.Fatalf("login audit action = %q, want %q", got, domain.ActionSessionCreate)
	}
}

func TestLoginUserNotFound(t *testing.T) {
	now := time.Now()
	clock := &fakeClock{now: now}

	tx := &fakeTransaction{
		users: &fakeUserRepository{
			users:      make(map[domain.UserID]*domain.User),
			byUsername: make(map[string]*domain.User),
		},
		authIds: &fakeAuthIdentityRepository{
			identities: make(map[domain.UserID]*ports.AuthIdentity),
		},
		sessions: &fakeSessionRepository{
			sessions: make(map[string]*ports.Session),
		},
		refreshTokens: &fakeRefreshTokenRepository{
			tokens: make(map[string]*ports.RefreshToken),
		},
	}

	uow := &fakeUnitOfWork{tx: tx}
	passwordHasher := &fakePasswordHasher{
		correctPassword: "", // Will never match
		correctHash:     "",
		needsRehash:     false,
	}
	secretGen := &fakeSecretGenerator{
		tokens: []string{"access-secret", "refresh-secret"},
	}

	uc := NewLoginUseCaseWithTTLs(uow, passwordHasher, secretGen, clock, 15*time.Minute, 7*24*time.Hour)

	output, err := uc.Execute(context.Background(), LoginInput{
		Username: "nonexistent",
		Password: "any-password",
	})

	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	if !errors.Is(err, ports.ErrCredentialsInvalid) {
		t.Errorf("Expected ErrCredentialsInvalid, got %v", err)
	}

	if output != nil {
		t.Errorf("Expected nil output, got %v", output)
	}

	// SEC-03: Verify no session/token was created
	if len(tx.sessions.sessions) != 0 {
		t.Error("Session should not be created for non-existent user")
	}
	if len(tx.refreshTokens.tokens) != 0 {
		t.Error("RefreshToken should not be created for non-existent user")
	}
}

func TestLoginUserWithoutAuthIdentityReturnsGenericError_SEC03(t *testing.T) {
	now := time.Now()
	clock := &fakeClock{now: now}

	user, _ := domain.NewUser(
		"user-234",
		"charlie",
		domain.ActorRoleTechnician,
		"Charlie",
		nil,
		now,
	)

	tx := &fakeTransaction{
		users: &fakeUserRepository{
			users:      map[domain.UserID]*domain.User{user.ID: user},
			byUsername: map[string]*domain.User{"charlie": user},
		},
		authIds: &fakeAuthIdentityRepository{
			identities: make(map[domain.UserID]*ports.AuthIdentity),
		},
		sessions: &fakeSessionRepository{
			sessions: make(map[string]*ports.Session),
		},
		refreshTokens: &fakeRefreshTokenRepository{
			tokens: make(map[string]*ports.RefreshToken),
		},
	}

	uow := &fakeUnitOfWork{tx: tx}
	passwordHasher := &fakePasswordHasher{}
	secretGen := &fakeSecretGenerator{
		tokens: []string{"access-secret", "refresh-secret"},
	}

	uc := NewLoginUseCaseWithTTLs(uow, passwordHasher, secretGen, clock, 15*time.Minute, 7*24*time.Hour)

	output, err := uc.Execute(context.Background(), LoginInput{
		Username: "charlie",
		Password: "input-password",
	})

	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	if !errors.Is(err, ports.ErrCredentialsInvalid) {
		t.Errorf("Expected ErrCredentialsInvalid, got %v", err)
	}
	if output != nil {
		t.Errorf("Expected nil output, got %v", output)
	}

	if len(tx.sessions.sessions) != 0 {
		t.Error("Session should not be created when auth identity is missing")
	}
	if len(tx.refreshTokens.tokens) != 0 {
		t.Error("RefreshToken should not be created when auth identity is missing")
	}

	if passwordHasher.verifyCalls != 1 {
		t.Errorf("Expected one dummy verify call, got %d", passwordHasher.verifyCalls)
	}
	if passwordHasher.lastVerifyPwd != "input-password" {
		t.Errorf("Verify called with wrong password, got %q", passwordHasher.lastVerifyPwd)
	}
	if passwordHasher.lastVerifyHash != "fake-hash-of-dummy-password-never-used" {
		t.Errorf("Verify should use dummy hash, got %q", passwordHasher.lastVerifyHash)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	now := time.Now()
	clock := &fakeClock{now: now}

	user, _ := domain.NewUser(
		"user-123",
		"bob",
		domain.ActorRoleDispatcher,
		"Bob",
		nil,
		now,
	)

	authId := &ports.AuthIdentity{
		UserID:       user.ID,
		PasswordHash: "fake-hash-of-correct-password",
	}

	tx := &fakeTransaction{
		users: &fakeUserRepository{
			users:      map[domain.UserID]*domain.User{user.ID: user},
			byUsername: map[string]*domain.User{"bob": user},
		},
		authIds: &fakeAuthIdentityRepository{
			identities: map[domain.UserID]*ports.AuthIdentity{user.ID: authId},
		},
		sessions: &fakeSessionRepository{
			sessions: make(map[string]*ports.Session),
		},
		refreshTokens: &fakeRefreshTokenRepository{
			tokens: make(map[string]*ports.RefreshToken),
		},
	}

	uow := &fakeUnitOfWork{tx: tx}
	passwordHasher := &fakePasswordHasher{
		correctPassword: "correct-password",
		correctHash:     authId.PasswordHash,
		needsRehash:     false,
	}
	secretGen := &fakeSecretGenerator{
		tokens: []string{"access-secret", "refresh-secret"},
	}

	uc := NewLoginUseCaseWithTTLs(uow, passwordHasher, secretGen, clock, 15*time.Minute, 7*24*time.Hour)

	output, err := uc.Execute(context.Background(), LoginInput{
		Username: "bob",
		Password: "wrong-password",
	})

	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	if !errors.Is(err, ports.ErrCredentialsInvalid) {
		t.Errorf("Expected ErrCredentialsInvalid, got %v", err)
	}

	if output != nil {
		t.Errorf("Expected nil output, got %v", output)
	}

	// Verify no session/token was created
	if len(tx.sessions.sessions) != 0 {
		t.Error("Session should not be created for wrong password")
	}
	if len(tx.refreshTokens.tokens) != 0 {
		t.Error("RefreshToken should not be created for wrong password")
	}
	if tx.audits == nil || len(tx.audits.events) != 1 {
		t.Fatalf("failed login audit event count = %d, want 1", len(tx.audits.events))
	}
	if got := tx.audits.events[0].Action; got != string(domain.ActionAuthLoginFailed) {
		t.Fatalf("failed login audit action = %q, want %q", got, domain.ActionAuthLoginFailed)
	}
}

func TestLoginDisabledUser(t *testing.T) {
	now := time.Now()
	clock := &fakeClock{now: now}

	// Create disabled user
	user, _ := domain.NewUser(
		"user-123",
		"charlie",
		domain.ActorRoleAdmin,
		"Charlie",
		nil,
		now,
	)
	user.IsActive = false

	authId := &ports.AuthIdentity{
		UserID:       user.ID,
		PasswordHash: "fake-hash-of-correct-password",
	}

	tx := &fakeTransaction{
		users: &fakeUserRepository{
			users:      map[domain.UserID]*domain.User{user.ID: user},
			byUsername: map[string]*domain.User{"charlie": user},
		},
		authIds: &fakeAuthIdentityRepository{
			identities: map[domain.UserID]*ports.AuthIdentity{user.ID: authId},
		},
		sessions: &fakeSessionRepository{
			sessions: make(map[string]*ports.Session),
		},
		refreshTokens: &fakeRefreshTokenRepository{
			tokens: make(map[string]*ports.RefreshToken),
		},
	}

	uow := &fakeUnitOfWork{tx: tx}
	passwordHasher := &fakePasswordHasher{
		correctPassword: "correct-password",
		correctHash:     authId.PasswordHash,
		needsRehash:     false,
	}
	secretGen := &fakeSecretGenerator{
		tokens: []string{"access-secret", "refresh-secret"},
	}

	uc := NewLoginUseCaseWithTTLs(uow, passwordHasher, secretGen, clock, 15*time.Minute, 7*24*time.Hour)

	output, err := uc.Execute(context.Background(), LoginInput{
		Username: "charlie",
		Password: "correct-password",
	})

	if err == nil {
		t.Fatal("Expected error for disabled user, got nil")
	}

	if !errors.Is(err, ports.ErrCredentialsInvalid) {
		t.Errorf("Expected ErrCredentialsInvalid, got %v", err)
	}

	if output != nil {
		t.Errorf("Expected nil output, got %v", output)
	}

	// Verify no session/token was created
	if len(tx.sessions.sessions) != 0 {
		t.Error("Session should not be created for disabled user")
	}
	if len(tx.refreshTokens.tokens) != 0 {
		t.Error("RefreshToken should not be created for disabled user")
	}
}

func TestLoginPasswordRehash(t *testing.T) {
	now := time.Now()
	clock := &fakeClock{now: now}

	user, _ := domain.NewUser(
		"user-123",
		"dave",
		domain.ActorRoleTechnician,
		"Dave",
		nil,
		now,
	)

	authId := &ports.AuthIdentity{
		UserID:       user.ID,
		PasswordHash: "fake-hash-of-old-algorithm",
	}

	tx := &fakeTransaction{
		users: &fakeUserRepository{
			users:      map[domain.UserID]*domain.User{user.ID: user},
			byUsername: map[string]*domain.User{"dave": user},
		},
		authIds: &fakeAuthIdentityRepository{
			identities: map[domain.UserID]*ports.AuthIdentity{user.ID: authId},
		},
		sessions: &fakeSessionRepository{
			sessions: make(map[string]*ports.Session),
		},
		refreshTokens: &fakeRefreshTokenRepository{
			tokens: make(map[string]*ports.RefreshToken),
		},
	}

	uow := &fakeUnitOfWork{tx: tx}
	passwordHasher := &fakePasswordHasher{
		correctPassword: "correct-password",
		correctHash:     authId.PasswordHash,
		needsRehash:     true, // Signal that hash needs update
	}
	secretGen := &fakeSecretGenerator{
		tokens: []string{"access-secret", "refresh-secret"},
	}

	uc := NewLoginUseCaseWithTTLs(uow, passwordHasher, secretGen, clock, 15*time.Minute, 7*24*time.Hour)

	output, err := uc.Execute(context.Background(), LoginInput{
		Username: "dave",
		Password: "correct-password",
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if output == nil {
		t.Fatal("Expected successful output")
	}

	// Verify that the auth identity was rehashed
	updatedIdentity := tx.authIds.identities[user.ID]
	if updatedIdentity.PasswordHash != "fake-hash-of-correct-password" {
		t.Errorf("Password hash not updated: got %q", updatedIdentity.PasswordHash)
	}
}

func TestLoginEmptyUsername(t *testing.T) {
	clock := &fakeClock{now: time.Now()}
	uow := &fakeUnitOfWork{tx: &fakeTransaction{}}
	passwordHasher := &fakePasswordHasher{}
	secretGen := &fakeSecretGenerator{}

	uc := NewLoginUseCaseWithTTLs(uow, passwordHasher, secretGen, clock, 15*time.Minute, 7*24*time.Hour)

	output, err := uc.Execute(context.Background(), LoginInput{
		Username: "",
		Password: "password",
	})

	if err == nil {
		t.Fatal("Expected error for empty username")
	}

	if !errors.Is(err, domain.ErrRequiredField) {
		t.Errorf("Expected ErrRequiredField, got %v", err)
	}

	if output != nil {
		t.Errorf("Expected nil output, got %v", output)
	}
}

func TestLoginEmptyPassword(t *testing.T) {
	clock := &fakeClock{now: time.Now()}
	uow := &fakeUnitOfWork{tx: &fakeTransaction{}}
	passwordHasher := &fakePasswordHasher{}
	secretGen := &fakeSecretGenerator{}

	uc := NewLoginUseCaseWithTTLs(uow, passwordHasher, secretGen, clock, 15*time.Minute, 7*24*time.Hour)

	output, err := uc.Execute(context.Background(), LoginInput{
		Username: "user",
		Password: "",
	})

	if err == nil {
		t.Fatal("Expected error for empty password")
	}

	if !errors.Is(err, domain.ErrRequiredField) {
		t.Errorf("Expected ErrRequiredField, got %v", err)
	}

	if output != nil {
		t.Errorf("Expected nil output, got %v", output)
	}
}

func TestLoginSessionExpiryTime(t *testing.T) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	clock := &fakeClock{now: now}

	user, _ := domain.NewUser("user-123", "eve", domain.ActorRoleTechnician, "Eve", nil, now)
	authId := &ports.AuthIdentity{UserID: user.ID, PasswordHash: "hash"}

	tx := &fakeTransaction{
		users: &fakeUserRepository{
			users:      map[domain.UserID]*domain.User{user.ID: user},
			byUsername: map[string]*domain.User{"eve": user},
		},
		authIds: &fakeAuthIdentityRepository{
			identities: map[domain.UserID]*ports.AuthIdentity{user.ID: authId},
		},
		sessions: &fakeSessionRepository{
			sessions: make(map[string]*ports.Session),
		},
		refreshTokens: &fakeRefreshTokenRepository{
			tokens: make(map[string]*ports.RefreshToken),
		},
	}

	uow := &fakeUnitOfWork{tx: tx}
	passwordHasher := &fakePasswordHasher{
		correctPassword: "pwd",
		correctHash:     "hash",
	}
	secretGen := &fakeSecretGenerator{
		tokens: []string{"access-secret", "refresh-secret"},
	}

	accessTTL := 15 * time.Minute
	refreshTTL := 7 * 24 * time.Hour

	uc := NewLoginUseCaseWithTTLs(uow, passwordHasher, secretGen, clock, accessTTL, refreshTTL)

	_, err := uc.Execute(context.Background(), LoginInput{
		Username: "eve",
		Password: "pwd",
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Verify session expiry
	if len(tx.sessions.sessions) != 1 {
		t.Fatalf("Expected 1 session, got %d", len(tx.sessions.sessions))
	}

	var session *ports.Session
	for _, s := range tx.sessions.sessions {
		session = s
		break
	}

	expectedSessionExpiry := now.Add(accessTTL)
	if !session.ExpiresAt.Equal(expectedSessionExpiry) {
		t.Errorf("Session expiry mismatch: got %v, want %v", session.ExpiresAt, expectedSessionExpiry)
	}

	// Verify refresh token expiry
	if len(tx.refreshTokens.tokens) != 1 {
		t.Fatalf("Expected 1 refresh token, got %d", len(tx.refreshTokens.tokens))
	}

	var refreshToken *ports.RefreshToken
	for _, rt := range tx.refreshTokens.tokens {
		refreshToken = rt
		break
	}

	expectedTokenExpiry := now.Add(refreshTTL)
	if !refreshToken.ExpiresAt.Equal(expectedTokenExpiry) {
		t.Errorf("RefreshToken expiry mismatch: got %v, want %v", refreshToken.ExpiresAt, expectedTokenExpiry)
	}
}

func TestLoginTokenHashesAreCorrect(t *testing.T) {
	now := time.Now()
	clock := &fakeClock{now: now}

	user, _ := domain.NewUser("user-123", "frank", domain.ActorRoleTechnician, "Frank", nil, now)
	authId := &ports.AuthIdentity{UserID: user.ID, PasswordHash: "hash"}

	tx := &fakeTransaction{
		users: &fakeUserRepository{
			users:      map[domain.UserID]*domain.User{user.ID: user},
			byUsername: map[string]*domain.User{"frank": user},
		},
		authIds: &fakeAuthIdentityRepository{
			identities: map[domain.UserID]*ports.AuthIdentity{user.ID: authId},
		},
		sessions: &fakeSessionRepository{
			sessions: make(map[string]*ports.Session),
		},
		refreshTokens: &fakeRefreshTokenRepository{
			tokens: make(map[string]*ports.RefreshToken),
		},
	}

	uow := &fakeUnitOfWork{tx: tx}
	passwordHasher := &fakePasswordHasher{
		correctPassword: "pwd",
		correctHash:     "hash",
	}

	const accessSecret = "my-access-secret"
	const refreshSecret = "my-refresh-secret"
	secretGen := &fakeSecretGenerator{
		tokens: []string{accessSecret, refreshSecret},
	}

	uc := NewLoginUseCaseWithTTLs(uow, passwordHasher, secretGen, clock, 15*time.Minute, 7*24*time.Hour)

	output, err := uc.Execute(context.Background(), LoginInput{
		Username: "frank",
		Password: "pwd",
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if output == nil {
		t.Fatal("Expected successful output")
	}

	// Verify access token hash
	expectedAccessHash := sha256.Sum256([]byte(accessSecret))
	var session *ports.Session
	for _, s := range tx.sessions.sessions {
		session = s
		break
	}
	if session == nil {
		t.Fatal("Session not found")
	}

	if len(session.TokenHash) != len(expectedAccessHash) {
		t.Errorf("Access token hash length mismatch: got %d, want %d",
			len(session.TokenHash), len(expectedAccessHash))
	}

	for i, b := range expectedAccessHash {
		if session.TokenHash[i] != b {
			t.Errorf("Access token hash mismatch at byte %d: got %d, want %d",
				i, session.TokenHash[i], b)
			break
		}
	}

	// Verify refresh token hash
	expectedRefreshHash := sha256.Sum256([]byte(refreshSecret))
	var refreshToken *ports.RefreshToken
	for _, rt := range tx.refreshTokens.tokens {
		refreshToken = rt
		break
	}
	if refreshToken == nil {
		t.Fatal("RefreshToken not found")
	}

	if len(refreshToken.TokenHash) != len(expectedRefreshHash) {
		t.Errorf("Refresh token hash length mismatch: got %d, want %d",
			len(refreshToken.TokenHash), len(expectedRefreshHash))
	}

	for i, b := range expectedRefreshHash {
		if refreshToken.TokenHash[i] != b {
			t.Errorf("Refresh token hash mismatch at byte %d: got %d, want %d",
				i, refreshToken.TokenHash[i], b)
			break
		}
	}
}

// Helper function
func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
