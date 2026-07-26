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

// newFakeAuth returns an authenticator that succeeds with the given identity and
// the permissions assigned to its active role in the migration seed.
func newFakeAuth(userID, role string) *fakeAuthenticator {
	permissions := make(map[string]struct{})
	for _, permission := range permissionsForRole(domain.ActorRole(role)) {
		permissions[permission] = struct{}{}
	}
	return &fakeAuthenticator{
		principal: &ports.Principal{
			UserID:      domain.UserID(userID),
			Role:        domain.ActorRole(role),
			Permissions: permissions,
		},
	}
}

func permissionsForRole(role domain.ActorRole) []string {
	switch role {
	case domain.ActorRoleTechnician:
		return []string{
			domain.PermissionRequestCreate,
			domain.PermissionRequestRead,
			domain.PermissionAllocationReturnRequest,
			domain.PermissionEventStreamOwn,
		}
	case domain.ActorRoleDispatcher:
		return []string{
			domain.PermissionRequestRead,
			domain.PermissionAllocationReturnRequest,
			domain.PermissionResourceTransferDirect,
			domain.PermissionEventStreamAll,
		}
	default:
		return nil
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

type fakeTransferResourceUseCase struct {
	in  usecases.TransferResourceInput
	err error
}

func (f *fakeTransferResourceUseCase) Execute(_ context.Context, in usecases.TransferResourceInput) error {
	f.in = in
	return f.err
}

type fakeEventStream struct {
	principal    ports.Principal
	events       chan ports.Event
	unsubscribed bool
}

func (f *fakeEventStream) Subscribe(principal ports.Principal) (<-chan ports.Event, func()) {
	f.principal = principal
	return f.events, func() {
		f.unsubscribed = true
	}
}

type blockingEventStream struct {
	entered chan struct{}
	release <-chan struct{}
	events  <-chan ports.Event
}

func (f *blockingEventStream) Subscribe(_ ports.Principal) (<-chan ports.Event, func()) {
	close(f.entered)
	<-f.release
	return f.events, func() {}
}

type flushSignalWriter struct {
	header  http.Header
	status  int
	flushed chan struct{}
}

func newFlushSignalWriter() *flushSignalWriter {
	return &flushSignalWriter{
		header:  make(http.Header),
		flushed: make(chan struct{}, 1),
	}
}

func (w *flushSignalWriter) Header() http.Header {
	return w.header
}

func (w *flushSignalWriter) WriteHeader(statusCode int) {
	w.status = statusCode
}

func (w *flushSignalWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return len(p), nil
}

func (w *flushSignalWriter) Flush() {
	select {
	case w.flushed <- struct{}{}:
	default:
	}
}

type responseError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// ── Auth Middleware ────────────────────────────────────────────────────────────

func TestAuthMiddlewareMissingHeader(t *testing.T) {
	h := NewHandlerWithClock(newFakeAuth("user-1", "dispatcher"), &fakeCreateRequestUseCase{}, &fakeGetRequestUseCase{}, &fakeRequestReturnUseCase{}, &fakeTransferResourceUseCase{}, time.Now)

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
	h := NewHandlerWithClock(a, &fakeCreateRequestUseCase{}, &fakeGetRequestUseCase{}, &fakeRequestReturnUseCase{}, &fakeTransferResourceUseCase{}, time.Now)

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
	wantUserID := domain.UserID("technician-42")
	wantRole := domain.ActorRoleTechnician
	createUC := &fakeCreateRequestUseCase{}
	h := NewHandlerWithClock(
		newFakeAuth(string(wantUserID), string(wantRole)),
		createUC, &fakeGetRequestUseCase{}, &fakeRequestReturnUseCase{}, &fakeTransferResourceUseCase{}, time.Now,
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/requests",
		strings.NewReader(`{"request_id":"req-1","requested_resource_classes":["rc-1"],"audit":{}}`))
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if createUC.in.Audit.ActorID != wantUserID {
		t.Fatalf("audit actor id = %q, want %q", createUC.in.Audit.ActorID, wantUserID)
	}
	if createUC.in.Audit.ActorRole != wantRole {
		t.Fatalf("audit actor role = %q, want %q", createUC.in.Audit.ActorRole, wantRole)
	}
	if createUC.in.TechnicianID != wantUserID {
		t.Fatalf("technician id = %q, want %q", createUC.in.TechnicianID, wantUserID)
	}
}

func TestHealthzPublicWithoutAuth(t *testing.T) {
	h := NewHandlerWithClock(
		&fakeAuthenticator{err: fmt.Errorf("should not be called")},
		&fakeCreateRequestUseCase{},
		&fakeGetRequestUseCase{},
		&fakeRequestReturnUseCase{},
		&fakeTransferResourceUseCase{},
		time.Now,
	)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if got["status"] != "ok" {
		t.Fatalf("status payload = %q, want ok", got["status"])
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
			body: `{"request_id":"req-1","requested_resource_classes":["rc-1"],"audit":{"actor_id":"attacker"}}`,
		},
		{
			name: "actor_role in audit",
			body: `{"request_id":"req-1","requested_resource_classes":["rc-1"],"audit":{"actor_role":"dispatcher"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandlerWithClock(
				newFakeAuth("user-1", "technician"),
				&fakeCreateRequestUseCase{}, &fakeGetRequestUseCase{}, &fakeRequestReturnUseCase{}, &fakeTransferResourceUseCase{}, time.Now,
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

func TestCreateRequestRejectsTechnicianIDFromBody_SEC01(t *testing.T) {
	createUC := &fakeCreateRequestUseCase{}
	h := NewHandlerWithClock(
		newFakeAuth("tech-authenticated", "technician"),
		createUC, &fakeGetRequestUseCase{}, &fakeRequestReturnUseCase{}, &fakeTransferResourceUseCase{}, time.Now,
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/requests",
		strings.NewReader(`{"request_id":"req-1","technician_id":"tech-foreign","requested_resource_classes":["rc-1"],"audit":{}}`))
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
	if createUC.in.RequestID != "" {
		t.Fatalf("use case should not be called, got request id = %q", createUC.in.RequestID)
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
		strings.NewReader(`{"request_id":"req-1","requested_resource_classes":["rc-1"],"audit":{}}`))
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
	h := NewHandlerWithClock(newFakeAuth("user-1", "technician"), &fakeCreateRequestUseCase{}, &fakeGetRequestUseCase{}, &fakeRequestReturnUseCase{}, &fakeTransferResourceUseCase{}, func() time.Time {
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
			h := NewHandlerWithClock(newFakeAuth("user-1", "technician"), createUC, &fakeGetRequestUseCase{}, &fakeRequestReturnUseCase{}, &fakeTransferResourceUseCase{}, func() time.Time {
				return time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC)
			})

			req := httptest.NewRequest(http.MethodPost, "/api/v1/requests",
				strings.NewReader(`{"request_id":"req-1","requested_resource_classes":["rc-1"],"audit":{}}`))
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
		newFakeAuth("tech-1", "technician"),
		createUC, &fakeGetRequestUseCase{}, &fakeRequestReturnUseCase{}, &fakeTransferResourceUseCase{},
		func() time.Time { return fixedNow },
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/requests",
		strings.NewReader(`{"request_id":"req-1","context_ref":"ctx","context_label":"ctx-label","requested_resource_classes":["rc-1"],"audit":{}}`))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("X-Client-Occurred-At", "2026-07-19T08:15:00Z")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	// Actor identity comes from the Principal (Bearer token), NOT body or any header.
	if createUC.in.Audit.ActorID != "tech-1" {
		t.Fatalf("audit actor id = %q, want tech-1", createUC.in.Audit.ActorID)
	}
	if createUC.in.Audit.ActorRole != domain.ActorRoleTechnician {
		t.Fatalf("audit actor role = %q, want technician", createUC.in.Audit.ActorRole)
	}
	if createUC.in.CreatedAt != fixedNow {
		t.Fatalf("created at = %v, want %v", createUC.in.CreatedAt, fixedNow)
	}
	if createUC.in.TechnicianID != "tech-1" {
		t.Fatalf("input technician id = %q, want tech-1", createUC.in.TechnicianID)
	}
	var response requestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.TechnicianID != "tech-1" {
		t.Fatalf("response technician id = %q, want tech-1", response.TechnicianID)
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
	h := NewHandlerWithClock(newFakeAuth("user-1", "dispatcher"), &fakeCreateRequestUseCase{}, getUC, &fakeRequestReturnUseCase{}, &fakeTransferResourceUseCase{}, time.Now)

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
	h := NewHandlerWithClock(newFakeAuth("user-1", "technician"), &fakeCreateRequestUseCase{}, &fakeGetRequestUseCase{}, returnUC, &fakeTransferResourceUseCase{}, time.Now)

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

func TestTransferResourceSuccess(t *testing.T) {
	fixedNow := time.Date(2026, 7, 19, 9, 45, 0, 0, time.UTC)
	transferUC := &fakeTransferResourceUseCase{}
	h := NewHandlerWithClock(
		newFakeAuth("dispatcher-1", "dispatcher"),
		&fakeCreateRequestUseCase{}, &fakeGetRequestUseCase{}, &fakeRequestReturnUseCase{}, transferUC,
		func() time.Time { return fixedNow },
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/resources/res-1/transfer", strings.NewReader(`{"old_allocation_id":"alloc-old","new_allocation_id":"alloc-new","target_request_id":"req-target","planned_from":"2026-07-19T10:00:00Z","planned_until":"2026-07-19T12:00:00Z","audit":{"client_seq":7,"note":"handover"}}`))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("X-Client-Occurred-At", "2026-07-19T09:40:00Z")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if transferUC.in.OldAllocationID != "alloc-old" {
		t.Fatalf("old allocation id = %s, want alloc-old", transferUC.in.OldAllocationID)
	}
	if transferUC.in.NewAllocationID != "alloc-new" {
		t.Fatalf("new allocation id = %s, want alloc-new", transferUC.in.NewAllocationID)
	}
	if transferUC.in.TargetRequestID != "req-target" {
		t.Fatalf("target request id = %s, want req-target", transferUC.in.TargetRequestID)
	}
	if !transferUC.in.PlannedFrom.Equal(time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("planned_from = %v, want 2026-07-19T10:00:00Z", transferUC.in.PlannedFrom)
	}
	if !transferUC.in.PlannedUntil.Equal(time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("planned_until = %v, want 2026-07-19T12:00:00Z", transferUC.in.PlannedUntil)
	}
	if transferUC.in.At != fixedNow {
		t.Fatalf("at = %v, want %v", transferUC.in.At, fixedNow)
	}
	if transferUC.in.Audit.ActorID != "dispatcher-1" {
		t.Fatalf("audit actor id = %q, want dispatcher-1", transferUC.in.Audit.ActorID)
	}
	if transferUC.in.Audit.ActorRole != domain.ActorRoleDispatcher {
		t.Fatalf("audit actor role = %q, want dispatcher", transferUC.in.Audit.ActorRole)
	}
	if transferUC.in.Audit.ClientSeq == nil || *transferUC.in.Audit.ClientSeq != 7 {
		t.Fatalf("audit client_seq = %v, want 7", transferUC.in.Audit.ClientSeq)
	}
	if transferUC.in.Audit.Note != "handover" {
		t.Fatalf("audit note = %q, want handover", transferUC.in.Audit.Note)
	}
	if transferUC.in.Audit.ClientOccurredAt == nil {
		t.Fatal("client occurred at should be set from X-Client-Occurred-At header")
	}
	wantClientOccurredAt := time.Date(2026, 7, 19, 9, 40, 0, 0, time.UTC)
	if !transferUC.in.Audit.ClientOccurredAt.Equal(wantClientOccurredAt) {
		t.Fatalf("client occurred at = %v, want %v", transferUC.in.Audit.ClientOccurredAt, wantClientOccurredAt)
	}
}

func TestTransferResourceBadRequest(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "malformed json", body: `{"old_allocation_id":`},
		{name: "actor_id in audit", body: `{"old_allocation_id":"alloc-old","new_allocation_id":"alloc-new","target_request_id":"req-target","planned_from":"2026-07-19T10:00:00Z","planned_until":"2026-07-19T12:00:00Z","audit":{"actor_id":"attacker"}}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandlerWithClock(
				newFakeAuth("dispatcher-1", "dispatcher"),
				&fakeCreateRequestUseCase{}, &fakeGetRequestUseCase{}, &fakeRequestReturnUseCase{}, &fakeTransferResourceUseCase{},
				time.Now,
			)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/resources/res-1/transfer", strings.NewReader(tt.body))
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

func TestTransferResourceUnauthorized(t *testing.T) {
	body := `{"old_allocation_id":"alloc-old","new_allocation_id":"alloc-new","target_request_id":"req-target","planned_from":"2026-07-19T10:00:00Z","planned_until":"2026-07-19T12:00:00Z","audit":{}}`

	t.Run("missing authorization header", func(t *testing.T) {
		transferUC := &fakeTransferResourceUseCase{}
		h := NewHandlerWithClock(
			newFakeAuth("dispatcher-1", "dispatcher"),
			&fakeCreateRequestUseCase{}, &fakeGetRequestUseCase{}, &fakeRequestReturnUseCase{}, transferUC,
			time.Now,
		)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/resources/res-1/transfer", strings.NewReader(body))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
		errRes := decodeErrorResponse(t, rec)
		if errRes.Error.Code != "unauthenticated" {
			t.Fatalf("error.code = %q, want unauthenticated", errRes.Error.Code)
		}
		if transferUC.in.OldAllocationID != "" {
			t.Fatalf("use case should not be called, got old allocation id = %q", transferUC.in.OldAllocationID)
		}
	})

	t.Run("invalid token", func(t *testing.T) {
		transferUC := &fakeTransferResourceUseCase{}
		h := NewHandlerWithClock(
			&fakeAuthenticator{err: fmt.Errorf("invalid token: %w", ports.ErrUnauthenticated)},
			&fakeCreateRequestUseCase{}, &fakeGetRequestUseCase{}, &fakeRequestReturnUseCase{}, transferUC,
			time.Now,
		)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/resources/res-1/transfer", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer invalid")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
		errRes := decodeErrorResponse(t, rec)
		if errRes.Error.Code != "unauthenticated" {
			t.Fatalf("error.code = %q, want unauthenticated", errRes.Error.Code)
		}
		if transferUC.in.OldAllocationID != "" {
			t.Fatalf("use case should not be called, got old allocation id = %q", transferUC.in.OldAllocationID)
		}
	})
}

func TestTransferResourceForbiddenRole(t *testing.T) {
	transferUC := &fakeTransferResourceUseCase{}
	h := NewHandlerWithClock(
		newFakeAuth("tech-1", "technician"),
		&fakeCreateRequestUseCase{}, &fakeGetRequestUseCase{}, &fakeRequestReturnUseCase{}, transferUC,
		time.Now,
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/resources/res-1/transfer", strings.NewReader(`{"old_allocation_id":"alloc-old","new_allocation_id":"alloc-new","target_request_id":"req-target","planned_from":"2026-07-19T10:00:00Z","planned_until":"2026-07-19T12:00:00Z","audit":{}}`))
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	errRes := decodeErrorResponse(t, rec)
	if errRes.Error.Code != "forbidden" {
		t.Fatalf("error.code = %q, want forbidden", errRes.Error.Code)
	}
	if transferUC.in.OldAllocationID != "" {
		t.Fatalf("use case should not be called, got old allocation id = %q", transferUC.in.OldAllocationID)
	}
}

func TestTransferResourceErrorMapping(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "not_found", err: ports.ErrNotFound, wantStatus: http.StatusNotFound, wantCode: "not_found"},
		{name: "conflict", err: ports.ErrConflict, wantStatus: http.StatusConflict, wantCode: "conflict"},
		{name: "already_completed", err: domain.ErrAlreadyCompleted, wantStatus: http.StatusConflict, wantCode: "conflict"},
		{name: "validation", err: domain.ErrRequiredField, wantStatus: http.StatusUnprocessableEntity, wantCode: "validation_error"},
	}

	body := `{"old_allocation_id":"alloc-old","new_allocation_id":"alloc-new","target_request_id":"req-target","planned_from":"2026-07-19T10:00:00Z","planned_until":"2026-07-19T12:00:00Z","audit":{}}`

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transferUC := &fakeTransferResourceUseCase{err: fmt.Errorf("wrapped: %w", tt.err)}
			h := NewHandlerWithClock(
				newFakeAuth("dispatcher-1", "dispatcher"),
				&fakeCreateRequestUseCase{}, &fakeGetRequestUseCase{}, &fakeRequestReturnUseCase{}, transferUC,
				time.Now,
			)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/resources/res-1/transfer", strings.NewReader(body))
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

func TestHandleEventsStreamSuccess(t *testing.T) {
	events := make(chan ports.Event, 1)
	events <- ports.Event{
		Type:         ports.EventTypeRequestCreated,
		RequestID:    "req-1",
		TechnicianID: "tech-1",
		OccurredAt:   time.Now().UTC(),
	}
	close(events)

	stream := &fakeEventStream{events: events}
	h := NewHandlerWithEventStreamAndClock(
		newFakeAuth("tech-1", "technician"),
		&fakeCreateRequestUseCase{},
		&fakeGetRequestUseCase{},
		&fakeRequestReturnUseCase{},
		&fakeTransferResourceUseCase{},
		stream,
		time.Now,
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("content-type = %q, want text/event-stream", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("cache-control = %q, want no-cache", got)
	}
	if got := rec.Header().Get("Connection"); got != "keep-alive" {
		t.Fatalf("connection = %q, want keep-alive", got)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "event: request.created") {
		t.Fatalf("response body missing event type, body = %q", body)
	}
	if !strings.Contains(body, `"request_id":"req-1"`) {
		t.Fatalf("response body missing request id, body = %q", body)
	}

	if stream.principal.UserID != "tech-1" {
		t.Fatalf("stream principal user id = %q, want tech-1", stream.principal.UserID)
	}
	if stream.principal.Role != domain.ActorRoleTechnician {
		t.Fatalf("stream principal role = %q, want technician", stream.principal.Role)
	}
	if !stream.unsubscribed {
		t.Fatal("stream must be unsubscribed after handler returns")
	}
}

func TestHandleEventsFlushesBeforeSubscribeReturns(t *testing.T) {
	events := make(chan ports.Event)
	close(events)

	release := make(chan struct{}, 1)
	stream := &blockingEventStream{
		entered: make(chan struct{}),
		release: release,
		events:  events,
	}
	h := NewHandlerWithEventStreamAndClock(
		newFakeAuth("dispatcher-1", "dispatcher"),
		&fakeCreateRequestUseCase{},
		&fakeGetRequestUseCase{},
		&fakeRequestReturnUseCase{},
		&fakeTransferResourceUseCase{},
		stream,
		time.Now,
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	writer := newFlushSignalWriter()

	done := make(chan struct{})
	go func() {
		h.ServeHTTP(writer, req)
		close(done)
	}()
	defer func() {
		select {
		case release <- struct{}{}:
		default:
		}
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("handler did not return after subscribe release")
		}
	}()

	select {
	case <-stream.entered:
	case <-time.After(time.Second):
		t.Fatal("handler did not call subscribe")
	}

	select {
	case <-writer.flushed:
	case <-time.After(time.Second):
		t.Fatal("expected SSE stream to flush response before subscribe returns")
	}

	if writer.status != http.StatusOK {
		t.Fatalf("status = %d, want 200", writer.status)
	}
}

func TestEventsAuthorizationMatrix(t *testing.T) {
	tests := []struct {
		name           string
		role           string
		wantStatus     int
		wantCode       string
		wantSubscribed bool
	}{
		{name: "technician allowed", role: "technician", wantStatus: http.StatusOK, wantSubscribed: true},
		{name: "dispatcher allowed", role: "dispatcher", wantStatus: http.StatusOK, wantSubscribed: true},
		{name: "admin forbidden", role: "admin", wantStatus: http.StatusForbidden, wantCode: "forbidden", wantSubscribed: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := make(chan ports.Event)
			close(events)

			stream := &fakeEventStream{events: events}
			h := NewHandlerWithEventStreamAndClock(
				newFakeAuth("user-1", tt.role),
				&fakeCreateRequestUseCase{},
				&fakeGetRequestUseCase{},
				&fakeRequestReturnUseCase{},
				&fakeTransferResourceUseCase{},
				stream,
				time.Now,
			)

			req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
			req.Header.Set("Authorization", "Bearer test-token")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}

			subscribed := stream.principal.UserID != ""
			if subscribed != tt.wantSubscribed {
				t.Fatalf("subscribed = %v, want %v", subscribed, tt.wantSubscribed)
			}

			if tt.wantCode != "" {
				e := decodeErrorResponse(t, rec)
				if e.Error.Code != tt.wantCode {
					t.Fatalf("error.code = %q, want %q", e.Error.Code, tt.wantCode)
				}
			}
		})
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

// ── requireRoles Middleware Unit Tests ─────────────────────────────────────────

// TestRequireRolesAllowedRole verifies that the inner handler is called exactly
// once and that the Principal remains accessible in the context.
func TestRequireRolesAllowedRole(t *testing.T) {
	callCount := 0
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		p, ok := PrincipalFromContext(r.Context())
		if !ok {
			t.Error("principal must still be in context after requireRoles")
		}
		if p.Role != domain.ActorRoleTechnician {
			t.Errorf("role = %q, want technician", p.Role)
		}
		w.WriteHeader(http.StatusOK)
	})

	mw := requireRoles(domain.ActorRoleTechnician)(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), contextKey, &ports.Principal{
		UserID: "tech-1",
		Role:   domain.ActorRoleTechnician,
	})
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req.WithContext(ctx))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if callCount != 1 {
		t.Fatalf("inner handler called %d times, want 1", callCount)
	}
}

// TestRequireRolesForbiddenRole verifies that a disallowed role receives 403,
// the inner handler is NOT called, and the JSON error envelope is correct.
func TestRequireRolesForbiddenRole(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	mw := requireRoles(domain.ActorRoleTechnician)(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), contextKey, &ports.Principal{
		UserID: "dispatcher-1",
		Role:   domain.ActorRoleDispatcher,
	})
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req.WithContext(ctx))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if called {
		t.Fatal("inner handler must not be called when role is disallowed")
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type = %q, want application/json", ct)
	}
	e := decodeErrorResponse(t, rec)
	if e.Error.Code != "forbidden" {
		t.Fatalf("error.code = %q, want forbidden", e.Error.Code)
	}
}

