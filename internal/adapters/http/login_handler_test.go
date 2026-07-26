package httpadapter

import (
	"context"
	"encoding/json"
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

type fakeLoginUseCase struct {
	in    usecases.LoginInput
	out   *usecases.LoginOutput
	err   error
	calls int
}

func (f *fakeLoginUseCase) Execute(_ context.Context, in usecases.LoginInput) (*usecases.LoginOutput, error) {
	f.calls++
	f.in = in
	if f.err != nil {
		return nil, f.err
	}
	return f.out, nil
}

func newLoginTestHandler(loginUC LoginUseCase) http.Handler {
	return NewHandlerWithEventStreamAndClockAndLogin(
		newFakeAuth("user-1", "dispatcher"),
		&fakeCreateRequestUseCase{},
		&fakeGetRequestUseCase{},
		&fakeRequestReturnUseCase{},
		&fakeTransferResourceUseCase{},
		nil,
		time.Now,
		loginUC,
	)
}

func TestLoginHandlerSuccess(t *testing.T) {
	loginUC := &fakeLoginUseCase{
		out: &usecases.LoginOutput{
			AccessToken:  "access-token-1",
			RefreshToken: "refresh-token-1",
		},
	}

	h := newLoginTestHandler(loginUC)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"alice","password":"secret"}`))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if loginUC.calls != 1 {
		t.Fatalf("calls = %d, want 1", loginUC.calls)
	}
	if loginUC.in.Username != "alice" || loginUC.in.Password != "secret" {
		t.Fatalf("input = %+v, want username=alice and password=secret", loginUC.in)
	}

	var got loginResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.AccessToken != "access-token-1" {
		t.Fatalf("access_token = %q, want access-token-1", got.AccessToken)
	}
	if got.RefreshToken != "refresh-token-1" {
		t.Fatalf("refresh_token = %q, want refresh-token-1", got.RefreshToken)
	}
}

func TestLoginHandlerBadRequestInvalidJSON(t *testing.T) {
	loginUC := &fakeLoginUseCase{}
	h := newLoginTestHandler(loginUC)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"alice","password":`))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if loginUC.calls != 0 {
		t.Fatalf("calls = %d, want 0", loginUC.calls)
	}

	errRes := decodeErrorResponse(t, rec)
	if errRes.Error.Code != "bad_request" {
		t.Fatalf("error.code = %q, want bad_request", errRes.Error.Code)
	}
}

func TestLoginHandlerCredentialsInvalidMapping(t *testing.T) {
	loginUC := &fakeLoginUseCase{err: fmt.Errorf("wrapped: %w", ports.ErrCredentialsInvalid)}
	h := newLoginTestHandler(loginUC)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"alice","password":"wrong"}`))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}

	errRes := decodeErrorResponse(t, rec)
	if errRes.Error.Message != "invalid credentials" {
		t.Fatalf("error.message = %q, want invalid credentials", errRes.Error.Message)
	}
}

func TestLoginHandlerRequiredFieldMapping(t *testing.T) {
	loginUC := &fakeLoginUseCase{err: fmt.Errorf("wrapped: %w", domain.ErrRequiredField)}
	h := newLoginTestHandler(loginUC)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"","password":""}`))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
