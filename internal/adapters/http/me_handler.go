package httpadapter

import (
	"fmt"
	"maps"
	"net/http"
	"slices"

	"bestelltool_be/internal/application/usecases"
	"bestelltool_be/internal/domain"
)

type meResponse struct {
	UserID      domain.UserID      `json:"user_id"`
	ActiveRole  domain.ActorRole   `json:"active_role"`
	Roles       []domain.ActorRole `json:"roles"`
	Permissions []string           `json:"permissions"`
}

func (h *handler) handleGetMe(w http.ResponseWriter, r *http.Request) {
	if h.getMe == nil {
		writeMappedError(w, fmt.Errorf("get me use case missing"))
		return
	}
	principal, ok := PrincipalFromContext(r.Context())
	if !ok {
		writeMappedError(w, fmt.Errorf("principal not in context: programming error"))
		return
	}
	out, err := h.getMe.Execute(r.Context(), usecases.GetMeInput{UserID: principal.UserID})
	if err != nil {
		writeMappedError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, meResponse{
		UserID:      principal.UserID,
		ActiveRole:  principal.Role,
		Roles:       out.Roles,
		Permissions: slices.Sorted(maps.Keys(principal.Permissions)),
	})
}