// TestRequireRolesMissingPrincipal verifies that calling requireRoles without a
// prior authentication step (no Principal in context) results in 500 — a loud
// signal that the middleware chain is incorrectly wired. Must never return 403.
func TestRequireRolesMissingPrincipal(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	mw := requireRoles(domain.ActorRoleTechnician)(inner)

	// No principal injected — simulates programming error.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (programming error)", rec.Code)
	}
	if called {
		t.Fatal("inner handler must not be called when principal is missing")
	}
	e := decodeErrorResponse(t, rec)
	if e.Error.Code != "internal_error" {
		t.Fatalf("error.code = %q, want internal_error", e.Error.Code)
	}
}

// ── Permission Middleware and Route Authorization Tests ───────────────────────

func TestRequirePermissionsRequiresAllPermissions_SEC13(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	})
	mw := requirePermissions(domain.PermissionRequestCreate, domain.PermissionRequestRead)(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), contextKey, &ports.Principal{Permissions: map[string]struct{}{
		domain.PermissionRequestCreate: {},
		domain.PermissionRequestRead:   {},
	}})
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req.WithContext(ctx))

	if rec.Code != http.StatusOK || !called {
		t.Fatalf("status = %d, called = %v; want 200 and called", rec.Code, called)
	}
}

func TestRequirePermissionsMissingPermissionForbidden_SEC13(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })
	mw := requirePermissions(domain.PermissionRequestCreate, domain.PermissionRequestRead)(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), contextKey, &ports.Principal{Permissions: map[string]struct{}{
		domain.PermissionRequestCreate: {},
	}})
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req.WithContext(ctx))

	if rec.Code != http.StatusForbidden || called {
		t.Fatalf("status = %d, called = %v; want 403 and not called", rec.Code, called)
	}
}

