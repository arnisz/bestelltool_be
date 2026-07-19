package postgres

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/application/usecases"
	"bestelltool_be/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL nicht gesetzt: PostgreSQL-Integrationstests werden übersprungen")
	}
	assertTestDatabaseURL(t, dbURL)
	if err := RunEmbeddedMigrations(t.Context(), dbURL); err != nil {
		t.Fatalf("RunEmbeddedMigrations() error = %v", err)
	}
	pool, err := NewPool(t.Context(), dbURL)
	if err != nil {
		t.Fatalf("NewPool() error = %v", err)
	}
	truncateAll(t, pool)
	t.Cleanup(func() {
		pool.Close()
	})
	return pool
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

func truncateAll(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(t.Context(), `
TRUNCATE TABLE
    audit_events,
    idempotency_outcomes,
    allocations,
    request_resource_classes,
    requests,
    resources,
    resource_classes,
    users
RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("truncate tables error = %v", err)
	}
}

func seedCoreRefs(t *testing.T, q interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}) {
	t.Helper()
	if _, err := q.Exec(t.Context(), `INSERT INTO users(id, role, display_name) VALUES
('tech-1', 'technician', 'Tech One'),
('dispatcher-1', 'dispatcher', 'Dispatcher One')`); err != nil {
		t.Fatalf("seed users error = %v", err)
	}
	if _, err := q.Exec(t.Context(), `INSERT INTO resource_classes(id, name) VALUES ('rc-1', 'RC1'),('rc-2', 'RC2')`); err != nil {
		t.Fatalf("seed resource_classes error = %v", err)
	}
}

func insertRequest(t *testing.T, q interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}, id string, now time.Time) {
	t.Helper()
	if _, err := q.Exec(t.Context(), `
INSERT INTO requests(id, technician_id, status, execution_state, execution_note, context_ref, context_label, wish_from, wish_until, note, version, created_at, updated_at)
VALUES ($1, 'tech-1', 'open', 'executable', '', 'ctx', 'ctx', NULL, NULL, '', 1, $2, $2)
`, id, now); err != nil {
		t.Fatalf("insert request %s error = %v", id, err)
	}
}

func insertResource(t *testing.T, q interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}, id string, now time.Time) {
	t.Helper()
	if _, err := q.Exec(t.Context(), `
INSERT INTO resources(id, resource_class_id, serial_number, status, block_reason, block_note, holder_id, location, valid_until, metadata, version, created_at, updated_at)
VALUES ($1, 'rc-1', $2, 'available', NULL, '', NULL, '', NULL, '{}'::jsonb, 1, $3, $3)
`, id, "serial-"+id, now); err != nil {
		t.Fatalf("insert resource %s error = %v", id, err)
	}
}

func insertAllocation(t *testing.T, q interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}, id string, reqID string, resID string, now time.Time) {
	t.Helper()
	if _, err := q.Exec(t.Context(), `
INSERT INTO allocations(id, request_id, resource_id, status, planned_from, planned_until, return_requested_at, shipped_at, received_at, version, created_at, updated_at)
VALUES ($1, $2, $3, 'allocated', $4, $5, NULL, NULL, NULL, 1, $4, $4)
`, id, reqID, resID, now, now.Add(2*time.Hour)); err != nil {
		t.Fatalf("insert allocation %s error = %v", id, err)
	}
}

func TestUnitOfWorkCommitAndRollback(t *testing.T) {
	pool := testPool(t)
	truncateAll(t, pool)
	seedCoreRefs(t, pool)
	uow := NewUnitOfWork(pool)
	now := time.Date(2026, 7, 18, 9, 0, 0, 0, time.UTC)
	insertRequest(t, pool, "req-uow", now)

	err := uow.WithinTransaction(t.Context(), func(ctx context.Context, tx ports.Transaction) error {
		if _, err := tx.Requests().GetForUpdate(ctx, "req-uow"); err != nil {
			return err
		}
		req, err := tx.Requests().GetForUpdate(ctx, "req-uow")
		if err != nil {
			return err
		}
		if err := req.UpdateExecutionState(domain.ExecutionStateBlocked, "blocked now"); err != nil {
			return err
		}
		if err := tx.Requests().Save(ctx, req); err != nil {
			return err
		}
		if err := tx.Audits().Write(ctx, domain.AuditEvent{
			ID:         "ae-uow-1",
			ActorID:    "dispatcher-1",
			ActorRole:  domain.ActorRoleDispatcher,
			EntityType: domain.EntityTypeRequest,
			EntityID:   "req-uow",
			Action:     "uow_commit",
			FromStatus: "executable",
			ToStatus:   "blocked",
		}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithinTransaction() commit path error = %v", err)
	}

	var state string
	if err := pool.QueryRow(t.Context(), `SELECT execution_state FROM requests WHERE id='req-uow'`).Scan(&state); err != nil {
		t.Fatalf("scan execution_state error = %v", err)
	}
	if state != "blocked" {
		t.Fatalf("execution_state = %s, want blocked", state)
	}

	var auditCount int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM audit_events WHERE id='ae-uow-1'`).Scan(&auditCount); err != nil {
		t.Fatalf("scan audit count error = %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("audit count = %d, want 1", auditCount)
	}

	err = uow.WithinTransaction(t.Context(), func(ctx context.Context, tx ports.Transaction) error {
		req, err := tx.Requests().GetForUpdate(ctx, "req-uow")
		if err != nil {
			return err
		}
		if err := req.UpdateExecutionState(domain.ExecutionStatePartiallyBlocked, "still blocked"); err != nil {
			return err
		}
		if err := tx.Requests().Save(ctx, req); err != nil {
			return err
		}
		if err := tx.Audits().Write(ctx, domain.AuditEvent{
			ID:         "ae-uow-rollback",
			ActorID:    "dispatcher-1",
			ActorRole:  domain.ActorRoleDispatcher,
			EntityType: domain.EntityTypeRequest,
			EntityID:   "req-uow",
			Action:     "uow_rollback",
			FromStatus: "blocked",
			ToStatus:   "partially_blocked",
		}); err != nil {
			return err
		}
		return errors.New("force rollback")
	})
	if err == nil {
		t.Fatalf("WithinTransaction() rollback path expected error")
	}

	if err := pool.QueryRow(t.Context(), `SELECT execution_state FROM requests WHERE id='req-uow'`).Scan(&state); err != nil {
		t.Fatalf("scan execution_state after rollback error = %v", err)
	}
	if state != "blocked" {
		t.Fatalf("execution_state after rollback = %s, want blocked", state)
	}

	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM audit_events WHERE id='ae-uow-rollback'`).Scan(&auditCount); err != nil {
		t.Fatalf("scan rollback audit count error = %v", err)
	}
	if auditCount != 0 {
		t.Fatalf("rollback audit count = %d, want 0", auditCount)
	}
}

func TestUnitOfWorkRollbackWhenAuditWriteFails(t *testing.T) {
	pool := testPool(t)
	truncateAll(t, pool)
	seedCoreRefs(t, pool)
	uow := NewUnitOfWork(pool)
	now := time.Date(2026, 7, 18, 9, 0, 0, 0, time.UTC)
	insertRequest(t, pool, "req-audit-fail", now)

	err := uow.WithinTransaction(t.Context(), func(ctx context.Context, tx ports.Transaction) error {
		req, err := tx.Requests().GetForUpdate(ctx, "req-audit-fail")
		if err != nil {
			return err
		}
		if err := req.UpdateExecutionState(domain.ExecutionStateBlocked, "will rollback"); err != nil {
			return err
		}
		if err := tx.Requests().Save(ctx, req); err != nil {
			return err
		}
		return tx.Audits().Write(ctx, domain.AuditEvent{
			ID:         "ae-invalid-role",
			ActorID:    "dispatcher-1",
			ActorRole:  domain.ActorRole("invalid-role"),
			EntityType: domain.EntityTypeRequest,
			EntityID:   "req-audit-fail",
			Action:     "broken_audit",
		})
	})
	if err == nil {
		t.Fatalf("WithinTransaction() expected audit error")
	}
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("WithinTransaction() error = %v, want validation", err)
	}

	var state string
	if err := pool.QueryRow(t.Context(), `SELECT execution_state FROM requests WHERE id='req-audit-fail'`).Scan(&state); err != nil {
		t.Fatalf("scan execution_state error = %v", err)
	}
	if state != "executable" {
		t.Fatalf("execution_state = %s, want executable", state)
	}
}

func TestIdempotencyOutcomeReplay(t *testing.T) {
	pool := testPool(t)
	truncateAll(t, pool)
	seedCoreRefs(t, pool)
	store := &idempotencyStore{q: pool}
	res := ports.IdempotencyResult{StatusCode: 409, Payload: []byte(`{"ok":false}`), ErrorText: "conflict"}

	if err := store.Save(t.Context(), "action-1", res); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := store.Get(t.Context(), "action-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got == nil || got.StatusCode != 409 || string(got.Payload) != string(res.Payload) || got.ErrorText != res.ErrorText {
		t.Fatalf("Get() = %+v, want %+v", got, res)
	}
	if err := store.Save(t.Context(), "action-1", res); err != nil {
		t.Fatalf("Save() duplicate error = %v", err)
	}

	var count int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM idempotency_outcomes WHERE client_action_id='action-1'`).Scan(&count); err != nil {
		t.Fatalf("count idempotency rows error = %v", err)
	}
	if count != 1 {
		t.Fatalf("idempotency row count = %d, want 1", count)
	}

	var wg sync.WaitGroup
	for i := range 2 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = store.Save(t.Context(), "action-par", ports.IdempotencyResult{StatusCode: 200, Payload: []byte(`{"idx":1}`), ErrorText: ""})
		}(i)
	}
	wg.Wait()
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM idempotency_outcomes WHERE client_action_id='action-par'`).Scan(&count); err != nil {
		t.Fatalf("count parallel idempotency rows error = %v", err)
	}
	if count != 1 {
		t.Fatalf("parallel idempotency row count = %d, want 1", count)
	}
}

func TestRequestRepositoryRoundTripAndConflicts(t *testing.T) {
	pool := testPool(t)
	truncateAll(t, pool)
	seedCoreRefs(t, pool)
	now := time.Date(2026, 7, 18, 9, 0, 0, 0, time.UTC)
	insertRequest(t, pool, "req-repo", now)
	if _, err := pool.Exec(t.Context(), `
