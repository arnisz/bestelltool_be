package httpadapter_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	authadapter "bestelltool_be/internal/adapters/auth"
	httpadapter "bestelltool_be/internal/adapters/http"
	"bestelltool_be/internal/adapters/postgres"
	"bestelltool_be/internal/adapters/sse"
	"bestelltool_be/internal/application/usecases"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	e2eTechAToken      = "tok-tech-a"
	e2eTechBToken      = "tok-tech-b"
	e2eDispatcherToken = "tok-dispatcher"
)

func TestResourceLifecycleE2E(t *testing.T) {
	pool := e2eTestPool(t)
	seedCoreRefs(t, pool)

	uow := postgres.NewUnitOfWork(pool)
	requestRepo := postgres.NewRequestRepository(pool)
	eventStream := sse.NewBroker(0)

	authenticator, err := authadapter.ParseStaticTokens(strings.Join([]string{
		e2eTechAToken + ":tech-a:technician",
		e2eTechBToken + ":tech-b:technician",
		e2eDispatcherToken + ":dispatcher-1:dispatcher",
	}, ","))
	if err != nil {
		t.Fatalf("ParseStaticTokens() error = %v", err)
	}

	h := httpadapter.NewHandlerWithEventStream(
		authenticator,
		usecases.NewCreateRequestUseCaseWithPublisher(uow, eventStream),
		usecases.NewGetRequestUseCase(requestRepo),
		usecases.NewRequestReturnUseCaseWithPublisher(uow, eventStream),
		usecases.NewTransferResourceUseCaseWithPublisher(uow, eventStream),
		eventStream,
	)

	server := httptest.NewServer(h)
	t.Cleanup(server.Close)

	now := time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC)
	requestAID := "req-e2e-a"
	requestBID := "req-e2e-b"
	resourceID := "res-e2e-1"
	oldAllocID := "alloc-e2e-old"
	newAllocID := "alloc-e2e-new"

	// Schritt A: Techniker A legt einen Request an.
	createBody := map[string]any{
		"request_id":                 requestAID,
		"technician_id":              "tech-a",
		"context_ref":                "ctx-e2e-a",
		"context_label":              "E2E A",
		"note":                       "created in e2e",
		"requested_resource_classes": []string{"rc-1"},
		"created_at":                 now,
		"audit": map[string]any{
			"client_seq": int64(1),
			"note":       "create request",
		},
	}
	resp := doJSONRequest(t, server.Client(), http.MethodPost, server.URL+"/api/v1/requests", e2eTechAToken, createBody)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/v1/requests status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}
	var created struct {
		ID string `json:"id"`
	}
	decodeJSONResponse(t, resp, &created)
	if created.ID != requestAID {
		t.Fatalf("created request id = %q, want %q", created.ID, requestAID)
	}

	// Schritt B: Dispatcher ruft den Request ab.
	resp = doJSONRequest(t, server.Client(), http.MethodGet, server.URL+"/api/v1/requests/"+requestAID, e2eDispatcherToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/requests/{id} status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var fetched struct {
		ID           string `json:"id"`
		TechnicianID string `json:"technician_id"`
	}
	decodeJSONResponse(t, resp, &fetched)
	if fetched.ID != requestAID {
		t.Fatalf("fetched request id = %q, want %q", fetched.ID, requestAID)
	}
	if fetched.TechnicianID != "tech-a" {
		t.Fatalf("fetched technician_id = %q, want %q", fetched.TechnicianID, "tech-a")
	}

	// Schritt C: Es existiert aktuell kein separater HTTP-Endpunkt für reguläre Zuweisung.
	seedDirectTransferPreconditions(t, pool, requestAID, requestBID, resourceID, oldAllocID, now)

	// Schritt D: Dispatcher führt Direct Transfer durch.
	transferAt := now.Add(45 * time.Minute)
	transferBody := map[string]any{
		"old_allocation_id": oldAllocID,
		"new_allocation_id": newAllocID,
		"target_request_id": requestBID,
		"planned_from":      now.Add(1 * time.Hour),
		"planned_until":     now.Add(4 * time.Hour),
		"at":                transferAt,
		"audit": map[string]any{
			"client_seq": int64(2),
			"note":       "dispatcher transfer",
		},
	}
	resp = doJSONRequest(t, server.Client(), http.MethodPost, server.URL+"/api/v1/resources/"+resourceID+"/transfer", e2eDispatcherToken, transferBody)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("POST /api/v1/resources/{id}/transfer status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	assertAllocationStatus(t, pool, oldAllocID, "completed")
	assertAllocationStatus(t, pool, newAllocID, "allocated")
	assertResourceState(t, pool, resourceID, "reserved", "tech-b")

	markAllocationWithTechnician(t, pool, newAllocID, transferAt.Add(5*time.Minute))

	// Schritt E: Techniker B fordert Rückgabe an.
	returnBody := map[string]any{
		"at": transferAt.Add(10 * time.Minute),
		"audit": map[string]any{
			"client_seq": int64(3),
			"note":       "return request",
		},
	}
	resp = doJSONRequest(t, server.Client(), http.MethodPost, server.URL+"/api/v1/allocations/"+newAllocID+"/return-request", e2eTechBToken, returnBody)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("POST /api/v1/allocations/{id}/return-request status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	assertAllocationStatus(t, pool, newAllocID, "return_requested")

	// Schritt F: Techniker A versucht Direct Transfer und wird per Rolle abgewiesen.
	resp = doJSONRequest(t, server.Client(), http.MethodPost, server.URL+"/api/v1/resources/"+resourceID+"/transfer", e2eTechAToken, transferBody)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("POST /api/v1/resources/{id}/transfer as technician status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
	var forbidden errorEnvelope
	decodeJSONResponse(t, resp, &forbidden)
	if forbidden.Error.Code != "forbidden" {
		t.Fatalf("forbidden error code = %q, want %q", forbidden.Error.Code, "forbidden")
	}
}

type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func doJSONRequest(t *testing.T, client *http.Client, method string, url string, token string, body any) *http.Response {
	t.Helper()

	var payload []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}
		payload = b
	}

	req, err := http.NewRequestWithContext(t.Context(), method, url, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("http.NewRequestWithContext() error = %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("client.Do() error = %v", err)
	}
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})

	return resp
}

