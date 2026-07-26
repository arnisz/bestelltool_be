package httpadapter_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	authadapter "bestelltool_be/internal/adapters/auth"
	httpadapter "bestelltool_be/internal/adapters/http"
	"bestelltool_be/internal/adapters/postgres"
	"bestelltool_be/internal/adapters/sse"
	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/application/usecases"
	"bestelltool_be/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	e2eTechAToken      = "tok-tech-a"
	e2eTechBToken      = "tok-tech-b"
	e2eDispatcherToken = "tok-dispatcher"
)

func TestResourceLifecycleE2E(t *testing.T) {
	pool := e2eTestPool(t)
	seedCoreRefs(t, pool)

	uow := postgres.NewUnitOfWork(pool)
	requestRepo := postgres.NewRequestRepository(pool)
	eventStream := sse.NewBroker(0)

	authenticator, err := authadapter.ParseStaticTokens(strings.Join([]string{
		e2eTechAToken + ":tech-a:technician",
		e2eTechBToken + ":tech-b:technician",
		e2eDispatcherToken + ":dispatcher-1:dispatcher",
	}, ","))
	if err != nil {
		t.Fatalf("ParseStaticTokens() error = %v", err)
	}

	h := httpadapter.NewHandlerWithEventStream(
		authenticator,
		usecases.NewCreateRequestUseCaseWithPublisher(uow, eventStream),
		usecases.NewGetRequestUseCase(requestRepo),
		usecases.NewRequestReturnUseCaseWithPublisher(uow, eventStream),
		usecases.NewTransferResourceUseCaseWithPublisher(uow, eventStream),
		eventStream,
	)

	server := httptest.NewServer(h)
	t.Cleanup(server.Close)

	now := time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC)
	requestAID := "req-e2e-a"
	requestBID := "req-e2e-b"
	resourceID := "res-e2e-1"
	oldAllocID := "alloc-e2e-old"
	newAllocID := "alloc-e2e-new"

	// Schritt A: Techniker A legt einen Request an.
	createBody := map[string]any{
		"request_id":                 requestAID,
		"context_ref":                "ctx-e2e-a",
		"context_label":              "E2E A",
		"note":                       "created in e2e",
		"requested_resource_classes": []string{"rc-1"},
		"created_at":                 now,
		"audit": map[string]any{
			"client_seq": int64(1),
			"note":       "create request",
		},
	}
	resp := doJSONRequest(t, server.Client(), http.MethodPost, server.URL+"/api/v1/requests", e2eTechAToken, createBody)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/v1/requests status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}
	var created struct {
		ID string `json:"id"`
	}
	decodeJSONResponse(t, resp, &created)
	if created.ID != requestAID {
		t.Fatalf("created request id = %q, want %q", created.ID, requestAID)
	}

	// Schritt B: Dispatcher ruft den Request ab.
	resp = doJSONRequest(t, server.Client(), http.MethodGet, server.URL+"/api/v1/requests/"+requestAID, e2eDispatcherToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/requests/{id} status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var fetched struct {
		ID           string `json:"id"`
		TechnicianID string `json:"technician_id"`
	}
	decodeJSONResponse(t, resp, &fetched)
	if fetched.ID != requestAID {
		t.Fatalf("fetched request id = %q, want %q", fetched.ID, requestAID)
	}
	if fetched.TechnicianID != "tech-a" {
		t.Fatalf("fetched technician_id = %q, want %q", fetched.TechnicianID, "tech-a")
	}

	// Schritt C: Es existiert aktuell kein separater HTTP-Endpunkt für reguläre Zuweisung.
	seedDirectTransferPreconditions(t, uow, requestAID, requestBID, resourceID, oldAllocID, now)

	// Schritt D: Dispatcher führt Direct Transfer durch.
	transferAt := now.Add(45 * time.Minute)
	transferBody := map[string]any{
		"old_allocation_id": oldAllocID,
		"new_allocation_id": newAllocID,
		"target_request_id": requestBID,
		"planned_from":      now.Add(1 * time.Hour),
		"planned_until":     now.Add(4 * time.Hour),
		"at":                transferAt,
		"audit": map[string]any{
			"client_seq": int64(2),
			"note":       "dispatcher transfer",
		},
	}
	resp = doJSONRequest(t, server.Client(), http.MethodPost, server.URL+"/api/v1/resources/"+resourceID+"/transfer", e2eDispatcherToken, transferBody)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("POST /api/v1/resources/{id}/transfer status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	assertAllocationStatus(t, pool, oldAllocID, "completed")
	assertAllocationStatus(t, pool, newAllocID, "allocated")
	assertResourceState(t, pool, resourceID, "reserved", "tech-b")

	markAllocationWithTechnician(t, pool, newAllocID, transferAt.Add(5*time.Minute))

	// Schritt E: Techniker B fordert Rückgabe an.
	returnBody := map[string]any{
		"at": transferAt.Add(10 * time.Minute),
		"audit": map[string]any{
			"client_seq": int64(3),
			"note":       "return request",
		},
	}
	resp = doJSONRequest(t, server.Client(), http.MethodPost, server.URL+"/api/v1/allocations/"+newAllocID+"/return-request", e2eTechBToken, returnBody)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("POST /api/v1/allocations/{id}/return-request status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	assertAllocationStatus(t, pool, newAllocID, "return_requested")

	// Schritt F: Techniker A versucht Direct Transfer und wird per Rolle abgewiesen.
	resp = doJSONRequest(t, server.Client(), http.MethodPost, server.URL+"/api/v1/resources/"+resourceID+"/transfer", e2eTechAToken, transferBody)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("POST /api/v1/resources/{id}/transfer as technician status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
	var forbidden errorEnvelope
	decodeJSONResponse(t, resp, &forbidden)
	if forbidden.Error.Code != "forbidden" {
		t.Fatalf("forbidden error code = %q, want %q", forbidden.Error.Code, "forbidden")
	}
}

type e2eClock struct {
	now time.Time
}

func (c *e2eClock) Now() time.Time {
	return c.now
}

func TestSwitchActiveRoleAndGetMeE2E(t *testing.T) {
	pool := e2eTestPool(t)
	now := time.Date(2026, time.July, 26, 17, 0, 0, 0, time.UTC)
	clock := &e2eClock{now: now}
	uow := postgres.NewUnitOfWork(pool)

	if _, err := pool.Exec(t.Context(), `
INSERT INTO users(id, username, role, display_name, is_active)
VALUES ('multi-user', 'multi-user', 'technician', 'Multi Role User', true);
INSERT INTO user_roles(user_id, role_code, assigned_by) VALUES
('multi-user', 'technician', 'system-bootstrap'),
('multi-user', 'dispatcher', 'system-bootstrap');`); err != nil {
		t.Fatalf("seed multi-role user error = %v", err)
	}

	passwordHasher := authadapter.NewArgon2Hasher(authadapter.DefaultArgon2Config())
	passwordHash, err := passwordHasher.Hash("correct-password")
	if err != nil {
		t.Fatalf("hash password error = %v", err)
	}
	if err := uow.WithinTransaction(t.Context(), func(ctx context.Context, tx ports.Transaction) error {
		return tx.AuthIdentities().Save(ctx, &ports.AuthIdentity{UserID: "multi-user", PasswordHash: passwordHash})
	}); err != nil {
		t.Fatalf("seed auth identity error = %v", err)
	}

	secretGenerator := authadapter.NewSecretGenerator(authadapter.DefaultTokenConfig())
	encryptor, err := authadapter.NewTokenEncryptor(make([]byte, 32))
	if err != nil {
		t.Fatalf("NewTokenEncryptor() error = %v", err)
	}
	login := usecases.NewLoginUseCase(uow, passwordHasher, secretGenerator, clock)
	refresh := usecases.NewRefreshSessionUseCase(uow, secretGenerator, encryptor, clock)
	switchRole := usecases.NewSwitchActiveRoleUseCase(uow, secretGenerator, clock)
	authenticator := authadapter.NewSessionAuthenticator(uow, postgres.NewPermissionResolver(pool), clock, 0)
	h := httpadapter.NewHandlerWithEventStreamAndAuthenticationAndSecurity(
		authenticator,
		nil,
		nil,
		nil,
		nil,
		nil,
		login,
		refresh,
		usecases.NewLogoutUseCase(uow, clock),
		usecases.NewChangeOwnPasswordUseCase(uow, passwordHasher, clock),
		httpadapter.NewRateLimiter(10, time.Minute, false, clock.Now),
		switchRole,
		usecases.NewGetMeUseCase(uow),
	)
	server := httptest.NewServer(h)
	t.Cleanup(server.Close)

	loginResp := doJSONRequest(t, server.Client(), http.MethodPost, server.URL+"/api/v1/auth/login", "", map[string]string{"username": "multi-user", "password": "correct-password"})
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200", loginResp.StatusCode)
	}
	var initialTokens struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	decodeJSONResponse(t, loginResp, &initialTokens)

	forbiddenSwitchResp := doJSONRequest(t, server.Client(), http.MethodPost, server.URL+"/api/v1/auth/switch-role", initialTokens.AccessToken, map[string]string{"active_role": "admin"})
	if forbiddenSwitchResp.StatusCode != http.StatusForbidden {
		t.Fatalf("switch to unheld admin role status = %d, want 403", forbiddenSwitchResp.StatusCode)
	}
	var forbiddenSwitchError errorEnvelope
	decodeJSONResponse(t, forbiddenSwitchResp, &forbiddenSwitchError)
	if forbiddenSwitchError.Error.Code != "forbidden" {
		t.Fatalf("switch to unheld admin role error code = %q, want forbidden", forbiddenSwitchError.Error.Code)
	}
	t.Log(`POST /api/v1/auth/switch-role (admin) -> 403 {"error":{"code":"forbidden"}}`)
	var sessionCount int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM sessions WHERE user_id = 'multi-user'`).Scan(&sessionCount); err != nil {
		t.Fatalf("count sessions after forbidden switch error = %v", err)
	}
	if sessionCount != 1 {
		t.Fatalf("sessions after forbidden switch = %d, want 1", sessionCount)
	}

	switchResp := doJSONRequest(t, server.Client(), http.MethodPost, server.URL+"/api/v1/auth/switch-role", initialTokens.AccessToken, map[string]string{"active_role": "dispatcher"})
	if switchResp.StatusCode != http.StatusOK {
		t.Fatalf("switch role status = %d, want 200", switchResp.StatusCode)
	}
	var switched struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ActiveRole   string `json:"active_role"`
	}
	decodeJSONResponse(t, switchResp, &switched)
	if switched.ActiveRole != string(domain.ActorRoleDispatcher) || switched.AccessToken == "" || switched.RefreshToken == "" {
		t.Fatalf("switch response = %+v, want new dispatcher tokens", switched)
	}
	t.Log(`POST /api/v1/auth/switch-role -> 200 {"access_token":"[redacted]","refresh_token":"[redacted]","active_role":"dispatcher"}`)

	oldRefreshResp := doJSONRequest(t, server.Client(), http.MethodPost, server.URL+"/api/v1/auth/refresh", "", map[string]string{"refresh_token": initialTokens.RefreshToken})
	if oldRefreshResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old refresh status = %d, want 401", oldRefreshResp.StatusCode)
	}
	var oldRefreshError errorEnvelope
	decodeJSONResponse(t, oldRefreshResp, &oldRefreshError)
	if oldRefreshError.Error.Code != "unauthorized" || oldRefreshError.Error.Message != "invalid refresh token" {
		t.Fatalf("old refresh error = %+v, want generic invalid refresh token", oldRefreshError)
	}
	t.Log(`POST /api/v1/auth/refresh (old token) -> 401 {"error":{"code":"unauthorized","message":"invalid refresh token"}}`)

	meResp := doJSONRequest(t, server.Client(), http.MethodGet, server.URL+"/api/v1/auth/me", switched.AccessToken, nil)
	if meResp.StatusCode != http.StatusOK {
		t.Fatalf("get me status = %d, want 200", meResp.StatusCode)
	}
	var me struct {
		UserID      string   `json:"user_id"`
		ActiveRole  string   `json:"active_role"`
		Roles       []string `json:"roles"`
		Permissions []string `json:"permissions"`
	}
	decodeJSONResponse(t, meResp, &me)
	if me.UserID != "multi-user" || me.ActiveRole != string(domain.ActorRoleDispatcher) {
		t.Fatalf("get me identity = %+v, want multi-user dispatcher", me)
	}
	if !slices.Contains(me.Roles, string(domain.ActorRoleTechnician)) || !slices.Contains(me.Roles, string(domain.ActorRoleDispatcher)) {
		t.Fatalf("get me roles = %v, want technician and dispatcher", me.Roles)
	}
	if !slices.Contains(me.Permissions, domain.PermissionResourceTransferDirect) || slices.Contains(me.Permissions, domain.PermissionRequestCreate) {
		t.Fatalf("get me permissions = %v, want dispatcher-only permissions", me.Permissions)
	}
	meBody, err := json.Marshal(me)
	if err != nil {
		t.Fatalf("marshal get me response for log: %v", err)
	}
	t.Logf("GET /api/v1/auth/me -> 200 %s", meBody)

	var sessionID string
	if err := pool.QueryRow(t.Context(), `SELECT id FROM sessions WHERE token_hash = $1`, authadapter.HashTokenSecret(strings.Split(switched.AccessToken, ".")[1])).Scan(&sessionID); err != nil {
		t.Fatalf("find switched session error = %v", err)
	}
	refreshTokenID := strings.TrimSuffix(strings.TrimPrefix(switched.RefreshToken, "rp_rt_"), "."+strings.Split(switched.RefreshToken, ".")[1])
	var refreshSessionID string
	if err := pool.QueryRow(t.Context(), `SELECT session_id FROM refresh_tokens WHERE id = $1`, refreshTokenID).Scan(&refreshSessionID); err != nil {
		t.Fatalf("find switched refresh token session error = %v", err)
	}
	if refreshSessionID != sessionID {
		t.Fatalf("refresh session id = %q, want switched access session %q", refreshSessionID, sessionID)
	}
	if _, err := pool.Exec(t.Context(), `DELETE FROM user_roles WHERE user_id = 'multi-user' AND role_code = 'dispatcher'`); err != nil {
		t.Fatalf("remove dispatcher role error = %v", err)
	}
	var activeRoleAssignments int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM user_roles WHERE user_id = 'multi-user' AND role_code = 'dispatcher'`).Scan(&activeRoleAssignments); err != nil {
		t.Fatalf("count dispatcher role assignments error = %v", err)
	}
	if activeRoleAssignments != 0 {
		t.Fatalf("dispatcher role assignments = %d, want 0", activeRoleAssignments)
	}
	roleRemovedRefresh := doJSONRequest(t, server.Client(), http.MethodPost, server.URL+"/api/v1/auth/refresh", "", map[string]string{"refresh_token": switched.RefreshToken})
	if roleRemovedRefresh.StatusCode != http.StatusUnauthorized {
		t.Fatalf("refresh after role removal status = %d, want 401", roleRemovedRefresh.StatusCode)
	}
	var roleRemovedError errorEnvelope
	decodeJSONResponse(t, roleRemovedRefresh, &roleRemovedError)
	if roleRemovedError.Error.Code != "unauthorized" || roleRemovedError.Error.Message != "invalid refresh token" {
		t.Fatalf("refresh after role removal error = %+v, want generic invalid refresh token", roleRemovedError)
	}
	t.Log(`POST /api/v1/auth/refresh (removed active role) -> 401 {"error":{"code":"unauthorized","message":"invalid refresh token"}}`)
	secondRoleRemovedRefresh := doJSONRequest(t, server.Client(), http.MethodPost, server.URL+"/api/v1/auth/refresh", "", map[string]string{"refresh_token": switched.RefreshToken})
	if secondRoleRemovedRefresh.StatusCode != http.StatusUnauthorized {
		t.Fatalf("second refresh after role removal status = %d, want 401", secondRoleRemovedRefresh.StatusCode)
	}
	t.Log(`POST /api/v1/auth/refresh (already revoked session) -> 401 {"error":{"code":"unauthorized","message":"invalid refresh token"}}`)
	var (
		revokedAt  *time.Time
		activeRole string
	)
	if err := pool.QueryRow(t.Context(), `SELECT active_role, revoked_at FROM sessions WHERE id = $1`, sessionID).Scan(&activeRole, &revokedAt); err != nil {
		t.Fatalf("load revoked switched session error = %v", err)
	}
	if revokedAt == nil {
		t.Fatalf("session %s with active role %q was not revoked after active role removal", sessionID, activeRole)
	}
}

func TestLoginRefreshSuccessAndAccessTokenExpiresByAccessTTL_E2E(t *testing.T) {
	pool := e2eTestPool(t)
	now := time.Date(2026, time.July, 26, 18, 0, 0, 0, time.UTC)
	clock := &e2eClock{now: now}
	uow := postgres.NewUnitOfWork(pool)

	if _, err := pool.Exec(t.Context(), `
INSERT INTO users(id, username, role, display_name, is_active)
VALUES ('refresh-user', 'refresh-user', 'technician', 'Refresh User', true);
INSERT INTO user_roles(user_id, role_code, assigned_by)
VALUES ('refresh-user', 'technician', 'system-bootstrap');`); err != nil {
		t.Fatalf("seed refresh user error = %v", err)
	}

	passwordHasher := authadapter.NewArgon2Hasher(authadapter.DefaultArgon2Config())
	passwordHash, err := passwordHasher.Hash("correct-password")
	if err != nil {
		t.Fatalf("hash password error = %v", err)
	}
	if err := uow.WithinTransaction(t.Context(), func(ctx context.Context, tx ports.Transaction) error {
		return tx.AuthIdentities().Save(ctx, &ports.AuthIdentity{UserID: "refresh-user", PasswordHash: passwordHash})
	}); err != nil {
		t.Fatalf("seed auth identity error = %v", err)
	}

	secretGenerator := authadapter.NewSecretGenerator(authadapter.DefaultTokenConfig())
	encryptor, err := authadapter.NewTokenEncryptor(make([]byte, 32))
	if err != nil {
		t.Fatalf("NewTokenEncryptor() error = %v", err)
	}
	login := usecases.NewLoginUseCase(uow, passwordHasher, secretGenerator, clock)
	refresh := usecases.NewRefreshSessionUseCase(uow, secretGenerator, encryptor, clock)
	authenticator := authadapter.NewSessionAuthenticator(uow, postgres.NewPermissionResolver(pool), clock, 0)
	h := httpadapter.NewHandlerWithEventStreamAndAuthenticationAndSecurity(
		authenticator,
		nil,
		nil,
		nil,
		nil,
		nil,
		login,
		refresh,
		usecases.NewLogoutUseCase(uow, clock),
		usecases.NewChangeOwnPasswordUseCase(uow, passwordHasher, clock),
		httpadapter.NewRateLimiter(10, time.Minute, false, clock.Now),
		usecases.NewSwitchActiveRoleUseCase(uow, secretGenerator, clock),
		usecases.NewGetMeUseCase(uow),
	)
	server := httptest.NewServer(h)
	t.Cleanup(server.Close)

	loginResp := doJSONRequest(t, server.Client(), http.MethodPost, server.URL+"/api/v1/auth/login", "", map[string]string{"username": "refresh-user", "password": "correct-password"})
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200", loginResp.StatusCode)
	}
	var loginTokens struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	decodeJSONResponse(t, loginResp, &loginTokens)

	refreshResp := doJSONRequest(t, server.Client(), http.MethodPost, server.URL+"/api/v1/auth/refresh", "", map[string]string{"refresh_token": loginTokens.RefreshToken})
	if refreshResp.StatusCode != http.StatusOK {
		t.Fatalf("refresh status = %d, want 200", refreshResp.StatusCode)
	}
	var refreshed struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	decodeJSONResponse(t, refreshResp, &refreshed)
	if refreshed.AccessToken == "" || refreshed.RefreshToken == "" {
		t.Fatalf("refresh response = %+v, want non-empty tokens", refreshed)
	}

	meOK := doJSONRequest(t, server.Client(), http.MethodGet, server.URL+"/api/v1/auth/me", refreshed.AccessToken, nil)
	if meOK.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/auth/me with fresh token status = %d, want 200", meOK.StatusCode)
	}

	clock.now = clock.now.Add(15*time.Minute + time.Second)
	meExpired := doJSONRequest(t, server.Client(), http.MethodGet, server.URL+"/api/v1/auth/me", refreshed.AccessToken, nil)
	if meExpired.StatusCode != http.StatusUnauthorized {
		t.Fatalf("GET /api/v1/auth/me with expired token status = %d, want 401", meExpired.StatusCode)
	}
	var expiredErr errorEnvelope
	decodeJSONResponse(t, meExpired, &expiredErr)
	if expiredErr.Error.Code != "unauthenticated" {
		t.Fatalf("expired token error code = %q, want unauthenticated", expiredErr.Error.Code)
	}
}

