### PostgreSQL-Integrationstests

- Voraussetzung: PostgreSQL 14+ (empfohlen: 15+).
- Die Tests verwenden ausschließlich `TEST_DATABASE_URL`.
- Die Datenbank darf nur Testdaten enthalten.

### Variable setzen

- PowerShell:
  - `$env:TEST_DATABASE_URL = "postgres://user:pass@localhost:5432/bestelltool_test?sslmode=disable"`
- Bash:
  - `export TEST_DATABASE_URL="postgres://user:pass@localhost:5432/bestelltool_test?sslmode=disable"`

### Testausführung

- Gesamttests:
  - `go test -count=1 ./...`
- PostgreSQL-Adapter explizit:
  - `go test -count=1 -v ./internal/adapters/postgres/...`

### Migrationsverhalten in Tests

- Tests lesen vorhandene `*.up.sql` aus `migrations/` in numerischer Reihenfolge.
- Es wird keine zweite Schemaquelle gepflegt.
- Vor jedem Testlauf wird das `public`-Schema per `DROP SCHEMA public CASCADE; CREATE SCHEMA public;` zurückgesetzt und anschließend über `RunEmbeddedMigrations` neu aufgebaut (kein `TRUNCATE` mehr, siehe `migrations/README.md`). Grund: Migration `000006` blockiert `TRUNCATE` auf `audit_events` per Trigger (append-only), und `audit_events.actor_id` referenziert `users(id)`, sodass ein `TRUNCATE users CASCADE` denselben Trigger auslösen würde. Der Reset läuft auf einer separaten Verbindung vor der Erzeugung des vom Test genutzten `pgxpool.Pool`, um pgx-v5-Probleme mit gecachten Prepared-Statement-Plänen/OIDs nach dem Reset zu vermeiden.