package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"strings"
	"sync"
	"time"

	"bestelltool_be/internal/application/ports"
)

const defaultPrincipalCacheTTL = time.Minute

const maxPrincipalCacheEntries = 1024

type cachedPrincipal struct {
	principal ports.Principal
	expiresAt time.Time
}

// SessionAuthenticator validates opaque access tokens against live sessions.
// Its bounded-TTL cache limits database reads without extending a revoked
// session's validity beyond the configured cache duration.
type SessionAuthenticator struct {
	uow      ports.UnitOfWork
	clock    ports.Clock
	cacheTTL time.Duration

	mu    sync.Mutex
	cache map[[32]byte]cachedPrincipal
}

func NewSessionAuthenticator(uow ports.UnitOfWork, clock ports.Clock) *SessionAuthenticator {
	return NewSessionAuthenticatorWithCacheTTL(uow, clock, defaultPrincipalCacheTTL)
}

func NewSessionAuthenticatorWithCacheTTL(uow ports.UnitOfWork, clock ports.Clock, cacheTTL time.Duration) *SessionAuthenticator {
	return &SessionAuthenticator{uow: uow, clock: clock, cacheTTL: cacheTTL, cache: make(map[[32]byte]cachedPrincipal)}
}

func (a *SessionAuthenticator) Authenticate(ctx context.Context, token string) (*ports.Principal, error) {
	secret, err := accessTokenSecret(token)
	if err != nil {
		return nil, ports.ErrUnauthenticated
	}
	cacheKey := sha256.Sum256([]byte(token))
	now := a.clock.Now()

	a.mu.Lock()
	if entry, ok := a.cache[cacheKey]; ok && now.Before(entry.expiresAt) {
		principal := entry.principal
		a.mu.Unlock()
		return &principal, nil
	}
	delete(a.cache, cacheKey)
	a.mu.Unlock()

	hash := sha256.Sum256([]byte(secret))
	var session *ports.Session
	err = a.uow.WithinTransaction(ctx, func(ctx context.Context, tx ports.Transaction) error {
		var err error
		session, err = tx.Sessions().GetByTokenHash(ctx, hash[:])
		return err
	})
	if err != nil || session == nil || session.RevokedAt != nil || !now.Before(session.ExpiresAt) || subtle.ConstantTimeCompare(hash[:], session.TokenHash) != 1 {
		return nil, ports.ErrUnauthenticated
	}

	principal := ports.Principal{UserID: session.UserID, Role: session.ActiveRole, SessionID: session.ID}
	if a.cacheTTL > 0 {
		a.mu.Lock()
		if len(a.cache) >= maxPrincipalCacheEntries {
			clear(a.cache)
		}
		a.cache[cacheKey] = cachedPrincipal{principal: principal, expiresAt: now.Add(a.cacheTTL)}
		a.mu.Unlock()
	}
	return &principal, nil
}

func accessTokenSecret(token string) (string, error) {
	rest, ok := strings.CutPrefix(token, "rp_at_")
	if !ok {
		return "", fmt.Errorf("invalid access token prefix")
	}
	id, secret, ok := strings.Cut(rest, ".")
	if !ok || id == "" || secret == "" || strings.Contains(secret, ".") {
		return "", fmt.Errorf("invalid access token format")
	}
	return secret, nil
}