func TestRefreshConcurrentConsumedPredecessorAndSuccessorNoDeadlock_SEC08(t *testing.T) {
	pool := e2eTestPool(t)
	now := time.Date(2026, time.July, 26, 19, 0, 0, 0, time.UTC)
	clock := &e2eClock{now: now}
	uow := postgres.NewUnitOfWork(pool)

	if _, err := pool.Exec(t.Context(), `
INSERT INTO users(id, username, role, display_name, is_active)
VALUES ('race-user', 'race-user', 'technician', 'Race User', true);
INSERT INTO user_roles(user_id, role_code, assigned_by)
VALUES ('race-user', 'technician', 'system-bootstrap');`); err != nil {
		t.Fatalf("seed race user error = %v", err)
	}

	passwordHasher := authadapter.NewArgon2Hasher(authadapter.DefaultArgon2Config())
	passwordHash, err := passwordHasher.Hash("correct-password")
	if err != nil {
		t.Fatalf("hash password error = %v", err)
	}
	if err := uow.WithinTransaction(t.Context(), func(ctx context.Context, tx ports.Transaction) error {
		return tx.AuthIdentities().Save(ctx, &ports.AuthIdentity{UserID: "race-user", PasswordHash: passwordHash})
	}); err != nil {
		t.Fatalf("seed auth identity error = %v", err)
	}

	secretGenerator := authadapter.NewSecretGenerator(authadapter.DefaultTokenConfig())
	encryptor, err := authadapter.NewTokenEncryptor(make([]byte, 32))
	if err != nil {
		t.Fatalf("NewTokenEncryptor() error = %v", err)
	}
	login := usecases.NewLoginUseCase(uow, passwordHasher, secretGenerator, clock)
	refresh := usecases.NewRefreshSessionUseCase(uow, secretGenerator, encryptor, clock)
	authenticator := authadapter.NewSessionAuthenticator(uow, postgres.NewPermissionResolver(pool), clock, 0)
	h := httpadapter.NewHandlerWithEventStreamAndAuthenticationAndSecurity(
		authenticator,
		nil,
		nil,
		nil,
		nil,
		nil,
		login,
		refresh,
		usecases.NewLogoutUseCase(uow, clock),
		usecases.NewChangeOwnPasswordUseCase(uow, passwordHasher, clock),
		httpadapter.NewRateLimiter(10, time.Minute, false, clock.Now),
		usecases.NewSwitchActiveRoleUseCase(uow, secretGenerator, clock),
		usecases.NewGetMeUseCase(uow),
	)
	server := httptest.NewServer(h)
	t.Cleanup(server.Close)

	loginResp := doJSONRequest(t, server.Client(), http.MethodPost, server.URL+"/api/v1/auth/login", "", map[string]string{"username": "race-user", "password": "correct-password"})
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200", loginResp.StatusCode)
	}
	var firstTokens struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	decodeJSONResponse(t, loginResp, &firstTokens)

	firstRefreshResp := doJSONRequest(t, server.Client(), http.MethodPost, server.URL+"/api/v1/auth/refresh", "", map[string]string{"refresh_token": firstTokens.RefreshToken})
	if firstRefreshResp.StatusCode != http.StatusOK {
		t.Fatalf("first refresh status = %d, want 200", firstRefreshResp.StatusCode)
	}
	var successorTokens struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	decodeJSONResponse(t, firstRefreshResp, &successorTokens)

	clock.now = clock.now.Add(31 * time.Second)

	type refreshResult struct {
		status int
		body   []byte
		err    error
	}
	callRefresh := func(refreshToken string) refreshResult {
		payload, err := json.Marshal(map[string]string{"refresh_token": refreshToken})
		if err != nil {
			return refreshResult{err: err}
		}
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/api/v1/auth/refresh", bytes.NewReader(payload))
		if err != nil {
			return refreshResult{err: err}
		}
		req.Header.Set("Content-Type", "application/json")
		client := *server.Client()
		client.Timeout = 5 * time.Second
		resp, err := client.Do(req)
		if err != nil {
			return refreshResult{err: err}
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return refreshResult{err: err}
		}
		return refreshResult{status: resp.StatusCode, body: body}
	}

	start := make(chan struct{})
	results := make([]refreshResult, 2)
	var wg sync.WaitGroup
	wg.Go(func() {
		<-start
		results[0] = callRefresh(firstTokens.RefreshToken)
	})
	wg.Go(func() {
		<-start
		results[1] = callRefresh(successorTokens.RefreshToken)
	})
	close(start)
	wg.Wait()

	statusCount := map[int]int{}
	for i, result := range results {
		if result.err != nil {
			t.Fatalf("concurrent refresh call %d error = %v", i, result.err)
		}
		statusCount[result.status]++
		if result.status == http.StatusInternalServerError {
			t.Fatalf("concurrent refresh call %d returned 500 body=%s", i, string(result.body))
		}
	}
	if statusCount[http.StatusUnauthorized] == 0 {
		t.Fatalf("concurrent refresh statuses = %+v, want at least one 401", statusCount)
	}
	if statusCount[http.StatusOK]+statusCount[http.StatusUnauthorized] != 2 {
		t.Fatalf("concurrent refresh statuses = %+v, want only 200/401 outcomes", statusCount)
	}
}

