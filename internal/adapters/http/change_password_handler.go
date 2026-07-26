package httpadapter

import (
	"fmt"
	"net/http"

	"bestelltool_be/internal/application/usecases"
)

type changePasswordPayload struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

func (h *handler) handleChangeOwnPassword(w http.ResponseWriter, r *http.Request) {
	if h.changeOwnPassword == nil {
		writeMappedError(w, fmt.Errorf("change own password use case missing"))
		return
	}
	principal, ok := PrincipalFromContext(r.Context())
	if !ok {
		writeMappedError(w, fmt.Errorf("principal not in context: programming error"))
		return
	}
	var payload changePasswordPayload
	if err := decodeJSONBody(r, &payload); err != nil {
		writeBadRequest(w)
		return
	}
	if err := h.changeOwnPassword.Execute(r.Context(), usecases.ChangeOwnPasswordInput{
		UserID:           principal.UserID,
		ActorRole:        principal.Role,
		CurrentSessionID: principal.SessionID,
		OldPassword:      payload.OldPassword,
		NewPassword:      payload.NewPassword,
	}); err != nil {
		writeMappedError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
