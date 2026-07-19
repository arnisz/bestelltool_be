# Project Status: Resource Planning System (Go Backend)

## 🎯 Current Focus
Dockerfile und `docker-compose.yml` für das Backend und die PostgreSQL-Datenbank erstellen (Grundlage für das künftige Deployment).

## ⚙️ Server-Konfiguration (Umgebungsvariablen)

| Variable | Required | Default | Beschreibung |
|----------|----------|---------|-------------|
| `DATABASE_URL` | ✅ | — | PostgreSQL Connection String |
| `AUTH_STATIC_TOKENS` | ✅ | — | Statische Bearer-Token (`token:user-id:role,...`) |
| `RUN_MIGRATIONS` | ❌ | `false` | Führt beim Serverstart eingebettete Up-Migrationen aus (`true`/`1`) |
| `HTTP_ADDR` | ❌ | `:8080` | HTTP Listen-Adresse |
| `HTTP_READ_TIMEOUT` | ❌ | `15s` | Read-Timeout |
| `HTTP_WRITE_TIMEOUT` | ❌ | `15s` | Write-Timeout |
| `HTTP_IDLE_TIMEOUT` | ❌ | `60s` | Idle-Timeout |
| `HTTP_SHUTDOWN_TIMEOUT` | ❌ | `10s` | Graceful-Shutdown-Timeout |