func TestRefreshRoleRemovalConcurrentWithRefreshPreventsNewTokens_SEC14(t *testing.T) {
	pool := e2eTestPool(t)
	now := time.Date(2026, time.July, 26, 20, 0, 0, 0, time.UTC)
	clock := &e2eClock{now: now}
	uow := postgres.NewUnitOfWork(pool)

	if _, err := pool.Exec(t.Context(), `
INSERT INTO users(id, username, role, display_name, is_active)
VALUES ('toctou-user', 'toctou-user', 'technician', 'TOCTOU User', true);
INSERT INTO user_roles(user_id, role_code, assigned_by)
VALUES ('toctou-user', 'technician', 'system-bootstrap');`); err != nil {
		t.Fatalf("seed toctou user error = %v", err)
	}

	passwordHasher := authadapter.NewArgon2Hasher(authadapter.DefaultArgon2Config())
	passwordHash, err := passwordHasher.Hash("correct-password")
	if err != nil {
		t.Fatalf("hash password error = %v", err)
	}
	if err := uow.WithinTransaction(t.Context(), func(ctx context.Context, tx ports.Transaction) error {
		return tx.AuthIdentities().Save(ctx, &ports.AuthIdentity{UserID: "toctou-user", PasswordHash: passwordHash})
	}); err != nil {
		t.Fatalf("seed auth identity error = %v", err)
	}

	secretGenerator := authadapter.NewSecretGenerator(authadapter.DefaultTokenConfig())
	encryptor, err := authadapter.NewTokenEncryptor(make([]byte, 32))
	if err != nil {
		t.Fatalf("NewTokenEncryptor() error = %v", err)
	}
	login := usecases.NewLoginUseCase(uow, passwordHasher, secretGenerator, clock)
	refresh := usecases.NewRefreshSessionUseCase(uow, secretGenerator, encryptor, clock)
	authenticator := authadapter.NewSessionAuthenticator(uow, postgres.NewPermissionResolver(pool), clock, 0)
	h := httpadapter.NewHandlerWithEventStreamAndAuthenticationAndSecurity(
		authenticator,
		nil,
		nil,
		nil,
		nil,
		nil,
		login,
		refresh,
		usecases.NewLogoutUseCase(uow, clock),
		usecases.NewChangeOwnPasswordUseCase(uow, passwordHasher, clock),
		httpadapter.NewRateLimiter(10, time.Minute, false, clock.Now),
		usecases.NewSwitchActiveRoleUseCase(uow, secretGenerator, clock),
		usecases.NewGetMeUseCase(uow),
	)
	server := httptest.NewServer(h)
	t.Cleanup(server.Close)

	loginResp := doJSONRequest(t, server.Client(), http.MethodPost, server.URL+"/api/v1/auth/login", "", map[string]string{"username": "toctou-user", "password": "correct-password"})
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200", loginResp.StatusCode)
	}
	var tokens struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	decodeJSONResponse(t, loginResp, &tokens)

	refreshTokenID := strings.TrimPrefix(tokens.RefreshToken, "rp_rt_")
	refreshTokenID = strings.Split(refreshTokenID, ".")[0]
	var sessionID string
	if err := pool.QueryRow(t.Context(), `SELECT session_id FROM refresh_tokens WHERE id = $1`, refreshTokenID).Scan(&sessionID); err != nil {
		t.Fatalf("find refresh token session error = %v", err)
	}

	txDelete, err := pool.BeginTx(t.Context(), pgx.TxOptions{})
	if err != nil {
		t.Fatalf("begin delete tx error = %v", err)
	}
	t.Cleanup(func() { _ = txDelete.Rollback(t.Context()) })
	if _, err := txDelete.Exec(t.Context(), `SELECT 1 FROM user_roles WHERE user_id = 'toctou-user' AND role_code = 'technician' FOR UPDATE`); err != nil {
		t.Fatalf("lock role row for delete error = %v", err)
	}

	refreshDone := make(chan *http.Response, 1)
	go func() {
		refreshDone <- doJSONRequest(t, server.Client(), http.MethodPost, server.URL+"/api/v1/auth/refresh", "", map[string]string{"refresh_token": tokens.RefreshToken})
	}()

	time.Sleep(100 * time.Millisecond)
	if _, err := txDelete.Exec(t.Context(), `DELETE FROM user_roles WHERE user_id = 'toctou-user' AND role_code = 'technician'`); err != nil {
		t.Fatalf("delete role row error = %v", err)
	}
	if err := txDelete.Commit(t.Context()); err != nil {
		t.Fatalf("commit role delete tx error = %v", err)
	}

	refreshResp := <-refreshDone
	if refreshResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("refresh during concurrent role removal status = %d, want 401", refreshResp.StatusCode)
	}

	var tokenCount int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM refresh_tokens WHERE session_id = $1`, sessionID).Scan(&tokenCount); err != nil {
		t.Fatalf("count refresh tokens for session error = %v", err)
	}
	if tokenCount != 1 {
		t.Fatalf("refresh token count for session = %d, want 1 (no newly issued token)", tokenCount)
	}
}

type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func doJSONRequest(t *testing.T, client *http.Client, method string, url string, token string, body any) *http.Response {
	t.Helper()

	var payload []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}
		payload = b
	}

	req, err := http.NewRequestWithContext(t.Context(), method, url, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("http.NewRequestWithContext() error = %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("client.Do() error = %v", err)
	}
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})

	return resp
}

func decodeJSONResponse(t *testing.T, resp *http.Response, dst any) {
	t.Helper()
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		t.Fatalf("decode response body error = %v", err)
	}
}

func assertAllocationStatus(t *testing.T, pool *pgxpool.Pool, allocationID string, wantStatus string) {
	t.Helper()
	var got string
	if err := pool.QueryRow(t.Context(), `SELECT status FROM allocations WHERE id = $1`, allocationID).Scan(&got); err != nil {
		t.Fatalf("query allocation status %s error = %v", allocationID, err)
	}
	if got != wantStatus {
		t.Fatalf("allocation %s status = %q, want %q", allocationID, got, wantStatus)
	}
}

func assertResourceState(t *testing.T, pool *pgxpool.Pool, resourceID string, wantStatus string, wantHolderID string) {
	t.Helper()
	var gotStatus string
	var gotHolderID string
	if err := pool.QueryRow(t.Context(), `SELECT status, holder_id FROM resources WHERE id = $1`, resourceID).Scan(&gotStatus, &gotHolderID); err != nil {
		t.Fatalf("query resource state %s error = %v", resourceID, err)
	}
	if gotStatus != wantStatus {
		t.Fatalf("resource %s status = %q, want %q", resourceID, gotStatus, wantStatus)
	}
	if gotHolderID != wantHolderID {
		t.Fatalf("resource %s holder_id = %q, want %q", resourceID, gotHolderID, wantHolderID)
	}
}

func markAllocationWithTechnician(t *testing.T, pool *pgxpool.Pool, allocationID string, at time.Time) {
	t.Helper()

	_, err := pool.Exec(t.Context(), `
