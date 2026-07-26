package httpadapter

import (
	"fmt"
	"net/http"

	"bestelltool_be/internal/application/usecases"
)

func (h *handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	if h.logout == nil {
		writeMappedError(w, fmt.Errorf("logout use case missing"))
		return
	}
	principal, ok := PrincipalFromContext(r.Context())
	if !ok {
		writeMappedError(w, fmt.Errorf("principal not in context: programming error"))
		return
	}
	if err := h.logout.Execute(r.Context(), usecases.LogoutInput{SessionID: principal.SessionID, ActorID: principal.UserID, ActorRole: principal.Role}); err != nil {
		writeMappedError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
