package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"bestelltool_be/internal/application/ports"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type querier interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type txState struct {
	q querier
}

type txAdapter struct {
	state *txState
}

func (t *txAdapter) Users() ports.UserRepository {
	return &userRepository{q: t.state.q}
}

func (t *txAdapter) UserRoles() ports.UserRoleRepository {
	return &userRoleRepository{q: t.state.q}
}

func (t *txAdapter) ResourceClasses() ports.ResourceClassRepository {
	return &resourceClassRepository{q: t.state.q}
}

func (t *txAdapter) Requests() ports.RequestRepository {
	return &requestRepository{q: t.state.q}
}

func (t *txAdapter) Resources() ports.ResourceRepository {
	return &resourceRepository{q: t.state.q}
}

func (t *txAdapter) Allocations() ports.AllocationRepository {
	return &allocationRepository{q: t.state.q}
}

func (t *txAdapter) Audits() ports.AuditWriter {
	return &auditWriter{q: t.state.q}
}

func (t *txAdapter) AuditEvents() ports.AuditRepository {
	return &auditRepository{q: t.state.q}
}

func (t *txAdapter) Idempotency() ports.IdempotencyStore {
	return &idempotencyStore{q: t.state.q}
}

func (t *txAdapter) AuthIdentities() ports.AuthIdentityRepository {
	return &authIdentityRepository{q: t.state.q}
}

func (t *txAdapter) Sessions() ports.SessionRepository {
	return &sessionRepository{q: t.state.q}
}

func (t *txAdapter) RefreshTokens() ports.RefreshTokenRepository {
	return &refreshTokenRepository{q: t.state.q}
}

type UnitOfWork struct {
	pool *pgxpool.Pool
}

func NewUnitOfWork(pool *pgxpool.Pool) *UnitOfWork {
	return &UnitOfWork{pool: pool}
}

func (u *UnitOfWork) WithinTransaction(ctx context.Context, fn func(ctx context.Context, tx ports.Transaction) error) error {
	tx, err := u.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	wrapper := &txAdapter{state: &txState{q: tx}}
	cbErr := fn(ctx, wrapper)
	if cbErr != nil {
		rbErr := tx.Rollback(ctx)
		if rbErr != nil && rbErr != pgx.ErrTxClosed {
			return fmt.Errorf("callback failed: %w; rollback failed: %v", cbErr, rbErr)
		}
		return cbErr
	}

	if err := tx.Commit(ctx); err != nil {
		rbErr := tx.Rollback(ctx)
		if rbErr != nil && rbErr != pgx.ErrTxClosed {
			return fmt.Errorf("commit failed: %w; rollback after commit failed: %v", err, rbErr)
		}
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

func marshalMetadata(meta map[string]any) ([]byte, error) {
	if meta == nil {
		return []byte("{}"), nil
	}
	b, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}
	return b, nil
}

func unmarshalMetadata(b []byte) (map[string]any, error) {
	if len(b) == 0 {
		return map[string]any{}, nil
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("unmarshal metadata: %w", err)
	}
	if out == nil {
		return map[string]any{}, nil
	}
	return out, nil
}

func ptr[T any](v T) *T {
	return &v
}

func prevVersion(v int64) int64 {
	if v <= 1 {
		return 1
	}
	return v - 1
}

func optionalTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return *t
}