func decodeJSONResponse(t *testing.T, resp *http.Response, dst any) {
	t.Helper()
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		t.Fatalf("decode response body error = %v", err)
	}
}

func assertAllocationStatus(t *testing.T, pool *pgxpool.Pool, allocationID string, wantStatus string) {
	t.Helper()
	var got string
	if err := pool.QueryRow(t.Context(), `SELECT status FROM allocations WHERE id = $1`, allocationID).Scan(&got); err != nil {
		t.Fatalf("query allocation status %s error = %v", allocationID, err)
	}
	if got != wantStatus {
		t.Fatalf("allocation %s status = %q, want %q", allocationID, got, wantStatus)
	}
}

func assertResourceState(t *testing.T, pool *pgxpool.Pool, resourceID string, wantStatus string, wantHolderID string) {
	t.Helper()
	var gotStatus string
	var gotHolderID string
	if err := pool.QueryRow(t.Context(), `SELECT status, holder_id FROM resources WHERE id = $1`, resourceID).Scan(&gotStatus, &gotHolderID); err != nil {
		t.Fatalf("query resource state %s error = %v", resourceID, err)
	}
	if gotStatus != wantStatus {
		t.Fatalf("resource %s status = %q, want %q", resourceID, gotStatus, wantStatus)
	}
	if gotHolderID != wantHolderID {
		t.Fatalf("resource %s holder_id = %q, want %q", resourceID, gotHolderID, wantHolderID)
	}
}

func markAllocationWithTechnician(t *testing.T, pool *pgxpool.Pool, allocationID string, at time.Time) {
	t.Helper()

	_, err := pool.Exec(t.Context(), `
UPDATE allocations
SET status = 'with_technician',
	shipped_at = $2,
	received_at = $2,
	updated_at = $2,
	version = version + 1
WHERE id = $1
`, allocationID, at)
	if err != nil {
		t.Fatalf("mark allocation %s with_technician error = %v", allocationID, err)
	}
}

func e2eTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL nicht gesetzt: E2E-Tests werden übersprungen")
	}
	assertTestDatabaseURL(t, dbURL)
	resetSchema(t, dbURL)
	if err := postgres.RunEmbeddedMigrations(t.Context(), dbURL); err != nil {
		t.Fatalf("RunEmbeddedMigrations() error = %v", err)
	}

	pool, err := postgres.NewPool(t.Context(), dbURL)
	if err != nil {
		t.Fatalf("NewPool() error = %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
	})

	return pool
}

