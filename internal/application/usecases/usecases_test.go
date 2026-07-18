package usecases

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/domain"
)

var errAuditFailed = errors.New("audit failed")

type txContextKey struct{}

type fakeUoW struct {
	tx           *fakeTx
	nextTxID     int
	commits      int
	rollbacks    int
	lastCommitID int
	lastErr      error
}

func (u *fakeUoW) WithinTransaction(ctx context.Context, fn func(ctx context.Context, tx ports.Transaction) error) error {
	u.nextTxID++
	txID := u.nextTxID
	txCtx := context.WithValue(ctx, txContextKey{}, txID)

	u.tx.id = txID
	err := fn(txCtx, u.tx)
	if err != nil {
		u.rollbacks++
		u.lastErr = err
		return err
	}

	u.commits++
	u.lastCommitID = txID
	u.lastErr = nil
	return nil
}

type fakeTx struct {
	id          int
	allocations *fakeAllocationRepo
	requests    *fakeRequestRepo
	resources   *fakeResourceRepo
	audits      *fakeAuditWriter
}

func (t *fakeTx) Requests() ports.RequestRepository       { return t.requests }
func (t *fakeTx) Resources() ports.ResourceRepository     { return t.resources }
func (t *fakeTx) Allocations() ports.AllocationRepository { return t.allocations }
func (t *fakeTx) Audits() ports.AuditWriter               { return t.audits }
func (t *fakeTx) Idempotency() ports.IdempotencyStore     { return nil }

type fakeAllocationRepo struct {
	items       map[domain.AllocationID]*domain.Allocation
	saves       int
	savedTxIDs  []int
	loadedTxIDs []int
}

func (r *fakeAllocationRepo) GetByID(_ context.Context, id domain.AllocationID) (*domain.Allocation, error) {
	return r.items[id], nil
}

func (r *fakeAllocationRepo) GetForUpdate(ctx context.Context, id domain.AllocationID) (*domain.Allocation, error) {
	txID, _ := ctx.Value(txContextKey{}).(int)
	r.loadedTxIDs = append(r.loadedTxIDs, txID)
	return r.items[id], nil
}

func (r *fakeAllocationRepo) Save(ctx context.Context, allocation *domain.Allocation) error {
	txID, _ := ctx.Value(txContextKey{}).(int)
	r.saves++
	r.savedTxIDs = append(r.savedTxIDs, txID)
	r.items[allocation.ID] = allocation
	return nil
}

type fakeRequestRepo struct {
	items      map[domain.RequestID]*domain.Request
	saves      int
	savedTxIDs []int
}

func (r *fakeRequestRepo) GetByID(_ context.Context, id domain.RequestID) (*domain.Request, error) {
	return r.items[id], nil
}

func (r *fakeRequestRepo) GetForUpdate(_ context.Context, id domain.RequestID) (*domain.Request, error) {
	return r.items[id], nil
}

func (r *fakeRequestRepo) Save(ctx context.Context, req *domain.Request) error {
	txID, _ := ctx.Value(txContextKey{}).(int)
	r.saves++
	r.savedTxIDs = append(r.savedTxIDs, txID)
	r.items[req.ID] = req
	return nil
}

type fakeResourceRepo struct {
	items      map[domain.ResourceID]*domain.Resource
	saves      int
	savedTxIDs []int
}

func (r *fakeResourceRepo) GetByID(_ context.Context, id domain.ResourceID) (*domain.Resource, error) {
	return r.items[id], nil
}

func (r *fakeResourceRepo) GetForUpdate(_ context.Context, id domain.ResourceID) (*domain.Resource, error) {
	return r.items[id], nil
}

func (r *fakeResourceRepo) Save(ctx context.Context, res *domain.Resource) error {
	txID, _ := ctx.Value(txContextKey{}).(int)
	r.saves++
	r.savedTxIDs = append(r.savedTxIDs, txID)
	r.items[res.ID] = res
	return nil
}

