package httpadapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/application/usecases"
	"bestelltool_be/internal/domain"
)

// ── Fakes ─────────────────────────────────────────────────────────────────────

type fakeAuthenticator struct {
	principal *ports.Principal
	err       error
}

func (f *fakeAuthenticator) Authenticate(_ context.Context, _ string) (*ports.Principal, error) {
	return f.principal, f.err
}

// newFakeAuth returns an authenticator that always succeeds with the given identity.
func newFakeAuth(userID, role string) *fakeAuthenticator {
	return &fakeAuthenticator{
		principal: &ports.Principal{
			UserID: domain.UserID(userID),
			Role:   domain.ActorRole(role),
		},
	}
}

type fakeCreateRequestUseCase struct {
	in  usecases.CreateRequestInput
	out *domain.Request
	err error
}

func (f *fakeCreateRequestUseCase) Execute(_ context.Context, in usecases.CreateRequestInput) (*domain.Request, error) {
	f.in = in
	return f.out, f.err
}

type fakeGetRequestUseCase struct {
	requestID domain.RequestID
	out       *domain.Request
	err       error
}

func (f *fakeGetRequestUseCase) Execute(_ context.Context, requestID domain.RequestID) (*domain.Request, error) {
	f.requestID = requestID
	return f.out, f.err
}

type fakeRequestReturnUseCase struct {
	in  usecases.RequestReturnInput
	err error
}

func (f *fakeRequestReturnUseCase) Execute(_ context.Context, in usecases.RequestReturnInput) error {
	f.in = in
	return f.err
}

type responseError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// ── Auth Middleware ────────────────────────────────────────────────────────────

func TestAuthMiddlewareMissingHeader(t *testing.T) {
	h := NewHandlerWithClock(newFakeAuth("user-1", "dispatcher"), &fakeCreateRequestUseCase{}, &fakeGetRequestUseCase{}, &fakeRequestReturnUseCase{}, time.Now)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/requests", strings.NewReader(`{}`))
	// No Authorization header.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	errRes := decodeErrorResponse(t, rec)
	if errRes.Error.Code != "unauthenticated" {
		t.Fatalf("error.code = %q, want unauthenticated", errRes.Error.Code)
	}
}

func TestAuthMiddlewareInvalidToken(t *testing.T) {
	a := &fakeAuthenticator{err: fmt.Errorf("bad token: %w", ports.ErrUnauthenticated)}
	h := NewHandlerWithClock(a, &fakeCreateRequestUseCase{}, &fakeGetRequestUseCase{}, &fakeRequestReturnUseCase{}, time.Now)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/requests", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer invalid-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	errRes := decodeErrorResponse(t, rec)
	if errRes.Error.Code != "unauthenticated" {
		t.Fatalf("error.code = %q, want unauthenticated", errRes.Error.Code)
	}
}

// TestAuthMiddlewareValidTokenPropagatesPrincipal verifies that a valid token
// results in the Principal's identity reaching AuditMeta inside the use case.
func TestAuthMiddlewareValidTokenPropagatesPrincipal(t *testing.T) {
	wantUserID := domain.UserID("dispatcher-42")
	wantRole := domain.ActorRoleDispatcher
	createUC := &fakeCreateRequestUseCase{}
	h := NewHandlerWithClock(
		newFakeAuth(string(wantUserID), string(wantRole)),
		createUC, &fakeGetRequestUseCase{}, &fakeRequestReturnUseCase{}, time.Now,
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/requests",
		strings.NewReader(`{"request_id":"req-1","technician_id":"tech-1","requested_resource_classes":["rc-1"],"audit":{}}`))
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if createUC.in.Audit.ActorID != wantUserID {
		t.Fatalf("audit actor id = %q, want %q", createUC.in.Audit.ActorID, wantUserID)
	}
	if createUC.in.Audit.ActorRole != wantRole {
		t.Fatalf("audit actor role = %q, want %q", createUC.in.Audit.ActorRole, wantRole)
	}
}

// ── Actor Field Rejection ──────────────────────────────────────────────────────

