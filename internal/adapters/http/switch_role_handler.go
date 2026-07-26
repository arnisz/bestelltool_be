package httpadapter

import (
	"fmt"
	"net/http"

	"bestelltool_be/internal/application/usecases"
	"bestelltool_be/internal/domain"
)

type switchActiveRolePayload struct {
	ActiveRole domain.ActorRole `json:"active_role"`
}

type switchActiveRoleResponse struct {
	AccessToken  string           `json:"access_token"`
	RefreshToken string           `json:"refresh_token"`
	ActiveRole   domain.ActorRole `json:"active_role"`
}

func (h *handler) handleSwitchActiveRole(w http.ResponseWriter, r *http.Request) {
	if h.switchActiveRole == nil {
		writeMappedError(w, fmt.Errorf("switch active role use case missing"))
		return
	}
	principal, ok := PrincipalFromContext(r.Context())
	if !ok {
		writeMappedError(w, fmt.Errorf("principal not in context: programming error"))
		return
	}
	var payload switchActiveRolePayload
	if err := decodeJSONBody(r, &payload); err != nil {
		writeBadRequest(w)
		return
	}
	out, err := h.switchActiveRole.Execute(r.Context(), usecases.SwitchActiveRoleInput{
		UserID:           principal.UserID,
		CurrentSessionID: principal.SessionID,
		RequestedRole:    payload.ActiveRole,
	})
	if err != nil {
		writeMappedError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, switchActiveRoleResponse{AccessToken: out.AccessToken, RefreshToken: out.RefreshToken, ActiveRole: out.ActiveRole})
}
