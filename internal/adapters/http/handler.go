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

// LoginUseCase is the inbound port for user login.
type LoginUseCase interface {
	Execute(ctx context.Context, in usecases.LoginInput) (*usecases.LoginOutput, error)
}

// RefreshSessionUseCase is the inbound port for refresh-token rotation.
type RefreshSessionUseCase interface {
	Execute(ctx context.Context, in usecases.RefreshSessionInput) (*usecases.RefreshSessionOutput, error)
}

// LogoutUseCase is the inbound port for session revocation.
type LogoutUseCase interface {
	Execute(ctx context.Context, in usecases.LogoutInput) error
}

// ChangeOwnPasswordUseCase is the inbound port for changing the authenticated
// user's password.
type ChangeOwnPasswordUseCase interface {
	Execute(ctx context.Context, in usecases.ChangeOwnPasswordInput) error
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

// TransferResourceUseCase is the inbound port for direct resource transfer.
type TransferResourceUseCase interface {
	Execute(ctx context.Context, in usecases.TransferResourceInput) error
}

// EventStream is the local narrow contract for SSE subscriptions.
type EventStream interface {
	Subscribe(principal ports.Principal) (<-chan ports.Event, func())
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
	login             LoginUseCase
	refreshSession    RefreshSessionUseCase
	logout            LogoutUseCase
	changeOwnPassword ChangeOwnPasswordUseCase
	rateLimiter       *RateLimiter
	createRequest     CreateRequestUseCase
	getRequest        GetRequestUseCase
	requestReturn     RequestReturnUseCase
	transferResource  TransferResourceUseCase
	eventStream       EventStream
	now               func() time.Time
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

type transferResourcePayload struct {
	OldAllocationID string       `json:"old_allocation_id"`
	NewAllocationID string       `json:"new_allocation_id"`
	TargetRequestID string       `json:"target_request_id"`
	PlannedFrom     time.Time    `json:"planned_from"`
	PlannedUntil    time.Time    `json:"planned_until"`
	At              *time.Time   `json:"at,omitzero"`
	Audit           auditPayload `json:"audit"`
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

type healthzResponse struct {
	Status string `json:"status"`
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
	transferResource TransferResourceUseCase,
) http.Handler {
	return NewHandlerWithClock(auth, createRequest, getRequest, requestReturn, transferResource, time.Now)
}

// NewHandlerWithEventStream builds the HTTP adapter and wires SSE streaming.
func NewHandlerWithEventStream(
	auth Authenticator,
	createRequest CreateRequestUseCase,
	getRequest GetRequestUseCase,
	requestReturn RequestReturnUseCase,
	transferResource TransferResourceUseCase,
	eventStream EventStream,
) http.Handler {
	return NewHandlerWithEventStreamAndClock(auth, createRequest, getRequest, requestReturn, transferResource, eventStream, time.Now)
}

// NewHandlerWithEventStreamAndLogin builds the HTTP adapter, wires SSE
// streaming and registers the public login route.
func NewHandlerWithEventStreamAndLogin(
	auth Authenticator,
	createRequest CreateRequestUseCase,
	getRequest GetRequestUseCase,
	requestReturn RequestReturnUseCase,
	transferResource TransferResourceUseCase,
	eventStream EventStream,
	login LoginUseCase,
) http.Handler {
	return NewHandlerWithEventStreamAndClockAndLogin(auth, createRequest, getRequest, requestReturn, transferResource, eventStream, time.Now, login)
}

// NewHandlerWithEventStreamAndAuthentication builds the HTTP adapter with all
// Phase-2 authentication endpoints wired.
func NewHandlerWithEventStreamAndAuthentication(
	auth Authenticator,
	createRequest CreateRequestUseCase,
	getRequest GetRequestUseCase,
	requestReturn RequestReturnUseCase,
	transferResource TransferResourceUseCase,
	eventStream EventStream,
	login LoginUseCase,
	refreshSession RefreshSessionUseCase,
	logout LogoutUseCase,
) http.Handler {
	return NewHandlerWithEventStreamAndClockAndAuthenticationAndSecurity(auth, createRequest, getRequest, requestReturn, transferResource, eventStream, time.Now, login, refreshSession, logout, nil, nil)
}

// NewHandlerWithEventStreamAndAuthenticationAndSecurity builds the HTTP
// adapter with Phase-2 authentication endpoints and rate limiting.
func NewHandlerWithEventStreamAndAuthenticationAndSecurity(
	auth Authenticator,
	createRequest CreateRequestUseCase,
	getRequest GetRequestUseCase,
	requestReturn RequestReturnUseCase,
	transferResource TransferResourceUseCase,
	eventStream EventStream,
	login LoginUseCase,
	refreshSession RefreshSessionUseCase,
	logout LogoutUseCase,
	changeOwnPassword ChangeOwnPasswordUseCase,
	rateLimiter *RateLimiter,
) http.Handler {
	return NewHandlerWithEventStreamAndClockAndAuthenticationAndSecurity(auth, createRequest, getRequest, requestReturn, transferResource, eventStream, time.Now, login, refreshSession, logout, changeOwnPassword, rateLimiter)
}

// NewHandlerWithClock builds the HTTP adapter and allows deterministic tests.
// All /api/v1/* routes are protected by the auth middleware followed by per-route
// role checks via requireRoles. Unprotected routes (e.g. GET /health) must be
// registered on the outer mux in main.go — they are intentionally outside this function.
func NewHandlerWithClock(
	auth Authenticator,
	createRequest CreateRequestUseCase,
	getRequest GetRequestUseCase,
	requestReturn RequestReturnUseCase,
	transferResource TransferResourceUseCase,
	now func() time.Time,
) http.Handler {
	return NewHandlerWithEventStreamAndClock(auth, createRequest, getRequest, requestReturn, transferResource, nil, now)
}

// NewHandlerWithEventStreamAndClock builds the HTTP adapter and allows deterministic tests.
func NewHandlerWithEventStreamAndClock(
	auth Authenticator,
	createRequest CreateRequestUseCase,
	getRequest GetRequestUseCase,
	requestReturn RequestReturnUseCase,
	transferResource TransferResourceUseCase,
	eventStream EventStream,
	now func() time.Time,
) http.Handler {
	return NewHandlerWithEventStreamAndClockAndLogin(auth, createRequest, getRequest, requestReturn, transferResource, eventStream, now, nil)
}

// NewHandlerWithEventStreamAndClockAndLogin builds the HTTP adapter, allows
// deterministic tests and wires the public login route.
func NewHandlerWithEventStreamAndClockAndLogin(
	auth Authenticator,
	createRequest CreateRequestUseCase,
	getRequest GetRequestUseCase,
	requestReturn RequestReturnUseCase,
	transferResource TransferResourceUseCase,
	eventStream EventStream,
	now func() time.Time,
	login LoginUseCase,
) http.Handler {
	return NewHandlerWithEventStreamAndClockAndAuthenticationAndSecurity(auth, createRequest, getRequest, requestReturn, transferResource, eventStream, now, login, nil, nil, nil, nil)
}

// NewHandlerWithEventStreamAndClockAndAuthentication builds the HTTP adapter
// with deterministic time and all Phase-2 authentication use cases.
func NewHandlerWithEventStreamAndClockAndAuthentication(
	auth Authenticator,
	createRequest CreateRequestUseCase,
	getRequest GetRequestUseCase,
	requestReturn RequestReturnUseCase,
	transferResource TransferResourceUseCase,
	eventStream EventStream,
	now func() time.Time,
	login LoginUseCase,
	refreshSession RefreshSessionUseCase,
	logout LogoutUseCase,
) http.Handler {
	return NewHandlerWithEventStreamAndClockAndAuthenticationAndSecurity(auth, createRequest, getRequest, requestReturn, transferResource, eventStream, now, login, refreshSession, logout, nil, nil)
}

// NewHandlerWithEventStreamAndClockAndAuthenticationAndSecurity builds the
// HTTP adapter with deterministic dependencies for all Phase-2 endpoints.
func NewHandlerWithEventStreamAndClockAndAuthenticationAndSecurity(
	auth Authenticator,
	createRequest CreateRequestUseCase,
	getRequest GetRequestUseCase,
	requestReturn RequestReturnUseCase,
	transferResource TransferResourceUseCase,
	eventStream EventStream,
	now func() time.Time,
	login LoginUseCase,
	refreshSession RefreshSessionUseCase,
	logout LogoutUseCase,
	changeOwnPassword ChangeOwnPasswordUseCase,
	rateLimiter *RateLimiter,
) http.Handler {
	if now == nil {
		now = time.Now
	}
	if rateLimiter == nil {
		rateLimiter = NewRateLimiter(10, time.Minute, false, now)
	}

	h := &handler{
		login:             login,
		refreshSession:    refreshSession,
		logout:            logout,
		changeOwnPassword: changeOwnPassword,
		rateLimiter:       rateLimiter,
		createRequest:     createRequest,
		getRequest:        getRequest,
		requestReturn:     requestReturn,
		transferResource:  transferResource,
		eventStream:       eventStream,
		now:               now,
	}

	// Inner mux: each route carries an explicit role allowlist via requireRoles.
	// Order: authMiddleware → requireRoles → handler.
	protected := http.NewServeMux()
	protected.Handle("POST /api/v1/requests",
		requireRoles(domain.ActorRoleTechnician)(
			http.HandlerFunc(h.handleCreateRequest),
		))
	protected.Handle("GET /api/v1/requests/{id}",
		requireRoles(domain.ActorRoleTechnician, domain.ActorRoleDispatcher, domain.ActorRoleAdmin)(
			http.HandlerFunc(h.handleGetRequest),
		))
	protected.Handle("POST /api/v1/allocations/{id}/return-request",
		requireRoles(domain.ActorRoleTechnician)(
			http.HandlerFunc(h.handleRequestReturn),
		))
	protected.Handle("POST /api/v1/resources/{id}/transfer",
		requireRoles(domain.ActorRoleDispatcher)(
			http.HandlerFunc(h.handleTransferResource),
		))
	protected.Handle("GET /api/v1/events",
		requireRoles(domain.ActorRoleDispatcher, domain.ActorRoleTechnician)(
			http.HandlerFunc(h.handleEvents),
		))
	protected.Handle("POST /api/v1/auth/logout",
		requireRoles(domain.ActorRoleTechnician, domain.ActorRoleDispatcher, domain.ActorRoleAdmin)(
			http.HandlerFunc(h.handleLogout),
		))
	protected.Handle("POST /api/v1/auth/password/change",
		requireRoles(domain.ActorRoleTechnician, domain.ActorRoleDispatcher, domain.ActorRoleAdmin)(
			http.HandlerFunc(h.handleChangeOwnPassword),
		))

	// Outer mux: public routes live directly on this mux; /api/v1/ stays
	// auth-guarded except explicit public endpoints.
	mux := http.NewServeMux()
	mux.Handle("POST /api/v1/auth/login", h.rateLimiter.limitLogin(http.HandlerFunc(h.handleLogin)))
	mux.Handle("POST /api/v1/auth/refresh", h.rateLimiter.limitRefresh(http.HandlerFunc(h.handleRefreshSession)))
	mux.HandleFunc("GET /healthz", h.handleHealthz)
	mux.Handle("/api/v1/", authMiddleware(auth, protected))

	return mux
}

// requireRoles returns a middleware that enforces one of the allowed roles on the
// authenticated Principal. MUST be applied after authMiddleware.
//
//   - Missing Principal → 500 (programming error — middleware chain incorrectly wired).
//   - Correct role → next handler is called with the unchanged context.
//   - Wrong role → 403 Forbidden.
func requireRoles(allowed ...domain.ActorRole) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := PrincipalFromContext(r.Context())
			if !ok {
				// Programming error: auth middleware was bypassed.
				writeMappedError(w, fmt.Errorf("principal not in context: programming error"))
				return
			}
			for _, role := range allowed {
				if p.Role == role {
					next.ServeHTTP(w, r)
					return
				}
			}
			writeMappedError(w, ports.ErrForbidden)
		})
	}
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

