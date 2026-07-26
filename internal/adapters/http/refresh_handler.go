package httpadapter

import (
	"errors"
	"log/slog"
	"net/http"

	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/application/usecases"
)

type refreshPayload struct {
	RefreshToken string `json:"refresh_token"`
}

type refreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func (h *handler) handleRefreshSession(w http.ResponseWriter, r *http.Request) {
	if h.refreshSession == nil {
		slog.Error("refresh session use case not wired")
		writeJSON(w, http.StatusInternalServerError, errorEnvelope{Error: errorBody{Code: "internal_error", Message: "internal server error"}})
		return
	}

	var payload refreshPayload
	if err := decodeJSONBody(r, &payload); err != nil {
		writeBadRequest(w)
		return
	}
	out, err := h.refreshSession.Execute(r.Context(), usecases.RefreshSessionInput{RefreshToken: payload.RefreshToken})
	if err != nil {
		switch {
		case errors.Is(err, ports.ErrTokenInvalid), errors.Is(err, ports.ErrTokenExpired), errors.Is(err, ports.ErrUnauthenticated):
			writeJSON(w, http.StatusUnauthorized, errorEnvelope{Error: errorBody{Code: "unauthorized", Message: "invalid refresh token"}})
		default:
			slog.Error("refresh session request failed", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorEnvelope{Error: errorBody{Code: "internal_error", Message: "internal server error"}})
		}
		return
	}
	writeJSON(w, http.StatusOK, refreshResponse{AccessToken: out.AccessToken, RefreshToken: out.RefreshToken})
}
