package postgres

import (
	"cmp"
	"context"
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"

	embeddedmigrations "bestelltool_be/migrations"

	"github.com/jackc/pgx/v5/pgxpool"
)

const migrationsAdvisoryLockKey int64 = 892347120331

type migrationFile struct {
	version int64
	name    string
	query   string
}

// RunEmbeddedMigrations führt alle eingebetteten Up-Migrationen gegen die angegebene Datenbank aus.
func RunEmbeddedMigrations(ctx context.Context, dbURL string) error {
	return RunMigrations(ctx, dbURL, embeddedmigrations.Files)
}

// RunMigrations führt alle Up-Migrationen aus dem angegebenen FS gegen die angegebene Datenbank aus.
func RunMigrations(ctx context.Context, dbURL string, migrationFS fs.FS) error {
	pool, err := NewPool(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("create postgres pool for migrations: %w", err)
	}
	defer pool.Close()

	if err := runMigrationsWithPool(ctx, pool, migrationFS); err != nil {
		return err
	}

	return nil
}

func runMigrationsWithPool(ctx context.Context, pool *pgxpool.Pool, migrationFS fs.FS) error {
	migrations, err := collectUpMigrations(migrationFS)
	if err != nil {
		return fmt.Errorf("collect migrations: %w", err)
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrationsAdvisoryLockKey); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, migrationsAdvisoryLockKey)
	}()

	if _, err := conn.Exec(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
	version BIGINT PRIMARY KEY,
	name TEXT NOT NULL,
	applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`); err != nil {
		return fmt.Errorf("ensure schema_migrations table: %w", err)
	}

	for _, migration := range migrations {
		if err := applyMigration(ctx, conn, migration); err != nil {
			return fmt.Errorf("apply migration %s: %w", migration.name, err)
		}
	}

	return nil
}

func collectUpMigrations(migrationFS fs.FS) ([]migrationFile, error) {
	entries, err := fs.ReadDir(migrationFS, ".")
	if err != nil {
		return nil, fmt.Errorf("read migration directory: %w", err)
	}

	migrations := make([]migrationFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}

		version, err := parseMigrationVersion(name)
		if err != nil {
			return nil, err
		}

		query, err := fs.ReadFile(migrationFS, name)
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", name, err)
		}

		migrations = append(migrations, migrationFile{
			version: version,
			name:    name,
			query:   string(query),
		})
	}

	slices.SortFunc(migrations, func(a migrationFile, b migrationFile) int {
		return cmp.Compare(a.version, b.version)
	})

	for i := 1; i < len(migrations); i++ {
		if migrations[i-1].version == migrations[i].version {
			return nil, fmt.Errorf("duplicate migration version %d (%s, %s)", migrations[i].version, migrations[i-1].name, migrations[i].name)
		}
	}

	return migrations, nil
}

func parseMigrationVersion(name string) (int64, error) {
	prefix, _, ok := strings.Cut(name, "_")
	if !ok {
		return 0, fmt.Errorf("invalid migration filename %q: missing version prefix", name)
	}

	version, err := strconv.ParseInt(prefix, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid migration filename %q: parse version: %w", name, err)
	}

	return version, nil
}

func applyMigration(ctx context.Context, conn *pgxpool.Conn, migration migrationFile) error {
	var alreadyApplied bool
	if err := conn.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)`, migration.version).Scan(&alreadyApplied); err != nil {
		return fmt.Errorf("check already applied: %w", err)
	}
	if alreadyApplied {
		return nil
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()

	if _, err := tx.Exec(ctx, migration.query); err != nil {
		return fmt.Errorf("execute migration statement: %w", err)
	}

	if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations(version, name) VALUES($1, $2)`, migration.version, migration.name); err != nil {
		return fmt.Errorf("insert migration marker: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migration transaction: %w", err)
	}

	return nil
}