UPDATE allocations
SET status = 'with_technician',
	shipped_at = $2,
	received_at = $2,
	updated_at = $2,
	version = version + 1
WHERE id = $1
`, allocationID, at)
	if err != nil {
		t.Fatalf("mark allocation %s with_technician error = %v", allocationID, err)
	}
}

func e2eTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL nicht gesetzt: E2E-Tests werden übersprungen")
	}
	assertTestDatabaseURL(t, dbURL)
	resetSchema(t, dbURL)
	if err := postgres.RunEmbeddedMigrations(t.Context(), dbURL); err != nil {
		t.Fatalf("RunEmbeddedMigrations() error = %v", err)
	}

	pool, err := postgres.NewPool(t.Context(), dbURL)
	if err != nil {
		t.Fatalf("NewPool() error = %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
	})

	return pool
}

// resetSchema drops and recreates the public schema on a dedicated, short-lived
// connection - never on the pgxpool.Pool the test itself will use.
//
// Why not TRUNCATE (E2): audit_events.actor_id references users(id). Migration
// 000006 blocks UPDATE/DELETE/TRUNCATE on audit_events with a BEFORE trigger, so
// `TRUNCATE users CASCADE` would cascade into audit_events and fail the trigger,
// breaking test setup itself. A full schema reset sidesteps the append-only
// guard entirely instead of trying to punch a hole in it for tests.
//
// Why a dedicated connection (E1): pgx v5 caches prepared statement plans and
// type OIDs per connection. If the schema were rebuilt using a connection already
// pooled by pgxpool, subsequent queries on that same connection can fail with
// "cached plan must not change result type" or stale-OID errors. Resetting on a
// throwaway connection that is closed before the pgxpool.Pool is ever created
// avoids the problem by construction - the pool never sees a pre-reset plan.
func resetSchema(t *testing.T, dbURL string) {
	t.Helper()

	conn, err := pgx.Connect(t.Context(), dbURL)
	if err != nil {
		t.Fatalf("connect for schema reset error = %v", err)
	}
	defer conn.Close(t.Context())
	if _, err := conn.Exec(t.Context(), `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatalf("reset schema error = %v", err)
	}
}