func TestRequireAnyPermissionAllowsOnePermission_SEC13(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })
	mw := requireAnyPermission(domain.PermissionEventStreamOwn, domain.PermissionEventStreamAll)(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), contextKey, &ports.Principal{Permissions: map[string]struct{}{
		domain.PermissionEventStreamAll: {},
	}})
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req.WithContext(ctx))

	if rec.Code != http.StatusOK || !called {
		t.Fatalf("status = %d, called = %v; want 200 and called", rec.Code, called)
	}
}

func TestRequireAnyPermissionWithoutPermissionForbidden_SEC13(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })
	mw := requireAnyPermission(domain.PermissionEventStreamOwn, domain.PermissionEventStreamAll)(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), contextKey, &ports.Principal{Permissions: map[string]struct{}{}})
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req.WithContext(ctx))

	if rec.Code != http.StatusForbidden || called {
		t.Fatalf("status = %d, called = %v; want 403 and not called", rec.Code, called)
	}
}

func TestRequireAuthenticatedMissingPrincipalReturnsInternalError_SEC01(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })
	rec := httptest.NewRecorder()
	requireAuthenticated()(inner).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusInternalServerError || called {
		t.Fatalf("status = %d, called = %v; want 500 and not called", rec.Code, called)
	}
}

func TestAllProtectedRoutesDeclarePermissionsOrAreSelfService_SEC13(t *testing.T) {
	routes := &routeRegistry{}
	registerProtectedRoutes(http.NewServeMux(), routes, &handler{})

	if len(routes.routes) != 9 {
		t.Fatalf("registered protected routes = %d, want 9", len(routes.routes))
	}
	for _, route := range routes.routes {
		switch route.Kind {
		case routeAuthorizationPermission, routeAuthorizationAnyPermission:
			if len(route.Permissions) == 0 {
				t.Errorf("%s %s has no permission declaration", route.Method, route.Pattern)
			}
		case routeAuthorizationSelfService:
			if len(route.Permissions) != 0 {
				t.Errorf("%s %s self-service route declares permissions", route.Method, route.Pattern)
			}
		default:
			t.Errorf("%s %s has undeclared authorization kind %q", route.Method, route.Pattern, route.Kind)
		}
	}
}

