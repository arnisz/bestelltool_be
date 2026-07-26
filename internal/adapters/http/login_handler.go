package httpadapter

import (
	"errors"
	"log/slog"
	"net/http"

	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/application/usecases"
	"bestelltool_be/internal/domain"
)

type loginPayload struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func (h *handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	if h.login == nil {
		slog.Error("login use case not wired")
		writeJSON(w, http.StatusInternalServerError, errorEnvelope{Error: errorBody{Code: "internal_error", Message: "internal server error"}})
		return
	}

	var payload loginPayload
	if err := decodeJSONBody(r, &payload); err != nil {
		writeBadRequest(w)
		return
	}

	out, err := h.login.Execute(r.Context(), usecases.LoginInput{
		Username: payload.Username,
		Password: payload.Password,
	})
	if err != nil {
		writeLoginError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, loginResponse{
		AccessToken:  out.AccessToken,
		RefreshToken: out.RefreshToken,
	})
}

func writeLoginError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ports.ErrCredentialsInvalid):
		writeJSON(w, http.StatusUnauthorized, errorEnvelope{Error: errorBody{Code: "unauthorized", Message: "invalid credentials"}})
	case errors.Is(err, ports.ErrThrottled):
		writeJSON(w, http.StatusTooManyRequests, errorEnvelope{Error: errorBody{Code: "throttled", Message: "too many requests"}})
	case errors.Is(err, domain.ErrRequiredField):
		writeJSON(w, http.StatusBadRequest, errorEnvelope{Error: errorBody{Code: "bad_request", Message: "missing required field"}})
	default:
		slog.Error("login request failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorEnvelope{Error: errorBody{Code: "internal_error", Message: "internal server error"}})
	}
}