INSERT INTO request_resource_classes(request_id, position, resource_class_id)
VALUES ('req-repo',0,'rc-1'),('req-repo',1,'rc-2'),('req-repo',2,'rc-1')`); err != nil {
		t.Fatalf("insert request_resource_classes error = %v", err)
	}

	repo := &requestRepository{q: pool}
	req, err := repo.GetByID(t.Context(), "req-repo")
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if len(req.RequestedResourceClasses) != 3 || req.RequestedResourceClasses[0] != "rc-1" || req.RequestedResourceClasses[1] != "rc-2" || req.RequestedResourceClasses[2] != "rc-1" {
		t.Fatalf("RequestedResourceClasses = %#v, want [rc-1 rc-2 rc-1]", req.RequestedResourceClasses)
	}
	if err := req.UpdateExecutionState(domain.ExecutionStateBlocked, "needs part"); err != nil {
		t.Fatalf("UpdateExecutionState() error = %v", err)
	}
	req.RequestedResourceClasses = []domain.ResourceClassID{"rc-2", "rc-2", "rc-1"}
	if err := repo.Save(t.Context(), req); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	stored, err := repo.GetByID(t.Context(), "req-repo")
	if err != nil {
		t.Fatalf("GetByID() after save error = %v", err)
	}
	if stored.Version != 2 {
		t.Fatalf("Version = %d, want 2", stored.Version)
	}
	if len(stored.RequestedResourceClasses) != 3 || stored.RequestedResourceClasses[0] != "rc-2" || stored.RequestedResourceClasses[1] != "rc-2" || stored.RequestedResourceClasses[2] != "rc-1" {
		t.Fatalf("RequestedResourceClasses after save = %#v, want [rc-2 rc-2 rc-1]", stored.RequestedResourceClasses)
	}

	_, err = repo.GetByID(t.Context(), "missing-request")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetByID(missing) error = %v, want not found", err)
	}

	beforeState := stored.ExecutionState
	conflict := *stored
	conflict.Version = 3
	if err := repo.Save(t.Context(), &conflict); !errors.Is(err, ErrConflict) {
		t.Fatalf("Save(conflict) error = %v, want conflict", err)
	}
	afterConflict, err := repo.GetByID(t.Context(), "req-repo")
	if err != nil {
		t.Fatalf("GetByID() after conflict error = %v", err)
	}
	if afterConflict.ExecutionState != beforeState || afterConflict.Version != 2 {
		t.Fatalf("state/version changed after conflict: %s/%d", afterConflict.ExecutionState, afterConflict.Version)
	}
}

func TestResourceRepositoryRoundTripAndLocking(t *testing.T) {
	pool := testPool(t)
	truncateAll(t, pool)
	seedCoreRefs(t, pool)
	now := time.Date(2026, 7, 18, 9, 0, 0, 0, time.UTC)
	insertResource(t, pool, "res-repo", now)
	repo := &resourceRepository{q: pool}

	res, err := repo.GetByID(t.Context(), "res-repo")
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	validUntil := now.Add(24 * time.Hour)
	blockReason := domain.BlockReasonMaintenance
	holder := domain.UserID("tech-1")
	res.Status = domain.ResourceStatusBlocked
	res.BlockReason = &blockReason
	res.BlockNote = "maint"
	res.HolderID = &holder
	res.Location = "shelf-1"
	res.ValidUntil = &validUntil
	res.Metadata = map[string]any{"k": "v", "n": float64(1)}
	res.Version = 2
	if err := repo.Save(t.Context(), res); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	stored, err := repo.GetByID(t.Context(), "res-repo")
	if err != nil {
		t.Fatalf("GetByID() after save error = %v", err)
	}
	if stored.Version != 2 || stored.BlockReason == nil || *stored.BlockReason != domain.BlockReasonMaintenance || stored.ValidUntil == nil || !stored.ValidUntil.Equal(validUntil) {
		t.Fatalf("stored resource mismatch: %+v", stored)
	}

	_, err = repo.GetByID(t.Context(), "res-missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetByID(missing) error = %v, want not found", err)
	}

	conflict := *stored
	conflict.Version = 4
	if err := repo.Save(t.Context(), &conflict); !errors.Is(err, ErrConflict) {
		t.Fatalf("Save(conflict) error = %v, want conflict", err)
	}

	invalid := *stored
	invalid.Version = 3
	invalid.Status = domain.ResourceStatusBlocked
	invalid.BlockReason = nil
	if err := repo.Save(t.Context(), &invalid); !errors.Is(err, ErrValidation) {
		t.Fatalf("Save(invalid block reason) error = %v, want validation", err)
	}

	txA, err := pool.BeginTx(t.Context(), pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx A error = %v", err)
	}
	t.Cleanup(func() { _ = txA.Rollback(t.Context()) })
	if _, err := (&resourceRepository{q: txA}).GetForUpdate(t.Context(), "res-repo"); err != nil {
		t.Fatalf("GetForUpdate A error = %v", err)
	}

	txB, err := pool.BeginTx(t.Context(), pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx B error = %v", err)
	}
	t.Cleanup(func() { _ = txB.Rollback(t.Context()) })

	blockedCtx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	err = txB.QueryRow(blockedCtx, `SELECT id FROM resources WHERE id = $1 FOR UPDATE`, "res-repo").Scan(new(string))
	if err == nil {
		t.Fatalf("expected lock timeout for txB")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("txB lock error = %v, want deadline exceeded", err)
	}

	if err := txA.Commit(t.Context()); err != nil {
		t.Fatalf("Commit A error = %v", err)
	}

	// txB's connection was poisoned by the context cancellation (pgx marks it closed).
	// Open a fresh transaction to verify the lock is now acquirable.
	txC, err := pool.BeginTx(t.Context(), pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx C error = %v", err)
	}
	t.Cleanup(func() { _ = txC.Rollback(t.Context()) })

	unlockCtx, unlockCancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer unlockCancel()
	var id string
	if err := txC.QueryRow(unlockCtx, `SELECT id FROM resources WHERE id = $1 FOR UPDATE`, "res-repo").Scan(&id); err != nil {
		t.Fatalf("txC lock after release error = %v", err)
	}
}

func TestAllocationRepositoryRoundTripAndUniqueActive(t *testing.T) {
	pool := testPool(t)
	truncateAll(t, pool)
	seedCoreRefs(t, pool)
	now := time.Date(2026, 7, 18, 9, 0, 0, 0, time.UTC)
	insertRequest(t, pool, "req-alloc", now)
	insertResource(t, pool, "res-alloc", now)
	insertAllocation(t, pool, "alloc-1", "req-alloc", "res-alloc", now)
	repo := &allocationRepository{q: pool}

	a, err := repo.GetByID(t.Context(), "alloc-1")
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	shippedAt := now.Add(30 * time.Minute)
	a.Status = domain.AllocationStatusShippedBack
	a.ShippedAt = &shippedAt
	a.Version = 2
	a.UpdatedAt = now.Add(10 * time.Minute)
	if err := repo.Save(t.Context(), a); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	stored, err := repo.GetByID(t.Context(), "alloc-1")
	if err != nil {
		t.Fatalf("GetByID() after save error = %v", err)
	}
	if stored.Version != 2 || stored.ShippedAt == nil || !stored.ShippedAt.Equal(shippedAt) {
		t.Fatalf("allocation mismatch after save: %+v", stored)
	}

	_, err = repo.GetByID(t.Context(), "alloc-missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetByID(missing) error = %v, want not found", err)
	}

	conflict := *stored
	conflict.Version = 5
	if err := repo.Save(t.Context(), &conflict); !errors.Is(err, ErrConflict) {
		t.Fatalf("Save(conflict) error = %v, want conflict", err)
	}

	insertRequest(t, pool, "req-alloc-2", now)
	// Insert alloc-2 as 'completed' (terminal, not in uq_allocations_single_active_resource).
	// alloc-1 is still 'shipped_back' (active per constraint), so inserting as any active
	// status would fire the unique index at INSERT time before repo.Save can map it to
	// ErrConflict. Starting terminal lets us test the conflict path via repo.Save instead.
	if _, err := pool.Exec(t.Context(), `