func assertTestDatabaseURL(t *testing.T, dbURL string) {
	t.Helper()

	u, err := url.Parse(dbURL)
	if err != nil {
		t.Fatalf("TEST_DATABASE_URL parse error = %v", err)
	}

	dbName := strings.TrimPrefix(u.Path, "/")
	if dbName == "" {
		t.Fatalf("TEST_DATABASE_URL ohne Datenbankname: %q", dbURL)
	}
	if !strings.Contains(strings.ToLower(dbName), "test") {
		t.Fatalf("unsichere Testdatenbank %q: Datenbankname muss klar als Testdatenbank erkennbar sein", dbName)
	}
}

func seedCoreRefs(t *testing.T, q interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}) {
	t.Helper()

	if _, err := q.Exec(t.Context(), `INSERT INTO users(id, username, role, display_name) VALUES
('tech-a', 'tech-a', 'technician', 'Technician A'),
('tech-b', 'tech-b', 'technician', 'Technician B'),
('dispatcher-1', 'dispatcher-1', 'dispatcher', 'Dispatcher One')`); err != nil {
		t.Fatalf("seed users error = %v", err)
	}

	if _, err := q.Exec(t.Context(), `INSERT INTO resource_classes(id, name) VALUES ('rc-1', 'Resource Class 1')`); err != nil {
		t.Fatalf("seed resource classes error = %v", err)
	}
}

