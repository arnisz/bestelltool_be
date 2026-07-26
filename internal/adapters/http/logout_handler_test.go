package httpadapter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"bestelltool_be/internal/application/usecases"
)

type logoutUseCaseFake struct {
	in  usecases.LogoutInput
	err error
}

func (f *logoutUseCaseFake) Execute(_ context.Context, in usecases.LogoutInput) error {
	f.in = in
	return f.err
}

func TestLogoutRouteRequiresAuthenticationAndRevokesSession(t *testing.T) {
	auth := newFakeAuth("user-1", "technician")
	auth.principal.SessionID = "session-1"
	logout := &logoutUseCaseFake{}
	h := NewHandlerWithEventStreamAndAuthentication(auth, nil, nil, nil, nil, nil, nil, nil, logout)

	unauthenticated := httptest.NewRecorder()
	h.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("missing token status = %d, want 401", unauthenticated.Code)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer access-token")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", res.Code)
	}
	if logout.in.SessionID != "session-1" || string(logout.in.ActorID) != "user-1" {
		t.Fatalf("logout input = %#v, want authenticated principal values", logout.in)
	}
}
