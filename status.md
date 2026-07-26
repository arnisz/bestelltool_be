# Project Status: Resource Planning System (Go Backend)

## 🎯 Current Focus
**Phase 1, Punkt 2:** Migration `000006` vorbereiten und umsetzen: `audit_events.actor_role` um `admin` erweitern sowie administrative `entity_type`-Werte ergänzen. Das ist die erste fachliche Änderung für die Benutzerverwaltung, weil ohne diese Audit-Erweiterung keine administrative Aktion revisionssicher protokolliert werden kann.

## ⚙️ Server-Konfiguration (Umgebungsvariablen)

### Bestehend

| Variable | Required | Default | Beschreibung |
|----------|----------|---------|-------------|
| `DATABASE_URL` | ✅ | — | PostgreSQL Connection String |
| `APP_ENV` | ✅ | — | `dev` / `staging` / `prod`; steuert die Zulässigkeit von `AUTH_STATIC_TOKENS` |
| `AUTH_STATIC_TOKENS` | nur `dev` | — | Statische Bearer-Token (`token:user-id:role,...`) — Übergangslösung, ab Phase 2 nur noch bei `APP_ENV=dev` zulässig (SEC-26) |
| `RUN_MIGRATIONS` | ❌ | `false` | Führt beim Serverstart eingebettete Up-Migrationen aus (`true`/`1`) |
| `HTTP_ADDR` | ❌ | `:8080` | HTTP Listen-Adresse |
| `HTTP_READ_TIMEOUT` | ❌ | `15s` | Read-Timeout |
| `HTTP_WRITE_TIMEOUT` | ❌ | `15s` | Write-Timeout |
| `HTTP_IDLE_TIMEOUT` | ❌ | `60s` | Idle-Timeout |
| `HTTP_SHUTDOWN_TIMEOUT` | ❌ | `10s` | Graceful-Shutdown-Timeout |

### Geplant (Benutzerverwaltung, Phase 2–3)

