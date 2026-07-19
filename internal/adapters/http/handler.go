package httpadapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/application/usecases"
	"bestelltool_be/internal/domain"
)

// CreateRequestUseCase is the inbound port for creating requests.
type CreateRequestUseCase interface {
	Execute(ctx context.Context, in usecases.CreateRequestInput) (*domain.Request, error)
}

// GetRequestUseCase is the inbound port for loading requests.
type GetRequestUseCase interface {
	Execute(ctx context.Context, requestID domain.RequestID) (*domain.Request, error)
}

// RequestReturnUseCase is the inbound port for requesting allocation return.
type RequestReturnUseCase interface {
	Execute(ctx context.Context, in usecases.RequestReturnInput) error
}

type handler struct {
	createRequest CreateRequestUseCase
	getRequest    GetRequestUseCase
	requestReturn RequestReturnUseCase
	now           func() time.Time
}

type createRequestPayload struct {
	RequestID                string       `json:"request_id"`
	TechnicianID             string       `json:"technician_id"`
	ContextRef               string       `json:"context_ref"`
	ContextLabel             string       `json:"context_label"`
	WishFrom                 *time.Time   `json:"wish_from,omitzero"`
	WishUntil                *time.Time   `json:"wish_until,omitzero"`
	Note                     string       `json:"note"`
	RequestedResourceClasses []string     `json:"requested_resource_classes"`
	CreatedAt                *time.Time   `json:"created_at,omitzero"`
	Audit                    auditPayload `json:"audit"`
}

type requestReturnPayload struct {
	At    *time.Time   `json:"at,omitzero"`
	Audit auditPayload `json:"audit"`
}

type auditPayload struct {
	ActorID          string     `json:"actor_id"`
	ActorRole        string     `json:"actor_role"`
	ClientOccurredAt *time.Time `json:"client_occurred_at,omitzero"`
	ClientSeq        *int64     `json:"client_seq,omitzero"`
	Note             string     `json:"note"`
}