// ── Per-Endpoint Authorization Matrix Tests ───────────────────────────────────

// TestCreateRequest_AuthorizationMatrix covers the POST /api/v1/requests endpoint.
func TestCreateRequest_AuthorizationMatrix(t *testing.T) {
	body := `{"request_id":"req-auth","requested_resource_classes":["rc-1"],"audit":{}}`

	tests := []struct {
		name       string
		role       string
		wantStatus int
		wantCode   string
		wantCalled bool
	}{
		{name: "technician allowed", role: "technician", wantStatus: http.StatusCreated, wantCalled: true},
		{name: "dispatcher forbidden", role: "dispatcher", wantStatus: http.StatusForbidden, wantCode: "forbidden", wantCalled: false},
		{name: "admin forbidden", role: "admin", wantStatus: http.StatusForbidden, wantCode: "forbidden", wantCalled: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			createUC := &fakeCreateRequestUseCase{}
			h := NewHandlerWithClock(
				newFakeAuth("tech-1", tt.role),
				createUC, &fakeGetRequestUseCase{}, &fakeRequestReturnUseCase{}, &fakeTransferResourceUseCase{},
				time.Now,
			)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/requests", strings.NewReader(body))
			req.Header.Set("Authorization", "Bearer test-token")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			called := createUC.in.RequestID != ""
			if called != tt.wantCalled {
				t.Fatalf("use case called = %v, want %v", called, tt.wantCalled)
			}
			if tt.wantCode != "" {
				if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
					t.Fatalf("content-type = %q, want application/json", ct)
				}
				e := decodeErrorResponse(t, rec)
				if e.Error.Code != tt.wantCode {
					t.Fatalf("error.code = %q, want %q", e.Error.Code, tt.wantCode)
				}
			}
		})
	}
}

