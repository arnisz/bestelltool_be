# Project Status: Resource Planning System (Go Backend)

## 🎯 Current Focus
Autorisierung (Rollenprüfung pro Endpunkt): welche Endpunkte erfordern welche Rollen (Technician vs. Dispatcher vs. Admin). Klärt auch, wer einen Direct Transfer auslösen darf (bis dahin: nur Dispatcher).

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

## 🔄 In Progress
- [ ] Autorisierung (Rollenprüfung pro Endpunkt): welche Endpunkte erfordern welche Rollen.

## ⏭️ Next Steps (in order)
1. HTTP-Endpunkt für Direct Transfer anbinden (nach Autorisierung, da rollenpflichtig).
2. SSE-Adapter vorbereiten (nach Autorisierung, da Event-Streams rollengefiltert sein müssen: ELZ sieht alles, Techniker nur eigene Requests).
3. End-to-End-Tests ergänzen.
4. Migrations-Anwendung für den Server klären (Migrations-Runner beim Serverstart oder CLI-Befehl) — aktuell spielen nur die Integrationstests Migrationen ein; `resource_dev` wird manuell migriert.

## ⚠️ Known Issues / Tech Debt
- **StaticTokenAuthenticator** (`internal/adapters/auth`) ist eine Übergangslösung. Tokens stehen im Klartext in der Umgebungsvariable `AUTH_STATIC_TOKENS`. Vor Produktion durch eine echte Session-/Token-basierte Authentifizierung ersetzen (z. B. JWT mit Schlüsselrotation oder externe IdP-Integration).
- Offene fachliche Entscheidung: Darf ein Direct Transfer nur von der ELZ freigegeben werden oder auch von Technikern untereinander (relevant für Offline-Sync und Autorisierung)? Bis zur Entscheidung: nur Dispatcher (documentiert in `systemdesign.md` Abschnitt 3).

## 📝 Rules for the AI Agent
- **READ THIS FILE FIRST** at the start of every session or task.
- **UPDATE THIS FILE** immediately when a task from "In Progress" or "Next Steps" is finished.
- Move completed items to the "Completed" list and keep the "Current Focus" sharply aligned with the current immediate goal.
- At the end of every phase, run the PostgreSQL integration tests against the real test database (`TEST_DATABASE_URL` → `resource_test`) — green skips are not sufficient once the environment is available.