type requestResponse struct {
	ID                       string     `json:"id"`
	TechnicianID             string     `json:"technician_id"`
	Status                   string     `json:"status"`
	ExecutionState           string     `json:"execution_state"`
	ExecutionNote            string     `json:"execution_note"`
	ContextRef               string     `json:"context_ref"`
	ContextLabel             string     `json:"context_label"`
	WishFrom                 *time.Time `json:"wish_from,omitzero"`
	WishUntil                *time.Time `json:"wish_until,omitzero"`
	Note                     string     `json:"note"`
	RequestedResourceClasses []string   `json:"requested_resource_classes"`
	Version                  int64      `json:"version"`
	CreatedAt                time.Time  `json:"created_at"`
	UpdatedAt                time.Time  `json:"updated_at"`
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// NewHandler builds the HTTP adapter using Go 1.22 method-pattern routes.
func NewHandler(createRequest CreateRequestUseCase, getRequest GetRequestUseCase, requestReturn RequestReturnUseCase) http.Handler {
	return NewHandlerWithClock(createRequest, getRequest, requestReturn, time.Now)
}

// NewHandlerWithClock builds the HTTP adapter and allows deterministic tests.
func NewHandlerWithClock(
	createRequest CreateRequestUseCase,
	getRequest GetRequestUseCase,
	requestReturn RequestReturnUseCase,
	now func() time.Time,
) http.Handler {
	if now == nil {
		now = time.Now
	}

	h := &handler{
		createRequest: createRequest,
		getRequest:    getRequest,
		requestReturn: requestReturn,
		now:           now,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/requests", h.handleCreateRequest)
	mux.HandleFunc("GET /api/v1/requests/{id}", h.handleGetRequest)
	mux.HandleFunc("POST /api/v1/allocations/{id}/return-request", h.handleRequestReturn)

	return mux
}

func (h *handler) handleCreateRequest(w http.ResponseWriter, r *http.Request) {
	if h.createRequest == nil {
		writeMappedError(w, fmt.Errorf("create request use case missing"))
		return
	}

	var payload createRequestPayload
	if err := decodeJSONBody(r, &payload); err != nil {
		writeBadRequest(w)
		return
	}

	audit, err := parseAuditMeta(r, payload.Audit)
	if err != nil {
		writeMappedError(w, err)
		return
	}

	createdAt := h.now().UTC()
	if payload.CreatedAt != nil {
		createdAt = payload.CreatedAt.UTC()
	}

	in := usecases.CreateRequestInput{
		RequestID:                domain.RequestID(payload.RequestID),
		TechnicianID:             domain.UserID(payload.TechnicianID),
		ContextRef:               payload.ContextRef,
		ContextLabel:             payload.ContextLabel,
		WishFrom:                 payload.WishFrom,
		WishUntil:                payload.WishUntil,
		Note:                     payload.Note,
		RequestedResourceClasses: toResourceClassIDs(payload.RequestedResourceClasses),
		CreatedAt:                createdAt,
		Audit:                    audit,
	}

	req, err := h.createRequest.Execute(r.Context(), in)
	if err != nil {
		writeMappedError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, requestFromDomain(req))
}

func (h *handler) handleGetRequest(w http.ResponseWriter, r *http.Request) {
	if h.getRequest == nil {
		writeMappedError(w, fmt.Errorf("get request use case missing"))
		return
	}

	req, err := h.getRequest.Execute(r.Context(), domain.RequestID(r.PathValue("id")))
	if err != nil {
		writeMappedError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, requestFromDomain(req))
}

func (h *handler) handleRequestReturn(w http.ResponseWriter, r *http.Request) {
	if h.requestReturn == nil {
		writeMappedError(w, fmt.Errorf("request return use case missing"))
		return
	}

	var payload requestReturnPayload
	if err := decodeJSONBody(r, &payload); err != nil {
		writeBadRequest(w)
		return
	}

	audit, err := parseAuditMeta(r, payload.Audit)
	if err != nil {
		writeMappedError(w, err)
		return
	}

	at := h.now().UTC()
	if payload.At != nil {
		at = payload.At.UTC()
	}

	err = h.requestReturn.Execute(r.Context(), usecases.RequestReturnInput{
		AllocationID: domain.AllocationID(r.PathValue("id")),
		At:           at,
		Audit:        audit,
	})
	if err != nil {
		writeMappedError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func toResourceClassIDs(items []string) []domain.ResourceClassID {
	out := make([]domain.ResourceClassID, 0, len(items))
	for _, item := range items {
		out = append(out, domain.ResourceClassID(item))
	}
	return out
}

func requestFromDomain(req *domain.Request) requestResponse {
	if req == nil {
		return requestResponse{}
	}

	classes := make([]string, 0, len(req.RequestedResourceClasses))
	for _, classID := range req.RequestedResourceClasses {
		classes = append(classes, string(classID))
	}

	return requestResponse{
		ID:                       string(req.ID),
		TechnicianID:             string(req.TechnicianID),
		Status:                   string(req.Status),
		ExecutionState:           string(req.ExecutionState),
		ExecutionNote:            req.ExecutionNote,
		ContextRef:               req.ContextRef,
		ContextLabel:             req.ContextLabel,
		WishFrom:                 req.WishFrom,
		WishUntil:                req.WishUntil,
		Note:                     req.Note,
		RequestedResourceClasses: classes,
		Version:                  req.Version,
		CreatedAt:                req.CreatedAt,
		UpdatedAt:                req.UpdatedAt,
	}
}

func parseAuditMeta(r *http.Request, payload auditPayload) (usecases.AuditMeta, error) {
	role, err := parseActorRole(firstNonEmpty(r.Header.Get("X-Actor-Role"), payload.ActorRole))
	if err != nil {
		return usecases.AuditMeta{}, err
	}

	clientOccurredAt := payload.ClientOccurredAt
	if raw := r.Header.Get("X-Client-Occurred-At"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return usecases.AuditMeta{}, fmt.Errorf("audit client occurred at header: %w", domain.ErrInvalidTimeRange)
		}
		clientOccurredAt = new(parsed)
	}

	return usecases.AuditMeta{
		ActorID:          domain.UserID(firstNonEmpty(r.Header.Get("X-Actor-ID"), payload.ActorID)),
		ActorRole:        role,
		ClientOccurredAt: clientOccurredAt,
		ClientSeq:        payload.ClientSeq,
		Note:             payload.Note,
	}, nil
}

func parseActorRole(raw string) (domain.ActorRole, error) {
	role := domain.ActorRole(raw)
	switch role {
	case "", domain.ActorRoleTechnician, domain.ActorRoleDispatcher, domain.ActorRoleSystem:
		return role, nil
	default:
		return "", fmt.Errorf("audit actor role: %w", domain.ErrInvalidState)
	}
}

func firstNonEmpty(primary string, secondary string) string {
	if primary != "" {
		return primary
	}
	return secondary
}

func decodeJSONBody(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		return err
	}

	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("json trailing data")
	}

	return nil
}

func writeMappedError(w http.ResponseWriter, err error) {
	status, code, message := mapHTTPError(err)
	writeJSON(w, status, errorEnvelope{Error: errorBody{Code: code, Message: message}})
}

func writeBadRequest(w http.ResponseWriter) {
	writeJSON(w, http.StatusBadRequest, errorEnvelope{Error: errorBody{Code: "bad_request", Message: "invalid json body"}})
}

func mapHTTPError(err error) (int, string, string) {
	switch {
	case errors.Is(err, ports.ErrNotFound):
		return http.StatusNotFound, "not_found", "resource not found"
	case errors.Is(err, ports.ErrConflict):
		return http.StatusConflict, "conflict", "conflict"
	case errors.Is(err, ports.ErrValidation),
		errors.Is(err, domain.ErrRequiredField),
		errors.Is(err, domain.ErrInvalidState),
		errors.Is(err, domain.ErrInvalidTimeRange),
		errors.Is(err, domain.ErrReasonRequired),
		errors.Is(err, domain.ErrInvalidTransition),
		errors.Is(err, domain.ErrAlreadyCompleted):
		return http.StatusUnprocessableEntity, "validation_error", "validation failed"
	default:
		return http.StatusInternalServerError, "internal_error", "internal server error"
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
