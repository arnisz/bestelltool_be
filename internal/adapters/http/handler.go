package httpadapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/application/usecases"
	"bestelltool_be/internal/domain"
)

// Authenticator is the narrow local port for token verification.
type Authenticator interface {
	Authenticate(ctx context.Context, token string) (*ports.Principal, error)
}

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

// contextKeyType is an unexported type for context keys to avoid collisions.
type contextKeyType struct{}

var contextKey = contextKeyType{}

// PrincipalFromContext retrieves the authenticated principal from the context.
func PrincipalFromContext(ctx context.Context) (*ports.Principal, bool) {
	p, ok := ctx.Value(contextKey).(*ports.Principal)
	return p, ok
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

// auditPayload carries only client-side timing metadata.
// Actor identity fields (actor_id, actor_role) are intentionally absent — they are
// derived exclusively from the authenticated Principal in the request context.
// Any body field named actor_id or actor_role will be rejected with 400 by
// decodeJSONBody (DisallowUnknownFields).
type auditPayload struct {
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
func NewHandler(
	auth Authenticator,
	createRequest CreateRequestUseCase,
	getRequest GetRequestUseCase,
	requestReturn RequestReturnUseCase,
) http.Handler {
	return NewHandlerWithClock(auth, createRequest, getRequest, requestReturn, time.Now)
}

// NewHandlerWithClock builds the HTTP adapter and allows deterministic tests.
// All /api/v1/* routes are protected by the auth middleware.
// Unprotected routes (e.g. GET /health) must be registered on the outer mux in
// main.go after calling NewHandler — they are intentionally outside this function.
func NewHandlerWithClock(
	auth Authenticator,
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

	// Inner mux: specific method+path routes, always reached after auth.
	protected := http.NewServeMux()
	protected.HandleFunc("POST /api/v1/requests", h.handleCreateRequest)
	protected.HandleFunc("GET /api/v1/requests/{id}", h.handleGetRequest)
	protected.HandleFunc("POST /api/v1/allocations/{id}/return-request", h.handleRequestReturn)

	// Outer mux: /api/v1/ subtree is auth-guarded.
	// Add unprotected routes (health checks, etc.) directly to this mux in main.go.
	mux := http.NewServeMux()
	mux.Handle("/api/v1/", authMiddleware(auth, protected))

	return mux
}

// authMiddleware extracts and validates the Bearer token, stores the Principal in
// the request context, and rejects missing or invalid credentials with 401.
func authMiddleware(auth Authenticator, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const prefix = "Bearer "
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, prefix) {
			writeJSON(w, http.StatusUnauthorized, errorEnvelope{Error: errorBody{
				Code:    "unauthenticated",
				Message: "missing or invalid authorization header",
			}})
			return
		}
		token := strings.TrimPrefix(header, prefix)
		p, err := auth.Authenticate(r.Context(), token)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, errorEnvelope{Error: errorBody{
				Code:    "unauthenticated",
				Message: "invalid token",
			}})
			return
		}
		ctx := context.WithValue(r.Context(), contextKey, p)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
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

	audit, err := buildAuditMeta(r, payload.Audit)
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

	audit, err := buildAuditMeta(r, payload.Audit)
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

// buildAuditMeta constructs AuditMeta exclusively from the authenticated Principal
// in the request context, plus optional client-side timing metadata.
//
// Missing Principal → 500, NOT 401.
// A missing principal signals a programming error (middleware was bypassed) and
// must surface loudly, not silently pass as an auth error.
//
// X-Client-Occurred-At header and ClientOccurredAt body field are informational
// (offline client timestamp) and remain legitimate — they carry no identity.
func buildAuditMeta(r *http.Request, payload auditPayload) (usecases.AuditMeta, error) {
	p, ok := PrincipalFromContext(r.Context())
	if !ok {
		return usecases.AuditMeta{}, fmt.Errorf("principal not in context: programming error")
	}

	clientOccurredAt := payload.ClientOccurredAt
	if raw := r.Header.Get("X-Client-Occurred-At"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return usecases.AuditMeta{}, fmt.Errorf("client occurred at header: %w", domain.ErrInvalidTimeRange)
		}
		t := parsed
		clientOccurredAt = &t
	}

	return usecases.AuditMeta{
		ActorID:          p.UserID,
		ActorRole:        p.Role,
		ClientOccurredAt: clientOccurredAt,
		ClientSeq:        payload.ClientSeq,
		Note:             payload.Note,
	}, nil
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
	case errors.Is(err, ports.ErrUnauthenticated):
		return http.StatusUnauthorized, "unauthenticated", "invalid or missing credentials"
	case errors.Is(err, ports.ErrNotFound):
		return http.StatusNotFound, "not_found", "resource not found"
	case errors.Is(err, ports.ErrConflict),
		errors.Is(err, domain.ErrAlreadyCompleted):
		return http.StatusConflict, "conflict", "conflict"
	case errors.Is(err, ports.ErrValidation),
		errors.Is(err, domain.ErrRequiredField),
		errors.Is(err, domain.ErrInvalidState),
		errors.Is(err, domain.ErrInvalidTimeRange),
		errors.Is(err, domain.ErrReasonRequired),
		errors.Is(err, domain.ErrInvalidTransition):
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
