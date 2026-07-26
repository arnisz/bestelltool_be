package httpadapter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/application/usecases"
	"bestelltool_be/internal/domain"
)

type fakeSwitchActiveRoleUseCase struct {
	in  usecases.SwitchActiveRoleInput
	out *usecases.SwitchActiveRoleOutput
	err error
}

func (f *fakeSwitchActiveRoleUseCase) Execute(_ context.Context, in usecases.SwitchActiveRoleInput) (*usecases.SwitchActiveRoleOutput, error) {
	f.in = in
	return f.out, f.err
}

type fakeGetMeUseCase struct {
	in  usecases.GetMeInput
	out *usecases.GetMeOutput
	err error
}

func (f *fakeGetMeUseCase) Execute(_ context.Context, in usecases.GetMeInput) (*usecases.GetMeOutput, error) {
	f.in = in
	return f.out, f.err
}

func TestHandleSwitchActiveRoleUsesAuthenticatedPrincipal_SEC01(t *testing.T) {
	uc := &fakeSwitchActiveRoleUseCase{out: &usecases.SwitchActiveRoleOutput{
		AccessToken:  "rp_at_new.access",
		RefreshToken: "rp_rt_new.refresh",
		ActiveRole:   domain.ActorRoleDispatcher,
	}}
	h := &handler{switchActiveRole: uc}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/switch-role", strings.NewReader(`{"active_role":"dispatcher"}`))
	req = req.WithContext(context.WithValue(req.Context(), contextKey, &ports.Principal{UserID: "user-1", SessionID: "session-1", Role: domain.ActorRoleTechnician}))
	rec := httptest.NewRecorder()

	h.handleSwitchActiveRole(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if uc.in.UserID != "user-1" || uc.in.CurrentSessionID != "session-1" || uc.in.RequestedRole != domain.ActorRoleDispatcher {
		t.Fatalf("use case input = %+v, want authenticated user/session and dispatcher", uc.in)
	}
}

func TestHandleGetMeReturnsSortedActiveRolePermissions(t *testing.T) {
	uc := &fakeGetMeUseCase{out: &usecases.GetMeOutput{Roles: []domain.ActorRole{domain.ActorRoleTechnician, domain.ActorRoleDispatcher}}}
	h := &handler{getMe: uc}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKey, &ports.Principal{
		UserID:      "user-1",
		Role:        domain.ActorRoleDispatcher,
		Permissions: map[string]struct{}{"resource.transfer_direct": {}, "request.read": {}},
	}))
	rec := httptest.NewRecorder()

	h.handleGetMe(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var response meResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.UserID != "user-1" || response.ActiveRole != domain.ActorRoleDispatcher {
		t.Fatalf("response identity = %+v", response)
	}
	if got, want := response.Permissions, []string{"request.read", "resource.transfer_direct"}; !equalStrings(got, want) {
		t.Fatalf("permissions = %v, want %v", got, want)
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