| Variable | Required | Default | Beschreibung |
|----------|----------|---------|-------------|
| `AUTH_MODE` | ❌ | `session` | `session` (Login/Refresh) oder `static` (nur `dev`) |
| `ACCESS_TOKEN_TTL` | ❌ | `15m` | Lebensdauer des Access-Tokens |
| `REFRESH_TOKEN_TTL` | ❌ | `720h` | Lebensdauer des Refresh-Tokens (30 d, muss die Offline-Phase der Techniker abdecken) |
| `SESSION_IDLE_TTL` | ❌ | `720h` | Idle-Timeout der Session |
| `SESSION_MAX_LIFETIME` | ❌ | `2160h` | Absolute Obergrenze (90 d) |
| `REFRESH_REPLAY_GRACE` | ❌ | `30s` | Toleranzfenster für wiederholte Refresh-Requests (Entscheidung D-2); `0` deaktiviert |
| `PRINCIPAL_CACHE_TTL` | ❌ | `30s` | Obergrenze der Widerrufsverzögerung (SEC-11) |
| `ARGON2_MEMORY_KIB` | ❌ | `19456` | Argon2id-Speicher (OWASP-Minimum 19 MiB) |
| `ARGON2_TIME` | ❌ | `2` | Argon2id-Iterationen |
| `ARGON2_PARALLELISM` | ❌ | `1` | Argon2id-Parallelität |
| `LOGIN_MAX_ATTEMPTS` | ❌ | `10` | Fehlversuche bis zur zeitlichen Sperre |
| `LOGIN_LOCKOUT_WINDOW` | ❌ | `15m` | Dauer der zeitlichen Sperre |
| `TRUST_PROXY_HEADERS` | ❌ | `false` | Nur aktivieren, wenn ausschließlich der eigene Reverse Proxy (Caddy) erreichbar ist — sonst ist die IP-basierte Drosselung fälschbar |

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
- [x] SSE-Handshake gehärtet: `GET /api/v1/events` flush't jetzt direkt nach dem Schreiben der SSE-Header (vor `Subscribe`), damit Clients die Verbindung sofort erkennen, auch wenn das Stream-Backend beim Subscriben kurz blockiert. Regressionstest `TestHandleEventsFlushesBeforeSubscribeReturns` ergänzt.
- [x] Fachentscheidung finalisiert und dokumentiert: Direct Transfers und operative Ressourcenallokationen liegen ausschließlich beim Dispatcher; Techniker-zu-Techniker-Transfer ist ausgeschlossen (`systemdesign.md`).
- [x] End-to-End-Test ergänzt: `TestResourceLifecycleE2E` verifiziert den vollständigen HTTP→UseCase→Repository→PostgreSQL-Durchstich über `httptest.Server` inkl. Rollen-Negativfall `403` für Techniker-Direct-Transfer (`internal/adapters/http/e2e_test.go`, Skip ohne `TEST_DATABASE_URL`); die Vorbedingungen für Schritt C werden dabei aktuell per Roh-SQL-Fixture gesetzt (nicht über den Produktivpfad).
- [x] Migrations-Runner für den Serverstart implementiert: SQL-Migrationen via `go:embed` ins Binary eingebettet; zentraler Runner im Postgres-Adapter; optionaler Startup-Lauf über `RUN_MIGRATIONS` mit fail-fast bei Fehler.
- [x] Deployment-Strategie für automatische Migrationen beim Serverstart dokumentiert (`docs/deployment.md`): klare Umgebungsregeln für `RUN_MIGRATIONS`, Multi-Instanz-Empfehlung (`RUN_MIGRATIONS=false` in Staging/Prod), dedizierter Pre-Deployment-Migrationsschritt und manuelle Rollback-Policy für Down-Migrationen.
- [x] **Architektur um Identity-, Autorisierungs- und Benutzerverwaltungsmodell erweitert** (`systemdesign.md`, Abschnitte 5–13): Datenmodell (`users`, `auth_identities`, `roles`, `permissions`, `role_permissions`, `user_roles`, `sessions`, `refresh_tokens`), Berechtigungsmodell mit Deny-by-Default, Session-/Token-Konzept mit Rotation und Replay-Erkennung, `active_role` bei Mehrfachrollen, Audit-Erweiterung, Datenschutz-/Aufbewahrungsvorgaben, numerierte Sicherheitsanforderungen `SEC-01`–`SEC-27`, offene Entscheidungen `D-1`–`D-5` und Migrationsphasen.
- [x] **Phase 0 Deployment-Nachweise erbracht**: `compose.yaml` validiert, Backend-Container gestartet, `/healthz` liefert `200`, Migrationen beim Containerstart nachgewiesen (`RUN_MIGRATIONS=true`) und Container läuft als `65532:65532`; Image-Inspektion ohne eingebettete `AUTH_STATIC_TOKENS`.
- [x] **SEC-26 im Container negativ nachgewiesen**: `APP_ENV=prod` mit gesetztem `AUTH_STATIC_TOKENS` führt zu fatalem Startup-Abbruch.
- [x] **B5-Nachweis gegen leere Testdatenbank erbracht**: `emptydb.bat` + `go test -count=1 -p 1 ./...` mit gesetzter `TEST_DATABASE_URL` grün; Migrationsanwendung erfolgt in `testPool`/`e2eTestPool` via `RunEmbeddedMigrations` (kein `TestMain` nötig).

## ⏭️ Next Steps (in order)

### Phase 0 — Deployment-Grundlage
1. [x] Dockerfile (Multi-Stage, non-root User, `CGO_ENABLED=0`) und `compose.yaml` für Backend + PostgreSQL erstellt und verifiziert. Secrets nicht im Image; `AUTH_STATIC_TOKENS` nur zur Laufzeit im dev-Compose. Ein Container-`HEALTHCHECK` ist bewusst noch offen (Distroless ohne Shell/curl/wget; eigenes Binary wäre nötig, `/healthz` ist extern prüfbar).
   - [x] 1c umgesetzt: Integrations-/E2E-Tests wenden Embedded Up-Migrationen beim Teststart über `RunEmbeddedMigrations` in `testPool`/`e2eTestPool` auf `TEST_DATABASE_URL` an (ohne Refactoring auf `TestMain`).
   - [x] 1d (ehemals Punkt 15) umgesetzt: `AUTH_STATIC_TOKENS` ist nur bei `APP_ENV=dev` zulässig; bei anderem `APP_ENV` bricht der Startup mit Fehler ab (SEC-26).

