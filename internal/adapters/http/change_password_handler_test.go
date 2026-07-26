package httpadapter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/application/usecases"
	"bestelltool_be/internal/domain"
)

type fakeChangeOwnPasswordUseCase struct {
	in    usecases.ChangeOwnPasswordInput
	err   error
	calls int
}

func (f *fakeChangeOwnPasswordUseCase) Execute(_ context.Context, in usecases.ChangeOwnPasswordInput) error {
	f.calls++
	f.in = in
	return f.err
}

func TestHandleChangeOwnPasswordUsesAuthenticatedPrincipal(t *testing.T) {
	uc := &fakeChangeOwnPasswordUseCase{}
	h := &handler{changeOwnPassword: uc}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password/change", strings.NewReader(`{"old_password":"old","new_password":"new"}`))
	req = req.WithContext(context.WithValue(req.Context(), contextKey, &ports.Principal{UserID: "user-123", Role: domain.ActorRoleTechnician, SessionID: "session-123"}))
	rec := httptest.NewRecorder()

	h.handleChangeOwnPassword(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if uc.calls != 1 || uc.in.UserID != "user-123" || uc.in.ActorRole != domain.ActorRoleTechnician || uc.in.CurrentSessionID != "session-123" {
		t.Fatalf("input = %+v, want authenticated principal data", uc.in)
	}
	if uc.in.OldPassword != "old" || uc.in.NewPassword != "new" {
		t.Fatalf("password input = %+v", uc.in)
	}
}
