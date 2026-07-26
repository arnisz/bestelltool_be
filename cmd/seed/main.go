// Command seed populates a development PostgreSQL database with typed dummy
// data (users across the assignable roles, one resource class, and two
// resources) using the same UnitOfWork and repositories as production code -
// never hand-written SQL. It replaces the former scripts/dev-seed.sql, which
// could drift from the schema exactly like the E2E fixtures did (see Tech
// Debt entry in status.md).
//
// It is dev-only by design (APP_ENV must be "dev") and safe to run repeatedly
// against the same database: each record is created in its own transaction,
// and an already-existing record (primary key or unique constraint conflict)
// is logged and skipped rather than treated as a failure.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"bestelltool_be/internal/adapters/postgres"
	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/domain"
)

func main() {
	if err := run(); err != nil {
		slog.Error("seed failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	appEnv := strings.TrimSpace(os.Getenv("APP_ENV"))
	if appEnv != "dev" {
		return fmt.Errorf(`cmd/seed is dev-only: APP_ENV must be "dev", got %q`, appEnv)
	}

	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}

	slog.Info("seed: starting", "app_env", appEnv)

	ctx := context.Background()
	pool, err := postgres.NewPool(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("create postgres pool: %w", err)
	}
	defer pool.Close()

	uow := postgres.NewUnitOfWork(pool)
	now := time.Now().UTC()

	if err := seedUser(ctx, uow, "dev-dispatcher", domain.ActorRoleDispatcher, "Dev Dispatcher", now); err != nil {
		return err
	}
	if err := seedUser(ctx, uow, "dev-technician", domain.ActorRoleTechnician, "Dev Technician", now); err != nil {
		return err
	}
	if err := seedUser(ctx, uow, "dev-admin", domain.ActorRoleAdmin, "Dev Admin", now); err != nil {
		return err
	}

	if err := seedResourceClass(ctx, uow, "rc-dev-1", "Dev Resource Class", "Seeded development resource class"); err != nil {
		return err
	}

	if err := seedAvailableResource(ctx, uow, "res-dev-available", "rc-dev-1", "dev-serial-available", "warehouse"); err != nil {
		return err
	}
	if err := seedInUseResource(ctx, uow, "res-dev-in-use", "rc-dev-1", "dev-serial-in-use", "site-dev", "dev-technician"); err != nil {
		return err
	}

	slog.Info("seed: done")
	return nil
}

// seedUser creates one dummy user in its own transaction. The username
// matches id, same as migration 000007's backfill for pre-existing rows;
// dev fixtures get no email.
func seedUser(ctx context.Context, uow ports.UnitOfWork, id domain.UserID, role domain.ActorRole, displayName string, now time.Time) error {
	u, err := domain.NewUser(id, string(id), role, displayName, nil, now)
	if err != nil {
		return fmt.Errorf("build user %s: %w", id, err)
	}

	err = uow.WithinTransaction(ctx, func(ctx context.Context, tx ports.Transaction) error {
		return tx.Users().Create(ctx, u)
	})
	return reportOutcome("user", string(id), err)
}

// seedResourceClass creates one dummy resource class in its own transaction.
func seedResourceClass(ctx context.Context, uow ports.UnitOfWork, id domain.ResourceClassID, name, description string) error {
	rc, err := domain.NewResourceClass(id, name, description, nil)
	if err != nil {
		return fmt.Errorf("build resource class %s: %w", id, err)
	}

	err = uow.WithinTransaction(ctx, func(ctx context.Context, tx ports.Transaction) error {
		return tx.ResourceClasses().Create(ctx, rc)
	})
	return reportOutcome("resource class", string(id), err)
}

// seedAvailableResource creates one dummy resource in its initial "available"
// state, in its own transaction.
func seedAvailableResource(ctx context.Context, uow ports.UnitOfWork, id domain.ResourceID, classID domain.ResourceClassID, serialNumber, location string) error {
	res, err := domain.NewResource(id, classID, serialNumber, location, nil, nil)
	if err != nil {
		return fmt.Errorf("build resource %s: %w", id, err)
	}

	err = uow.WithinTransaction(ctx, func(ctx context.Context, tx ports.Transaction) error {
		return tx.Resources().Create(ctx, res)
	})
	return reportOutcome("resource", string(id), err)
}

// seedInUseResource creates one dummy resource and drives it through the same
// domain state machine a real dispatch would (available -> reserved -> issued
// -> in_use) before creating it, so the seeded row is exactly as valid as one
// produced by the application, in its own transaction.
func seedInUseResource(ctx context.Context, uow ports.UnitOfWork, id domain.ResourceID, classID domain.ResourceClassID, serialNumber, location string, holder domain.UserID) error {
	res, err := domain.NewResource(id, classID, serialNumber, location, nil, nil)
	if err != nil {
		return fmt.Errorf("build resource %s: %w", id, err)
	}
	if err := res.Reserve(holder); err != nil {
		return fmt.Errorf("reserve resource %s: %w", id, err)
	}
	if err := res.MarkIssued(); err != nil {
		return fmt.Errorf("issue resource %s: %w", id, err)
	}
	if err := res.MarkInUse(); err != nil {
		return fmt.Errorf("mark resource %s in use: %w", id, err)
	}

	err = uow.WithinTransaction(ctx, func(ctx context.Context, tx ports.Transaction) error {
		return tx.Resources().Create(ctx, res)
	})
	return reportOutcome("resource", string(id), err)
}

// reportOutcome makes seeding idempotent: an ErrConflict (record already
// exists) is logged and treated as success instead of aborting the run, so
// the tool can be run repeatedly against the same development database.
func reportOutcome(kind, id string, err error) error {
	switch {
	case err == nil:
		slog.Info("seed: created", "kind", kind, "id", id)
		return nil
	case errors.Is(err, ports.ErrConflict):
		slog.Info("seed: already exists, skipping", "kind", kind, "id", id)
		return nil
	default:
		return fmt.Errorf("seed %s %s: %w", kind, id, err)
	}
}
