### Überblick
- Die Migrationen unter `migrations/` sind die einzige Schemaquelle.
- Reihenfolge:
  - `000001_create_core_tables.*`: Kerntabellen für Aggregate und Beziehungen.
  - `000002_create_audit_and_idempotency.*`: Audit-Trail, Idempotency-Outcome-Replay, Append-only-Regel.
  - `000003_create_indexes.*`: Zugriffspfade, Dispatch-Indizes, partielle Exklusivitätsregel.
  - `000004_align_actor_role_dispatcher.*` / `000005_unify_role_dispatcher.*`: Rollenbezeichnung auf `dispatcher` vereinheitlicht.
  - `000006_extend_audit_admin_appendonly.*`: `audit_events.actor_role` um `admin`, `entity_type` um die administrativen Werte erweitert; Append-Only-Trigger erweitert um `TRUNCATE` und um SQLSTATE `42501`; vorbereitende `REVOKE`-Statements (siehe unten).

### Tabellen
- `users`: persistierbare Benutzerstammdaten (`technician`, `dispatcher`, `admin`) mit Deaktivierungsflag.
- `resource_classes`: Klasse mit `metadata jsonb`.
- `resources`: konkrete Ressource inkl. Status, Sperrgrund, Halter, `valid_until`, `version`.
- `requests`: Anfrage inkl. Status, Execution-State, Wunschzeitraum, Kontext, `version`.
- `request_resource_classes`: normalisierte Zuordnung Request ↔ ResourceClass über `position` (Duplikate pro Klasse bleiben möglich).
- `allocations`: Zuordnung Request ↔ Resource inkl. Zeitfenster, Lifecycle-Zeitpunkten, `version`.
- `audit_events`: append-only Persistierung von Audit-Einträgen mit `server_recorded_at DEFAULT clock_timestamp()`.
- `idempotency_outcomes`: Outcome-Replay pro `client_action_id` inkl. gespeicherten Ergebnisfeldern (`status_code`, `payload`, `error_text`).

### ID-Strategie
- Aggregate- und Referenz-IDs sind `text` entsprechend den Domain-ID-Typen.
- Keine ID-Erzeugung über Sequenzen oder Defaults.
- `client_action_id` ist `text` (Port nutzt `string`, keine UUID-Vorgabe im Vertrag).

### Zeitstempelstrategie
- Fachliche Zeitpunkte werden als `timestamptz` gespeichert.
- Audit/Idempotency verwenden autoritative Datenbankzeit über `clock_timestamp()`.
- `client_occurred_at` und `client_seq` bleiben für Offline-Fälle nullable.

### Statusmodellierung und Constraints
- Statusfelder sind `text` mit expliziten `CHECK`-Constraints (keine PostgreSQL-Enums).
- Statuswerte werden exakt aus den Domain-Konstanten übernommen.
- `resources`: `status='blocked'` erzwingt `block_reason`; andere Status verbieten `block_reason`.
- `requests`: `blocked`/`partially_blocked` erzwingen nicht-leere `execution_note`; `executable` erzwingt leere Note.
- Zeitbereiche:
  - `requests`: `wish_until > wish_from`, falls beide gesetzt.
  - `allocations`: `planned_until > planned_from`.
- Optimistic Locking: `version bigint NOT NULL CHECK (version >= 1)` auf `requests`, `resources`, `allocations`.

### Löschregeln
- Zentrale Aggregate verwenden `ON DELETE RESTRICT`.
- Reine Beziehungstabelle `request_resource_classes` nutzt `ON DELETE CASCADE` auf `requests`.
- Keine Kaskadenlöschung für `audit_events`.

### Wichtige Indizes
- `resources`: `idx_resources_valid_until`, `idx_resources_dispatch_lookup`.
- `requests`: Status-, Techniker-, Kontext- sowie Status+Aktualisierungszeit-Index.
- `allocations`: Indizes für Request/Resource/Status/Planned-Until und Dispatch-Fenster.
- `uq_allocations_single_active_resource`: partieller Unique Index verhindert doppelte aktive Allocation pro Resource.
- `audit_events`: `(entity_type, entity_id, server_recorded_at)`, `(actor_id, server_recorded_at)`, `server_recorded_at`.
- `idempotency_outcomes`: `(actor_id, client_seq)` für potenzielle Replay-/Sync-Analysen.

