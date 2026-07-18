package postgres

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/application/usecases"
	"bestelltool_be/internal/domain"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL nicht gesetzt: PostgreSQL-Integrationstests werden übersprungen")
	}
	pool, err := NewPool(t.Context(), url)
	if err != nil {
		t.Fatalf("NewPool() error = %v", err)
	}
	applyMigrations(t, pool)
	truncateAll(t, pool)
	t.Cleanup(func() {
		pool.Close()
	})
	return pool
}

func applyMigrations(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	entries, err := os.ReadDir(filepath.FromSlash("migrations"))
	if err != nil {
		t.Fatalf("ReadDir migrations error = %v", err)
	}
	files := make([]string, 0)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".up.sql") {
			files = append(files, filepath.Join("migrations", name))
		}
	}
	sort.Strings(files)
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("ReadFile %s error = %v", f, err)
		}
		if _, err := pool.Exec(t.Context(), string(b)); err != nil {
			if strings.Contains(err.Error(), "already exists") {
				continue
			}
			t.Fatalf("exec migration %s error = %v", f, err)
		}
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
('dispatcher-1', 'elz', 'Dispatcher One')`); err != nil {
		t.Fatalf("seed users error = %v", err)
	}
	if _, err := q.Exec(t.Context(), `INSERT INTO resource_classes(id, name) VALUES ('rc-1', 'RC1')`); err != nil {
		t.Fatalf("seed resource_classes error = %v", err)
	}
}

func TestUnitOfWorkCommitAndRollback(t *testing.T) {
	pool := testPool(t)
	uow := NewUnitOfWork(pool)

	err := uow.WithinTransaction(t.Context(), func(ctx context.Context, tx ports.Transaction) error {
		seedCoreRefs(t, pool)
		now := time.Date(2026, 7, 18, 9, 0, 0, 0, time.UTC)
		req, createErr := domain.NewRequest("req-1", "tech-1", "ctx", "ctx", nil, nil, "", []domain.ResourceClassID{"rc-1"}, now)
		if createErr != nil {
			t.Fatalf("NewRequest() error = %v", createErr)
		}
		if saveErr := tx.Requests().Save(ctx, req); saveErr != nil {
			return saveErr
		}
		return nil
	})
	if err == nil {
		t.Fatalf("WithinTransaction() expected error because request does not exist before update")
	}

	if !strings.Contains(err.Error(), ErrNotFound.Error()) {
		t.Fatalf("WithinTransaction() error = %v, want not found", err)
	}
}

func TestIdempotencyOutcomeReplay(t *testing.T) {
	pool := testPool(t)
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
}

func TestUseCaseWithPostgresUoW(t *testing.T) {
	pool := testPool(t)
	uow := NewUnitOfWork(pool)
	truncateAll(t, pool)

	seedCoreRefs(t, pool)
	createdAt := time.Date(2026, 7, 18, 9, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(t.Context(), `
INSERT INTO requests(id, technician_id, status, execution_state, execution_note, context_ref, context_label, wish_from, wish_until, note, version, created_at, updated_at)
VALUES ('req-1', 'tech-1', 'open', 'executable', '', 'ctx', 'ctx', NULL, NULL, '', 1, $1, $1)
`, createdAt); err != nil {
		t.Fatalf("insert request error = %v", err)
	}
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
			ServerRecordedAt: time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC),
			ActorID:          "dispatcher-1",
			ActorRole:        domain.ActorRoleDispatcher,
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

	var count int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM audit_events WHERE entity_type = 'request' AND entity_id = 'req-1'`).Scan(&count); err != nil {
		t.Fatalf("scan audit count error = %v", err)
	}
	if count != 1 {
		t.Fatalf("audit count = %d, want 1", count)
	}
}