### Phase 1 — Audit-Fundament (Voraussetzung für alles Administrative)
2. [ ] Migration `000006`: `audit_events.actor_role` CHECK um `'admin'` erweitern; `entity_type` um `user`, `role`, `user_role`, `resource_class`, `resource_class_membership`, `session`, `auth_identity` erweitern. Kein Foreign Key auf `roles(code)` (Audit muss von Stammdaten unabhängig bleiben, SEC-20-Begründung in `systemdesign.md` §8.1).
3. [ ] Append-Only technisch erzwingen (SEC-20): `REVOKE UPDATE, DELETE ON audit_events` für die Anwendungsrolle **plus** `BEFORE UPDATE OR DELETE`-Trigger mit `RAISE EXCEPTION`. Integrationstest, der ein `UPDATE`/`DELETE` versucht und den Fehler erwartet.
4. [ ] Aktions-Taxonomie im Code als Konstanten festschreiben (`user.create`, `role.assign`, `session.revoke`, `session.replay_detected`, `auth.login_failed`, …) statt freier Strings.

### Phase 2 — Auth-Kern (löst `AUTH_STATIC_TOKENS` ab)
- [ ] Tech Debt vor Phase 2, Punkt 5: `seedDirectTransferPreconditions` in `internal/adapters/http/e2e_test.go` und `scripts/dev-seed.sql` enthalten handgeschriebenes Schemawissen (zweite/dritte Kopie außerhalb der Repositories/Use Cases). Spätestens mit `users.username` (`NOT NULL` + `UNIQUE`) in Phase 2, Punkt 5 droht erneuter Drift/Bruch. Vorher auf Seeding über Repositories/Use Cases innerhalb einer UnitOfWork umstellen.
5. [ ] Migration `000007`: `users` um `username` (Backfill → `NOT NULL` → `UNIQUE`), `email`, `version`, `created_at`, `updated_at` erweitern. `users.role` bleibt vorerst bestehen (Expand/Contract).
6. [ ] Migration `000008`: `auth_identities`, `sessions`, `refresh_tokens` inkl. Indizes anlegen.
7. [ ] Ports ergänzen: `UserRepository`, `AuthIdentityRepository`, `SessionRepository`, `RefreshTokenRepository`, `PasswordHasher`, `SecretGenerator`, `Clock`.
8. [ ] Adapter `internal/adapters/auth/argon2`: Argon2id-Hasher mit PHC-Kodierung, konfigurierbaren Parametern, `NeedsRehash` und Rehash-on-Login (SEC-02).
9. [ ] Token-Adapter: `rp_at_<id>.<secret>` / `rp_rt_<id>.<secret>`, ≥32 Byte aus `crypto/rand`, Speicherung nur als SHA-256-Hash, Vergleich über `crypto/subtle` (SEC-04, SEC-07).
10. [ ] Use Cases `Login`, `RefreshSession`, `Logout`, `ChangeOwnPassword` — je mit Audit-Event in derselben Transaktion (SEC-21).
11. [ ] Refresh-Rotation inkl. Replay-Erkennung: verbrauchter Token außerhalb `REFRESH_REPLAY_GRACE` ⇒ gesamte Token-Familie + Session widerrufen und auditieren (SEC-08). Unit-Test für Familien-Widerruf und für das Grace-Fenster (D-2).
12. [ ] `SessionAuthenticator` als neuer `ports.Authenticator`; Principal-Cache mit harter TTL (SEC-11).
13. [ ] Drosselung für `login`/`refresh` pro Konto und pro Quell-IP, `429` + `Retry-After` in `mapHTTPError` ergänzen (SEC-05). IP-Ermittlung nur über Proxy-Header, wenn `TRUST_PROXY_HEADERS=true`.
14. [ ] Timing-Gleichheit bei unbekanntem Benutzer: Dummy-Argon2id-Verifikation, einheitliche Fehlermeldung (SEC-03). Test, der die drei Fehlerfälle auf identische Response prüft.
### Phase 3 — Rollen & Berechtigungen
16. [ ] Migration `000009`: `roles`, `permissions`, `role_permissions`, `user_roles` (inkl. `CHECK (assigned_by <> user_id)`) anlegen; Katalog aus `systemdesign.md` §6.2 seeden; `system.is_assignable = false`.
17. [ ] Migration `000010`: `user_roles` aus `users.role` backfillen.
18. [ ] `PermissionResolver`-Port + PostgreSQL-Adapter; `requirePermissions`-Middleware; alle bestehenden Routen von `requireRoles` umstellen.
19. [ ] Deny-by-Default-Test: enumeriert alle registrierten Routen und schlägt fehl, sobald eine Route keine Permission deklariert (SEC-13).
20. [ ] `active_role` in der Session; `SwitchActiveRole` erzeugt eine **neue** Session und widerruft die alte; Validierung von `active_role` gegen `user_roles` bei jedem Refresh (SEC-14).
21. [ ] `GET /api/v1/auth/me` liefert `user_id`, `active_role`, `roles`, effektive Permissions.