// resetSchema drops and recreates the public schema on a dedicated, short-lived
// connection - never on the pgxpool.Pool the test itself will use.
//
// Why not TRUNCATE (E2): audit_events.actor_id references users(id). Migration
// 000006 blocks UPDATE/DELETE/TRUNCATE on audit_events with a BEFORE trigger, so
// `TRUNCATE users CASCADE` would cascade into audit_events and fail the trigger,
// breaking test setup itself. A full schema reset sidesteps the append-only
// guard entirely instead of trying to punch a hole in it for tests.
//
// Why a dedicated connection (E1): pgx v5 caches prepared statement plans and
// type OIDs per connection. If the schema were rebuilt using a connection already
// pooled by pgxpool, subsequent queries on that same connection can fail with
// "cached plan must not change result type" or stale-OID errors. Resetting on a
// throwaway connection that is closed before the pgxpool.Pool is ever created
// avoids the problem by construction - the pool never sees a pre-reset plan.
func resetSchema(t *testing.T, dbURL string) {
	t.Helper()

	conn, err := pgx.Connect(t.Context(), dbURL)
	if err != nil {
		t.Fatalf("connect for schema reset error = %v", err)
	}
	defer conn.Close(t.Context())
	if _, err := conn.Exec(t.Context(), `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatalf("reset schema error = %v", err)
	}
}

func assertTestDatabaseURL(t *testing.T, dbURL string) {
	t.Helper()

	u, err := url.Parse(dbURL)
	if err != nil {
		t.Fatalf("TEST_DATABASE_URL parse error = %v", err)
	}

	dbName := strings.TrimPrefix(u.Path, "/")
	if dbName == "" {
		t.Fatalf("TEST_DATABASE_URL ohne Datenbankname: %q", dbURL)
	}
	if !strings.Contains(strings.ToLower(dbName), "test") {
		t.Fatalf("unsichere Testdatenbank %q: Datenbankname muss klar als Testdatenbank erkennbar sein", dbName)
	}
}

func seedCoreRefs(t *testing.T, q interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}) {
	t.Helper()

	if _, err := q.Exec(t.Context(), `INSERT INTO users(id, role, display_name) VALUES
('tech-a', 'technician', 'Technician A'),
('tech-b', 'technician', 'Technician B'),
('dispatcher-1', 'dispatcher', 'Dispatcher One')`); err != nil {
		t.Fatalf("seed users error = %v", err)
	}

	if _, err := q.Exec(t.Context(), `INSERT INTO resource_classes(id, name) VALUES ('rc-1', 'Resource Class 1')`); err != nil {
		t.Fatalf("seed resource classes error = %v", err)
	}
}

func seedDirectTransferPreconditions(
	t *testing.T,
	q interface {
		Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	},
	sourceRequestID string,
	targetRequestID string,
	resourceID string,
	oldAllocationID string,
	now time.Time,
) {
	t.Helper()

	if _, err := q.Exec(t.Context(), `
INSERT INTO requests(id, technician_id, status, execution_state, execution_note, context_ref, context_label, wish_from, wish_until, note, version, created_at, updated_at)
VALUES ($1, 'tech-b', 'open', 'executable', '', 'ctx-e2e-b', 'E2E B', NULL, NULL, '', 1, $2, $2)
`, targetRequestID, now.Add(1*time.Minute)); err != nil {
		t.Fatalf("insert target request %s error = %v", targetRequestID, err)
	}

	if _, err := q.Exec(t.Context(), `
INSERT INTO request_resource_classes(request_id, position, resource_class_id)
VALUES ($1, 0, 'rc-1')
`, targetRequestID); err != nil {
		t.Fatalf("insert target request_resource_class %s error = %v", targetRequestID, err)
	}

	if _, err := q.Exec(t.Context(), `
INSERT INTO resources(id, resource_class_id, serial_number, status, block_reason, block_note, holder_id, location, valid_until, metadata, version, created_at, updated_at)
VALUES ($1, 'rc-1', $2, 'in_use', NULL, '', 'tech-a', 'site-a', NULL, '{}'::jsonb, 1, $3, $3)
`, resourceID, "serial-"+resourceID, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("insert resource %s error = %v", resourceID, err)
	}

	if _, err := q.Exec(t.Context(), `
INSERT INTO allocations(id, request_id, resource_id, status, planned_from, planned_until, return_requested_at, shipped_at, received_at, version, created_at, updated_at)
VALUES ($1, $2, $3, 'with_technician', $4, $5, NULL, $4, $4, 2, $4, $4)
`, oldAllocationID, sourceRequestID, resourceID, now.Add(3*time.Minute), now.Add(3*time.Hour)); err != nil {
		t.Fatalf("insert old allocation %s error = %v", oldAllocationID, err)
	}
}