### Audit Append-only
- Trigger `trg_audit_events_no_update`, `trg_audit_events_no_delete` (beide `FOR EACH ROW`) und, seit `000006`, `trg_audit_events_no_truncate` (`FOR EACH STATEMENT`) blockieren Änderungen, Löschungen und `TRUNCATE`.
- Seit `000006` löst `reject_audit_events_mutation()` mit `RAISE EXCEPTION ... USING ERRCODE = '42501'` aus. Konsumenten (und Tests) müssen auf den SQLSTATE `42501` prüfen, nicht auf den Fehlertext — der Text kann sich ändern, der Code ist der stabile Vertrag.
- Für kontrollierte spätere Migrationen kann die Regel temporär angepasst werden, indem Trigger/Funktion in einer eigenen Migration gezielt gedroppt/ersetzt und danach wiederhergestellt werden.
- `000006` enthält zusätzlich ein vorbereitendes `REVOKE UPDATE, DELETE ON audit_events FROM PUBLIC`-Statement. Dieses ist **nicht** bereits eine wirksame Zugriffskontrolle, solange keine dedizierte Least-Privilege-Rolle für das Backend existiert (Phase 5, `systemdesign.md` §13) — siehe `docs/deployment.md` Abschnitt 6. Der `TRUNCATE`-Schutz stammt bewusst ausschließlich vom Trigger `trg_audit_events_no_truncate`, nicht vom `REVOKE`-Statement.

### `000006` Down-Migration: bekannte Fehlschlagbedingung
- `000006_extend_audit_admin_appendonly.down.sql` stellt die alten `CHECK`-Constraints wieder her (`actor_role IN ('technician','dispatcher','system')`, `entity_type IN ('request','allocation','resource')`).
- Diese Down-Migration **schlägt absichtlich fehl**, sobald bereits mindestens eine Zeile mit `actor_role = 'admin'` oder einem der neuen `entity_type`-Werte (`user`, `role`, `user_role`, `resource_class`, `resource_class_membership`, `session`, `auth_identity`) persistiert wurde — die wiederhergestellte `CHECK`-Constraint würde diese Zeilen sonst stillschweigend verletzen. Ein Downgrade ist in diesem Fall nur nach manueller Datenbereinigung/-migration möglich, niemals automatisiert.
- Die `REVOKE`-Statements aus der Up-Migration werden von der Down-Migration bewusst **nicht** zurückgenommen (kein `GRANT ... TO PUBLIC`): sie entziehen `PUBLIC` Rechte, die Tabelleneigentümer ohnehin nicht betreffen, und ein erneutes `GRANT` an `PUBLIC` wäre ein Sicherheitsrückschritt, kein Rollback.

### Testaufbau: Schema-Reset statt `TRUNCATE` (E1/E2)
- `audit_events.actor_id` referenziert `users(id)`. Seit `000006` würde ein `TRUNCATE users CASCADE` (oder ein direktes `TRUNCATE audit_events`) den Append-Only-Trigger auslösen und mit SQLSTATE `42501` fehlschlagen — **CASCADE-Truncates auf Tabellen, die in `audit_events` münden, sind damit für den Testaufbau verboten.**
- Die Integrationstests (`internal/adapters/postgres/postgres_test.go`, `internal/adapters/http/e2e_test.go`) setzen deshalb zwingend auf einen **Schema-Reset** (`DROP SCHEMA public CASCADE; CREATE SCHEMA public;`) gefolgt von `RunEmbeddedMigrations`, nicht auf `TRUNCATE`.
- Der Schema-Reset läuft auf einer eigenen, kurzlebigen Verbindung (`pgx.Connect`), die vor der Instanziierung des von den Tests genutzten `pgxpool.Pool` geöffnet und wieder geschlossen wird. Grund: pgx v5 cacht Prepared-Statement-Pläne und Typ-OIDs pro Verbindung; ein Schema-Reset auf einer bereits vom Pool verwendeten Verbindung kann zu `cached plan must not change result type` oder OID-Fehlern in nachfolgenden Queries führen. Der Reset muss daher entweder vor der Pool-Erzeugung auf einer separaten Verbindung laufen (so implementiert) oder der Pool muss danach explizit geschlossen und neu erzeugt werden.

### Bewusst offene Entscheidungen
- Fristen-Policy bleibt offen: maßgeblich ist fachlich noch nicht entschieden (`client_occurred_at` vs. `server_recorded_at`).
- Techniker-Geräteidentität zur formalen Prüfung monotoner `client_seq` ist noch nicht abschließend modelliert.
- PostgreSQL-Adapter, transaktionsgebundene SQL-Abfragen und konkrete Repository-Implementierungen folgen im nächsten Arbeitsschritt.

### Modellierungsnotiz (konservative Kompatibilität)
- Domain speichert angeforderte ResourceClasses als Slice ohne explizite Mengenregel.
- `request_resource_classes` verwendet daher `position` statt `(request_id, resource_class_id)` als Schlüssel, um potenzielle Mehrfachvorkommen nicht stillschweigend zu verbieten.
- Rollenbezeichnung ist einheitlich `dispatcher` in allen Schichten (Domain, HTTP, DB). Migration `000005_unify_role_dispatcher.*` hat den früheren, abweichenden DB-Wert abgelöst.