### Phase 4 — Admin-Endpunkte
22. [ ] Use Cases: `ListUsers`, `GetUser`, `CreateUser`, `UpdateUser`, `DisableUser`, `ReactivateUser`, `AssignRole`, `RevokeRole`, `RevokeUserSessions`, `ResetLocalPassword`, `ListUserAuditEvents`.
23. [ ] Invariante „letzter aktiver Admin" **race-frei** implementieren (SEC-18): `SELECT … FOR UPDATE` über die Admin-Zuweisungen bzw. transaktionsgebundener Advisory Lock. Integrationstest mit zwei parallelen Transaktionen, der beweist, dass nicht beide durchkommen.
24. [ ] Selbstzuweisung von Rollen im Use Case ablehnen (SEC-16); Negativtest gegen den DB-`CHECK` als zweite Verteidigungslinie.
25. [ ] Session-Widerruf bei Deaktivierung, Rollenänderung und Passwort-Reset (SEC-10); Test, dass ein vorher gültiger Access-Token innerhalb `PRINCIPAL_CACHE_TTL` ungültig wird.
26. [ ] `GET /api/v1/admin/audit-events` mit Permission `audit.read`, Filter nach Zeitraum, Actor und Entity; keine Schreibpfade.
27. [ ] E2E-Test `TestUserAdministrationE2E`: Admin legt Benutzer an, weist Rolle zu, Benutzer loggt sich ein, Admin deaktiviert ihn, Folgeaufruf ⇒ `401`; Negativfall: Admin versucht Direct Transfer ⇒ `403` (SEC-15).

### Phase 5 — Datenbank-Privilegien & .NET-Grenze
28. [ ] Drei getrennte PostgreSQL-Rollen: `app_backend` (DML, kein DDL), `app_migrator` (DDL), `refdata_tool` (nur Referenztabellen). `GRANT`/`REVOKE`-Skripte versionieren und im Deployment ausführen (SEC-24, SEC-25).
29. [ ] Integrationstest, der mit `refdata_tool`-Credentials beweist, dass Zugriffe auf `users`, `auth_identities`, `sessions`, `refresh_tokens` und `audit_events` scheitern.
30. [ ] `docs/deployment.md` um Rollen-, Secret- und TLS-Vorgaben ergänzen (SEC-27).

### Phase 6 — Optional / später
31. [ ] Aufbewahrungs-Job für Session- und Audit-Daten mit konfigurierbaren Fristen (Abschnitt 9).
32. [ ] OIDC-Adapter (Entra ID) mit Authorization Code Flow + PKCE, externer Systembrowser (RFC 8252), Mapping `provider_subject` → interner Benutzer. Abhängig von D-5.
33. [ ] Contract-Migration: `users.role` entfernen, sobald kein Codepfad mehr darauf liest (nicht im selben Release wie Phase 3).
34. [ ] Optional: `prev_hash`-Kette in `audit_events` für Manipulationsnachweis.

### Begleitend
35. [ ] `agents.md` um die neuen Architekturregeln ergänzen (Text siehe unten).

## 📌 Neue Regeln für `agents.md`
- Routen deklarieren **Permissions**, nie Rollen. Eine Route ohne Permission-Deklaration ist ein Fehler und muss `403` liefern (SEC-13).
- Actor-Identität und `actor_role` kommen ausschließlich aus dem authentifizierten Principal, niemals aus dem Request-Body (SEC-01).
- Kryptografie (Argon2id, `crypto/rand`, Hashing, Token-Kodierung) lebt ausschließlich in Adaptern. Die Domain kennt nur die Ports `PasswordHasher`, `SecretGenerator` und `Clock`.
- Jede sicherheitsrelevante Mutation läuft über die `UnitOfWork` und schreibt ihr `AuditEvent` in derselben Transaktion (SEC-21).
- Keine Zeit aus `time.Now()` in Domain oder Application — immer über den `Clock`-Port.
- Secrets, Token und Hashes erscheinen nie in Logs, URLs, Fehlermeldungen oder Audit-Payloads (SEC-06, SEC-23).
- Administrative Berechtigungen implizieren niemals operative (SEC-15).