// TestGetRequest_AuthorizationMatrix covers the GET /api/v1/requests/{id} endpoint.
func TestGetRequest_AuthorizationMatrix(t *testing.T) {
	tests := []struct {
		name       string
		role       string
		wantStatus int
		wantCode   string
		wantCalled bool
	}{
		{name: "technician allowed", role: "technician", wantStatus: http.StatusOK, wantCalled: true},
		{name: "dispatcher allowed", role: "dispatcher", wantStatus: http.StatusOK, wantCalled: true},
		{name: "admin forbidden without request read SEC-15", role: "admin", wantStatus: http.StatusForbidden, wantCode: "forbidden", wantCalled: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getUC := &fakeGetRequestUseCase{}
			h := NewHandlerWithClock(
				newFakeAuth("user-1", tt.role),
				&fakeCreateRequestUseCase{}, getUC, &fakeRequestReturnUseCase{}, &fakeTransferResourceUseCase{},
				time.Now,
			)

			req := httptest.NewRequest(http.MethodGet, "/api/v1/requests/req-1", nil)
			req.Header.Set("Authorization", "Bearer test-token")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			called := getUC.requestID != ""
			if called != tt.wantCalled {
				t.Fatalf("use case called = %v, want %v", called, tt.wantCalled)
			}
			if tt.wantCode != "" {
				if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
					t.Fatalf("content-type = %q, want application/json", ct)
				}
				e := decodeErrorResponse(t, rec)
				if e.Error.Code != tt.wantCode {
					t.Fatalf("error.code = %q, want %q", e.Error.Code, tt.wantCode)
				}
			}
		})
	}
}