INSERT INTO allocations(id, request_id, resource_id, status, planned_from, planned_until,
                        return_requested_at, shipped_at, received_at, version, created_at, updated_at)
VALUES ($1, $2, $3, 'completed', $4, $5, NULL, NULL, NULL, 1, $4, $4)
`, "alloc-2", "req-alloc-2", "res-alloc", now, now.Add(2*time.Hour)); err != nil {
		t.Fatalf("insert alloc-2 as completed error = %v", err)
	}
	dup, err := repo.GetByID(t.Context(), "alloc-2")
	if err != nil {
		t.Fatalf("GetByID alloc-2 error = %v", err)
	}
	// Try to activate alloc-2 while alloc-1 is still active (shipped_back) → expect ErrConflict.
	dup.Status = domain.AllocationStatusAllocated
	dup.Version = 2
	dup.UpdatedAt = now.Add(20 * time.Minute)
	if err := repo.Save(t.Context(), dup); !errors.Is(err, ErrConflict) {
		t.Fatalf("Save(active duplicate) error = %v, want conflict", err)
	}

	a.Status = domain.AllocationStatusCompleted
	a.Version = 3
	a.UpdatedAt = now.Add(40 * time.Minute)
	if err := repo.Save(t.Context(), a); err != nil {
		t.Fatalf("Save(completed alloc-1) error = %v", err)
	}
	// alloc-1 is now terminal; the failed conflict-save left alloc-2 at version=1 in DB.
	// Activate alloc-2 (completed → allocated, then ships) — should now succeed.
	dup.Status = domain.AllocationStatusAllocated
	dup.Version = 2
	dup.UpdatedAt = now.Add(50 * time.Minute)
	if err := repo.Save(t.Context(), dup); err != nil {
		t.Fatalf("Save(active alloc-2 after completion) error = %v", err)
	}
}

func TestAuditWriterAndAppendOnlyTrigger(t *testing.T) {
	pool := testPool(t)
	truncateAll(t, pool)
	seedCoreRefs(t, pool)
	writer := &auditWriter{q: pool}
	clientSeq := int64(7)
	clientTime := time.Date(2026, 7, 18, 7, 0, 0, 0, time.UTC)

	if err := writer.Write(t.Context(), domain.AuditEvent{
		ID:               "ae-1",
		ClientOccurredAt: &clientTime,
		ClientSeq:        &clientSeq,
		ActorID:          "dispatcher-1",
		ActorRole:        domain.ActorRoleDispatcher,
		EntityType:       domain.EntityTypeRequest,
		EntityID:         "x",
		Action:           "changed",
		FromStatus:       "open",
		ToStatus:         "blocked",
		Note:             "ok",
	}); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	var actorRole string
	var serverRecordedAt time.Time
	if err := pool.QueryRow(t.Context(), `SELECT actor_role, server_recorded_at FROM audit_events WHERE id='ae-1'`).Scan(&actorRole, &serverRecordedAt); err != nil {
		t.Fatalf("scan audit event error = %v", err)
	}
	if actorRole != "dispatcher" {
		t.Fatalf("actor_role = %s, want dispatcher", actorRole)
	}
	if serverRecordedAt.Equal(clientTime) {
		t.Fatalf("server_recorded_at should be database controlled, got %v", serverRecordedAt)
	}

	if _, err := pool.Exec(t.Context(), `UPDATE audit_events SET note='changed' WHERE id='ae-1'`); err == nil {
		t.Fatalf("UPDATE audit_events expected append-only error")
	}
	if _, err := pool.Exec(t.Context(), `DELETE FROM audit_events WHERE id='ae-1'`); err == nil {
		t.Fatalf("DELETE audit_events expected append-only error")
	}
}

func TestUseCaseWithPostgresUoW(t *testing.T) {
	pool := testPool(t)
	uow := NewUnitOfWork(pool)
	truncateAll(t, pool)

	seedCoreRefs(t, pool)
	createdAt := time.Date(2026, 7, 18, 9, 0, 0, 0, time.UTC)
	insertRequest(t, pool, "req-1", createdAt)
	if _, err := pool.Exec(t.Context(), `