## ⚠️ Known Issues / Tech Debt
- **Fehlender Startup-Konfigurationsbeleg im Container-Log**: `docker compose logs backend` ist beim Start leer; der Server schreibt keine Startzeile mit aufgelöster Konfiguration (`APP_ENV`, `HTTP_ADDR`, `RUN_MIGRATIONS`, Anzahl angewendeter Migrationen). Ohne diesen Logeintrag fehlt im Deployment ein Nachweis, mit welcher Konfiguration eine Instanz läuft. Umsetzung nur ohne Secrets in Logs (SEC-06).
- **SQL-Fixture-Drift im E2E- und Dev-Seed-Setup**: `seedDirectTransferPreconditions` in `internal/adapters/http/e2e_test.go` und `scripts/dev-seed.sql` enthalten handgeschriebenes Schemawissen außerhalb der Repositories/Use Cases (zweite/dritte Kopie). Der aufgetretene `SQLSTATE 23502` bei `request_resource_classes.position` war eine direkte Folge. Der nächste kritische Driftpunkt liegt in Phase 2, Punkt 5 (`users.username` als `NOT NULL` + `UNIQUE`). Folgeaufgabe: Seeding auf Repository-/Use-Case-Pfad innerhalb einer UnitOfWork umstellen.
- **StaticTokenAuthenticator** (`internal/adapters/auth`) ist eine Übergangslösung. Tokens stehen im Klartext in `AUTH_STATIC_TOKENS`. Ablösung in Phase 2; danach nur noch bei `APP_ENV=dev` zulässig und in Phase 3 vollständig entfernen.
- **Audit-Schema unvollständig**: `actor_role` kennt `admin` nicht, `entity_type` kennt keine administrativen Typen. Bis Migration `000006` können administrative Aktionen nicht protokolliert werden — daher dürfen vorher **keine** Admin-Endpunkte ausgeliefert werden.
- **Audit-Unveränderlichkeit ist bisher nur eine Design-Zusage**, technisch noch nicht erzwungen (kein `REVOKE`, kein Trigger).
- **Keine Drosselung** auf schreibenden oder künftigen Login-Endpunkten; `429` fehlt im zentralen Fehler-Mapping.
- **`users`-Tabelle zu eng** (`id`, `role`, `display_name`, `is_active`): kein `username`, keine Mehrfachrollen, keine Versionierung.
- **Rollenprüfung direkt an den Routen** (`requireRoles`) skaliert nicht auf Benutzer-, Gruppen- und Referenzverwaltung.
- **Datenbank-Privilegien nicht getrennt**: Backend, Migrationen und das .NET-Tool nutzen bislang dasselbe Konto. Die dokumentierte Grenze des .NET-Tools ist damit reine Konvention und nicht durchgesetzt (SEC-25). Die Credentials eines kopierbaren Desktop-Clients sind als kompromittiert zu betrachten.
- **Offene Entscheidung D-3**: Techniker dürfen derzeit alle Requests lesen. Bewusst gewählt und auditiert, aber vor dem Feldeinsatz mobiler Clients auf `request.read.own` einzuschränken.
- **Kein Aufbewahrungskonzept** implementiert; Audit- und Session-Daten wachsen unbegrenzt (Datenschutz, Abschnitt 9).

## 📝 Rules for the AI Agent
- **READ THIS FILE FIRST** at the start of every session or task.
- **UPDATE THIS FILE** immediately when a task from "In Progress" or "Next Steps" is finished.
- Move completed items to the "Completed" list and keep the "Current Focus" sharply aligned with the current immediate goal.
- Reference the relevant `SEC-xx` requirement from `systemdesign.md` §11 in commit messages and tests for every security-relevant change.
- No administrative endpoint ships before Phase 1 (audit extension) is complete.
- At the end of every phase, run the PostgreSQL integration tests against the real test database (`TEST_DATABASE_URL` → `resource_test`) — green skips are not sufficient once the environment is available.