## ✅ Completed
- [x] System Architecture and Requirements defined (`systemdesign.md`).
- [x] AI Agent rules and architectural constraints defined (`agents.md`).
- [x] Project Status tracking initialized (`status.md`).
- [x] Go-Modul initialisiert.
- [x] Hexagonale Verzeichnisstruktur erstellt.
- [x] Domain-Entitäten implementiert.
- [x] Domain-Zustandsautomaten implementiert.
- [x] Domain-Unit-Tests implementiert.
- [x] Application Ports definiert.
- [x] Unit-of-Work-Vertrag definiert.
- [x] Repository-Verträge für erste transaktionale Use Cases definiert.
- [x] AuditWriter- und IdempotencyStore-Ports definiert.
- [x] Erste transaktionale Use Cases implementiert.
- [x] Application-Unit-Tests mit In-Memory-Fakes implementiert.
- [x] PostgreSQL-Schema entworfen.
- [x] Initiale Up- und Down-Migrationen erstellt.
- [x] Audit- und Idempotency-Tabellen erstellt.
- [x] Fachliche Constraints und Indizes definiert.
- [x] Migrationen dokumentiert (`migrations/README.md`).
- [x] PostgreSQL-Connection-Pool-Adapter implementiert.
- [x] PostgreSQL-Unit-of-Work implementiert.
- [x] Transaktionsgebundene Repositories implementiert.
- [x] PostgreSQL-AuditWriter implementiert.
- [x] PostgreSQL-IdempotencyStore implementiert.
- [x] PostgreSQL-Integrationstests ergänzt (mit Skip bei fehlender `TEST_DATABASE_URL`).
- [x] Konfliktbehandlung in PostgreSQL-Repositories geschärft: reine Versionssprünge ohne fachliche Feldänderung werden als Optimistic-Locking-Konflikt behandelt (Request/Resource/Allocation).
- [x] Server-Konfiguration über Umgebungsvariablen implementiert (`DATABASE_URL`, `HTTP_ADDR`, Read/Write/Idle/Shutdown-Timeouts).
- [x] Composition Root im Server-Bootstrap verdrahtet (pgx-Pool, UnitOfWork, Repositories, Use Cases, HTTP-Server, Graceful Shutdown).
- [x] HTTP-Adapter mit Go 1.22 `ServeMux`-Method-Pattern-Routing implementiert.
- [x] Einheitliches JSON-Fehlerformat implementiert: `{"error":{"code":"...","message":"..."}}` inkl. zentralem Error-Mapping (`404/409/422/500`).
- [x] Erste Endpunkte angebunden: Request anlegen, Request abrufen, Allocation-Return anfordern (schreibend über Use Cases/UoW inkl. AuditEvents).
- [x] Handler-Tests mit `httptest` und In-Memory-Fakes ergänzt (inkl. Fehler-Mapping 404/409/422 und ungültiges JSON 400).
- [x] Authentifizierungs-Port implementiert: `ports.Principal`, `ports.Authenticator`, `ports.ErrUnauthenticated` (→ 401).
- [x] Auth-Middleware implementiert: Bearer-Token-Extraktion, Principal in Request-Context; fehlender/ungültiger Token → 401.
- [x] Security-Befund behoben: `actor_id`/`actor_role` aus `auditPayload` entfernt; `parseAuditMeta`/`firstNonEmpty` gelöscht; `buildAuditMeta` liest Actor-Identität ausschließlich aus dem Principal. Fehlender Principal → 500 (Programmierfehler). `X-Client-Occurred-At` bleibt erhalten.
- [x] StaticTokenAuthenticator implementiert (`internal/adapters/auth`), konfigurierbar über `AUTH_STATIC_TOKENS`.
- [x] Composition Root verdrahtet: Authenticator in `main.go`; `AUTH_STATIC_TOKENS` required, Startup-Fehler bei fehlendem Wert.
- [x] Handler-Tests und Auth-Tests vollständig aktualisiert und erweitert (Middleware-Tests, Actor-Feld-Negativ-Tests, Principal→AuditMeta-Test, 500-Programmierfehlertest).
- [x] PostgreSQL-Integrationstests gegen die Raspberry-Pi-Testdatenbank vollständig ausgeführt — alle 8 Tests grün. Zwei Testfehler behoben: (1) `txB` nach Context-Timeout nicht wiederverwendet (poisoned connection → `txC`); (2) `alloc-2` als `completed` (terminal) inseriert, damit die Unique-Constraint-Prüfung korrekt über `repo.Save` läuft.
- [x] Direct-Transfer-Regel in `systemdesign.md` dokumentiert (Abschnitt 3, korrekte Position nach Roles); Architecture nach Abschnitt 4 verschoben.
- [x] Domain-Zustandsautomat um Direct-Transfer-Pfad erweitert: `Resource.TransferDirect` (`in_use` → `reserved`, Block-Guard, leere HolderID-Guard) und `Allocation.CompleteDirectTransfer` (`with_technician` → `completed`, toleriert pending `ReturnRequestedAt`).
- [x] `AllocationRepository.Create` Port + PostgreSQL-Adapter implementiert (benötigt für neue Allocations innerhalb einer Transaktion).
- [x] `TransferResourceUseCase` implementiert: atomare Transaktion mit korrekter Reihenfolge (Lock: Request → Allocation → Resource; Save: alte Allocation first, dann Resource, dann neue Allocation erstellen). Auditiert beide Allocations. Override-Note bei pending Return-Request.
- [x] `ErrAlreadyCompleted` → HTTP 409 (war 422): `mapHTTPError` in `handler.go` korrigiert.
- [x] `newAuditEvent` generiert automatisch eindeutige IDs via `crypto/rand` (behebt latentes PK-Konflikt-Problem bei mehreren Audit-Events pro Transaktion).
- [x] Domain-Unit-Tests für `TransferDirect` und `CompleteDirectTransfer` ergänzt (Success, Blocked, EmptyHolder, ShippedBack, WrongState, AlreadyCompleted, PendingReturnRequest).
- [x] Use-Case-Unit-Tests für `TransferResourceUseCase` ergänzt (Success, BlockedGuard, AuditRollback, TerminalTarget, SameRequestGuard).
- [x] PostgreSQL-Integrationstests für Direct-Transfer ergänzt — alle 10 Tests grün: `TestTransferResourceWithPostgres` (voller Use Case inkl. Unique-Index-Validierung) und `TestTransferResourceConflictNewAllocWhileOldActive` (Gegentest: Index feuert korrekt).
- [x] **Rollenbasierte Autorisierung implementiert**: `ports.ErrForbidden` (→ 403), `requireRoles`-Middleware, explizite Rollenfreigabe an jeder Route. Neue Rollenkonstante `domain.ActorRoleAdmin` eingeführt. `StaticTokenAuthenticator` akzeptiert jetzt `technician`, `dispatcher`, `admin`, `system`. Berechtigungsmatrix in `systemdesign.md` dokumentiert. Architekturregel in `agents.md` festgeschrieben. Alle bestehenden Tests aktualisiert; neue Middleware- und Autorisierungsmatrix-Tests ergänzt. PostgreSQL-Integrationstests — alle 10 grün gegen Raspberry-Pi-Testdatenbank.
- [x] **Rollenbezeichnung vereinheitlicht**: Migration `000005` setzt `'dispatcher'` als einzigen kanonischen Rollenwert in `users.role` und `audit_events.actor_role` durch. `mapActorRole`-Mapping-Funktion entfernt. Alle Schichten (Domain, HTTP, Token-Config, DB) verwenden jetzt denselben Wert `"dispatcher"`. Cross-Technician-Verhalten als bewusste Designentscheidung in `systemdesign.md` dokumentiert (vollständig auditsicher durch AuditEvent pro Transaktion).
- [x] HTTP-Endpunkt für Direct Transfer angebunden (`POST /api/v1/resources/{id}/transfer`) mit strikter Rollenfreigabe nur für Dispatcher; Handler nutzt `decodeJSONBody`, `buildAuditMeta` und zentrales Fehler-Mapping. Composition Root in `main.go` verdrahtet. Handler-Tests für Success/400/401/403/404/409/422 ergänzt.
- [x] SSE-Adapter vorbereitet: neuer Event-Port (`internal/application/ports/event.go`), in-memory SSE-Broker (`internal/adapters/sse`), geschützter Stream-Endpunkt `GET /api/v1/events` mit Rollenfilter (Dispatcher: alle Events, Techniker: nur eigene). Schreibende Use Cases publizieren typisierte Events; Composition Root verdrahtet Publisher + Stream. Unit-Tests für Broker/Handler ergänzt.
- [x] SSE-Handshake gehärtet: `GET /api/v1/events` flush’t jetzt direkt nach dem Schreiben der SSE-Header (vor `Subscribe`), damit Clients die Verbindung sofort erkennen, auch wenn das Stream-Backend beim Subscriben kurz blockiert. Regressionstest `TestHandleEventsFlushesBeforeSubscribeReturns` ergänzt.
- [x] Fachentscheidung finalisiert und dokumentiert: Direct Transfers und operative Ressourcenallokationen liegen ausschließlich beim Dispatcher; Techniker-zu-Techniker-Transfer ist ausgeschlossen (`systemdesign.md`).
- [x] End-to-End-Test ergänzt: `TestResourceLifecycleE2E` verifiziert den vollständigen HTTP→UseCase→Repository→PostgreSQL-Durchstich über `httptest.Server` inkl. Rollen-Negativfall `403` für Techniker-Direct-Transfer (`internal/adapters/http/e2e_test.go`, Skip ohne `TEST_DATABASE_URL`).
- [x] Migrations-Runner für den Serverstart implementiert: SQL-Migrationen via `go:embed` ins Binary eingebettet; zentraler Runner im Postgres-Adapter; optionaler Startup-Lauf über `RUN_MIGRATIONS` mit fail-fast bei Fehler.
- [x] Deployment-Strategie für automatische Migrationen beim Serverstart dokumentiert (`docs/deployment.md`): klare Umgebungsregeln für `RUN_MIGRATIONS`, Multi-Instanz-Empfehlung (`RUN_MIGRATIONS=false` in Staging/Prod), dedizierter Pre-Deployment-Migrationsschritt und manuelle Rollback-Policy für Down-Migrationen.

## ⏭️ Next Steps (in order)
1. Dockerfile und `docker-compose.yml` für das Backend und die PostgreSQL-Datenbank erstellen (Grundlage für das künftige Deployment).
2. Tech-Debt auflösen: Echte Session-/Token-basierte Authentifizierung statt `StaticTokenAuthenticator` umsetzen.

## ⚠️ Known Issues / Tech Debt
- **StaticTokenAuthenticator** (`internal/adapters/auth`) ist eine Übergangslösung. Tokens stehen im Klartext in der Umgebungsvariable `AUTH_STATIC_TOKENS`. Vor Produktion durch eine echte Session-/Token-basierte Authentifizierung ersetzen (z. B. JWT mit Schlüsselrotation oder externe IdP-Integration).

## 📝 Rules for the AI Agent
- **READ THIS FILE FIRST** at the start of every session or task.
- **UPDATE THIS FILE** immediately when a task from "In Progress" or "Next Steps" is finished.
- Move completed items to the "Completed" list and keep the "Current Focus" sharply aligned with the current immediate goal.
- At the end of every phase, run the PostgreSQL integration tests against the real test database (`TEST_DATABASE_URL` → `resource_test`) — green skips are not sufficient once the environment is available.