INSERT INTO request_resource_classes(request_id, position, resource_class_id) VALUES ('req-1', 0, 'rc-1')
`); err != nil {
		t.Fatalf("insert request_resource_classes error = %v", err)
	}

	uc := usecases.NewUpdateExecutionStateUseCase(uow)
	err := uc.Execute(t.Context(), usecases.UpdateExecutionStateInput{
		RequestID: "req-1",
		State:     domain.ExecutionStateBlocked,
		Note:      "x",
		Audit: usecases.AuditMeta{
			ActorID:   "dispatcher-1",
			ActorRole: domain.ActorRoleDispatcher,
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	row := pool.QueryRow(t.Context(), `SELECT execution_state, version FROM requests WHERE id = 'req-1'`)
	var state string
	var version int64
	if err := row.Scan(&state, &version); err != nil {
		t.Fatalf("scan request state error = %v", err)
	}
	if state != string(domain.ExecutionStateBlocked) || version != 2 {
		t.Fatalf("state/version = %s/%d, want blocked/2", state, version)
	}

	var fromStatus, toStatus string
	if err := pool.QueryRow(t.Context(), `SELECT from_status, to_status FROM audit_events WHERE entity_type='request' AND entity_id='req-1'`).Scan(&fromStatus, &toStatus); err != nil {
		t.Fatalf("scan audit transition error = %v", err)
	}
	if fromStatus != string(domain.ExecutionStateExecutable) || toStatus != string(domain.ExecutionStateBlocked) {
		t.Fatalf("audit transition = %s -> %s, want executable -> blocked", fromStatus, toStatus)
	}

	err = uc.Execute(t.Context(), usecases.UpdateExecutionStateInput{
		RequestID: "req-1",
		State:     domain.ExecutionStatePartiallyBlocked,
		Note:      "rollback",
		Audit: usecases.AuditMeta{
			ActorID:   "missing-user",
			ActorRole: domain.ActorRoleDispatcher,
		},
	})
	if err == nil {
		t.Fatalf("Execute() expected audit FK failure")
	}

	if err := pool.QueryRow(t.Context(), `SELECT execution_state, version FROM requests WHERE id = 'req-1'`).Scan(&state, &version); err != nil {
		t.Fatalf("scan request state after rollback error = %v", err)
	}
	if state != string(domain.ExecutionStateBlocked) || version != 2 {
		t.Fatalf("state/version after rollback = %s/%d, want blocked/2", state, version)
	}
}

// TestTransferResourceWithPostgres verifies the full direct-transfer use case against
// a real PostgreSQL database. Critical invariant: old allocation is saved as completed
// BEFORE the new allocation is created, so the unique partial index never sees two
// active allocations for the same resource in the same transaction.
func TestTransferResourceWithPostgres(t *testing.T) {
	pool := testPool(t)
	uow := NewUnitOfWork(pool)
	truncateAll(t, pool)
	seedCoreRefs(t, pool)
	now := time.Date(2026, 7, 18, 9, 0, 0, 0, time.UTC)

	// Seed target technician (not in seedCoreRefs)
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO users(id, role, display_name) VALUES ('tech-2', 'technician', 'Tech Two')`); err != nil {
		t.Fatalf("seed tech-2 error = %v", err)
	}

	// Source request (tech-1) and target request (tech-2)
	insertRequest(t, pool, "req-src", now)
	if _, err := pool.Exec(t.Context(), `
INSERT INTO requests(id, technician_id, status, execution_state, execution_note,
    context_ref, context_label, wish_from, wish_until, note, version, created_at, updated_at)
VALUES ('req-tgt', 'tech-2', 'open', 'executable', '', 'ctx', 'ctx', NULL, NULL, '', 1, $1, $1)`,
		now); err != nil {
		t.Fatalf("insert req-tgt error = %v", err)
	}

	// Resource in in_use state (version 4 = new+reserve+issue+in_use)
	if _, err := pool.Exec(t.Context(), `
INSERT INTO resources(id, resource_class_id, serial_number, status, block_reason, block_note,
    holder_id, location, valid_until, metadata, version, created_at, updated_at)
VALUES ('res-transfer', 'rc-1', 'serial-transfer', 'in_use', NULL, '',
    'tech-1', '', NULL, '{}'::jsonb, 4, $1, $1)`,
		now); err != nil {
		t.Fatalf("insert resource error = %v", err)
	}

	// Old allocation in with_technician state (version 3 = alloc+ship+receive)
	shippedAt := now.Add(10 * time.Minute)
	receivedAt := now.Add(20 * time.Minute)
	if _, err := pool.Exec(t.Context(), `
INSERT INTO allocations(id, request_id, resource_id, status, planned_from, planned_until,
    return_requested_at, shipped_at, received_at, version, created_at, updated_at)
VALUES ('alloc-src', 'req-src', 'res-transfer', 'with_technician',
    $1, $2, NULL, $3, $4, 3, $1, $4)`,
		now, now.Add(2*time.Hour), shippedAt, receivedAt); err != nil {
		t.Fatalf("insert alloc-src error = %v", err)
	}

	at := now.Add(30 * time.Minute)
	err := usecases.NewTransferResourceUseCase(uow).Execute(t.Context(), usecases.TransferResourceInput{
		OldAllocationID: "alloc-src",
		NewAllocationID: "alloc-tgt",
		TargetRequestID: "req-tgt",
		PlannedFrom:     at,
		PlannedUntil:    at.Add(2 * time.Hour),
		At:              at,
		Audit: usecases.AuditMeta{
			ActorID:   "dispatcher-1",
			ActorRole: domain.ActorRoleDispatcher,
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Old allocation must be completed
	var oldStatus string
	if err := pool.QueryRow(t.Context(),
		`SELECT status FROM allocations WHERE id = 'alloc-src'`).Scan(&oldStatus); err != nil {
		t.Fatalf("scan old alloc status error = %v", err)
	}
	if oldStatus != "completed" {
		t.Fatalf("old alloc status = %s, want completed", oldStatus)
	}

	// New allocation must exist and be active (allocated)
	var newStatus, newReqID, newResID string
	if err := pool.QueryRow(t.Context(),
		`SELECT status, request_id, resource_id FROM allocations WHERE id = 'alloc-tgt'`).
		Scan(&newStatus, &newReqID, &newResID); err != nil {
		t.Fatalf("scan new alloc error = %v", err)
	}
	if newStatus != "allocated" || newReqID != "req-tgt" || newResID != "res-transfer" {
		t.Fatalf("new alloc = {%s,%s,%s}, want {allocated,req-tgt,res-transfer}",
			newStatus, newReqID, newResID)
	}

	// Resource must be reserved with tech-2 as holder
	var resStatus, resHolder string
	if err := pool.QueryRow(t.Context(),
		`SELECT status, holder_id FROM resources WHERE id = 'res-transfer'`).
		Scan(&resStatus, &resHolder); err != nil {
		t.Fatalf("scan resource error = %v", err)
	}
	if resStatus != "reserved" || resHolder != "tech-2" {
		t.Fatalf("resource = {%s,%s}, want {reserved,tech-2}", resStatus, resHolder)
	}

	// Exactly 2 audit events for the 2 allocation operations
	var auditCount int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM audit_events WHERE entity_type = 'allocation'`).
		Scan(&auditCount); err != nil {
		t.Fatalf("scan audit count error = %v", err)
	}
	if auditCount != 2 {
		t.Fatalf("audit events = %d, want 2", auditCount)
	}
}

// TestTransferResourceConflictNewAllocWhileOldActive verifies that trying to create
// a new active allocation for a resource that already has an active allocation
// returns ErrConflict (uq_allocations_single_active_resource fires).
func TestTransferResourceConflictNewAllocWhileOldActive(t *testing.T) {
	pool := testPool(t)
	truncateAll(t, pool)
	seedCoreRefs(t, pool)
	now := time.Date(2026, 7, 18, 9, 0, 0, 0, time.UTC)

	insertRequest(t, pool, "req-1", now)
	insertRequest(t, pool, "req-2", now)
	insertResource(t, pool, "res-conflict", now)
	// Insert an active allocation for res-conflict (status = 'allocated' counts as active)
	insertAllocation(t, pool, "alloc-active", "req-1", "res-conflict", now)

	repo := &allocationRepository{q: pool}

	// Attempt to create another active allocation for the same resource
	newAlloc, err := domain.NewAllocation("alloc-new", "req-2", "res-conflict",
		now, now.Add(time.Hour), now)
	if err != nil {
		t.Fatalf("NewAllocation() error = %v", err)
	}
	if err := repo.Create(t.Context(), newAlloc); !errors.Is(err, ErrConflict) {
		t.Fatalf("Create() error = %v, want ErrConflict", err)
	}
}