// TestBodyWithActorFieldsReturns400 verifies that sending actor_id or actor_role
// inside the audit object is rejected with 400 (DisallowUnknownFields).
// This is the primary regression test for the security finding.
func TestBodyWithActorFieldsReturns400(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "actor_id in audit",
			body: `{"request_id":"req-1","technician_id":"tech-1","requested_resource_classes":["rc-1"],"audit":{"actor_id":"attacker"}}`,
		},
		{
			name: "actor_role in audit",
			body: `{"request_id":"req-1","technician_id":"tech-1","requested_resource_classes":["rc-1"],"audit":{"actor_role":"dispatcher"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandlerWithClock(
				newFakeAuth("user-1", "dispatcher"),
				&fakeCreateRequestUseCase{}, &fakeGetRequestUseCase{}, &fakeRequestReturnUseCase{}, time.Now,
			)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/requests", strings.NewReader(tt.body))
			req.Header.Set("Authorization", "Bearer test-token")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
			errRes := decodeErrorResponse(t, rec)
			if errRes.Error.Code != "bad_request" {
				t.Fatalf("error.code = %q, want bad_request", errRes.Error.Code)
			}
		})
	}
}

// ── Missing Principal (Programming Error) ─────────────────────────────────────

// TestHandlerMissingPrincipalReturns500 calls the handler method DIRECTLY without
// going through the middleware, so no Principal is in the context.
// This simulates a programming error (e.g. a route bypasses the auth middleware).
// Expected result: 500 (not 401), so the bug surfaces loudly.
func TestHandlerMissingPrincipalReturns500(t *testing.T) {
	h := &handler{
		createRequest: &fakeCreateRequestUseCase{},
		now:           time.Now,
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/requests",
		strings.NewReader(`{"request_id":"req-1","technician_id":"tech-1","requested_resource_classes":["rc-1"],"audit":{}}`))
	rec := httptest.NewRecorder()
	h.handleCreateRequest(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	errRes := decodeErrorResponse(t, rec)
	if errRes.Error.Code != "internal_error" {
		t.Fatalf("error.code = %q, want internal_error", errRes.Error.Code)
	}
}

// ── Existing Tests (updated for Bearer auth) ──────────────────────────────────

func TestCreateRequestInvalidJSONReturnsBadRequest(t *testing.T) {
	h := NewHandlerWithClock(newFakeAuth("user-1", "dispatcher"), &fakeCreateRequestUseCase{}, &fakeGetRequestUseCase{}, &fakeRequestReturnUseCase{}, func() time.Time {
		return time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/requests", strings.NewReader(`{"request_id":`))
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	errRes := decodeErrorResponse(t, rec)
	if errRes.Error.Code != "bad_request" {
		t.Fatalf("error.code = %q, want bad_request", errRes.Error.Code)
	}
}

func TestCreateRequestErrorMapping(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "not_found", err: ports.ErrNotFound, wantStatus: http.StatusNotFound, wantCode: "not_found"},
		{name: "conflict", err: ports.ErrConflict, wantStatus: http.StatusConflict, wantCode: "conflict"},
		{name: "validation", err: domain.ErrRequiredField, wantStatus: http.StatusUnprocessableEntity, wantCode: "validation_error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			createUC := &fakeCreateRequestUseCase{err: fmt.Errorf("wrapped: %w", tt.err)}
			h := NewHandlerWithClock(newFakeAuth("user-1", "dispatcher"), createUC, &fakeGetRequestUseCase{}, &fakeRequestReturnUseCase{}, func() time.Time {
				return time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC)
			})

			req := httptest.NewRequest(http.MethodPost, "/api/v1/requests",
				strings.NewReader(`{"request_id":"req-1","technician_id":"tech-1","requested_resource_classes":["rc-1"],"audit":{}}`))
			req.Header.Set("Authorization", "Bearer test-token")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			errRes := decodeErrorResponse(t, rec)
			if errRes.Error.Code != tt.wantCode {
				t.Fatalf("error.code = %q, want %q", errRes.Error.Code, tt.wantCode)
			}
		})
	}
}

// TestCreateRequestSuccessAuditFromPrincipalAndDefaultCreatedAt replaces the old
// "UsesHeaderAudit" test. Actor identity now comes from the Bearer token / Principal,
// not from X-Actor-ID or audit body fields. X-Client-Occurred-At remains valid.
func TestCreateRequestSuccessAuditFromPrincipalAndDefaultCreatedAt(t *testing.T) {
	fixedNow := time.Date(2026, 7, 19, 8, 30, 0, 0, time.UTC)
	created, err := domain.NewRequest("req-1", "tech-1", "ctx", "ctx-label", nil, nil, "", []domain.ResourceClassID{"rc-1"}, fixedNow)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	createUC := &fakeCreateRequestUseCase{out: created}
	h := NewHandlerWithClock(
		newFakeAuth("dispatcher-token-user", "dispatcher"),
		createUC, &fakeGetRequestUseCase{}, &fakeRequestReturnUseCase{},
		func() time.Time { return fixedNow },
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/requests",
		strings.NewReader(`{"request_id":"req-1","technician_id":"tech-1","context_ref":"ctx","context_label":"ctx-label","requested_resource_classes":["rc-1"],"audit":{}}`))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("X-Client-Occurred-At", "2026-07-19T08:15:00Z")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	// Actor identity comes from the Principal (Bearer token), NOT body or any header.
	if createUC.in.Audit.ActorID != "dispatcher-token-user" {
		t.Fatalf("audit actor id = %q, want dispatcher-token-user", createUC.in.Audit.ActorID)
	}
	if createUC.in.Audit.ActorRole != domain.ActorRoleDispatcher {
		t.Fatalf("audit actor role = %q, want dispatcher", createUC.in.Audit.ActorRole)
	}
	if createUC.in.CreatedAt != fixedNow {
		t.Fatalf("created at = %v, want %v", createUC.in.CreatedAt, fixedNow)
	}
	// X-Client-Occurred-At (informational client timestamp) is still accepted.
	if createUC.in.Audit.ClientOccurredAt == nil {
		t.Fatal("client occurred at should be set from X-Client-Occurred-At header")
	}
	want := time.Date(2026, 7, 19, 8, 15, 0, 0, time.UTC)
	if !createUC.in.Audit.ClientOccurredAt.Equal(want) {
		t.Fatalf("client occurred at = %v, want %v", createUC.in.Audit.ClientOccurredAt, want)
	}
}

func TestGetRequestSuccess(t *testing.T) {
	createdAt := time.Date(2026, 7, 18, 9, 0, 0, 0, time.UTC)
	reqEntity, err := domain.NewRequest("req-1", "tech-1", "ctx", "ctx-label", nil, nil, "", []domain.ResourceClassID{"rc-1"}, createdAt)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	getUC := &fakeGetRequestUseCase{out: reqEntity}
	h := NewHandlerWithClock(newFakeAuth("user-1", "dispatcher"), &fakeCreateRequestUseCase{}, getUC, &fakeRequestReturnUseCase{}, time.Now)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/requests/req-1", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if getUC.requestID != "req-1" {
		t.Fatalf("request id = %s, want req-1", getUC.requestID)
	}
}

func TestRequestReturnValidationErrorMapping(t *testing.T) {
	returnUC := &fakeRequestReturnUseCase{err: fmt.Errorf("wrapped: %w", domain.ErrRequiredField)}
	h := NewHandlerWithClock(newFakeAuth("user-1", "dispatcher"), &fakeCreateRequestUseCase{}, &fakeGetRequestUseCase{}, returnUC, time.Now)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/allocations/alloc-1/return-request",
		strings.NewReader(`{"audit":{}}`))
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	errRes := decodeErrorResponse(t, rec)
	if errRes.Error.Code != "validation_error" {
		t.Fatalf("error.code = %q, want validation_error", errRes.Error.Code)
	}
	if !errors.Is(returnUC.err, domain.ErrRequiredField) {
		t.Fatalf("expected wrapped validation error")
	}
}

func decodeErrorResponse(t *testing.T, rec *httptest.ResponseRecorder) responseError {
	t.Helper()
	var out responseError
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	return out
}