// TestRequestReturn_AuthorizationMatrix covers POST /api/v1/allocations/{id}/return-request.
func TestRequestReturn_AuthorizationMatrix(t *testing.T) {
	body := `{"audit":{}}`

	tests := []struct {
		name       string
		role       string
		wantStatus int
		wantCode   string
		wantCalled bool
	}{
		{name: "technician allowed", role: "technician", wantStatus: http.StatusNoContent, wantCalled: true},
		{name: "dispatcher allowed", role: "dispatcher", wantStatus: http.StatusNoContent, wantCalled: true},
		{name: "admin forbidden", role: "admin", wantStatus: http.StatusForbidden, wantCode: "forbidden", wantCalled: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			returnUC := &fakeRequestReturnUseCase{}
			h := NewHandlerWithClock(
				newFakeAuth("user-1", tt.role),
				&fakeCreateRequestUseCase{}, &fakeGetRequestUseCase{}, returnUC, &fakeTransferResourceUseCase{},
				time.Now,
			)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/allocations/alloc-1/return-request", strings.NewReader(body))
			req.Header.Set("Authorization", "Bearer test-token")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			called := returnUC.in.AllocationID != ""
			if called != tt.wantCalled {
				t.Fatalf("use case called = %v, want %v", called, tt.wantCalled)
			}
			if tt.wantCode != "" {
				if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
					t.Fatalf("content-type = %q, want application/json", ct)
				}
				e := decodeErrorResponse(t, rec)
				if e.Error.Code != tt.wantCode {
					t.Fatalf("error.code = %q, want %q", e.Error.Code, tt.wantCode)
				}
			}
		})
	}
}