type fakeAuditWriter struct {
	events      []domain.AuditEvent
	txIDs       []int
	failOnWrite bool
}

func (w *fakeAuditWriter) Write(ctx context.Context, event domain.AuditEvent) error {
	if w.failOnWrite {
		return errAuditFailed
	}
	txID, _ := ctx.Value(txContextKey{}).(int)
	w.txIDs = append(w.txIDs, txID)
	w.events = append(w.events, event)
	return nil
}

func TestRequestReturnExecuteSuccess(t *testing.T) {
	tx := newFakeTx(t)
	uow := &fakeUoW{tx: tx}
	uc := NewRequestReturnUseCase(uow)

	allocation := mustAllocationInWithTechnicianState(t)
	tx.allocations.items[allocation.ID] = allocation

	err := uc.Execute(t.Context(), RequestReturnInput{
		AllocationID: allocation.ID,
		At:           allocation.UpdatedAt.Add(time.Minute),
		Audit:        validAuditMeta(t),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if allocation.Status != domain.AllocationStatusReturnRequested {
		t.Fatalf("allocation status = %s, want %s", allocation.Status, domain.AllocationStatusReturnRequested)
	}
	if allocation.Version != 4 {
		t.Fatalf("allocation version = %d, want 4", allocation.Version)
	}
	if tx.allocations.saves != 1 {
		t.Fatalf("save calls = %d, want 1", tx.allocations.saves)
	}
	if len(tx.audits.events) != 1 {
		t.Fatalf("audit events = %d, want 1", len(tx.audits.events))
	}
	if uow.commits != 1 {
		t.Fatalf("commits = %d, want 1", uow.commits)
	}
	if tx.allocations.savedTxIDs[0] != tx.audits.txIDs[0] {
		t.Fatalf("save tx id %d != audit tx id %d", tx.allocations.savedTxIDs[0], tx.audits.txIDs[0])
	}
}

func TestRequestReturnExecuteDomainErrorDoesNotSave(t *testing.T) {
	tx := newFakeTx(t)
	uow := &fakeUoW{tx: tx}
	uc := NewRequestReturnUseCase(uow)

	from := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	allocation, err := domain.NewAllocation("a-1", "r-1", "res-1", from, from.Add(time.Hour), from)
	if err != nil {
		t.Fatalf("NewAllocation() error = %v", err)
	}
	tx.allocations.items[allocation.ID] = allocation

	err = uc.Execute(t.Context(), RequestReturnInput{
		AllocationID: allocation.ID,
		At:           from.Add(2 * time.Hour),
		Audit:        validAuditMeta(t),
	})
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("Execute() error = %v, want ErrInvalidTransition", err)
	}
	if tx.allocations.saves != 0 {
		t.Fatalf("save calls = %d, want 0", tx.allocations.saves)
	}
	if len(tx.audits.events) != 0 {
		t.Fatalf("audit events = %d, want 0", len(tx.audits.events))
	}
	if uow.commits != 0 {
		t.Fatalf("commits = %d, want 0", uow.commits)
	}
}

func TestRequestReturnExecuteAuditErrorRollsBack(t *testing.T) {
	tx := newFakeTx(t)
	tx.audits.failOnWrite = true
	uow := &fakeUoW{tx: tx}
	uc := NewRequestReturnUseCase(uow)

	allocation := mustAllocationInWithTechnicianState(t)
	tx.allocations.items[allocation.ID] = allocation

	err := uc.Execute(t.Context(), RequestReturnInput{
		AllocationID: allocation.ID,
		At:           allocation.UpdatedAt.Add(time.Minute),
		Audit:        validAuditMeta(t),
	})
	if !errors.Is(err, errAuditFailed) {
		t.Fatalf("Execute() error = %v, want errAuditFailed", err)
	}
	if tx.allocations.saves != 1 {
		t.Fatalf("save calls = %d, want 1", tx.allocations.saves)
	}
	if uow.commits != 0 {
		t.Fatalf("commits = %d, want 0", uow.commits)
	}
	if uow.rollbacks != 1 {
		t.Fatalf("rollbacks = %d, want 1", uow.rollbacks)
	}
}

func TestUpdateExecutionStateExecuteSuccess(t *testing.T) {
	tx := newFakeTx(t)
	uow := &fakeUoW{tx: tx}
	uc := NewUpdateExecutionStateUseCase(uow)

	req := mustRequest(t)
	tx.requests.items[req.ID] = req

	err := uc.Execute(t.Context(), UpdateExecutionStateInput{
		RequestID: req.ID,
		State:     domain.ExecutionStateBlocked,
		Note:      "Material fehlt",
		Audit:     validAuditMeta(t),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if req.ExecutionState != domain.ExecutionStateBlocked {
		t.Fatalf("execution state = %s, want %s", req.ExecutionState, domain.ExecutionStateBlocked)
	}
	if req.Version != 2 {
		t.Fatalf("request version = %d, want 2", req.Version)
	}
	if tx.requests.saves != 1 {
		t.Fatalf("save calls = %d, want 1", tx.requests.saves)
	}
	if len(tx.audits.events) != 1 {
		t.Fatalf("audit events = %d, want 1", len(tx.audits.events))
	}
}

func TestReactivateResourceExecuteSuccess(t *testing.T) {
	tx := newFakeTx(t)
	uow := &fakeUoW{tx: tx}
	uc := NewReactivateResourceUseCase(uow)

	res := mustBlockedResource(t)
	tx.resources.items[res.ID] = res

	err := uc.Execute(t.Context(), ReactivateResourceInput{
		ResourceID: res.ID,
		Audit:      validAuditMeta(t),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if res.Status != domain.ResourceStatusAvailable {
		t.Fatalf("resource status = %s, want %s", res.Status, domain.ResourceStatusAvailable)
	}
	if res.BlockReason != nil {
		t.Fatalf("block reason should be cleared")
	}
	if res.Version != 8 {
		t.Fatalf("resource version = %d, want 8", res.Version)
	}
	if tx.resources.saves != 1 {
		t.Fatalf("save calls = %d, want 1", tx.resources.saves)
	}
}

func newFakeTx(t *testing.T) *fakeTx {
	t.Helper()
	return &fakeTx{
		allocations: &fakeAllocationRepo{items: map[domain.AllocationID]*domain.Allocation{}},
		requests:    &fakeRequestRepo{items: map[domain.RequestID]*domain.Request{}},
		resources:   &fakeResourceRepo{items: map[domain.ResourceID]*domain.Resource{}},
		audits:      &fakeAuditWriter{},
	}
}

func validAuditMeta(t *testing.T) AuditMeta {
	t.Helper()
	return AuditMeta{
		ServerRecordedAt: time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC),
		ActorID:          domain.UserID("dispatcher-1"),
		ActorRole:        domain.ActorRoleDispatcher,
	}
}

func mustAllocationInWithTechnicianState(t *testing.T) *domain.Allocation {
	t.Helper()
	from := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	allocation, err := domain.NewAllocation("a-1", "r-1", "res-1", from, from.Add(2*time.Hour), from)
	if err != nil {
		t.Fatalf("NewAllocation() error = %v", err)
	}
	if err := allocation.MarkShipped(from.Add(10 * time.Minute)); err != nil {
		t.Fatalf("MarkShipped() error = %v", err)
	}
	if err := allocation.MarkReceivedByTechnician(from.Add(20 * time.Minute)); err != nil {
		t.Fatalf("MarkReceivedByTechnician() error = %v", err)
	}

	return allocation
}

func mustRequest(t *testing.T) *domain.Request {
	t.Helper()
	createdAt := time.Date(2026, 7, 18, 9, 0, 0, 0, time.UTC)
	req, err := domain.NewRequest(
		"req-1",
		"tech-1",
		"ctx-ref",
		"ctx-label",
		nil,
		nil,
		"",
		[]domain.ResourceClassID{"rc-1"},
		createdAt,
	)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	return req
}

func mustBlockedResource(t *testing.T) *domain.Resource {
	t.Helper()
	res, err := domain.NewResource("res-1", "rc-1", "S-1", "LOC-1", nil, nil)
	if err != nil {
		t.Fatalf("NewResource() error = %v", err)
	}
	if err := res.Reserve("tech-1"); err != nil {
		t.Fatalf("Reserve() error = %v", err)
	}
	if err := res.MarkIssued(); err != nil {
		t.Fatalf("MarkIssued() error = %v", err)
	}
	if err := res.MarkInUse(); err != nil {
		t.Fatalf("MarkInUse() error = %v", err)
	}
	if err := res.MarkShippedBack(); err != nil {
		t.Fatalf("MarkShippedBack() error = %v", err)
	}
	if err := res.StartInspection(); err != nil {
		t.Fatalf("StartInspection() error = %v", err)
	}
	if err := res.CompleteInspectionBlocked(domain.BlockReasonDefective, "defekt"); err != nil {
		t.Fatalf("CompleteInspectionBlocked() error = %v", err)
	}

	return res
}

func TestMarkAllocationShippedBackExecuteSuccess(t *testing.T) {
	tx := newFakeTx(t)
	uow := &fakeUoW{tx: tx}
	uc := NewMarkAllocationShippedBackUseCase(uow)

	allocation := mustAllocationInWithTechnicianState(t)
	if err := allocation.RequestReturn(allocation.UpdatedAt.Add(time.Minute)); err != nil {
		t.Fatalf("RequestReturn() error = %v", err)
	}
	tx.allocations.items[allocation.ID] = allocation

	err := uc.Execute(t.Context(), MarkAllocationShippedBackInput{
		AllocationID: allocation.ID,
		At:           allocation.UpdatedAt.Add(time.Minute),
		Audit:        validAuditMeta(t),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if allocation.Status != domain.AllocationStatusShippedBack {
		t.Fatalf("allocation status = %s, want %s", allocation.Status, domain.AllocationStatusShippedBack)
	}
	if allocation.Version != 5 {
		t.Fatalf("allocation version = %d, want 5", allocation.Version)
	}

	if len(tx.audits.events) != 1 {
		t.Fatalf("audit events = %d, want 1", len(tx.audits.events))
	}
	e := tx.audits.events[0]
	if e.FromStatus != string(domain.AllocationStatusReturnRequested) {
		t.Fatalf("audit from = %s, want %s", e.FromStatus, domain.AllocationStatusReturnRequested)
	}
	if e.ToStatus != string(domain.AllocationStatusShippedBack) {
		t.Fatalf("audit to = %s, want %s", e.ToStatus, domain.AllocationStatusShippedBack)
	}
}

func TestUpdateExecutionStateExecuteDomainErrorDoesNotSave(t *testing.T) {
	tx := newFakeTx(t)
	uow := &fakeUoW{tx: tx}
	uc := NewUpdateExecutionStateUseCase(uow)

	req := mustRequest(t)
	tx.requests.items[req.ID] = req

	err := uc.Execute(t.Context(), UpdateExecutionStateInput{
		RequestID: req.ID,
		State:     domain.ExecutionStateBlocked,
		Note:      "",
		Audit:     validAuditMeta(t),
	})
	if !errors.Is(err, domain.ErrReasonRequired) {
		t.Fatalf("Execute() error = %v, want ErrReasonRequired", err)
	}
	if tx.requests.saves != 0 {
		t.Fatalf("save calls = %d, want 0", tx.requests.saves)
	}
}

func TestValidateAuditMetaErrorsAreComparable(t *testing.T) {
	err := validateAuditMeta(AuditMeta{})
	if !errors.Is(err, domain.ErrRequiredField) {
		t.Fatalf("validateAuditMeta() error = %v, want ErrRequiredField", err)
	}
}

func ExampleNewRequestReturnUseCase() {
	_ = NewRequestReturnUseCase(nil)
	fmt.Println("ok")
	// Output: ok
}
