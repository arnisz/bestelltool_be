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
- Vor jedem Testlauf werden Tabellen per `TRUNCATE ... CASCADE` geleert.