// seedDirectTransferPreconditions sets up the target request, the in-use resource
// and the with_technician allocation that Schritt D (direct transfer) needs, using
// the same UnitOfWork + repositories + domain state machine that production code
// uses - not hand-written SQL. This keeps the fixture from drifting out of sync
// with schema/constraint changes (see Tech Debt entry in status.md).
func seedDirectTransferPreconditions(
	t *testing.T,
	uow ports.UnitOfWork,
	sourceRequestID string,
	targetRequestID string,
	resourceID string,
	oldAllocationID string,
	now time.Time,
) {
	t.Helper()

	err := uow.WithinTransaction(t.Context(), func(ctx context.Context, tx ports.Transaction) error {
		targetReq, err := domain.NewRequest(
			domain.RequestID(targetRequestID),
			domain.UserID("tech-b"),
			"ctx-e2e-b",
			"E2E B",
			nil,
			nil,
			"",
			[]domain.ResourceClassID{"rc-1"},
			now.Add(1*time.Minute),
		)
		if err != nil {
			return fmt.Errorf("build target request: %w", err)
		}
		if err := tx.Requests().Create(ctx, targetReq); err != nil {
			return fmt.Errorf("create target request: %w", err)
		}

		res, err := domain.NewResource(
			domain.ResourceID(resourceID),
			domain.ResourceClassID("rc-1"),
			"serial-"+resourceID,
			"site-a",
			nil,
			nil,
		)
		if err != nil {
			return fmt.Errorf("build resource: %w", err)
		}
		// available -> reserved -> issued -> in_use, holder tech-a: the same
		// lifecycle a real dispatch would drive the resource through.
		if err := res.Reserve(domain.UserID("tech-a")); err != nil {
			return fmt.Errorf("reserve resource: %w", err)
		}
		if err := res.MarkIssued(); err != nil {
			return fmt.Errorf("issue resource: %w", err)
		}
		if err := res.MarkInUse(); err != nil {
			return fmt.Errorf("mark resource in use: %w", err)
		}
		if err := tx.Resources().Create(ctx, res); err != nil {
			return fmt.Errorf("create resource: %w", err)
		}

		oldAlloc, err := domain.NewAllocation(
			domain.AllocationID(oldAllocationID),
			domain.RequestID(sourceRequestID),
			domain.ResourceID(resourceID),
			now.Add(3*time.Minute),
			now.Add(3*time.Hour),
			now.Add(3*time.Minute),
		)
		if err != nil {
			return fmt.Errorf("build old allocation: %w", err)
		}
		// allocated -> shipped -> with_technician: matches the resource already
		// being in_use with tech-a as holder above.
		if err := oldAlloc.MarkShipped(now.Add(3 * time.Minute)); err != nil {
			return fmt.Errorf("ship old allocation: %w", err)
		}
		if err := oldAlloc.MarkReceivedByTechnician(now.Add(3 * time.Minute)); err != nil {
			return fmt.Errorf("receive old allocation: %w", err)
		}
		if err := tx.Allocations().Create(ctx, oldAlloc); err != nil {
			return fmt.Errorf("create old allocation: %w", err)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("seedDirectTransferPreconditions: %v", err)
	}
}