// TestTransferResource_AuthorizationMatrix covers POST /api/v1/resources/{id}/transfer.
func TestTransferResource_AuthorizationMatrix(t *testing.T) {
	body := `{"old_allocation_id":"alloc-old","new_allocation_id":"alloc-new","target_request_id":"req-target","planned_from":"2026-07-19T10:00:00Z","planned_until":"2026-07-19T12:00:00Z","audit":{}}`

	tests := []struct {
		name       string
		role       string
		wantStatus int
		wantCode   string
		wantCalled bool
	}{
		{name: "dispatcher allowed", role: "dispatcher", wantStatus: http.StatusNoContent, wantCalled: true},
		{name: "technician forbidden", role: "technician", wantStatus: http.StatusForbidden, wantCode: "forbidden", wantCalled: false},
		{name: "admin forbidden", role: "admin", wantStatus: http.StatusForbidden, wantCode: "forbidden", wantCalled: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transferUC := &fakeTransferResourceUseCase{}
			h := NewHandlerWithClock(
				newFakeAuth("user-1", tt.role),
				&fakeCreateRequestUseCase{}, &fakeGetRequestUseCase{}, &fakeRequestReturnUseCase{}, transferUC,
				time.Now,
			)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/resources/res-1/transfer", strings.NewReader(body))
			req.Header.Set("Authorization", "Bearer test-token")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			called := transferUC.in.OldAllocationID != ""
			if called != tt.wantCalled {
				t.Fatalf("use case called = %v, want %v", called, tt.wantCalled)
			}
			if tt.wantCode != "" {
				if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
					t.Fatalf("content-type = %q, want application/json", ct)
				}
				e := decodeErrorResponse(t, rec)
				if e.Error.Code != tt.wantCode {
					t.Fatalf("error.code = %q, want %q", e.Error.Code, tt.wantCode)
				}
			}
		})
	}
}
