package httpadapter

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/application/usecases"
)

type refreshUseCaseFake struct {
	out *usecases.RefreshSessionOutput
	err error
}

func (f refreshUseCaseFake) Execute(_ context.Context, _ usecases.RefreshSessionInput) (*usecases.RefreshSessionOutput, error) {
	return f.out, f.err
}

func TestHandleRefreshSession(t *testing.T) {
	h := &handler{refreshSession: refreshUseCaseFake{out: &usecases.RefreshSessionOutput{AccessToken: "access", RefreshToken: "refresh"}}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", strings.NewReader(`{"refresh_token":"old"}`))
	res := httptest.NewRecorder()

	h.handleRefreshSession(res, req)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"access_token":"access"`) {
		t.Fatalf("response = %d %s, want token response", res.Code, res.Body.String())
	}
}

func TestHandleRefreshSessionRejectsMalformedJSON(t *testing.T) {
	h := &handler{refreshSession: refreshUseCaseFake{}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", strings.NewReader(`{"refresh_token":`))
	res := httptest.NewRecorder()

	h.handleRefreshSession(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", res.Code)
	}
}

func TestHandleRefreshSessionMapsInvalidToken(t *testing.T) {
	h := &handler{refreshSession: refreshUseCaseFake{err: errors.Join(errors.New("lookup"), ports.ErrTokenInvalid)}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", strings.NewReader(`{"refresh_token":"old"}`))
	res := httptest.NewRecorder()

	h.handleRefreshSession(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", res.Code)
	}
}
