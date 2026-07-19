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

func TestCreateRequestInvalidJSONReturnsBadRequest(t *testing.T) {
	h := NewHandlerWithClock(&fakeCreateRequestUseCase{}, &fakeGetRequestUseCase{}, &fakeRequestReturnUseCase{}, func() time.Time {
		return time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/requests", strings.NewReader(`{"request_id":`))
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
			h := NewHandlerWithClock(createUC, &fakeGetRequestUseCase{}, &fakeRequestReturnUseCase{}, func() time.Time {
				return time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC)
			})

			req := httptest.NewRequest(http.MethodPost, "/api/v1/requests", strings.NewReader(`{"request_id":"req-1","technician_id":"tech-1","requested_resource_classes":["rc-1"],"audit":{"actor_id":"dispatcher-1","actor_role":"dispatcher"}}`))
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

func TestCreateRequestSuccessUsesHeaderAuditAndDefaultCreatedAt(t *testing.T) {
	fixedNow := time.Date(2026, 7, 19, 8, 30, 0, 0, time.UTC)
	created, err := domain.NewRequest("req-1", "tech-1", "ctx", "ctx-label", nil, nil, "", []domain.ResourceClassID{"rc-1"}, fixedNow)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	createUC := &fakeCreateRequestUseCase{out: created}
	h := NewHandlerWithClock(createUC, &fakeGetRequestUseCase{}, &fakeRequestReturnUseCase{}, func() time.Time {
		return fixedNow
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/requests", strings.NewReader(`{"request_id":"req-1","technician_id":"tech-1","context_ref":"ctx","context_label":"ctx-label","requested_resource_classes":["rc-1"],"audit":{"actor_id":"dispatcher-body","actor_role":"dispatcher"}}`))
	req.Header.Set("X-Actor-ID", "dispatcher-header")
	req.Header.Set("X-Client-Occurred-At", "2026-07-19T08:15:00Z")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if createUC.in.Audit.ActorID != "dispatcher-header" {
		t.Fatalf("audit actor id = %s, want dispatcher-header", createUC.in.Audit.ActorID)
	}
	if createUC.in.CreatedAt != fixedNow {
		t.Fatalf("created at = %v, want %v", createUC.in.CreatedAt, fixedNow)
	}
	if createUC.in.Audit.ClientOccurredAt == nil {
		t.Fatalf("client occurred at should be set from header")
	}
	wantClientOccurredAt := time.Date(2026, 7, 19, 8, 15, 0, 0, time.UTC)
	if !createUC.in.Audit.ClientOccurredAt.Equal(wantClientOccurredAt) {
		t.Fatalf("client occurred at = %v, want %v", createUC.in.Audit.ClientOccurredAt, wantClientOccurredAt)
	}
}

func TestGetRequestSuccess(t *testing.T) {
	createdAt := time.Date(2026, 7, 18, 9, 0, 0, 0, time.UTC)
	reqEntity, err := domain.NewRequest("req-1", "tech-1", "ctx", "ctx-label", nil, nil, "", []domain.ResourceClassID{"rc-1"}, createdAt)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	getUC := &fakeGetRequestUseCase{out: reqEntity}
	h := NewHandlerWithClock(&fakeCreateRequestUseCase{}, getUC, &fakeRequestReturnUseCase{}, time.Now)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/requests/req-1", nil)
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
	h := NewHandlerWithClock(&fakeCreateRequestUseCase{}, &fakeGetRequestUseCase{}, returnUC, time.Now)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/allocations/alloc-1/return-request", strings.NewReader(`{"audit":{"actor_id":"dispatcher-1","actor_role":"dispatcher"}}`))
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