func (h *handler) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthzResponse{Status: "ok"})
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

func (h *handler) handleTransferResource(w http.ResponseWriter, r *http.Request) {
	if h.transferResource == nil {
		writeMappedError(w, fmt.Errorf("transfer resource use case missing"))
		return
	}

	resourceID := domain.ResourceID(r.PathValue("id"))
	if resourceID == "" {
		writeMappedError(w, fmt.Errorf("resource id: %w", domain.ErrRequiredField))
		return
	}

	var payload transferResourcePayload
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

	err = h.transferResource.Execute(r.Context(), usecases.TransferResourceInput{
		OldAllocationID: domain.AllocationID(payload.OldAllocationID),
		NewAllocationID: domain.AllocationID(payload.NewAllocationID),
		TargetRequestID: domain.RequestID(payload.TargetRequestID),
		PlannedFrom:     payload.PlannedFrom.UTC(),
		PlannedUntil:    payload.PlannedUntil.UTC(),
		At:              at,
		Audit:           audit,
	})
	if err != nil {
		writeMappedError(w, fmt.Errorf("transfer resource %s: %w", resourceID, err))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) handleEvents(w http.ResponseWriter, r *http.Request) {
	if h.eventStream == nil {
		writeMappedError(w, fmt.Errorf("event stream missing"))
		return
	}

	principal, ok := PrincipalFromContext(r.Context())
	if !ok {
		writeMappedError(w, fmt.Errorf("principal not in context: programming error"))
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeMappedError(w, fmt.Errorf("streaming unsupported"))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	events, unsubscribe := h.eventStream.Subscribe(*principal)
	defer unsubscribe()

	if _, err := io.WriteString(w, ": connected\n\n"); err != nil {
		return
	}
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case event, streamOpen := <-events:
			if !streamOpen {
				return
			}

			if err := writeSSEEvent(w, event); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func writeSSEEvent(w io.Writer, event ports.Event) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	if _, err := fmt.Fprintf(w, "event: %s\n", event.Type); err != nil {
		return fmt.Errorf("write event type: %w", err)
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
		return fmt.Errorf("write event payload: %w", err)
	}

	return nil
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
	case errors.Is(err, ports.ErrCredentialsInvalid):
		return http.StatusUnauthorized, "unauthenticated", "invalid credentials"
	case errors.Is(err, ports.ErrForbidden):
		return http.StatusForbidden, "forbidden", "forbidden"
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
	case errors.Is(err, ports.ErrThrottled):
		return http.StatusTooManyRequests, "throttled", "too many requests"
	default:
		return http.StatusInternalServerError, "internal_error", "internal server error"
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
